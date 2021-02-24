// The error kernel shall handle errors for a given process.
// This will be cases where the process itself where unable
// to handle the error on it's own, and we might need to
// restart the process, or send a message back to the operator
// that the action which the message where supposed to trigger,
// or that an event where unable to be processed.

package steward

import (
	"fmt"
	"log"
)

// errorKernel is the structure that will hold all the error
// handling values and logic.
type errorKernel struct {
	// TODO: The errorKernel should probably have a concept
	// of error-state which is a map of all the processes,
	// how many times a process have failed over the same
	// message etc...

	// errorCh is used to report errors from a process
	errorCh chan errProcess
}

// newErrorKernel will initialize and return a new error kernel
func newErrorKernel() *errorKernel {
	return &errorKernel{
		errorCh: make(chan errProcess, 2),
	}
}

// startErrorKernel will start the error kernel and check if there
// have been reveived any errors from any of the processes, and
// handle them appropriately.
// TODO: Since a process will be locked while waiting to send the error
// on the errorCh maybe it makes sense to have a channel inside the
// processes error handling with a select so we can send back to the
// process if it should continue or not based not based on how severe
// the error where. This should be right after sending the error
// sending in the process.
func (e *errorKernel) startErrorKernel() {
	// TODO: For now it will just print the error messages to the
	// console.
	go func() {

		for {
			er := <-e.errorCh

			// We should be able to handle each error individually and
			// also concurrently, so the handler is started in it's
			// own go routine
			go func() {
				// TODO: Here we should check the severity of the error,
				// and also possibly the the error-state of the process
				// that fails, so we can decide if we should stop and
				// start a new process to replace to old one, or if we
				// should just kill the process and send message back to
				// the operator....or other ?
				//
				// Just print the error, and tell the process to continue

				// log.Printf("*** error_kernel: %#v, type=%T\n", er, er)
				log.Printf("TESTING, we received and error from the process, but we're telling the process back to continue\n")
				er.errorActionCh <- errActionContinue
			}()
		}
	}()
}

type errorAction int

const (
	// errActionJustPrint should just print the error,
	// and the worker process should continue.
	errActionContinue errorAction = iota
	// errActionKillAndSpawnNew should log the error,
	// stop the current worker process, and spawn a new.
	errActionKill errorAction = iota
	// errActionKillAndDie should log the error, stop the
	// current worker process, and send a message back to
	// the master supervisor that it was unable to complete
	// the action of the current message. The error message
	// should contain a copy of the original message.
)

type errProcess struct {
	// Channel for communicating the action to take back to
	// to the process who triggered the error
	errorActionCh chan errorAction
	// Some informational text
	infoText string
	// The process structure that belongs to a given process
	process process
	// The message that where in progress when error occured
	message Message
}

func (e errProcess) Error() string {
	return fmt.Sprintf("worker error: proc = %#v, message = %#v", e.process, e.message)
}
