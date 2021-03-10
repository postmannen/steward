package steward

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// processKind are either kindSubscriber or kindPublisher, and are
// used to distinguish the kind of process to spawn and to know
// the process kind put in the process map.
type processKind string

const (
	processKindSubscriber processKind = "subscriber"
	processKindPublisher  processKind = "publisher"
)

// process are represent the communication to one individual host
type process struct {
	messageID int
	// the subject used for the specific process. One process
	// can contain only one sender on a message bus, hence
	// also one subject
	subject Subject
	// Put a node here to be able know the node a process is at.
	// NB: Might not be needed later on.
	node node
	// The processID for the current process
	processID int
	// errorCh is used to report errors from a process
	// NB: Implementing this as an int to report for testing
	errorCh     chan errProcess
	processKind processKind
	// Who are we allowed to receive from ?
	allowedReceivers map[node]struct{}
	// methodsAvailable
	methodsAvailable MethodsAvailable
	// Helper or service function that can do some kind of work
	// for the process.
	// The idea is that this can hold for example the map of the
	// the hello nodes to limit shared resources in the system as
	// a whole for sharing a map from the *server level.
	procFunc procFunc
	// The channel to send a messages to the procFunc go routine.
	// This is typically used within the methodHandler.
	procFuncCh chan Message
	// copy of the configuration from server
	configuration *Configuration
	// The new messages channel copied from *Server
	newMessagesCh chan<- []subjectAndMessage
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func newProcess(processes *processes, newMessagesCh chan<- []subjectAndMessage, configuration *Configuration, subject Subject, errCh chan errProcess, processKind processKind, allowedReceivers []node, procFunc func() error) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	processes.lastProcessID++

	// make the slice of allowedReceivers into a map value for easy lookup.
	m := make(map[node]struct{})
	for _, a := range allowedReceivers {
		m[a] = struct{}{}
	}

	var method Method

	proc := process{
		messageID:        0,
		subject:          subject,
		node:             node(subject.ToNode),
		processID:        processes.lastProcessID,
		errorCh:          errCh,
		processKind:      processKind,
		allowedReceivers: m,
		methodsAvailable: method.GetMethodsAvailable(),
		newMessagesCh:    newMessagesCh,
		configuration:    configuration,
	}

	return proc
}

// procFunc is a helper function that will do some extra work for
// a message received for a process. This allows us to ACK back
// to the publisher that the message was received, but we can let
// the processFunc keep on working.
// This can also be used to wrap in other types which we want to
// work with that come from the outside. An example can be handling
// of metrics which the message have no notion of, but a procFunc
// can have that wrapped in from when it was constructed.
type procFunc func() error

// The purpose of this function is to check if we should start a
// publisher or subscriber process, where a process is a go routine
// that will handle either sending or receiving messages on one
// subject.
//
// It will give the process the next available ID, and also add the
// process to the processes map in the server structure.
func (p process) spawnWorker(s *server) {
	// We use the full name of the subject to identify a unique
	// process. We can do that since a process can only handle
	// one message queue.
	var pn processName
	if p.processKind == processKindPublisher {
		pn = processNameGet(p.subject.name(), processKindPublisher)
	}
	if p.processKind == processKindSubscriber {
		pn = processNameGet(p.subject.name(), processKindSubscriber)
	}

	// Add information about the new process to the started processes map.
	s.processes.mu.Lock()
	s.processes.active[pn] = p
	s.processes.mu.Unlock()

	// Start a publisher worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if p.processKind == processKindPublisher {
		// If there is a procFunc for the process, start it.
		if p.procFunc != nil {
			// REMOVED: p.procFuncCh = make(chan Message)
			// Start the procFunc in it's own anonymous func so we are able
			// to get the return error.
			go func() {
				err := p.procFunc()
				if err != nil {
					log.Printf("error: spawnWorker: procFunc failed: %v\n", err)
				}
			}()
		}

		go p.publishMessages(s.natsConn, s.processes)
	}

	// Start a subscriber worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if p.processKind == processKindSubscriber {
		// If there is a procFunc for the process, start it.
		if p.procFunc != nil {
			// REMOVED: p.procFuncCh = make(chan Message)
			// Start the procFunc in it's own anonymous func so we are able
			// to get the return error.
			go func() {
				err := p.procFunc()
				if err != nil {
					log.Printf("error: spawnWorker: procFunc failed: %v\n", err)
				}
			}()
		}

		//fmt.Printf("-- DEBUG 1.1: %#v, %#v, %#v\n\n", p.subject.name(), p.procFunc, p.procFuncCh)
		p.subscribeMessages(s)
	}
}

// messageDeliverNats will take care of the delivering the message
// as converted to gob format as a nats.Message. It will also take
// care of checking timeouts and retries specified for the message.
func (p process) messageDeliverNats(natsConn *nats.Conn, message Message) {
	retryAttempts := 0

	for {
		dataPayload, err := gobEncodeMessage(message)
		if err != nil {
			log.Printf("error: createDataPayload: %v\n", err)
		}

		msg := &nats.Msg{
			Subject: string(p.subject.name()),
			// Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "CLICommand"),
			// Structure of the reply message are:
			// reply.<nodename>.<message type>.<method>
			Reply: fmt.Sprintf("reply.%s", p.subject.name()),
			Data:  dataPayload,
		}

		// The SubscribeSync used in the subscriber, will get messages that
		// are sent after it started subscribing, so we start a publisher
		// that sends out a message every second.
		//
		// Create a subscriber for the reply message.
		subReply, err := natsConn.SubscribeSync(msg.Reply)
		if err != nil {
			log.Printf("error: nc.SubscribeSync failed: failed to create reply message: %v\n", err)
			continue
		}

		// Publish message
		err = natsConn.PublishMsg(msg)
		if err != nil {
			log.Printf("error: publish failed: %v\n", err)
			continue
		}

		// If the message is an ACK type of message we must check that a
		// reply, and if it is not we don't wait here at all.
		fmt.Printf("info: messageDeliverNats: preparing to send message: %v\n", message)
		if p.subject.CommandOrEvent == CommandACK || p.subject.CommandOrEvent == EventACK {
			// Wait up until timeout specified for a reply,
			// continue and resend if noo reply received,
			// or exit if max retries for the message reached.
			msgReply, err := subReply.NextMsg(time.Second * time.Duration(message.Timeout))
			if err != nil {
				log.Printf("error: subReply.NextMsg failed for node=%v, subject=%v: %v\n", p.node, p.subject.name(), err)

				// did not receive a reply, decide what to do..
				retryAttempts++
				fmt.Printf("Retry attempts:%v, retries: %v, timeout: %v\n", retryAttempts, message.Retries, message.Timeout)
				switch {
				case message.Retries == 0:
					// 0 indicates unlimited retries
					continue
				case retryAttempts >= message.Retries:
					// max retries reached
					log.Printf("info: max retries for message reached, breaking out: %v", retryAttempts)
					return
				default:
					// none of the above matched, so we've not reached max retries yet
					continue
				}
			}
			log.Printf("<--- publisher: received ACK for message: %s\n", msgReply.Data)
		}
		return
	}
}

// subscriberHandler will deserialize the message when a new message is
// received, check the MessageType field in the message to decide what
// kind of message it is and then it will check how to handle that message type,
// and then call the correct method handler for it.
//
// This handler function should be started in it's own go routine,so
// one individual handler is started per message received so we can keep
// the state of the message being processed, and then reply back to the
// correct sending process's reply, meaning so we ACK back to the correct
// publisher.
func (p process) subscriberHandler(natsConn *nats.Conn, thisNode string, msg *nats.Msg, s *server) {

	message := Message{}

	// Create a buffer to decode the gob encoded binary data back
	// to it's original structure.
	buf := bytes.NewBuffer(msg.Data)
	gobDec := gob.NewDecoder(buf)
	err := gobDec.Decode(&message)
	if err != nil {
		log.Printf("error: gob decoding failed: %v\n", err)
	}

	// TODO: Maybe the handling of the errors within the subscriber
	// should also involve the error-kernel to report back centrally
	// that there was a problem like missing method to handle a specific
	// method etc.
	switch {
	case p.subject.CommandOrEvent == CommandACK || p.subject.CommandOrEvent == EventACK:
		// REMOVED: log.Printf("info: subscriberHandler: ACK Message received received, preparing to call handler: %v\n", p.subject.name())
		mh, ok := p.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			// TODO: Check how errors should be handled here!!!
			log.Printf("error: subscriberHandler: method type not available: %v\n", p.subject.CommandOrEvent)
		}

		out := []byte("not allowed from " + message.FromNode)
		var err error

		// Check if we are allowed to receive from that host
		_, arOK1 := p.allowedReceivers[message.FromNode]
		_, arOK2 := p.allowedReceivers["*"]

		if arOK1 || arOK2 {
			// Start the method handler for that specific subject type.
			// The handler started here is what actually doing the action
			// that executed a CLI command, or writes to a log file on
			// the node who received the message.
			out, err = mh.handler(p, message, thisNode)

			if err != nil {
				// TODO: Send to error kernel ?
				log.Printf("error: subscriberHandler: failed to execute event: %v\n", err)
			}
		} else {
			log.Printf("info: we don't allow receiving from: %v, %v\n", message.FromNode, p.subject)
		}

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, out)

		// TESTING: Simulate that we also want to send some error that occured
		// to the errorCentral
		{
			err := fmt.Errorf("error: some testing error we want to send out from %v", p.node)
			sendErrorLogMessage(s.newMessagesCh, node(thisNode), err)
		}
	case p.subject.CommandOrEvent == CommandNACK || p.subject.CommandOrEvent == EventNACK:
		// REMOVED: log.Printf("info: subscriberHandler: ACK Message received received, preparing to call handler: %v\n", p.subject.name())
		mf, ok := p.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			// TODO: Check how errors should be handled here!!!
			log.Printf("error: subscriberHandler: method type not available: %v\n", p.subject.CommandOrEvent)
		}

		// Start the method handler for that specific subject type.
		// The handler started here is what actually doing the action
		// that executed a CLI command, or writes to a log file on
		// the node who received the message.
		//
		// since we don't send a reply for a NACK message, we don't care about the
		// out return when calling mf.handler
		//fmt.Printf("-- DEBUG 2.2.1: %#v\n\n", p.subject)
		_, err := mf.handler(p, message, thisNode)

		if err != nil {
			// TODO: Send to error kernel ?
			log.Printf("error: subscriberHandler: failed to execute event: %v\n", err)
		}
	default:
		log.Printf("info: did not find that specific type of command: %#v\n", p.subject.CommandOrEvent)
	}
}

// Subscribe will start up a Go routine under the hood calling the
// callback function specified when a new message is received.
func (p process) subscribeMessages(s *server) {
	//fmt.Printf("-- DEBUG 2.1: %#v, %#v, %#v\n\n", p.subject.name(), p.procFunc, p.procFuncCh)
	subject := string(p.subject.name())
	_, err := s.natsConn.Subscribe(subject, func(msg *nats.Msg) {
		// We start one handler per message received by using go routines here.
		// This is for being able to reply back the current publisher who sent
		// the message.
		//fmt.Printf("-- DEBUG 2.2: %#v, %#v, %#v\n\n", p.subject.name(), p.procFunc, p.procFuncCh)
		go p.subscriberHandler(s.natsConn, s.nodeName, msg, s)
	})
	if err != nil {
		log.Printf("error: Subscribe failed: %v\n", err)
	}
}

// publishMessages will do the publishing of messages for one single
// process.
func (p process) publishMessages(natsConn *nats.Conn, processes *processes) {
	for {
		// Wait and read the next message on the message channel
		m := <-p.subject.messageCh

		// Get the process name so we can look up the process in the
		// processes map, and increment the message counter.
		pn := processNameGet(p.subject.name(), processKindPublisher)
		m.ID = processes.active[pn].messageID

		p.messageDeliverNats(natsConn, m)

		// Signaling back to the ringbuffer that we are done with the
		// current message, and it can remove it from the ringbuffer.
		m.done <- struct{}{}

		// Increment the counter for the next message to be sent.
		p.messageID++
		processes.mu.Lock()
		processes.active[pn] = p
		processes.mu.Unlock()
		// REMOVED: sleep
		//time.Sleep(time.Second * 1)

		// NB: simulate that we get an error, and that we can send that
		// out of the process and receive it in another thread.
		// REMOVED: Error simulation
		// ep := errProcess{
		// 	infoText:      "process failed",
		// 	process:       p,
		// 	message:       m,
		// 	errorActionCh: make(chan errorAction),
		// }
		// s.errorKernel.errorCh <- ep
		//
		// // Wait for the response action back from the error kernel, and
		// // decide what to do. Should we continue, quit, or .... ?
		// switch <-ep.errorActionCh {
		// case errActionContinue:
		// 	log.Printf("The errAction was continue...so we're continuing\n")
		// }
	}
}
