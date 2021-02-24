package steward

import (
	"fmt"
	"log"
	"os"
)

// subscriberServices will hold all the helper services needed for
// the different subcribers. Example of a help service can be a log
// subscriber needs a way to write logs locally or send them to some
// other central logging system.
type subscriberServices struct {
	// Where we should put the data to write to a log file
	logCh chan []byte

	// sayHelloNodes are the register where the register where nodes
	// who have sent an sayHello are stored. Since the sayHello
	// subscriber is a handler that will be just be called when a
	// hello message is received we need to store the metrics somewhere
	// else, that is why we store it here....at least for now.
	sayHelloNodes map[node]struct{}
}

//newSubscriberServices will prepare and return a *subscriberServices
func newSubscriberServices() *subscriberServices {
	s := subscriberServices{
		logCh:         make(chan []byte),
		sayHelloNodes: make(map[node]struct{}),
	}

	return &s
}

// ---

// startTextLogging will open a file ready for writing log messages to,
// and the input for writing to the file is given via the logCh argument.
func (s *subscriberServices) startTextLogging() {
	fileName := "./textlogging.log"

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModeAppend)
	if err != nil {
		log.Printf("Failed to open file %v\n", err)
		return
	}
	defer f.Close()

	for b := range s.logCh {
		fmt.Printf("***** Trying to write to file : %s\n\n", b)
		_, err := f.Write(b)
		f.Sync()
		if err != nil {
			log.Printf("Failed to open file %v\n", err)
		}
	}

}
