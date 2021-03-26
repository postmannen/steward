package steward

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/fsnotify/fsnotify"
)

// getMessagesFromFile will start a file watcher for the given directory
// and filename. It will take a channel of []byte as input, and it is
// in this channel the content of a file that has changed is returned.
func (s *server) getMessagesFromFile(directoryToCheck string, fileName string, inputFromFileCh chan []subjectAndMessage) {
	fileUpdated := make(chan bool)
	go fileWatcherStart(directoryToCheck, fileUpdated)

	for range fileUpdated {

		//load file, read it's content
		b, err := readTruncateMessageFile(fileName)
		if err != nil {
			log.Printf("error: reading file: %v", err)
		}

		// Start on top again if the file did not contain
		// any data.
		if len(b) == 0 {
			continue
		}

		// unmarshal the JSON into a struct
		js, err := jsonFromFileData(b)
		if err != nil {
			er := fmt.Errorf("error: malformed json: %v", err)
			sendErrorLogMessage(s.newMessagesCh, node(s.nodeName), er)
			continue
		}

		for i := range js {
			fmt.Printf("*** Checking message found in file: messageType type: %T, messagetype contains: %#v\n", js[i].Subject.CommandOrEvent, js[i].Subject.CommandOrEvent)
			// Fill in the value for the FromNode field, so the receiver
			// can check this field to know where it came from.
			js[i].Message.FromNode = node(s.nodeName)
		}

		// Send the data back to be consumed
		inputFromFileCh <- js
	}
	er := fmt.Errorf("error: getMessagesFromFile stopped")
	sendErrorLogMessage(s.newMessagesCh, node(s.nodeName), er)
}

type subjectAndMessage struct {
	Subject `json:"subject" yaml:"subject"`
	Message `json:"message" yaml:"message"`
}

// jsonFromFileData will range over the message given in json format. For
// each element found the Message type will be converted into a SubjectAndMessage
// type value and appended to a slice, and the slice is returned to the caller.
func jsonFromFileData(b []byte) ([]subjectAndMessage, error) {
	MsgSlice := []Message{}

	err := json.Unmarshal(b, &MsgSlice)
	if err != nil {
		return nil, fmt.Errorf("error: unmarshal of file failed: %#v", err)
	}

	sam := []subjectAndMessage{}

	// Range over all the messages parsed from json, and create a subject for
	// each message.
	for _, m := range MsgSlice {
		sm, err := newSAM(m)
		if err != nil {
			log.Printf("error: jsonFromFileData: %v\n", err)
			continue
		}
		sam = append(sam, sm)
	}

	return sam, nil
}

// newSAM will look up the correct values and value types to
// be used in a subject for a Message, and return the a combined structure
// of type subjectAndMessage.
func newSAM(m Message) (subjectAndMessage, error) {
	// We need to create a tempory method type to look up the kind for the
	// real method for the message.
	var mt Method

	//fmt.Printf("-- \n getKind contains: %v\n\n", mt.getHandler(m.Method).getKind())

	tmpH := mt.getHandler(m.Method)
	if tmpH == nil {
		return subjectAndMessage{}, fmt.Errorf("error: method value did not exist in map")
	}

	sub := Subject{
		ToNode:         string(m.ToNode),
		CommandOrEvent: tmpH.getKind(),
		Method:         m.Method,
	}

	sm := subjectAndMessage{
		Subject: sub,
		Message: m,
	}

	return sm, nil
}

// readTruncateMessageFile, will read all the messages in the given
// file, and truncate the file after read.
// A []byte will be returned with the content read.
func readTruncateMessageFile(fileName string) ([]byte, error) {

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModeAppend)
	if err != nil {
		log.Printf("error: readTruncateMessageFile: Failed to open file: %v\n", err)
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	lines := []byte{}

	for scanner.Scan() {
		lines = append(lines, scanner.Bytes()...)
	}

	// empty the file after all is read
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("f.Seek failed: %v", err)
	}

	err = f.Truncate(0)
	if err != nil {
		return nil, fmt.Errorf("f.Truncate failed: %v", err)
	}

	return lines, nil
}

// Start the file watcher that will check if the in pipe for new operator
// messages are updated with new content.
func fileWatcherStart(directoryToCheck string, fileUpdated chan bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Failed fsnotify.NewWatcher")
		return
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		//Give a true value to updated so it reads the file the first time.
		fileUpdated <- true
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					// log.Println("info: steward.sock file updated, processing input: ", event.Name)
					//testing with an update chan to get updates
					fileUpdated <- true
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(directoryToCheck)
	if err != nil {
		log.Printf("error: watcher add: %v\n", err)
	}
	<-done
}
