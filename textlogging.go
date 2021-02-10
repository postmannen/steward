package steward

import (
	"fmt"
	"log"
	"os"
)

// startTextLogging will open a file ready for writing log messages to,
// and the input for writing to the file is given via the logCh argument.
func (s *server) startTextLogging(logCh chan []byte) {
	fileName := "./textlogging.log"

	f, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModeAppend)
	if err != nil {
		log.Printf("Failed to open file %v\n", err)
		return
	}
	defer f.Close()

	for b := range logCh {
		fmt.Printf("***** Trying to write to file : %s\n\n", b)
		_, err := f.Write(b)
		f.Sync()
		if err != nil {
			log.Printf("Failed to open file %v\n", err)
		}
	}

}
