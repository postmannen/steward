// The error kernel shall handle errors for a given process.
// This will be cases where the process itself were unable
// to handle the error on it's own, and we might need to
// restart the process, or send a message back to the operator
// that the action which the message where supposed to trigger
// failed, or that an event where unable to be processed.

package steward

import (
	"context"
	"fmt"
	"log"
	"time"
)

// errorKernel is the structure that will hold all the error
// handling values and logic.
type errorKernel struct {
	// NOTE: The errorKernel should probably have a concept
	// of error-state which is a map of all the processes,
	// how many times a process have failed over the same
	// message etc...

	// errorCh is used to report errors from a process
	errorCh chan errorEvent

	ctx     context.Context
	cancel  context.CancelFunc
	metrics *metrics
}

// newErrorKernel will initialize and return a new error kernel
func newErrorKernel(ctx context.Context, m *metrics) *errorKernel {
	ctxC, cancel := context.WithCancel(ctx)

	return &errorKernel{
		errorCh: make(chan errorEvent, 2),
		ctx:     ctxC,
		cancel:  cancel,
		metrics: m,
	}
}

// startErrorKernel will start the error kernel and check if there
// have been reveived any errors from any of the processes, and
// handle them appropriately.
//
// NOTE: Since a process will be locked while waiting to send the error
// on the errorCh maybe it makes sense to have a channel inside the
// processes error handling with a select so we can send back to the
// process if it should continue or not based not based on how severe
// the error where. This should be right after sending the error
// sending in the process.
func (e *errorKernel) start(newMessagesCh chan<- []subjectAndMessage) error {
	// NOTE: For now it will just print the error messages to the
	// console.

	for {
		var errEvent errorEvent
		select {
		case errEvent = <-e.errorCh:
		case <-e.ctx.Done():
			return fmt.Errorf("info: stopping errorKernel")
		}

		// Check the type of the error to decide what to do.
		//
		// We should be able to handle each error individually and
		// also concurrently, so the handler is started in it's
		// own go routine
		//
		// Here we should check the severity of the error,
		// and also possibly the the error-state of the process
		// that fails, so we can decide if we should stop and
		// start a new process to replace to old one, or if we
		// should just kill the process and send message back to
		// the operator....or other ?
		switch errEvent.errorType {

		case errTypeSendToCentralErrorLogger:
			fmt.Printf(" * case errTypeSend\n")
			// Just log the error, and don't use the errorAction channel
			// so the process who sent the error don't have to wait for
			// the error message to be sent before it can continue.
			go func() {
				// Add time stamp
				er := fmt.Sprintf("%v, node: %v, %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), errEvent.process.node, errEvent.err)

				sam := subjectAndMessage{
					Subject: newSubject(REQErrorLog, "errorCentral"),
					Message: Message{
						Directory:  "errorLog",
						ToNode:     "errorCentral",
						FromNode:   errEvent.process.node,
						FileName:   "error.log",
						Data:       []string{er},
						Method:     REQErrorLog,
						ACKTimeout: errEvent.process.configuration.ErrorMessageTimeout,
						Retries:    errEvent.process.configuration.ErrorMessageRetries,
					},
				}

				newMessagesCh <- []subjectAndMessage{sam}

				e.metrics.promErrorMessagesSentTotal.Inc()
			}()

		case errTypeWithAction:
			// TODO: Look into how to implement error actions.

			fmt.Printf(" * case errTypeWithAction\n")
			// Just print the error, and tell the process to continue. The
			// process who sent the error should block andwait for receiving
			// an errActionContinue message.

			go func() {

				// log.Printf("*** error_kernel: %#v, type=%T\n", er, er)
				log.Printf("TESTING, we received and error from the process, but we're telling the process back to continue\n")

				select {
				case errEvent.errorActionCh <- errActionContinue:
				case <-e.ctx.Done():
					log.Printf("info: errorKernel: got ctx.Done, will stop waiting for errAction\n")
					return
				}
			}()

		default:
			fmt.Printf(" * case default\n")
		}
	}
}

func (e *errorKernel) stop() {
	e.cancel()
}

// sendError will just send an error to the errorCentral.
func (e *errorKernel) errSend(proc process, msg Message, err error) {
	ev := errorEvent{
		err:       err,
		errorType: errTypeSendToCentralErrorLogger,
		process:   proc,
		message:   msg,
		// We don't want to create any actions when just
		// sending errors.
		// errorActionCh: make(chan errorAction),
	}

	e.errorCh <- ev
}

// errWithAction
//
// TODO: Look into how to implement error actions.
func (e *errorKernel) errWithAction(proc process, msg Message, err error) chan errorAction {
	// Create the channel where to receive what action to do.
	errAction := make(chan errorAction)

	ev := errorEvent{
		//errorType:     logOnly,
		process:       proc,
		message:       msg,
		errorActionCh: errAction,
	}

	e.errorCh <- ev

	return errAction
}

// errorAction is used to tell the process who sent the error
// what it shall do. The process who sends the error will
// have to block and wait for the response on the errorActionCh.
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

// errorType
type errorType int

const (
	// errSend will just send the content of the error to the
	// central error logger.
	errTypeSendToCentralErrorLogger errorType = iota
	errTypeWithAction               errorType = iota
)

type errorEvent struct {
	// The actual error
	err error
	// Channel for communicating the action to take back to
	// to the process who triggered the error
	errorActionCh chan errorAction
	// Some informational text
	errorType errorType
	// The process structure that belongs to a given process
	process process
	// The message that where in progress when error occured
	message Message
}

func (e errorEvent) Error() string {
	return fmt.Sprintf("worker error: proc = %#v, message = %#v", e.process, e.message)
}
