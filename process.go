package steward

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	// "google.golang.org/protobuf/internal/errors"
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
	// isSubProcess is used to indentify subprocesses spawned by other processes.
	isSubProcess bool
	// server
	server *server
	// messageID
	messageID int
	// the subject used for the specific process. One process
	// can contain only one sender on a message bus, hence
	// also one subject
	subject Subject
	// Put a node here to be able know the node a process is at.
	// NB: Might not be needed later on.
	node Node
	// The processID for the current process
	processID   int
	processKind processKind
	// methodsAvailable
	methodsAvailable MethodsAvailable
	// procFunc is a function that will be started when a worker process
	// is started. If a procFunc is registered when creating a new process
	// the procFunc will be started as a go routine when the process is started,
	// and stopped when the process is stopped.
	//
	// A procFunc can be started both for publishing and subscriber processes.
	//
	// When used with a subscriber process the usecase is most likely to handle
	// some kind of state needed for a request type. The handlers themselves
	// can not hold state since they are only called once per message received,
	// and exits when the message is handled leaving no state behind. With a procfunc
	// we can have a process function running at all times tied to the process, and
	// this function can be able to hold the state needed in a certain scenario.
	//
	// With a subscriber handler you generally take the message in the handler and
	// pass it on to the procFunc by putting it on the procFuncCh<-, and the
	// message can then be read from the procFuncCh inside the procFunc, and we
	// can do some further work on it, for example update registry for metrics that
	// is needed for that specific request type.
	//
	// With a publisher process you can attach a static function that will do some
	// work to a request type, and publish the result.
	//
	// procFunc's can also be used to wrap in other types which we want to
	// work with. An example can be handling of metrics which the message
	// have no notion of, but a procFunc can have that wrapped in from when it was constructed.
	procFunc func(ctx context.Context, procFuncCh chan Message) error
	// The channel to send a messages to the procFunc go routine.
	// This is typically used within the methodHandler for so we
	// can pass messages between the procFunc and the handler.
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

	// handler is used to directly attach a handler to a process upon
	// creation of the process, like when a process is spawning a sub
	// process like REQCopySrc do. If we're not spawning a sub process
	// and it is a regular process the handler to use is found with the
	// getHandler method
	handler func(proc process, message Message, node string) ([]byte, error)

	// startup holds the startup functions for starting up publisher
	// or subscriber processes
	startup *startup
	// Signatures
	nodeAuth *nodeAuth
	// centralAuth
	centralAuth *centralAuth
	// errorKernel
	errorKernel *errorKernel
	// metrics
	metrics *metrics
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func newProcess(ctx context.Context, server *server, subject Subject, processKind processKind, procFunc func() error) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	server.processes.lastProcessID++

	ctx, cancel := context.WithCancel(ctx)

	var method Method

	proc := process{
		server:           server,
		messageID:        0,
		subject:          subject,
		node:             Node(server.configuration.NodeName),
		processID:        server.processes.lastProcessID,
		processKind:      processKind,
		methodsAvailable: method.GetMethodsAvailable(),
		toRingbufferCh:   server.toRingBufferCh,
		configuration:    server.configuration,
		processes:        server.processes,
		natsConn:         server.natsConn,
		ctx:              ctx,
		ctxCancel:        cancel,
		startup:          newStartup(server),
		nodeAuth:         server.nodeAuth,
		centralAuth:      server.centralAuth,
		errorKernel:      server.errorKernel,
		metrics:          server.metrics,
	}

	// We use the full name of the subject to identify a unique
	// process. We can do that since a process can only handle
	// one message queue.

	if proc.processKind == processKindPublisher {
		proc.processName = processNameGet(proc.subject.name(), processKindPublisher)
	}
	if proc.processKind == processKindSubscriber {
		proc.processName = processNameGet(proc.subject.name(), processKindSubscriber)
	}

	return proc
}

// The purpose of this function is to check if we should start a
// publisher or subscriber process, where a process is a go routine
// that will handle either sending or receiving messages on one
// subject.
//
// It will give the process the next available ID, and also add the
// process to the processes map in the server structure.
func (p process) spawnWorker() {

	// processName := processNameGet(p.subject.name(), p.processKind)

	// Add prometheus metrics for the process.
	p.metrics.promProcessesAllRunning.With(prometheus.Labels{"processName": string(p.processName)})

	// Start a publisher worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if p.processKind == processKindPublisher {

		// If there is a procFunc for the process, start it.
		if p.procFunc != nil {
			// Initialize the channel for communication between the proc and
			// the procFunc.
			p.procFuncCh = make(chan Message)

			// Start the procFunc in it's own anonymous func so we are able
			// to get the return error.
			go func() {
				err := p.procFunc(p.ctx, p.procFuncCh)
				if err != nil {
					er := fmt.Errorf("error: spawnWorker: start procFunc failed: %v", err)
					p.errorKernel.errSend(p, Message{}, er)
				}
			}()
		}

		go p.publishMessages(p.natsConn)
	}

	// Start a subscriber worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if p.processKind == processKindSubscriber {
		// If there is a procFunc for the process, start it.
		if p.procFunc != nil {
			// Initialize the channel for communication between the proc and
			// the procFunc.
			p.procFuncCh = make(chan Message)

			// Start the procFunc in it's own anonymous func so we are able
			// to get the return error.
			go func() {
				err := p.procFunc(p.ctx, p.procFuncCh)
				if err != nil {
					er := fmt.Errorf("error: spawnWorker: start procFunc failed: %v", err)
					p.errorKernel.errSend(p, Message{}, er)
				}
			}()
		}

		p.natsSubscription = p.subscribeMessages()
	}

	// Add information about the new process to the started processes map.
	p.processes.active.mu.Lock()
	p.processes.active.procNames[p.processName] = p
	p.processes.active.mu.Unlock()

	log.Printf("Successfully started process: %v\n", p.processName)
}

var (
	ErrACKSubscribeRetry = errors.New("steward: retrying to subscribe for ack message")
)

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

		// If it is a NACK message we just deliver the message and return
		// here so we don't create a ACK message and then stop waiting for it.
		if p.subject.Event == EventNACK {
			err := natsConn.PublishMsg(msg)
			if err != nil {
				er := fmt.Errorf("error: nats publish of hello failed: %v", err)
				log.Printf("%v\n", er)
				return
			}
			p.metrics.promNatsDeliveredTotal.Inc()

			// The reaming logic is for handling ACK messages, so we return here
			// since it was a NACK message, and all or now done.
			return
		}

		err := func() error {
			// The SubscribeSync used in the subscriber, will get messages that
			// are sent after it started subscribing.
			//
			// Create a subscriber for the ACK reply message.
			subReply, err := natsConn.SubscribeSync(msg.Reply)

			defer func() {
				err := subReply.Unsubscribe()
				if err != nil {
					log.Printf("error: nats SubscribeSync: failed when unsubscribing for ACK: %v\n", err)
				}
			}()

			if err != nil {
				er := fmt.Errorf("error: nats SubscribeSync failed: failed to create reply message for subject: %v, error: %v", msg.Reply, err)
				// sendErrorLogMessage(p.toRingbufferCh, node(p.node), er)
				log.Printf("%v, waiting %ds before retrying\n", er, subscribeSyncTimer)

				time.Sleep(time.Second * subscribeSyncTimer)
				// subReply.Unsubscribe()

				retryAttempts++
				return ErrACKSubscribeRetry
			}

			// Publish message
			err = natsConn.PublishMsg(msg)
			if err != nil {
				er := fmt.Errorf("error: nats publish failed: %v", err)
				// sendErrorLogMessage(p.toRingbufferCh, node(p.node), er)
				log.Printf("%v, waiting %ds before retrying\n", er, publishTimer)
				time.Sleep(time.Second * publishTimer)

				return ErrACKSubscribeRetry
			}

			// Wait up until ACKTimeout specified for a reply,
			// continue and resend if no reply received,
			// or exit if max retries for the message reached.
			//
			// The nats.Msg returned is discarded with '_' since
			// we don't use it.
			_, err = subReply.NextMsg(time.Second * time.Duration(message.ACKTimeout))
			if err != nil {
				if message.RetryWait < 0 {
					message.RetryWait = 0
				}

				switch {
				case err == nats.ErrNoResponders || err == nats.ErrTimeout:
					er := fmt.Errorf("error: ack receive failed: waiting for %v seconds before retrying:   subject=%v: %v", message.RetryWait, p.subject.name(), err)
					p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

					time.Sleep(time.Second * time.Duration(message.RetryWait))

					// Continue with the rest of the code to check number of retries etc..

				case err == nats.ErrBadSubscription || err == nats.ErrConnectionClosed:
					er := fmt.Errorf("error: ack receive failed: conneciton closed or bad subscription, will not retry message:   subject=%v: %v", p.subject.name(), err)
					p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

					return er

				default:
					er := fmt.Errorf("error: ack receive failed: the error was not defined, check if nats client have been updated with new error values, and update steward to handle the new error type:   subject=%v: %v", p.subject.name(), err)
					p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

					return er
				}

				// did not receive a reply, decide if we should try to retry sending.
				retryAttempts++
				er := fmt.Errorf("retry attempt:%v, retries: %v, ack timeout: %v, message.ID: %v", retryAttempts, message.Retries, message.ACKTimeout, message.ID)
				p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

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
						p.errorKernel.infoSend(p, message, er)
					}

					p.metrics.promNatsMessagesFailedACKsTotal.Inc()
					return er

				default:
					// none of the above matched, so we've not reached max retries yet
					er := fmt.Errorf("max retries for message not reached, retrying sending of message with ID %v", message.ID)
					p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

					p.metrics.promNatsMessagesMissedACKsTotal.Inc()

					return ErrACKSubscribeRetry
				}
			}

			return nil
		}()

		if err == ErrACKSubscribeRetry {
			continue
		}
		if err != nil {
			// All error printing are handled within the function that returns
			// the error, so we do nothing and return.
			// No more trying to deliver the message
			return
		}

		// Message were delivered successfully.
		p.metrics.promNatsDeliveredTotal.Inc()

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
func (p process) messageSubscriberHandler(natsConn *nats.Conn, thisNode string, msg *nats.Msg, subject string) {

	// Variable to hold a copy of the message data, so we don't mess with
	// the original data since the original is a pointer value.
	msgData := make([]byte, len(msg.Data))
	copy(msgData, msg.Data)

	// fmt.Printf(" * DEBUG: header value on subscriberHandler: %v\n", msg.Header)

	// If debugging is enabled, print the source node name of the nats messages received.
	if val, ok := msg.Header["fromNode"]; ok {
		er := fmt.Errorf("info: nats message received from %v, with subject %v ", val, subject)
		p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)
	}

	// If compression is used, decompress it to get the gob data. If
	// compression is not used it is the gob encoded data we already
	// got in msgData so we do nothing with it.
	if val, ok := msg.Header["cmp"]; ok {
		switch val[0] {
		case "z":
			zr, err := zstd.NewReader(nil)
			if err != nil {
				er := fmt.Errorf("error: zstd NewReader failed: %v", err)
				p.errorKernel.errSend(p, Message{}, er)
				return
			}
			msgData, err = zr.DecodeAll(msg.Data, nil)
			if err != nil {
				er := fmt.Errorf("error: zstd decoding failed: %v", err)
				p.errorKernel.errSend(p, Message{}, er)
				zr.Close()
				return
			}

			zr.Close()

		case "g":
			r := bytes.NewReader(msgData)
			gr, err := gzip.NewReader(r)
			if err != nil {
				er := fmt.Errorf("error: gzip NewReader failed: %v", err)
				p.errorKernel.errSend(p, Message{}, er)
				return
			}

			b, err := io.ReadAll(gr)
			if err != nil {
				er := fmt.Errorf("error: gzip ReadAll failed: %v", err)
				p.errorKernel.errSend(p, Message{}, er)
				return
			}

			gr.Close()

			msgData = b
		}
	}

	message := Message{}

	// Check if serialization is specified.
	// Will default to gob serialization if nothing or non existing value is specified.
	if val, ok := msg.Header["serial"]; ok {
		// fmt.Printf(" * DEBUG: ok = %v, map = %v, len of val = %v\n", ok, msg.Header, len(val))
		switch val[0] {
		case "cbor":
			err := cbor.Unmarshal(msgData, &message)
			if err != nil {
				er := fmt.Errorf("error: cbor decoding failed, subject: %v, header: %v, error: %v", subject, msg.Header, err)
				p.errorKernel.errSend(p, message, er)
				return
			}
		default: // Deaults to gob if no match was found.
			r := bytes.NewReader(msgData)
			gobDec := gob.NewDecoder(r)

			err := gobDec.Decode(&message)
			if err != nil {
				er := fmt.Errorf("error: gob decoding failed, subject: %v, header: %v, error: %v", subject, msg.Header, err)
				p.errorKernel.errSend(p, message, er)
				return
			}
		}

	} else {
		// Default to gob if serialization flag was not specified.
		r := bytes.NewReader(msgData)
		gobDec := gob.NewDecoder(r)

		err := gobDec.Decode(&message)
		if err != nil {
			er := fmt.Errorf("error: gob decoding failed, subject: %v, header: %v, error: %v", subject, msg.Header, err)
			p.errorKernel.errSend(p, message, er)
			return
		}
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

	// Check for ACK type Event.
	case p.subject.Event == EventACK:
		// When spawning sub processes we can directly assign handlers to the process upon
		// creation. We here check if a handler is already assigned, and if it is nil, we
		// lookup and find the correct handler to use if available.
		if p.handler == nil {
			// Look up the method handler for the specified method.
			mh, ok := p.methodsAvailable.CheckIfExists(message.Method)
			p.handler = mh.handler
			if !ok {
				er := fmt.Errorf("error: subscriberHandler: no such method type: %v", p.subject.Event)
				p.errorKernel.errSend(p, message, er)
			}
		}

		//var err error

		_ = p.callHandler(message, thisNode)

		// Send a confirmation message back to the publisher to ACK that the
		// message was received by the subscriber. The reply should be sent
		//no matter if the handler was executed successfully or not
		natsConn.Publish(msg.Reply, []byte{})

	case p.subject.Event == EventNACK:
		// When spawning sub processes we can directly assign handlers to the process upon
		// creation. We here check if a handler is already assigned, and if it is nil, we
		// lookup and find the correct handler to use if available.
		if p.handler == nil {
			// Look up the method handler for the specified method.
			mh, ok := p.methodsAvailable.CheckIfExists(message.Method)
			p.handler = mh.handler
			if !ok {
				er := fmt.Errorf("error: subscriberHandler: no such method type: %v", p.subject.Event)
				p.errorKernel.errSend(p, message, er)
			}
		}

		// We do not send reply messages for EventNACL, so we can discard the output.
		_ = p.callHandler(message, thisNode)

	default:
		er := fmt.Errorf("info: did not find that specific type of event: %#v", p.subject.Event)
		p.errorKernel.infoSend(p, message, er)

	}
}

// callHandler will call the handler for the Request type defined in the message.
// If checking signatures and/or acl's are enabled the signatures they will be
// verified, and if OK the handler is called.
func (p process) callHandler(message Message, thisNode string) []byte {
	//out := []byte{}
	var err error

	// Check if it is a message to run scheduled.
	var interval int
	var totalTime int
	var runAsScheduled bool
	switch {
	case len(message.Schedule) < 2:
		// Not at scheduled message,
	case len(message.Schedule) == 2:
		interval = message.Schedule[0]
		totalTime = message.Schedule[1]
		fallthrough

	case interval > 0 && totalTime > 0:
		runAsScheduled = true
	}

	// Call the handler if ACL/signature checking returns true.
	// If the handler is to be called in a scheduled manner, we we take care of that too.
	go func() {
		switch p.verifySigOrAclFlag(message) {

		case true:
			// Either ACL were verified OK, or ACL/Signature check was not enabled, so we call the handler.
			er := fmt.Errorf("info: subscriberHandler: Either ACL were verified OK, or ACL/Signature check was not enabled, so we call the handler: %v", true)
			p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

			switch {
			case !runAsScheduled:

				go func() {
					_, err = p.handler(p, message, thisNode)
					if err != nil {
						er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
						p.errorKernel.errSend(p, message, er)
						p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)
					}
				}()

			case runAsScheduled:
				// Create two tickers to use for the scheduling.
				intervalTicker := time.NewTicker(time.Second * time.Duration(interval))
				totalTimeTicker := time.NewTicker(time.Second * time.Duration(totalTime))

				// NB: Commented out this assignement of a specific message context
				// to be used within handlers, since it will override the structure
				// we have today. Keeping the code for a bit incase it makes sense
				// to implement later.
				//ctx, cancel := context.WithCancel(p.ctx)
				//message.ctx = ctx

				// Run the handler once, so we don't have to wait for the first ticker.
				go func() {
					_, err := p.handler(p, message, thisNode)
					if err != nil {
						er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
						p.errorKernel.errSend(p, message, er)
						p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)
					}
				}()

				for {
					select {
					case <-p.ctx.Done():
						er := fmt.Errorf("info: subscriberHandler: proc ctx done: toNode=%v, fromNode=%v, method=%v, methodArgs=%v", message.ToNode, message.FromNode, message.Method, message.MethodArgs)
						p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

						//cancel()
						return
					case <-totalTimeTicker.C:
						// Total time reached. End the process.
						//cancel()
						er := fmt.Errorf("info: subscriberHandler: schedule totalTime done: toNode=%v, fromNode=%v, method=%v, methodArgs=%v", message.ToNode, message.FromNode, message.Method, message.MethodArgs)
						p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

						return

					case <-intervalTicker.C:
						go func() {
							_, err := p.handler(p, message, thisNode)
							if err != nil {
								er := fmt.Errorf("error: subscriberHandler: handler method failed: %v", err)
								p.errorKernel.errSend(p, message, er)
								p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)
							}
						}()
					}
				}
			}

		case false:
			// ACL/Signature checking failed.
			er := fmt.Errorf("error: subscriberHandler: ACL were verified not-OK, doing nothing")
			p.errorKernel.errSend(p, message, er)
			log.Printf("%v\n", er)
		}
	}()

	return []byte{}
}

// verifySigOrAclFlag will do signature and/or acl checking based on which of
// those features are enabled, and then call the handler.
// The handler will also be called if neither signature or acl checking is enabled
// since it is up to the subscriber to decide if it want to use the auth features
// or not.
func (p process) verifySigOrAclFlag(message Message) bool {
	doHandler := false

	switch {

	// If no checking enabled we should just allow the message.
	case !p.nodeAuth.configuration.EnableSignatureCheck && !p.nodeAuth.configuration.EnableAclCheck:
		//log.Printf(" * DEBUG: verify acl/sig: no acl or signature checking at all is enabled, ALLOW the message, method=%v\n", message.Method)
		doHandler = true

	// If only sig check enabled, and sig OK, we should allow the message.
	case p.nodeAuth.configuration.EnableSignatureCheck && !p.nodeAuth.configuration.EnableAclCheck:
		sigOK := p.nodeAuth.verifySignature(message)

		log.Printf(" * DEBUG: verify acl/sig: Only signature checking enabled, ALLOW the message if sigOK, sigOK=%v, method %v\n", sigOK, message.Method)

		if sigOK {
			doHandler = true
		}

	// If both sig and acl check enabled, and sig and acl OK, we should allow the message.
	case p.nodeAuth.configuration.EnableSignatureCheck && p.nodeAuth.configuration.EnableAclCheck:
		sigOK := p.nodeAuth.verifySignature(message)
		aclOK := p.nodeAuth.verifyAcl(message)

		log.Printf(" * DEBUG: verify acl/sig:both signature and acl checking enabled, allow the message if sigOK and aclOK, or method is not REQCliCommand, sigOK=%v, aclOK=%v, method=%v\n", sigOK, aclOK, message.Method)

		if sigOK && aclOK {
			doHandler = true
		}

		// none of the verification options matched, we should keep the default value
		// of doHandler=false, so the handler is not done.
	default:
		log.Printf(" * DEBUG: verify acl/sig: None of the verify flags matched, not doing handler for message, method=%v\n", message.Method)
	}

	return doHandler
}

// SubscribeMessage will register the Nats callback function for the specified
// nats subject. This allows us to receive Nats messages for a given subject
// on a node.
func (p process) subscribeMessages() *nats.Subscription {
	subject := string(p.subject.name())
	// natsSubscription, err := p.natsConn.Subscribe(subject, func(msg *nats.Msg) {
	natsSubscription, err := p.natsConn.QueueSubscribe(subject, subject, func(msg *nats.Msg) {
		//_, err := p.natsConn.Subscribe(subject, func(msg *nats.Msg) {

		// Start up the subscriber handler.
		go p.messageSubscriberHandler(p.natsConn, p.configuration.NodeName, msg, subject)
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

	var zEnc *zstd.Encoder
	// Prepare a zstd encoder if enabled. By enabling it here before
	// looping over the messages to send below, we can reuse the zstd
	// encoder for all messages.
	switch p.configuration.Compression {
	case "z": // zstd
		// enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
		if err != nil {
			log.Printf("error: zstd new encoder failed: %v\n", err)
			os.Exit(1)
		}
		zEnc = enc
		defer zEnc.Close()

	}

	// Loop and handle 1 message at a time. If some part of the code
	// fails in the loop we should throw an error and use `continue`
	// to jump back here to the beginning of the loop and continue
	// with the next message.

	// Adding a timer that will be used for when to remove the sub process
	// publisher. The timer is reset each time a message is published with
	// the process, so the sub process publisher will not be removed until
	// it have not received any messages for the given amount of time.
	ticker := time.NewTicker(time.Second * time.Duration(p.configuration.KeepPublishersAliveFor))

	for {

		// Wait and read the next message on the message channel, or
		// exit this function if Cancel are received via ctx.
		select {
		case <-ticker.C:
			// We only want to remove subprocesses
			if p.isSubProcess {
				p.ctxCancel()

				p.processes.active.mu.Lock()
				delete(p.processes.active.procNames, p.processName)
				p.processes.active.mu.Unlock()
			}

		case m := <-p.subject.messageCh:
			ticker.Reset(time.Second * time.Duration(p.configuration.KeepPublishersAliveFor))
			// Sign the methodArgs, and add the signature to the message.
			m.ArgSignature = p.addMethodArgSignature(m)
			// fmt.Printf(" * DEBUG: add signature, fromNode: %v, method: %v,  len of signature: %v\n", m.FromNode, m.Method, len(m.ArgSignature))

			go p.publishAMessage(m, zEnc, once, natsConn)
		case <-p.ctx.Done():
			er := fmt.Errorf("info: canceling publisher: %v", p.processName)
			//sendErrorLogMessage(p.toRingbufferCh, Node(p.node), er)
			log.Printf("%v\n", er)
			return
		}
	}
}

func (p process) addMethodArgSignature(m Message) []byte {
	argsString := argsToString(m.MethodArgs)
	sign := ed25519.Sign(p.nodeAuth.SignPrivateKey, []byte(argsString))

	return sign
}

func (p process) publishAMessage(m Message, zEnc *zstd.Encoder, once sync.Once, natsConn *nats.Conn) {
	// Create the initial header, and set values below depending on the
	// various configuration options chosen.
	natsMsgHeader := make(nats.Header)
	natsMsgHeader["fromNode"] = []string{string(p.node)}

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
			p.errorKernel.errSend(p, m, er)
			return
		}

		natsMsgPayloadSerialized = b
		natsMsgHeader["serial"] = []string{p.configuration.Serialization}

	default:
		var bufGob bytes.Buffer
		gobEnc := gob.NewEncoder(&bufGob)
		err := gobEnc.Encode(m)
		if err != nil {
			er := fmt.Errorf("error: messageDeliverNats: gob encode message failed: %v", err)
			p.errorKernel.errSend(p, m, er)
			return
		}

		natsMsgPayloadSerialized = bufGob.Bytes()
		natsMsgHeader["serial"] = []string{"gob"}
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
		natsMsgPayloadCompressed = zEnc.EncodeAll(natsMsgPayloadSerialized, nil)
		natsMsgHeader["cmp"] = []string{p.configuration.Compression}

		// p.zEncMutex.Lock()
		// zEnc.Reset(nil)
		// p.zEncMutex.Unlock()

	case "g": // gzip
		var buf bytes.Buffer
		func() {
			gzipW := gzip.NewWriter(&buf)
			defer gzipW.Close()
			defer gzipW.Flush()
			_, err := gzipW.Write(natsMsgPayloadSerialized)
			if err != nil {
				log.Printf("error: failed to write gzip: %v\n", err)
				return
			}

		}()

		natsMsgPayloadCompressed = buf.Bytes()
		natsMsgHeader["cmp"] = []string{p.configuration.Compression}

	case "": // no compression
		natsMsgPayloadCompressed = natsMsgPayloadSerialized
		natsMsgHeader["cmp"] = []string{"none"}

	default: // no compression
		// Allways log the error to console.
		er := fmt.Errorf("error: publishing: compression type not defined, setting default to no compression")
		log.Printf("%v\n", er)

		// We only wan't to send the error message to errorCentral once.
		once.Do(func() {
			p.errorKernel.errSend(p, m, er)
		})

		// No compression, so we just assign the value of the serialized
		// data directly to the variable used with messageDeliverNats.
		natsMsgPayloadCompressed = natsMsgPayloadSerialized
		natsMsgHeader["cmp"] = []string{"none"}
	}

	// Create the Nats message with headers and payload, and do the
	// sending of the message.
	p.messageDeliverNats(natsMsgPayloadCompressed, natsMsgHeader, natsConn, m)

	select {
	case m.done <- struct{}{}:
		// Signaling back to the ringbuffer that we are done with the
		// current message, and it can remove it from the ringbuffer.
	case <-p.ctx.Done():
		return
	}

	// Increment the counter for the next message to be sent.
	p.messageID++

	{
		p.processes.active.mu.Lock()
		p.processes.active.procNames[pn] = p
		p.processes.active.mu.Unlock()
	}

	// // Handle the error.
	// //
	// // NOTE: None of the processes above generate an error, so the the
	// // if clause will never be triggered. But keeping it here as an example
	// // for now for how to handle errors.
	// if err != nil {
	// 	// Create an error type which also creates a channel which the
	// 	// errorKernel will send back the action about what to do.
	// 	ep := errorEvent{
	// 		//errorType:     logOnly,
	// 		process:       p,
	// 		message:       m,
	// 		errorActionCh: make(chan errorAction),
	// 	}
	// 	p.errorCh <- ep
	//
	// 	// Wait for the response action back from the error kernel, and
	// 	// decide what to do. Should we continue, quit, or .... ?
	// 	switch <-ep.errorActionCh {
	// 	case errActionContinue:
	// 		// Just log and continue
	// 		log.Printf("The errAction was continue...so we're continuing\n")
	// 	case errActionKill:
	// 		log.Printf("The errAction was kill...so we're killing\n")
	// 		// ....
	// 	default:
	// 		log.Printf("Info: publishMessages: The errAction was not defined, so we're doing nothing\n")
	// 	}
	// }
}
