package steward

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
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
	// allowedReceivers map[Node]struct{}
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
func newProcess(ctx context.Context, metrics *metrics, natsConn *nats.Conn, processes *processes, toRingbufferCh chan<- []subjectAndMessage, configuration *Configuration, subject Subject, errCh chan errProcess, processKind processKind, procFunc func() error) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	processes.lastProcessID++

	ctx, cancel := context.WithCancel(ctx)

	var method Method

	proc := process{
		messageID:        0,
		subject:          subject,
		node:             Node(configuration.NodeName),
		processID:        processes.lastProcessID,
		errorCh:          errCh,
		processKind:      processKind,
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

// procFunc is a function that will be started when a worker process
// is started. If a procFunc is registered when creating a new process
// the procFunc will be started as a go routine when the process is started,
// and stopped when the process is stopped.
//
// A procFunc can be started both for publishing and subscriber processes.
//
// When used with a subscriber process the usecase is most likely to handle
// some kind of state needed for a request type, since the handlers themselves
// can not hold state.
// With a subscriber handler you generally take the message in the handler and
// pass it on to the procFunc by putting it on the procFuncCh<-, and the
// message can then be read from the procFuncCh inside the procFunc, and we
// can do some further work on it, for example update registry for metrics that
// is needed for that specific request type.
//
// procFunc's can also be used to wrap in other types which we want to
// work with. An example can be handling of metrics which the message
// have no notion of, but a procFunc can have that wrapped in from when it was constructed.
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
	p.processes.metrics.promProcessesAllRunning.With(prometheus.Labels{"processName": string(processName)})

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
					er := fmt.Errorf("error: spawnWorker: start procFunc failed: %v", err)
					sendErrorLogMessage(p.configuration, procs.metrics, p.toRingbufferCh, Node(p.node), er)
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
					er := fmt.Errorf("error: spawnWorker: start procFunc failed: %v", err)
					sendErrorLogMessage(p.configuration, procs.metrics, p.toRingbufferCh, Node(p.node), er)
				}
			}()
		}

		p.natsSubscription = p.subscribeMessages()
	}

	p.processName = pn

	// Add information about the new process to the started processes map.
	procs.active.mu.Lock()
	procs.active.procNames[pn] = p
	procs.active.mu.Unlock()
}

// messageDeliverNats will create the Nats message with headers and payload.
// It will also take care of the delivering the message that is converted to
// gob or cbor format as a nats.Message. It will also take care of checking
// timeouts and retries specified for the message.
func (p process) messageDeliverNats(natsMsgPayload []byte, natsMsgHeader nats.Header, natsConn *nats.Conn, message Message) {
	retryAttempts := 0

	const publishTimer time.Duration = 5
	const subscribeSyncTimer time.Duration = 5

	// The for loop will run until the message is delivered successfully,
	// or that retries are reached.
	for {
		msg := &nats.Msg{
			Subject: string(p.subject.name()),
			// Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "CLICommandRequest"),
			// Structure of the reply message are:
			// <nodename>.<message type>.<method>.reply
			Reply:  fmt.Sprintf("%s.reply", p.subject.name()),
			Data:   natsMsgPayload,
			Header: natsMsgHeader,
		}

		//

		if p.subject.CommandOrEvent == CommandNACK || p.subject.CommandOrEvent == EventNACK {
			err := natsConn.PublishMsg(msg)
			if err != nil {
				er := fmt.Errorf("error: nats publish of hello failed: %v", err)
				log.Printf("%v\n", er)
				return
			}
			p.processes.metrics.promNatsDeliveredTotal.Inc()
			return
		}

		// The SubscribeSync used in the subscriber, will get messages that
		// are sent after it started subscribing.
		//
		// Create a subscriber for the reply message.
		subReply, err := natsConn.SubscribeSync(msg.Reply)
		if err != nil {
			er := fmt.Errorf("error: nats SubscribeSync failed: failed to create reply message for subject: %v, error: %v", msg.Reply, err)
			// sendErrorLogMessage(p.toRingbufferCh, node(p.node), er)
			log.Printf("%v, waiting %ds before retrying\n", er, subscribeSyncTimer)
			time.Sleep(time.Second * subscribeSyncTimer)
			subReply.Unsubscribe()
			continue
		}

		// Publish message
		err = natsConn.PublishMsg(msg)
		if err != nil {
			er := fmt.Errorf("error: nats publish failed: %v", err)
			// sendErrorLogMessage(p.toRingbufferCh, node(p.node), er)
			log.Printf("%v, waiting %ds before retrying\n", er, publishTimer)
			time.Sleep(time.Second * publishTimer)
			continue
		}

		// If the message is an ACK type of message we must check that a
		// reply, and if it is not we don't wait here at all.
		if p.subject.CommandOrEvent == CommandACK || p.subject.CommandOrEvent == EventACK {
			// Wait up until ACKTimeout specified for a reply,
			// continue and resend if no reply received,
			// or exit if max retries for the message reached.
			_, err := subReply.NextMsg(time.Second * time.Duration(message.ACKTimeout))
			if err != nil {
				er := fmt.Errorf("error: ack receive failed: subject=%v: %v", p.subject.name(), err)
				// sendErrorLogMessage(p.toRingbufferCh, p.node, er)
				log.Printf(" ** %v\n", er)

				// did not receive a reply, decide what to do..
				retryAttempts++
				log.Printf("Retry attempt:%v, retries: %v, ACKTimeout: %v, message.ID: %v\n", retryAttempts, message.Retries, message.ACKTimeout, message.ID)

				switch {
				//case message.Retries == 0:
				//	// 0 indicates unlimited retries
				//	continue
				case retryAttempts >= message.Retries:
					// max retries reached
					er := fmt.Errorf("info: toNode: %v, fromNode: %v, subject: %v, methodArgs: %v: max retries reached, check if node is up and running and if it got a subscriber started for the given REQ type", message.ToNode, message.FromNode, msg.Subject, message.MethodArgs)

					// We do not want to send errorLogs for REQErrorLog type since
					// it will just cause an endless loop.
					if message.Method != REQErrorLog {
						sendInfoLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, p.node, er)
					}

					log.Printf("%v\n", er)

					subReply.Unsubscribe()

					p.processes.metrics.promNatsMessagesFailedACKsTotal.Inc()
					return

				default:
					// none of the above matched, so we've not reached max retries yet
					log.Printf("max retries for message not reached, retrying sending of message with ID %v\n", message.ID)
					p.processes.metrics.promNatsMessagesMissedACKsTotal.Inc()
					continue
				}
			}
			// REMOVED: log.Printf("<--- publisher: received ACK from:%v, for: %v, data: %s\n", message.ToNode, message.Method, msgReply.Data)
		}

		subReply.Unsubscribe()

		p.processes.metrics.promNatsDeliveredTotal.Inc()

		return
	}
}

// messageSubscriberHandler will deserialize the message when a new message is
// received, check the MessageType field in the message to decide what
// kind of message it is and then it will check how to handle that message type,
// and then call the correct method handler for it.
//
// This handler function should be started in it's own go routine,so
// one individual handler is started per message received so we can keep
// the state of the message being processed, and then reply back to the
// correct sending process's reply, meaning so we ACK back to the correct
// publisher.
func (p process) messageSubscriberHandler(natsConn *nats.Conn, thisNode string, msg *nats.Msg) {

	// Variable to hold a copy of the message data, so we don't mess with
	// the original data since the original is a pointer value.
	msgData := make([]byte, len(msg.Data))
	copy(msgData, msg.Data)

	// fmt.Printf(" * DEBUG: header value on subscriberHandler: %v\n", msg.Header)

	// If compression is used, decompress it to get the gob data. If
	// compression is not used it is the gob encoded data we already
	// got in msgData so we do nothing with it.
	if val, ok := msg.Header["cmp"]; ok {
		// fmt.Printf(" * DEBUG: ok = %v, map = %v, len of val = %v\n", ok, msg.Header, len(val))
		switch val[0] {
		case "z":
			zr, err := zstd.NewReader(nil)
			if err != nil {
				log.Printf("error: zstd NewReader failed: %v\n", err)
				return
			}
			msgData, err = zr.DecodeAll(msg.Data, nil)
			if err != nil {
				er := fmt.Errorf("error: zstd decoding failed: %v", err)
				log.Printf("%v\n", er)
				sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
				return
			}

		}
	}

	message := Message{}

	// Check if serialization is specified.
	// Will default to gob serialization if nothing or non existing value is specified is specified.
	if val, ok := msg.Header["serial"]; ok {
		// fmt.Printf(" * DEBUG: ok = %v, map = %v, len of val = %v\n", ok, msg.Header, len(val))
		switch val[0] {
		case "cbor":
			err := cbor.Unmarshal(msgData, &message)
			if err != nil {
				er := fmt.Errorf("error: cbor decoding failed: %v", err)
				log.Printf("%v\n", er)
				sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
				return
			}
		default: // Deaults to gob if no match was found.
			r := bytes.NewReader(msgData)
			gobDec := gob.NewDecoder(r)

			err := gobDec.Decode(&message)
			if err != nil {
				er := fmt.Errorf("error: gob decoding failed: %v", err)
				log.Printf("%v\n", er)
				sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
				return
			}
		}

	} else {
		// Default to gob if serialization flag was not specified.
		r := bytes.NewReader(msgData)
		gobDec := gob.NewDecoder(r)

		err := gobDec.Decode(&message)
		if err != nil {
			er := fmt.Errorf("error: gob decoding failed: %v", err)
			log.Printf("%v\n", er)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
			return
		}
	}

	// Send final reply for a relayed message back to the originating node.
	//
	// Check if the previous message was a relayed message, and if true
	// make a copy of the current message where the to field is set to
	// the value of the previous message's RelayFromNode field, so we
	// also can send the a copy of the reply back to where it originated.
	if message.PreviousMessage != nil && message.PreviousMessage.RelayOriginalViaNode != "" {

		// make a copy of the message
		msgCopy := message
		msgCopy.ToNode = msgCopy.PreviousMessage.RelayFromNode

		// We set the replyMethod of the initial message.
		// If no RelayReplyMethod was found, we default to the reply
		// method of the previous message.
		switch {
		case msgCopy.PreviousMessage.RelayReplyMethod == "":
			er := fmt.Errorf("error: subscriberHandler: no PreviousMessage.RelayReplyMethod found, defaulting to the reply method of previous message: %v ", msgCopy)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, p.node, er)
			log.Printf("%v\n", er)
			msgCopy.Method = msgCopy.PreviousMessage.ReplyMethod
		case msgCopy.PreviousMessage.RelayReplyMethod != "":
			msgCopy.Method = msgCopy.PreviousMessage.RelayReplyMethod
		}

		// Reset the previousMessage relay fields so the message don't loop.
		message.PreviousMessage.RelayViaNode = ""
		message.PreviousMessage.RelayOriginalViaNode = ""

		// Create a SAM for the msg copy that will be sent back the where the
		// relayed message originated from.
		sam, err := newSubjectAndMessage(msgCopy)
		if err != nil {
			er := fmt.Errorf("error: subscriberHandler: newSubjectAndMessage : %v, message copy: %v", err, msgCopy)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, p.node, er)
			log.Printf("%v\n", er)
		}

		p.toRingbufferCh <- []subjectAndMessage{sam}
	}

	// Check if it is an ACK or NACK message, and do the appropriate action accordingly.
	//
	// With ACK messages Steward will keep the state of the message delivery, and try to
	// resend the message if an ACK is not received within the timeout/retries specified
	// in the message.
	// When a process sends an ACK message, it will stop and wait for the nats-reply message
	// for the time specified in the replyTimeout value. If no reply message is received
	// within the given timeout the publishing process will try to resend the message for
	// number of times specified in the retries field of the Steward message.
	// When receiving a Steward-message with ACK enabled we send a message back the the
	// node where the message originated using the msg.Reply subject field of the nats-message.
	//
	// With NACK messages we do not send a nats reply message, so the message will only be
	// sent from the publisher once, and if it is not delivered it will not be retried.
	switch {

	// Check for ACK type Commands or Event.
	case p.subject.CommandOrEvent == CommandACK || p.subject.CommandOrEvent == EventACK:
		// Look up the method handler for the specified method.
		mh, ok := p.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			er := fmt.Errorf("error: subscriberHandler: no such method type: %v", p.subject.CommandOrEvent)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
		}

		var out []byte
		var err error

		// Call the method handler for the specified method.
		out, err = mh.handler(p, message, thisNode)

		if err != nil {
			er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
		}

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, out)

	// Check for NACK type Commands or Event.
	case p.subject.CommandOrEvent == CommandNACK || p.subject.CommandOrEvent == EventNACK:
		mf, ok := p.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			er := fmt.Errorf("error: subscriberHandler: method type not available: %v", p.subject.CommandOrEvent)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
		}

		_, err := mf.handler(p, message, thisNode)

		if err != nil {
			er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
			sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)
		}

	default:
		er := fmt.Errorf("info: did not find that specific type of command or event: %#v", p.subject.CommandOrEvent)
		sendInfoLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(thisNode), er)

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
		go p.messageSubscriberHandler(p.natsConn, p.configuration.NodeName, msg)
	})
	if err != nil {
		log.Printf("error: Subscribe failed: %v\n", err)
		return nil
	}

	return natsSubscription
}

// publishMessages will do the publishing of messages for one single
// process. The function should be run as a goroutine, and will run
// as long as the process it belongs to is running.
func (p process) publishMessages(natsConn *nats.Conn) {
	var once sync.Once

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

		// Create the initial header, and set values below depending on the
		// various configuration options chosen.
		natsMsgHeader := nats.Header{}

		// The serialized value of the nats message payload
		var natsMsgPayloadSerialized []byte

		// encode the message structure into gob binary format before putting
		// it into a nats message.
		// Prepare a gob encoder with a buffer before we start the loop
		switch p.configuration.Serialization {
		case "cbor":
			b, err := cbor.Marshal(m)
			if err != nil {
				er := fmt.Errorf("error: messageDeliverNats: cbor encode message failed: %v", err)
				sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(p.node), er)
				continue
			}

			natsMsgPayloadSerialized = b
			natsMsgHeader["serial"] = []string{p.configuration.Serialization}

		default:
			var bufGob bytes.Buffer
			gobEnc := gob.NewEncoder(&bufGob)
			err = gobEnc.Encode(m)
			if err != nil {
				er := fmt.Errorf("error: messageDeliverNats: gob encode message failed: %v", err)
				sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(p.node), er)
				continue
			}

			natsMsgPayloadSerialized = bufGob.Bytes()
		}

		// Get the process name so we can look up the process in the
		// processes map, and increment the message counter.
		pn := processNameGet(p.subject.name(), processKindPublisher)
		m.ID = p.messageID

		// The compressed value of the nats message payload. The content
		// can either be compressed or in it's original form depening on
		// the outcome of the switch below, and if compression were chosen
		// or not.
		var natsMsgPayloadCompressed []byte

		// Compress the data payload if selected with configuration flag.
		// The compression chosen is later set in the nats msg header when
		// calling p.messageDeliverNats below.
		switch p.configuration.Compression {
		case "z": // zstd
			enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
			if err != nil {
				log.Printf("error: zstd write failed: %v\n", err)
			}
			natsMsgPayloadCompressed = enc.EncodeAll(natsMsgPayloadSerialized, nil)

			natsMsgHeader["cmp"] = []string{p.configuration.Compression}

		case "": // no compression
			natsMsgPayloadCompressed = natsMsgPayloadSerialized

		default: // no compression
			// Allways log the error to console.
			er := fmt.Errorf("error: compression type not defined: %v, setting default to zero compression", err)
			log.Printf("%v\n", er)

			// We only wan't to send the error message to errorCentral once.
			once.Do(func() {
				sendErrorLogMessage(p.configuration, p.processes.metrics, p.toRingbufferCh, Node(p.node), er)
			})

			natsMsgPayloadCompressed = natsMsgPayloadSerialized
		}

		// Create the Nats message with headers and payload, and do the
		// sending of the message.
		p.messageDeliverNats(natsMsgPayloadCompressed, natsMsgHeader, natsConn, m)

		// Signaling back to the ringbuffer that we are done with the
		// current message, and it can remove it from the ringbuffer.
		m.done <- struct{}{}

		// Increment the counter for the next message to be sent.
		//
		// TODO: Check if it is possible, or makes sense to move the
		// counter out of the map.
		p.messageID++

		{
			p.processes.active.mu.Lock()
			p.processes.active.procNames[pn] = p
			p.processes.active.mu.Unlock()
		}

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
