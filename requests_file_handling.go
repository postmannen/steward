package steward

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"

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
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFileAppend: failed to create toFileAppend directory tree:%v, subject: %v, %v", folderTree, proc.subject, err)
			proc.errorKernel.errSend(proc, message, er)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFileAppend.handler: failed to open file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er)
		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file : %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er)
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
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFile failed to create toFile directory tree: subject:%v, folderTree: %v, %v", proc.subject, folderTree, err)
			proc.errorKernel.errSend(proc, message, er)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFile.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)

		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQCopyFileFrom struct {
	event Event
}

func (m methodREQCopyFileFrom) getKind() Event {
	return m.event
}

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
func (m methodREQCopyFileFrom) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQCopyFileFrom: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		SrcFilePath := message.MethodArgs[0]
		DstNode := message.MethodArgs[1]
		DstFilePath := message.MethodArgs[2]

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		defer cancel()

		outCh := make(chan []byte)
		errCh := make(chan error)

		// Read the file, and put the result on the out channel to be sent when done reading.
		proc.processes.wg.Add(1)
		go copyFileFrom(ctx, &proc.processes.wg, SrcFilePath, errCh, outCh)

		// Wait here until we got the data to send, then create a new message
		// and send it.
		// Also checking the ctx.Done which calls Cancel will allow us to
		// kill all started go routines started by this message.
		select {
		case <-ctx.Done():
			er := fmt.Errorf("error: methodREQCopyFile: got <-ctx.Done(): %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)

			return
		case er := <-errCh:
			proc.errorKernel.errSend(proc, message, er)

			return
		case out := <-outCh:
			dstDir := filepath.Dir(DstFilePath)
			dstFile := filepath.Base(DstFilePath)

			// Prepare for sending a new message with the output

			// Copy the original message to get the defaults for timeouts etc,
			// and set new values for fields to change.
			msg := message
			msg.ToNode = Node(DstNode)
			//msg.Method = REQToFile
			msg.Method = REQCopyFileTo
			msg.Data = out
			msg.Directory = dstDir
			msg.FileName = dstFile

			// Create SAM and put the message on the send new message channel.

			sam, err := newSubjectAndMessage(msg)
			if err != nil {
				er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
				proc.errorKernel.errSend(proc, message, er)
			}

			proc.toRingbufferCh <- []subjectAndMessage{sam}

			replyData := fmt.Sprintf("info: succesfully read the file %v, and sent the content to %v\n", SrcFilePath, DstNode)

			newReplyMessage(proc, message, []byte(replyData))
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// copyFileFrom will read a file to be copied from the specified SrcFilePath.
// The result of be delivered on the provided outCh.
func copyFileFrom(ctx context.Context, wg *sync.WaitGroup, SrcFilePath string, errCh chan error, outCh chan []byte) {
	defer wg.Done()

	const natsMaxMsgSize = 1000000

	fi, err := os.Stat(SrcFilePath)

	// Check if the src file exists, and that it is not bigger than
	// the default limit used by nats which is 1MB.
	switch {
	case os.IsNotExist(err):
		errCh <- fmt.Errorf("error: methodREQCopyFile: src file not found: %v", SrcFilePath)
		return
	case fi.Size() > natsMaxMsgSize:
		errCh <- fmt.Errorf("error: methodREQCopyFile: src file to big. max size: %v", natsMaxMsgSize)
		return
	}

	fh, err := os.Open(SrcFilePath)
	if err != nil {
		errCh <- fmt.Errorf("error: methodREQCopyFile: failed to open file: %v, %v", SrcFilePath, err)
		return
	}

	b, err := io.ReadAll(fh)
	if err != nil {
		errCh <- fmt.Errorf("error: methodREQCopyFile: failed to read file: %v, %v", SrcFilePath, err)
		return
	}

	select {
	case outCh <- b:
		// fmt.Printf(" * DEBUG: after io.ReadAll: outCh <- b\n")
	case <-ctx.Done():
		return
	}
}

// ----

type methodREQCopyFileTo struct {
	event Event
}

func (m methodREQCopyFileTo) getKind() Event {
	return m.event
}

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
// Same as the REQToFile, but this requst type don't use the default data folder path
// for where to store files or add information about node names.
// This method also sends a msgReply back to the publisher if the method was done
// successfully, where REQToFile do not.
// This method will truncate and overwrite any existing files.
func (m methodREQCopyFileTo) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		defer cancel()

		// Put data that should be the result of the action done in the inner
		// go routine on the outCh.
		outCh := make(chan []byte)
		// Put errors from the inner go routine on the errCh.
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			// ---
			switch {
			case len(message.MethodArgs) < 3:
				er := fmt.Errorf("error: methodREQCopyFileTo: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
				proc.errorKernel.errSend(proc, message, er)

				return
			}

			// Pick up the values for the directory and filename for where
			// to store the file.
			DstFilePath := message.MethodArgs[2]
			dstDir := filepath.Dir(DstFilePath)
			dstFile := filepath.Base(DstFilePath)

			fileRealPath := path.Join(dstDir, dstFile)

			// Check if folder structure exist, if not create it.
			if _, err := os.Stat(dstDir); os.IsNotExist(err) {
				err := os.MkdirAll(dstDir, 0700)
				if err != nil {
					er := fmt.Errorf("failed to create toFile directory tree: subject:%v, folderTree: %v, %v", proc.subject, dstDir, err)
					errCh <- er
					return
				}

				{
					er := fmt.Errorf("info: MethodREQCopyFileTo: Creating folders %v", dstDir)
					proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
				}
			}

			// Open file and write data. Truncate and overwrite any existing files.
			file := filepath.Join(dstDir, dstFile)
			f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				er := fmt.Errorf("failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
				errCh <- er
				return
			}
			defer f.Close()

			_, err = f.Write(message.Data)
			f.Sync()
			if err != nil {
				er := fmt.Errorf("failed to write to file: file: %v, error: %v", file, err)
				errCh <- er
			}

			// All went ok, send a signal to the outer select statement.
			outCh <- []byte(fileRealPath)

			// ---

		}()

		// Wait for messages received from the inner go routine.
		select {
		case <-ctx.Done():
			er := fmt.Errorf("error: methodREQCopyFileTo: got <-ctx.Done(): %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)
			return

		case err := <-errCh:
			er := fmt.Errorf("error: methodREQCopyFileTo: %v", err)
			proc.errorKernel.errSend(proc, message, er)
			return

		case out := <-outCh:
			replyData := fmt.Sprintf("info: succesfully created and wrote the file %v\n", out)
			newReplyMessage(proc, message, []byte(replyData))
			return
		}

	}()

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
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQTailFile: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er)

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
			proc.errorKernel.errSend(proc, message, er)
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
