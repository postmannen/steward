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
		// fmt.Printf("*** OUTPUT AFTER UNMARSHALING JSON: %#v\n", js)
		if err != nil {
			log.Printf("%v\n", err)
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
}

type subjectAndMessage struct {
	Subject `json:"subject" yaml:"subject"`
	Message `json:"message" yaml:"message"`
}

func jsonFromFileData(b []byte) ([]subjectAndMessage, error) {
	MsgSlice := []Message{}

	err := json.Unmarshal(b, &MsgSlice)
	//fmt.Printf("*** OUTPUT DIRECTLY AFTER UNMARSHALING JSON: %#v\n", MsgSlice)
	// TODO: Look into also marshaling from yaml and toml later
	if err != nil {
		return nil, fmt.Errorf("error: unmarshal of file failed: %#v", err)
	}

	sam := []subjectAndMessage{}
	// We need to create a tempory method type to look up the kind for the
	// real method for the message.
	var mt Method

	// Range over all the messages parsed from json, and create a subject for
	// each message.
	for _, m := range MsgSlice {
		s := Subject{
			ToNode:         string(m.ToNode),
			CommandOrEvent: mt.getHandler(m.Method).getKind(),
			Method:         m.Method,
		}

		sm := subjectAndMessage{
			Subject: s,
			Message: m,
		}

		sam = append(sam, sm)
	}

	return sam, nil
}

// readTruncateMessageFile, will read all the messages in the given
// file, and truncate the file after read.
// A []byte will be returned with the content read.
func readTruncateMessageFile(fileName string) ([]byte, error) {

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR, os.ModeAppend)
	if err != nil {
		log.Printf("Failed to open file %v\n", err)
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
					// log.Println("info: inmsg.txt file updated, processing input: ", event.Name)
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
