package steward

import (
	"fmt"
	"log"
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
		err := os.MkdirAll(folderTree, 0770)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	//f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0660)
	f, err := os.OpenFile(file, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0660)

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
		err := os.MkdirAll(folderTree, 0770)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0660)
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
		err := os.MkdirAll(folderTree, 0770)
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
		err := os.MkdirAll(folderTree, 0770)
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
	case len(message.MethodArgs) > 0 && message.MethodArgs[0] == "stderr":
		log.Printf("* DEBUG: MethodArgs: got stderr \n")
		fmt.Fprintf(os.Stderr, "%v", string(message.Data))
		fmt.Println()
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
