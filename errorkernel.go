package steward

import (
	"fmt"
	"log"
)

// errorKernel is the structure that will hold all the error
// handling values and logic.
type errorKernel struct {
	// ringBuffer *ringBuffer
}

// newErrorKernel will initialize and return a new error kernel
func newErrorKernel() *errorKernel {
	return &errorKernel{
		// ringBuffer: newringBuffer(),
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
func (e *errorKernel) startErrorKernel(errorCh chan errProcess) {
	// TODO: For now it will just print the error messages to the
	// console.
	go func() {

		for {
			er := <-errorCh

			// We should be able to handle each error individually and
			// also concurrently, so the handler is start in it's own
			// go routine
			go func() {
				// Just print the error, and tell the process to continue
				log.Printf("*** error_kernel: %#v, type=%T\n", er, er)
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
	errorActionCh chan errorAction
	infoText      string
	process       process
	message       Message
}

func (e errProcess) Error() string {
	return fmt.Sprintf("worker error: proc = %#v, message = %#v", e.process, e.message)
}
