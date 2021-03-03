// Notes:
package steward

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
)

type processName string

func processNameGet(sn subjectName, pk processKind) processName {
	pn := fmt.Sprintf("%s_%s", sn, pk)
	return processName(pn)
}

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	// Configuration options used for running the server
	configuration *Configuration
	// The nats connection to the broker
	natsConn *nats.Conn
	// TODO: sessions should probably hold a slice/map of processes ?
	processes map[processName]process
	// The last processID created
	lastProcessID int
	// The name of the node
	nodeName string
	// Mutex for locking when writing to the process map
	mu sync.Mutex
	// The channel where we put new messages read from file,
	// or some other process who wants to send something via the
	// system
	// We can than range this channel for new messages to process.
	newMessagesCh chan []subjectAndMessage
	// errorKernel is doing all the error handling like what to do if
	// an error occurs.
	// TODO: Will also send error messages to cental error subscriber.
	errorKernel *errorKernel
	// used to check if the methods specified in message is valid
	methodsAvailable MethodsAvailable
	// Map who holds the command and event types available.
	// Used to check if the commandOrEvent specified in message is valid
	commandOrEventAvailable CommandOrEventAvailable
	// metric exporter
	metrics *metrics
	// subscriberServices are where we find the services and the API to
	// use services needed by subscriber.
	// For example, this can be a service that knows
	// how to forward the data for a received message of type log to a
	// central logger.
	subscriberServices *subscriberServices
	// Is this the central error logger ?
	// collection of the publisher services and the types to control them
	publisherServices  *publisherServices
	centralErrorLogger bool
	// default message timeout in seconds. This can be overridden on the message level
	defaultMessageTimeout int
	// default amount of retries that will be done before a message is thrown away, and out of the system
	defaultMessageRetries int
}

// newServer will prepare and return a server type
func NewServer(c *Configuration) (*server, error) {
	conn, err := nats.Connect(c.BrokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	var m Method
	var coe CommandOrEvent

	s := &server{
		configuration:           c,
		nodeName:                c.NodeName,
		natsConn:                conn,
		processes:               make(map[processName]process),
		newMessagesCh:           make(chan []subjectAndMessage),
		methodsAvailable:        m.GetMethodsAvailable(),
		commandOrEventAvailable: coe.GetCommandOrEventAvailable(),
		metrics:                 newMetrics(c.PromHostAndPort),
		subscriberServices:      newSubscriberServices(),
		publisherServices:       newPublisherServices(c.PublisherServiceSayhello),
		centralErrorLogger:      c.CentralErrorLogger,
		defaultMessageTimeout:   c.DefaultMessageTimeout,
		defaultMessageRetries:   c.DefaultMessageRetries,
	}

	// Create the default data folder for where subscribers should
	// write it's data if needed.

	// Check if data folder exist, and create it if needed.
	if _, err := os.Stat(c.SubscribersDataFolder); os.IsNotExist(err) {
		if c.SubscribersDataFolder == "" {
			return nil, fmt.Errorf("error: subscribersDataFolder value is empty, you need to provide the config or the flag value at startup %v: %v", c.SubscribersDataFolder, err)
		}
		err := os.Mkdir(c.SubscribersDataFolder, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create directory %v: %v", c.SubscribersDataFolder, err)
		}

		log.Printf("info: Creating subscribers data folder at %v\n", c.SubscribersDataFolder)
	}

	return s, nil

}

// Start will spawn up all the predefined subscriber processes.
// Spawning of publisher processes is done on the fly by checking
// if there is publisher process for a given message subject, and
// not exist it will spawn one.
func (s *server) Start() {
	// Start the error kernel that will do all the error handling
	// not done within a process.
	s.errorKernel = newErrorKernel()
	s.errorKernel.startErrorKernel(s.newMessagesCh)

	// Start collecting the metrics
	go s.startMetrics()

	// Start the checking the input file for new messages from operator.
	go s.getMessagesFromFile("./", "inmsg.txt", s.newMessagesCh)

	// if enabled, start the sayHello I'm here service at the given interval
	if s.publisherServices.sayHelloPublisher.interval != 0 {
		go s.publisherServices.sayHelloPublisher.start(s.newMessagesCh, node(s.nodeName))
	}

	// Start up the predefined subscribers.
	// TODO: What to subscribe on should be handled via flags, or config
	// files.
	s.subscribersStart()

	time.Sleep(time.Second * 2)
	s.printProcessesMap()

	// Start the processing of new messaging from an input channel.
	s.processNewMessages("./incommmingBuffer.db", s.newMessagesCh)

	select {}

}

func (s *server) printProcessesMap() {
	fmt.Println("--------------------------------------------------------------------------------------------")
	fmt.Printf("*** Output of processes map :\n")
	for _, v := range s.processes {
		fmt.Printf("*** - : %v\n", v)
	}

	s.metrics.metricsCh <- metricType{
		metric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "total_running_processes",
			Help: "The current number of total running processes",
		}),
		value: float64(len(s.processes)),
	}

	fmt.Println("--------------------------------------------------------------------------------------------")
}

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
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func (s *server) processPrepareNew(subject Subject, errCh chan errProcess, processKind processKind, allowedReceivers []node) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	s.lastProcessID++

	// make the slice of allowedReceivers into a map value for easy lookup.
	m := make(map[node]struct{})
	for _, a := range allowedReceivers {
		m[a] = struct{}{}
	}

	proc := process{
		messageID:        0,
		subject:          subject,
		node:             node(subject.ToNode),
		processID:        s.lastProcessID,
		errorCh:          errCh,
		processKind:      processKind,
		allowedReceivers: m,
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
func (s *server) spawnWorkerProcess(proc process) {
	s.mu.Lock()
	// We use the full name of the subject to identify a unique
	// process. We can do that since a process can only handle
	// one message queue.
	var pn processName
	if proc.processKind == processKindPublisher {
		pn = processNameGet(proc.subject.name(), processKindPublisher)
	}
	if proc.processKind == processKindSubscriber {
		pn = processNameGet(proc.subject.name(), processKindSubscriber)
	}

	// Add information about the new process to the started processes map.
	s.processes[pn] = proc
	s.mu.Unlock()

	// Start a publisher worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if proc.processKind == processKindPublisher {
		s.publishMessages(proc)
	}

	// Start a subscriber worker, which will start a go routine (process)
	// That will take care of all the messages for the subject it owns.
	if proc.processKind == processKindSubscriber {
		s.subscribeMessages(proc)
	}
}

// messageDeliverNats will take care of the delivering the message
// as converted to gob format as a nats.Message. It will also take
// care of checking timeouts and retries specified for the message.
func (s *server) messageDeliverNats(proc process, message Message) {
	retryAttempts := 0

	for {
		dataPayload, err := gobEncodeMessage(message)
		if err != nil {
			log.Printf("error: createDataPayload: %v\n", err)
		}

		msg := &nats.Msg{
			Subject: string(proc.subject.name()),
			// Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "CLICommand"),
			// Structure of the reply message are:
			// reply.<nodename>.<message type>.<method>
			Reply: fmt.Sprintf("reply.%s", proc.subject.name()),
			Data:  dataPayload,
		}

		// The SubscribeSync used in the subscriber, will get messages that
		// are sent after it started subscribing, so we start a publisher
		// that sends out a message every second.
		//
		// Create a subscriber for the reply message.
		subReply, err := s.natsConn.SubscribeSync(msg.Reply)
		if err != nil {
			log.Printf("error: nc.SubscribeSync failed: failed to create reply message: %v\n", err)
			continue
		}

		// Publish message
		err = s.natsConn.PublishMsg(msg)
		if err != nil {
			log.Printf("error: publish failed: %v\n", err)
			continue
		}

		// If the message is an ACK type of message we must check that a
		// reply, and if it is not we don't wait here at all.
		fmt.Printf("info: messageDeliverNats: preparing to send message: %v\n", message)
		if proc.subject.CommandOrEvent == CommandACK || proc.subject.CommandOrEvent == EventACK {
			// Wait up until timeout specified for a reply,
			// continue and resend if noo reply received,
			// or exit if max retries for the message reached.
			msgReply, err := subReply.NextMsg(time.Second * time.Duration(message.Timeout))
			if err != nil {
				log.Printf("error: subReply.NextMsg failed for node=%v, subject=%v: %v\n", proc.node, proc.subject.name(), err)

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
func (s *server) subscriberHandler(natsConn *nats.Conn, thisNode string, msg *nats.Msg, proc process) {

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
	case proc.subject.CommandOrEvent == CommandACK || proc.subject.CommandOrEvent == EventACK:
		log.Printf("info: subscriberHandler: ACK Message received received, preparing to call handler: %v\n", proc.subject.name())
		mf, ok := s.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			// TODO: Check how errors should be handled here!!!
			log.Printf("error: subscriberHandler: method type not available: %v\n", proc.subject.CommandOrEvent)
		}

		out := []byte("not allowed from " + message.FromNode)
		var err error

		// Check if we are allowed to receive from that host
		_, arOK1 := proc.allowedReceivers[message.FromNode]
		_, arOK2 := proc.allowedReceivers["*"]

		if arOK1 || arOK2 {
			// Start the method handler for that specific subject type.
			// The handler started here is what actually doing the action
			// that executed a CLI command, or writes to a log file on
			// the node who received the message.
			out, err = mf.handler(s, proc, message, thisNode)

			if err != nil {
				// TODO: Send to error kernel ?
				log.Printf("error: subscriberHandler: failed to execute event: %v\n", err)
			}
		} else {
			log.Printf("info: we don't allow receiving from: %v, %v\n", message.FromNode, proc.subject)
		}

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, out)

		// TESTING: Simulate that we also want to send some error that occured
		// to the errorCentral
		{
			err := fmt.Errorf("error: some testing error we want to send out")
			sendErrorLogMessage(s.newMessagesCh, node(thisNode), err)
		}
	case proc.subject.CommandOrEvent == CommandNACK || proc.subject.CommandOrEvent == EventNACK:
		log.Printf("info: subscriberHandler: ACK Message received received, preparing to call handler: %v\n", proc.subject.name())
		mf, ok := s.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			// TODO: Check how errors should be handled here!!!
			log.Printf("error: subscriberHandler: method type not available: %v\n", proc.subject.CommandOrEvent)
		}

		// Start the method handler for that specific subject type.
		// The handler started here is what actually doing the action
		// that executed a CLI command, or writes to a log file on
		// the node who received the message.
		//
		// since we don't send a reply for a NACK message, we don't care about the
		// out return when calling mf.handler
		_, err := mf.handler(s, proc, message, thisNode)

		if err != nil {
			// TODO: Send to error kernel ?
			log.Printf("error: subscriberHandler: failed to execute event: %v\n", err)
		}
	default:
		log.Printf("info: did not find that specific type of command: %#v\n", proc.subject.CommandOrEvent)
	}
}

// sendErrorMessage will put the error message directly on the channel that is
// read by the nats publishing functions.
func sendErrorLogMessage(newMessagesCh chan<- []subjectAndMessage, FromNode node, theError error) {
	// --- Testing
	sam := createErrorMsgContent(FromNode, theError)
	newMessagesCh <- []subjectAndMessage{sam}
}

// createErrorMsgContent will prepare a subject and message with the content
// of the error
func createErrorMsgContent(FromNode node, theError error) subjectAndMessage {
	// TESTING: Creating an error message to send to errorCentral
	fmt.Printf(" --- Sending error message to central !!!!!!!!!!!!!!!!!!!!!!!!!!!!\n")
	sam := subjectAndMessage{
		Subject: Subject{
			ToNode:         "errorCentral",
			CommandOrEvent: EventNACK,
			Method:         ErrorLog,
		},
		Message: Message{
			ToNode:   "errorCentral",
			FromNode: FromNode,
			Data:     []string{theError.Error()},
			Method:   ErrorLog,
		},
	}

	return sam
}
