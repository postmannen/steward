package steward

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
)

// processKind are either kindSubscriber or kindPublisher, and are
// used to distinguish the kind of process to spawn and to know
// the process kind put in the process map.
type processKind string

const (
	processKindSubscriber processKind = "subscriber"
	processKindPublisher  processKind = "publisher"
)

// process holds all the logic to handle a message type and it's
// method, subscription/publishin messages for a subject, and more.
type process struct {
	messageID int
	// the subject used for the specific process. One process
	// can contain only one sender on a message bus, hence
	// also one subject
	subject Subject
	// Put a node here to be able know the node a process is at.
	// NB: Might not be needed later on.
	node Node
	// The processID for the current process
	processID int
	// errorCh is the same channel the errorKernel uses to
	// read incomming errors. By having this channel available
	// within a process we can send errors to the error kernel,
	// the EK desided what to do, and sends the action about
	// what to do back to the process where the error came from.
	errorCh     chan errProcess
	processKind processKind
	// Who are we allowed to receive from ?
	allowedReceivers map[Node]struct{}
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
	toRingbufferCh chan<- []subjectAndMessage
	// The structure who holds all processes information
	processes *processes
	// nats connection
	natsConn *nats.Conn
	// natsSubscription returned when calling natsConn.Subscribe
	natsSubscription *nats.Subscription
	// context
	ctx context.Context
	// context cancelFunc
	ctxCancel context.CancelFunc
	// Process name
	processName processName

	// startup holds the startup functions for starting up publisher
	// or subscriber processes
	startup *startup
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func newProcess(ctx context.Context, metrics *metrics, natsConn *nats.Conn, processes *processes, toRingbufferCh chan<- []subjectAndMessage, configuration *Configuration, subject Subject, errCh chan errProcess, processKind processKind, allowedReceivers []Node, procFunc func() error) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	processes.lastProcessID++

	// make the slice of allowedReceivers into a map value for easy lookup.
	m := make(map[Node]struct{})
	for _, a := range allowedReceivers {
		m[a] = struct{}{}
	}

	ctx, cancel := context.WithCancel(ctx)

	var method Method

	proc := process{
		messageID:        0,
		subject:          subject,
		node:             Node(configuration.NodeName),
		processID:        processes.lastProcessID,
		errorCh:          errCh,
		processKind:      processKind,
		allowedReceivers: m,
		methodsAvailable: method.GetMethodsAvailable(),
		toRingbufferCh:   toRingbufferCh,
		configuration:    configuration,
		processes:        processes,
		natsConn:         natsConn,
		ctx:              ctx,
		ctxCancel:        cancel,
		startup:          newStartup(metrics),
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
type procFunc func(ctx context.Context) error

// The purpose of this function is to check if we should start a
// publisher or subscriber process, where a process is a go routine
// that will handle either sending or receiving messages on one
// subject.
//
// It will give the process the next available ID, and also add the
// process to the processes map in the server structure.
func (p process) spawnWorker(procs *processes, natsConn *nats.Conn) {
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

	processName := processNameGet(p.subject.name(), p.processKind)

	// Add prometheus metrics for the process.
	p.processes.metrics.promProcessesVec.With(prometheus.Labels{"processName": string(processName)})

	// Start a publisher worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if p.processKind == processKindPublisher {

		// If there is a procFunc for the process, start it.
		if p.procFunc != nil {
			// Start the procFunc in it's own anonymous func so we are able
			// to get the return error.
			go func() {
				err := p.procFunc(p.ctx)
				if err != nil {
					er := fmt.Errorf("error: spawnWorker: procFunc failed: %v", err)
					sendErrorLogMessage(p.toRingbufferCh, Node(p.node), er)
				}
			}()
		}

		go p.publishMessages(natsConn)
	}

	// Start a subscriber worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if p.processKind == processKindSubscriber {
		// If there is a procFunc for the process, start it.
		if p.procFunc != nil {

			// Start the procFunc in it's own anonymous func so we are able
			// to get the return error.
			go func() {
				err := p.procFunc(p.ctx)
				if err != nil {
					er := fmt.Errorf("error: spawnWorker: procFunc failed: %v", err)
					sendErrorLogMessage(p.toRingbufferCh, Node(p.node), er)
				}
			}()
		}

		p.natsSubscription = p.subscribeMessages()
	}

	p.processName = pn

	// Add information about the new process to the started processes map.
	idProcMap := make(map[int]process)
	idProcMap[p.processID] = p

	procs.mu.Lock()
	procs.active[pn] = idProcMap
	procs.mu.Unlock()
}

// messageDeliverNats will take care of the delivering the message
// that is converted to gob format as a nats.Message. It will also
// take care of checking timeouts and retries specified for the
// message.
func (p process) messageDeliverNats(natsConn *nats.Conn, message Message) {
	retryAttempts := 0

	const publishTimer time.Duration = 5
	const subscribeSyncTimer time.Duration = 5

	for {
		dataPayload, err := gobEncodeMessage(message)
		if err != nil {
			er := fmt.Errorf("error: createDataPayload: %v", err)
			sendErrorLogMessage(p.toRingbufferCh, Node(p.node), er)
			continue
		}

		msg := &nats.Msg{
			Subject: string(p.subject.name()),
			// Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "CLICommandRequest"),
			// Structure of the reply message are:
			// <nodename>.<message type>.<method>.reply
			Reply: fmt.Sprintf("%s.reply", p.subject.name()),
			Data:  dataPayload,
		}

		// The SubscribeSync used in the subscriber, will get messages that
		// are sent after it started subscribing.
		//
		// Create a subscriber for the reply message.
		subReply, err := natsConn.SubscribeSync(msg.Reply)
		if err != nil {
			er := fmt.Errorf("error: nc.SubscribeSync failed: failed to create reply message: %v", err)
			// sendErrorLogMessage(p.toRingbufferCh, node(p.node), er)
			log.Printf("%v, waiting %ds before retrying\n", er, subscribeSyncTimer)
			time.Sleep(time.Second * subscribeSyncTimer)
			continue
		}

		// Publish message
		err = natsConn.PublishMsg(msg)
		if err != nil {
			er := fmt.Errorf("error: publish failed: %v", err)
			// sendErrorLogMessage(p.toRingbufferCh, node(p.node), er)
			log.Printf("%v, waiting %ds before retrying\n", er, publishTimer)
			time.Sleep(time.Second * publishTimer)
			continue
		}

		// If the message is an ACK type of message we must check that a
		// reply, and if it is not we don't wait here at all.
		// fmt.Printf("info: messageDeliverNats: preparing to send message: %v\n", message)
		if p.subject.CommandOrEvent == CommandACK || p.subject.CommandOrEvent == EventACK {
			// Wait up until ACKTimeout specified for a reply,
			// continue and resend if noo reply received,
			// or exit if max retries for the message reached.
			msgReply, err := subReply.NextMsg(time.Second * time.Duration(message.ACKTimeout))
			if err != nil {
				er := fmt.Errorf("error: subReply.NextMsg failed for node=%v, subject=%v: %v", p.node, p.subject.name(), err)
				// sendErrorLogMessage(p.toRingbufferCh, p.node, er)
				log.Printf(" ** %v\n", er)

				// did not receive a reply, decide what to do..
				retryAttempts++
				log.Printf("Retry attempts:%v, retries: %v, ACKTimeout: %v\n", retryAttempts, message.Retries, message.ACKTimeout)

				switch {
				case message.Retries == 0:
					// 0 indicates unlimited retries
					continue
				case retryAttempts >= message.Retries:
					// max retries reached
					er := fmt.Errorf("info: toNode: %v, fromNode: %v, method: %v: max retries reached, check if node is up and running and if it got a subscriber for the given REQ type", message.ToNode, message.FromNode, message.Method)
					sendErrorLogMessage(p.toRingbufferCh, p.node, er)
					return

				default:
					// none of the above matched, so we've not reached max retries yet
					continue
				}
			}
			log.Printf("<--- publisher: received ACK from:%v, for: %v, data: %s\n", message.ToNode, message.Method, msgReply.Data)
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
func (p process) subscriberHandler(natsConn *nats.Conn, thisNode string, msg *nats.Msg) {

	message := Message{}

	// Create a buffer to decode the gob encoded binary data back
	// to it's original structure.
	buf := bytes.NewBuffer(msg.Data)
	gobDec := gob.NewDecoder(buf)
	err := gobDec.Decode(&message)
	if err != nil {
		er := fmt.Errorf("error: gob decoding failed: %v", err)
		sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
	}

	// Check if it is an ACK or NACK message, and do the appropriate action accordingly.
	switch {
	// Check for ACK type Commands or Event.
	case p.subject.CommandOrEvent == CommandACK || p.subject.CommandOrEvent == EventACK:
		mh, ok := p.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			er := fmt.Errorf("error: subscriberHandler: method type not available: %v", p.subject.CommandOrEvent)
			sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
		}

		out := []byte("not allowed from " + message.FromNode)
		//var err error

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
				er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
				sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
			}
		} else {
			er := fmt.Errorf("info: we don't allow receiving from: %v, %v", message.FromNode, p.subject)
			sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
		}

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, out)

	// Check for NACK type Commands or Event.
	case p.subject.CommandOrEvent == CommandNACK || p.subject.CommandOrEvent == EventNACK:
		mf, ok := p.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			er := fmt.Errorf("error: subscriberHandler: method type not available: %v", p.subject.CommandOrEvent)
			sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
		}

		// Check if we are allowed to receive from that host
		_, arOK1 := p.allowedReceivers[message.FromNode]
		_, arOK2 := p.allowedReceivers["*"]

		if arOK1 || arOK2 {

			// Start the method handler for that specific subject type.
			// The handler started here is what actually doing the action
			// that executed a CLI command, or writes to a log file on
			// the node who received the message.
			//
			// since we don't send a reply for a NACK message, we don't care about the
			// out return when calling mf.handler
			_, err := mf.handler(p, message, thisNode)

			if err != nil {
				er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
				sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
			}
		} else {
			er := fmt.Errorf("info: we don't allow receiving from: %v, %v", message.FromNode, p.subject)
			sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)
		}

	default:
		er := fmt.Errorf("info: did not find that specific type of command: %#v", p.subject.CommandOrEvent)
		sendErrorLogMessage(p.toRingbufferCh, Node(thisNode), er)

	}
}

// SubscribeMessage will register the Nats callback function for the specified
// nats subject. This allows us to receive Nats messages for a given subject
// on a node.
func (p process) subscribeMessages() *nats.Subscription {
	subject := string(p.subject.name())
	natsSubscription, err := p.natsConn.Subscribe(subject, func(msg *nats.Msg) {
		//_, err := p.natsConn.Subscribe(subject, func(msg *nats.Msg) {

		// Start up the subscriber handler.
		go p.subscriberHandler(p.natsConn, p.configuration.NodeName, msg)
	})
	if err != nil {
		log.Printf("error: Subscribe failed: %v\n", err)
		return nil
	}

	return natsSubscription
}

// publishMessages will do the publishing of messages for one single
// process.
func (p process) publishMessages(natsConn *nats.Conn) {
	for {
		var err error
		var m Message

		// Wait and read the next message on the message channel, or
		// exit this function if Cancel are received via ctx.
		select {
		case m = <-p.subject.messageCh:
		case <-p.ctx.Done():
			er := fmt.Errorf("info: canceling publisher: %v", p.subject.name())
			//sendErrorLogMessage(p.toRingbufferCh, Node(p.node), er)
			log.Printf("%v\n", er)
			return
		}
		// Get the process name so we can look up the process in the
		// processes map, and increment the message counter.
		pn := processNameGet(p.subject.name(), processKindPublisher)
		m.ID = p.messageID

		p.messageDeliverNats(natsConn, m)

		// Signaling back to the ringbuffer that we are done with the
		// current message, and it can remove it from the ringbuffer.
		m.done <- struct{}{}

		// Increment the counter for the next message to be sent.
		p.messageID++
		p.processes.mu.Lock()
		p.processes.active[pn][p.processID] = p
		p.processes.mu.Unlock()

		// Handle the error.
		//
		// NOTE: None of the processes above generate an error, so the the
		// if clause will never be triggered. But keeping it here as an example
		// for now for how to handle errors.
		if err != nil {
			// Create an error type which also creates a channel which the
			// errorKernel will send back the action about what to do.
			ep := errProcess{
				infoText:      "process failed",
				process:       p,
				message:       m,
				errorActionCh: make(chan errorAction),
			}
			p.errorCh <- ep

			// Wait for the response action back from the error kernel, and
			// decide what to do. Should we continue, quit, or .... ?
			switch <-ep.errorActionCh {
			case errActionContinue:
				// Just log and continue
				log.Printf("The errAction was continue...so we're continuing\n")
			case errActionKill:
				log.Printf("The errAction was kill...so we're killing\n")
				// ....
			default:
				log.Printf("Info: publishMessages: The errAction was not defined, so we're doing nothing\n")
			}
		}
	}
}
