package steward

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// -----

type methodREQHello struct {
	event Event
}

func (m methodREQHello) getKind() Event {
	return m.event
}

// Handler for receiving hello messages.
func (m methodREQHello) handler(proc process, message Message, node string) ([]byte, error) {
	data := fmt.Sprintf("%v, Received hello from %#v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.FromNode)

	fileName := message.FileName
	folderTree := filepath.Join(proc.configuration.SubscribersDataFolder, message.Directory, string(message.FromNode))

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	//f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	f, err := os.OpenFile(file, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)

	if err != nil {
		er := fmt.Errorf("error: methodREQHello.handler: failed to open file: %v", err)
		return nil, er
	}
	defer f.Close()

	_, err = f.Write([]byte(data))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: %v", err)
		proc.errorKernel.errSend(proc, message, er)
	}

	// --------------------------

	// send the message to the procFuncCh which is running alongside the process
	// and can hold registries and handle special things for an individual process.
	proc.procFuncCh <- message

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQErrorLog struct {
	event Event
}

func (m methodREQErrorLog) getKind() Event {
	return m.event
}

// Handle the writing of error logs.
func (m methodREQErrorLog) handler(proc process, message Message, node string) ([]byte, error) {
	proc.metrics.promErrorMessagesReceivedTotal.Inc()

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: methodREQErrorLog.handler: failed to open file: %v", err)
		return nil, er
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: %v", err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQPing struct {
	event Event
}

func (m methodREQPing) getKind() Event {
	return m.event
}

// Handle receving a ping.
func (m methodREQPing) handler(proc process, message Message, node string) ([]byte, error) {
	// Write to file that we received a ping

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQPing.handler: failed to create toFile directory tree: %v, %v", folderTree, err)
			proc.errorKernel.errSend(proc, message, er)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQPing.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)

		return nil, err
	}
	defer f.Close()

	// And write the data
	d := fmt.Sprintf("%v, ping received from %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.FromNode)
	_, err = f.Write([]byte(d))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodREQPing.handler: failed to write to file: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)
	}

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		newReplyMessage(proc, message, nil)
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQPong struct {
	event Event
}

func (m methodREQPong) getKind() Event {
	return m.event
}

// Handle receiving a pong.
func (m methodREQPong) handler(proc process, message Message, node string) ([]byte, error) {
	// Write to file that we received a pong

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQPong.handler: failed to create toFile directory tree %v: %v", folderTree, err)
			proc.errorKernel.errSend(proc, message, er)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQPong.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)

		return nil, err
	}
	defer f.Close()

	// And write the data
	d := fmt.Sprintf("%v, pong received from %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.PreviousMessage.ToNode)
	_, err = f.Write([]byte(d))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodREQPong.handler: failed to write to file: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQRelayInitial struct {
	event Event
}

func (m methodREQRelayInitial) getKind() Event {
	return m.event
}

// Handler to relay messages via a host.
func (m methodREQRelayInitial) handler(proc process, message Message, node string) ([]byte, error) {
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		defer cancel()

		outCh := make(chan []byte)
		errCh := make(chan error)
		nothingCh := make(chan struct{}, 1)

		var out []byte

		// If the actual Method for the message is REQCopyFileFrom we need to
		// do the actual file reading here so we can fill the data field of the
		// message with the content of the file before relaying it.
		switch {
		case message.RelayOriginalMethod == REQCopyFileFrom:
			switch {
			case len(message.MethodArgs) < 3:
				er := fmt.Errorf("error: methodREQRelayInitial: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
				proc.errorKernel.errSend(proc, message, er)

				return
			}

			SrcFilePath := message.MethodArgs[0]
			//DstFilePath := message.MethodArgs[2]

			// Read the file, and put the result on the out channel to be sent when done reading.
			proc.processes.wg.Add(1)
			go copyFileFrom(ctx, &proc.processes.wg, SrcFilePath, errCh, outCh)

			// Since we now have read the source file we don't need the REQCopyFileFrom
			// request method anymore, so we change the original method of the message
			// so it will write the data after the relaying.
			//dstDir := filepath.Dir(DstFilePath)
			//dstFile := filepath.Base(DstFilePath)
			message.RelayOriginalMethod = REQCopyFileTo
			//message.FileName = dstFile
			//message.Directory = dstDir
		default:
			// No request type that need special handling if relayed, so we should signal that
			// there is nothing to do for the select below.
			// We need to do this signaling in it's own go routine here, so we don't block here
			// since the select below  is in the same function.
			go func() {
				nothingCh <- struct{}{}
			}()
		}

		select {
		case <-ctx.Done():
			er := fmt.Errorf("error: methodREQRelayInitial: CopyFromFile: got <-ctx.Done(): %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)

			return
		case er := <-errCh:
			proc.errorKernel.errSend(proc, message, er)

			return
		case <-nothingCh:
			// Do nothing.
		case out = <-outCh:

		}

		// relay the message to the actual host here by prefixing the the RelayToNode
		// to the subject.
		relayTo := fmt.Sprintf("%v.%v", message.RelayToNode, message.RelayOriginalViaNode)
		// message.ToNode = message.RelayOriginalViaNode
		message.ToNode = Node(relayTo)
		message.FromNode = Node(node)
		message.Method = REQRelay
		message.Data = out

		sam, err := newSubjectAndMessage(message)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
			proc.errorKernel.errSend(proc, message, er)
		}

		proc.toRingbufferCh <- []subjectAndMessage{sam}
	}()

	// Send back an ACK message.
	ackMsg := []byte("confirmed REQRelay from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQRelay struct {
	event Event
}

func (m methodREQRelay) getKind() Event {
	return m.event
}

// Handler to relay messages via a host.
func (m methodREQRelay) handler(proc process, message Message, node string) ([]byte, error) {
	// relay the message here to the actual host here.

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		message.ToNode = message.RelayToNode
		message.FromNode = Node(node)
		message.Method = message.RelayOriginalMethod

		sam, err := newSubjectAndMessage(message)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		select {
		case proc.toRingbufferCh <- []subjectAndMessage{sam}:
		case <-proc.ctx.Done():
		}
	}()

	// Send back an ACK message.
	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQToConsole struct {
	event Event
}

func (m methodREQToConsole) getKind() Event {
	return m.event
}

// Handler to write directly to console.
// This handler handles the writing to console both for TUI and shell clients.
func (m methodREQToConsole) handler(proc process, message Message, node string) ([]byte, error) {

	switch {
	case proc.configuration.EnableTUI:
		if proc.processes.tui.toConsoleCh != nil {
			proc.processes.tui.toConsoleCh <- message.Data
		} else {
			er := fmt.Errorf("error: no tui client started")
			proc.errorKernel.errSend(proc, message, er)
		}
	default:
		fmt.Fprintf(os.Stdout, "%v", string(message.Data))
		fmt.Println()
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQTuiToConsole struct {
	event Event
}

func (m methodREQTuiToConsole) getKind() Event {
	return m.event
}

// Handler to write directly to console.
// DEPRECATED
func (m methodREQTuiToConsole) handler(proc process, message Message, node string) ([]byte, error) {

	if proc.processes.tui.toConsoleCh != nil {
		proc.processes.tui.toConsoleCh <- message.Data
	} else {
		er := fmt.Errorf("error: no tui client started")
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQTest struct {
	event Event
}

func (m methodREQTest) getKind() Event {
	return m.event
}

// handler to be used as a reply method when testing requests.
// We can then within the test listen on the testCh for received
// data and validate it.
// If no test is listening the data will be dropped.
func (m methodREQTest) handler(proc process, message Message, node string) ([]byte, error) {

	go func() {
		// Try to send the received message data on the test channel. If we
		// have a test started the data will be read from the testCh.
		// If no test is reading from the testCh the data will be dropped.
		select {
		case proc.errorKernel.testCh <- message.Data:
		default:
			// drop.
		}
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
