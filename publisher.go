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
)

type Message struct {
	ToNode node `json:"toNode" yaml:"toNode"`
	// The Unique ID of the message
	ID int `json:"id" yaml:"id"`
	// The actual data in the message
	// TODO: Change this to a slice instead...or maybe use an
	// interface type here to handle several data types ?
	Data []string `json:"data" yaml:"data"`
	// The type of the message being sent
	CommandOrEvent CommandOrEvent `json:"commandOrEvent" yaml:"commandOrEvent"`
	// method, what is this message doing, etc. shellCommand, syslog, etc.
	Method   Method `json:"method" yaml:"method"`
	FromNode node
	// done is used to signal when a message is fully processed.
	// This is used when choosing when to move the message from
	// the ringbuffer into the time series log.
	done chan struct{}
}

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	natsConn *nats.Conn
	// TODO: sessions should probably hold a slice/map of processes ?
	processes map[subjectName]process
	// The last processID created
	lastProcessID int
	// The name of the node
	nodeName string
	mu       sync.Mutex
	// The channel where we receive new messages from the outside to
	// insert into the system for being processed
	inputFromFileCh chan []subjectAndMessage
	// errorCh is used to report errors from a process
	// NB: Implementing this as an int to report for testing
	errorCh chan errProcess
	// errorKernel
	errorKernel *errorKernel
	// TODO: replace this with some structure to hold the logCh value
	logCh chan []byte
	// used to check if the methods specified in message is valid
	methodsAvailable MethodsAvailable
	// used to check if the commandOrEvent specified in message is valid
	commandOrEventAvailable CommandOrEventAvailable
}

// newServer will prepare and return a server type
func NewServer(brokerAddress string, nodeName string) (*server, error) {
	conn, err := nats.Connect(brokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	var m Method
	var co CommandOrEvent

	s := &server{
		nodeName:                nodeName,
		natsConn:                conn,
		processes:               make(map[subjectName]process),
		inputFromFileCh:         make(chan []subjectAndMessage),
		errorCh:                 make(chan errProcess, 2),
		logCh:                   make(chan []byte),
		methodsAvailable:        m.GetMethodsAvailable(),
		commandOrEventAvailable: co.GetCommandOrEventAvailable(),
	}

	// Start the error kernel that will do all the error handling
	// not done within a process.
	s.errorKernel = newErrorKernel()
	s.errorKernel.startErrorKernel(s.errorCh)

	return s, nil

}

// Start will spawn up all the defined subscriber processes.
// Spawning of publisher processes is done on the fly by checking
// if there is publisher process for a given message subject. This
// checking is also started here in Start by calling handleMessagesToPublish.
func (s *server) Start() {
	// Start the checking the input file for new messages from operator.
	go s.getMessagesFromFile("./", "inmsg.txt", s.inputFromFileCh)

	// Start the textLogging service that will run on the subscribers
	// TODO: Figure out how to structure event services like these
	go s.startTextLogging(s.logCh)

	// Start a subscriber for shellCommand messages
	{
		fmt.Printf("nodeName: %#v\n", s.nodeName)
		sub := newSubject(s.nodeName, "command", "shellCommand")
		proc := s.processPrepareNew(sub, s.errorCh, processKindSubscriber)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawnWorker(proc)
	}

	// Start a subscriber for textLogging messages
	{
		fmt.Printf("nodeName: %#v\n", s.nodeName)
		sub := newSubject(s.nodeName, "event", "textLogging")
		proc := s.processPrepareNew(sub, s.errorCh, processKindSubscriber)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawnWorker(proc)
	}

	time.Sleep(time.Second * 2)
	s.printProcessesMap()

	s.handleMessagesInRingbuffer()

	select {}

}

func (s *server) printProcessesMap() {
	fmt.Println("--------------------------------------------------------------------------------------------")
	fmt.Printf("*** Output of processes map :\n")
	for _, v := range s.processes {
		fmt.Printf("*** - : %v\n", v)
	}
	fmt.Println("--------------------------------------------------------------------------------------------")
}

// handleNewOperatorMessages will handle all the new operator messages
// given to the system, and route them to the correct subject queue.
func (s *server) handleMessagesInRingbuffer() {
	// Prepare and start a new ring buffer
	const bufferSize int = 100
	rb := newringBuffer(bufferSize)
	inCh := make(chan subjectAndMessage)
	outCh := make(chan subjectAndMessage)
	rb.start(inCh, outCh)

	// Start reading new messages received on the incomming message
	// pipe requested by operator, and fill them into the buffer.
	go func() {
		for samSlice := range s.inputFromFileCh {
			fmt.Println("----------------------DEBUG1--------------------------------")
			fmt.Printf("DEBUG!!!!!!!!!!!!!!\n")
			fmt.Println("----------------------DEBUG1END-----------------------------")
			for _, sam := range samSlice {
				inCh <- sam
			}
		}
		close(inCh)
	}()

	// Process the messages that are in the ring buffer. Check and
	// send if there are a specific subject for it, and no subject
	// exist throw an error.
	go func() {
		for sam := range outCh {
			// Check if the format of the message is correct.
			// TODO: Send a message to the error kernel here that
			// it was unable to process the message with the reason
			// why ?
			if _, ok := s.methodsAvailable.CheckIfExists(sam.Message.Method); !ok {
				continue
			}
			if !s.commandOrEventAvailable.CheckIfExists(sam.Message.CommandOrEvent) {
				continue
			}

			// Adding a label here so we are able to redo the sending
			// of the last message if a process with specified subject
			// is not present. The process will then be created, and
			// the code will loop back to the redo: label.

		redo:
			m := sam.Message
			subjName := sam.Subject.name()
			fmt.Printf("** handleNewOperatorMessages: message: %v, ** subject: %#v\n", m, sam.Subject)
			_, ok := s.processes[subjName]
			if ok {
				log.Printf("info: found the specific subject: %v\n", subjName)
				// Put the message on the correct process's messageCh
				s.processes[subjName].subject.messageCh <- m
			} else {
				// If a publisher do not exist for the given subject, create it, and
				// by using the goto at the end redo the process for this specific message.
				log.Printf("info: did not find that specific subject, starting new process for subject: %v\n", subjName)

				sub := newSubject(sam.Subject.Node, sam.Subject.CommandOrEvent, sam.Subject.Method)
				proc := s.processPrepareNew(sub, s.errorCh, processKindPublisher)
				// fmt.Printf("*** %#v\n", proc)
				go s.processSpawnWorker(proc)

				time.Sleep(time.Millisecond * 500)
				s.printProcessesMap()
				// Now when the process is spawned we jump back to the redo: label.
				goto redo
			}
		}
	}()
}

type node string

// subject contains the representation of a subject to be used with one
// specific process
type Subject struct {
	// node, the name of the node
	Node string `json:"node" yaml:"node"`
	// messageType, command/event
	CommandOrEvent CommandOrEvent `json:"commandOrEvent" yaml:"commandOrEvent"`
	// method, what is this message doing, etc. shellCommand, syslog, etc.
	Method Method `json:"method" yaml:"method"`
	// messageCh is the channel for receiving new content to be sent
	messageCh chan Message
}

// newSubject will return a new variable of the type subject, and insert
// all the values given as arguments. It will also create the channel
// to receive new messages on the specific subject.
func newSubject(node string, commandOrEvent CommandOrEvent, method Method) Subject {
	return Subject{
		Node:           node,
		CommandOrEvent: commandOrEvent,
		Method:         method,
		messageCh:      make(chan Message),
	}
}

// subjectName is the complete representation of a subject
type subjectName string

func (s Subject) name() subjectName {
	return subjectName(fmt.Sprintf("%s.%s.%s", s.Node, s.CommandOrEvent, s.Method))
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
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func (s *server) processPrepareNew(subject Subject, errCh chan errProcess, processKind processKind) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	s.lastProcessID++
	proc := process{
		messageID:   0,
		subject:     subject,
		node:        node(subject.Node),
		processID:   s.lastProcessID,
		errorCh:     errCh,
		processKind: processKind,
		//messageCh: make(chan Message),
	}

	return proc
}

// spawnProcess will spawn a new process. It will give the process
// the next available ID, and also add the process to the processes
// map.
func (s *server) processSpawnWorker(proc process) {
	s.mu.Lock()
	// We use the full name of the subject to identify a unique
	// process. We can do that since a process can only handle
	// one message queue.
	s.processes[proc.subject.name()] = proc
	s.mu.Unlock()

	// TODO: I think it makes most sense that the messages would come to
	// here from some other message-pickup-process, and that process will
	// give the message to the correct publisher process. A channel that
	// is listened on in the for loop below could be used to receive the
	// messages from the message-pickup-process.
	//
	// Handle publisher workers
	if proc.processKind == processKindPublisher {
		for {
			// Wait and read the next message on the message channel
			m := <-proc.subject.messageCh
			m.ID = s.processes[proc.subject.name()].messageID
			messageDeliver(proc, m, s.natsConn)
			m.done <- struct{}{}

			// Increment the counter for the next message to be sent.
			proc.messageID++
			s.processes[proc.subject.name()] = proc
			time.Sleep(time.Second * 1)

			// NB: simulate that we get an error, and that we can send that
			// out of the process and receive it in another thread.
			ep := errProcess{
				infoText:      "process failed",
				process:       proc,
				message:       m,
				errorActionCh: make(chan errorAction),
			}
			s.errorCh <- ep

			// Wait for the response action back from the error kernel, and
			// decide what to do. Should we continue, quit, or .... ?
			switch <-ep.errorActionCh {
			case errActionContinue:
				log.Printf("The errAction was continue...so we're continuing\n")
			}
		}
	}

	// handle subscriber workers
	if proc.processKind == processKindSubscriber {
		subject := string(proc.subject.name())

		// Subscribe will start up a Go routine under the hood calling the
		// callback function specified when a new message is received.
		_, err := s.natsConn.Subscribe(subject, func(msg *nats.Msg) {
			// We start one handler per message received by using go routines here.
			// This is for being able to reply back the current publisher who sent
			// the message.
			go s.subscriberHandler(s.natsConn, s.nodeName, msg)
		})
		if err != nil {
			log.Printf("error: Subscribe failed: %v\n", err)
		}
	}
}

func messageDeliver(proc process, message Message, natsConn *nats.Conn) {
	for {
		dataPayload, err := gobEncodeMessage(message)
		if err != nil {
			log.Printf("error: createDataPayload: %v\n", err)
		}

		msg := &nats.Msg{
			Subject: string(proc.subject.name()),
			// Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "shellCommand"),
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
		subReply, err := natsConn.SubscribeSync(msg.Reply)
		if err != nil {
			log.Printf("error: nc.SubscribeSync failed: %v\n", err)
			os.Exit(1)
			continue
		}

		// Publish message
		err = natsConn.PublishMsg(msg)
		if err != nil {
			log.Printf("error: publish failed: %v\n", err)
			continue
		}

		// Wait up until 10 seconds for a reply,
		// continue and resend if to reply received.
		msgReply, err := subReply.NextMsg(time.Second * 10)
		if err != nil {
			log.Printf("error: subRepl.NextMsg failed for node=%v, subject=%v: %v\n", proc.node, proc.subject.name(), err)
			// did not receive a reply, continuing from top again
			continue
		}
		log.Printf("publisher: received ACK: %s\n", msgReply.Data)
		return
	}
}

// gobEncodePayload will encode the message structure along with its
// valued in gob binary format.
// TODO: Check if it adds value to compress with gzip.
func gobEncodeMessage(m Message) ([]byte, error) {
	var buf bytes.Buffer
	gobEnc := gob.NewEncoder(&buf)
	err := gobEnc.Encode(m)
	if err != nil {
		return nil, fmt.Errorf("error: gob.Encode failed: %v", err)
	}

	return buf.Bytes(), nil
}

// handler will deserialize the message when a new message is received,
// check the MessageType field in the message to decide what kind of
// message it is and then it will check how to handle that message type,
// and handle it.
// This handler function should be started in it's own go routine,so
// one individual handler is started per message received so we can keep
// the state of the message being processed, and then reply back to the
// correct sending process's reply, meaning so we ACK back to the correct
// publisher.
func (s *server) subscriberHandler(natsConn *nats.Conn, node string, msg *nats.Msg) {

	message := Message{}

	// Create a buffer to decode the gob encoded binary data back
	// to it's original structure.
	buf := bytes.NewBuffer(msg.Data)
	gobDec := gob.NewDecoder(buf)
	err := gobDec.Decode(&message)
	if err != nil {
		log.Printf("error: gob decoding failed: %v\n", err)
	}

	//fmt.Printf("%v\n", msg)
	// TODO: Maybe the handling of the errors within the subscriber
	// should also involve the error-kernel to report back centrally
	// that there was a problem like missing method to handle a specific
	// method etc.
	switch {
	case message.CommandOrEvent == Command || message.CommandOrEvent == Event:
		mf, ok := s.methodsAvailable.CheckIfExists(message.Method)
		if !ok {
			// TODO: Check how errors should be handled here!!!
			log.Printf("*****METHOD MISSING	\n")
		}
		out, err := mf.handler(s, message, node)

		if err != nil {
			// TODO: Send to error kernel ?
			log.Printf("error: failed to execute event: %v\n", err)
		}

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, out)
	default:
		log.Printf("info: did not find that specific type of command: %#v\n", message.CommandOrEvent)
	}
}
