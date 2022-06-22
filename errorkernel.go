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
	// testCh is used within REQTest for receving data for tests.
	testCh chan []byte

	ctx     context.Context
	cancel  context.CancelFunc
	metrics *metrics
}

// newErrorKernel will initialize and return a new error kernel
func newErrorKernel(ctx context.Context, m *metrics) *errorKernel {
	ctxC, cancel := context.WithCancel(ctx)

	return &errorKernel{
		errorCh: make(chan errorEvent, 2),
		testCh:  make(chan []byte),
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
func (e *errorKernel) start(ringBufferBulkInCh chan<- []subjectAndMessage) error {
	// NOTE: For now it will just print the error messages to the
	// console.

	for {
		var errEvent errorEvent
		select {
		case errEvent = <-e.errorCh:
		case <-e.ctx.Done():
			return fmt.Errorf("info: stopping errorKernel")
		}

		sendErrorOrInfo := func(errEvent errorEvent) {

			er := fmt.Sprintf("%v, node: %v, %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), errEvent.process.node, errEvent.err)

			sam := subjectAndMessage{
				Subject: newSubject(REQErrorLog, "errorCentral"),
				Message: Message{
					Directory:  "errorLog",
					ToNode:     "errorCentral",
					FromNode:   errEvent.process.node,
					FileName:   "error.log",
					Data:       []byte(er),
					Method:     REQErrorLog,
					ACKTimeout: errEvent.process.configuration.ErrorMessageTimeout,
					Retries:    errEvent.process.configuration.ErrorMessageRetries,
				},
			}

			// Put the message on the channel to the ringbuffer.
			ringBufferBulkInCh <- []subjectAndMessage{sam}

			if errEvent.process.configuration.EnableDebug {
				log.Printf("%v\n", er)
			}
		}

		// Check the type of the error to decide what to do.
		//
		// We should be able to handle each error individually and
		// also concurrently, so each handler is started in it's
		// own go routine
		//
		// Here we should check the severity of the error,
		// and also possibly the the error-state of the process
		// that fails.
		switch errEvent.errorType {

		case errTypeSendError:
			// Just log the error by creating a message and send it
			// to the errorCentral log server.

			go func() {
				sendErrorOrInfo(errEvent)
				e.metrics.promErrorMessagesSentTotal.Inc()
			}()

		case errTypeSendInfo:
			// Just log the error by creating a message and send it
			// to the errorCentral log server.

			go func() {
				sendErrorOrInfo(errEvent)
				e.metrics.promInfoMessagesSentTotal.Inc()
			}()

		case errTypeWithAction:
			// Just print the error, and tell the process to continue. The
			// process who sent the error should block and wait for receiving
			// an errActionContinue message.

			go func() {
				log.Printf("TESTING, we received and error from the process, but we're telling the process back to continue\n")

				// Send a message back to where the errWithAction function
				// was called on the errorActionCh so the caller can decide
				// what to do based on the response.
				select {
				case errEvent.errorActionCh <- errActionContinue:
				case <-e.ctx.Done():
					log.Printf("info: errorKernel: got ctx.Done, will stop waiting for errAction\n")
					return
				}

				// We also want to log the error.
				e.errSend(errEvent.process, errEvent.message, errEvent.err)
			}()

		default:
			// fmt.Printf(" * case default\n")
		}
	}
}

func (e *errorKernel) stop() {
	e.cancel()
}

// errSend will just send an error message to the errorCentral.
func (e *errorKernel) errSend(proc process, msg Message, err error) {
	ev := errorEvent{
		err:       err,
		errorType: errTypeSendError,
		process:   proc,
		message:   msg,
		// We don't want to create any actions when just
		// sending errors.
		// errorActionCh: make(chan errorAction),
	}

	e.errorCh <- ev
}

// infoSend will just send an info message to the errorCentral.
func (e *errorKernel) infoSend(proc process, msg Message, err error) {
	ev := errorEvent{
		err:       err,
		errorType: errTypeSendInfo,
		process:   proc,
		message:   msg,
		// We don't want to create any actions when just
		// sending errors.
		// errorActionCh: make(chan errorAction),
	}

	e.errorCh <- ev
}

func (e *errorKernel) logConsoleOnlyIfDebug(err error, c *Configuration) {
	if c.EnableDebug {
		log.Printf("%v\n", err)
	}
}

// errorAction is used to tell the process who sent the error
// what it shall do. The process who sends the error will
// have to block and wait for the response on the errorActionCh.
type errorAction int

const (
	// errActionContinue is ment to be used when the a process
	// can just continue without taking any special care.
	errActionContinue errorAction = iota
	// TODO NOT IMPLEMENTED YET:
	// errActionKill should log the error,
	// and f.ex. stop the current work, and restart from start?
	// errActionKill errorAction = iota
)

// errorType
type errorType int

const (
	// errSend will just send the content of the error to the
	// central error logger.
	errTypeSendError  errorType = iota
	errTypeSendInfo   errorType = iota
	errTypeWithAction errorType = iota
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
