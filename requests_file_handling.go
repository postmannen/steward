package steward

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hpcloud/tail"
)

type methodREQToFileAppend struct {
	event Event
}

func (m methodREQToFileAppend) getKind() Event {
	return m.event
}

// Handle appending data to file.
func (m methodREQToFileAppend) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0770)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFileAppend: failed to create toFileAppend directory tree:%v, subject: %v, %v", folderTree, proc.subject, err)
			proc.errorKernel.errSend(proc, message, er, logWarning)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0660)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFileAppend.handler: failed to open file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er, logWarning)
		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file : %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er, logWarning)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// -----

type methodREQToFile struct {
	event Event
}

func (m methodREQToFile) getKind() Event {
	return m.event
}

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
func (m methodREQToFile) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0770)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFile failed to create toFile directory tree: subject:%v, folderTree: %v, %v", proc.subject, folderTree, err)
			proc.errorKernel.errSend(proc, message, er, logWarning)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFile.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er, logWarning)

		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er, logWarning)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- methodREQTailFile

type methodREQTailFile struct {
	event Event
}

func (m methodREQTailFile) getKind() Event {
	return m.event
}

// handler to run a tailing of files with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
func (m methodREQTailFile) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- TailFile REQUEST received from: %v, containing: %v", message.FromNode, message.Data)
	proc.errorKernel.logDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQTailFile: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er, logWarning)

			return
		}

		fp := message.MethodArgs[0]

		// var ctx context.Context
		// var cancel context.CancelFunc

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		// Note: Replacing the 0 timeout with specific timeout.
		// if message.MethodTimeout != 0 {
		// 	ctx, cancel = context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))
		// } else {
		// 	ctx, cancel = context.WithCancel(proc.ctx)
		// }

		outCh := make(chan []byte)
		t, err := tail.TailFile(fp, tail.Config{Follow: true, Location: &tail.SeekInfo{
			Offset: 0,
			Whence: os.SEEK_END,
		}})
		if err != nil {
			er := fmt.Errorf("error: methodREQToTailFile: tailFile: %v", err)
			proc.errorKernel.errSend(proc, message, er, logWarning)
		}

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			for {
				select {
				case line := <-t.Lines:
					outCh <- []byte(line.Text + "\n")
				case <-ctx.Done():
					return
				}

			}
		}()

		for {
			select {
			case <-ctx.Done():
				cancel()
				// Close the lines channel so we exit the reading lines
				// go routine.
				// close(t.Lines)
				er := fmt.Errorf("info: method timeout reached REQTailFile, canceling: %v", message.MethodArgs)
				proc.errorKernel.infoSend(proc, message, er)

				return
			case out := <-outCh:

				// Prepare and queue for sending a new message with the output
				// of the action executed.
				newReplyMessage(proc, message, out)
			}
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
