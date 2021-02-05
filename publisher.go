// Notes:
package steward

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type MessageType string

// TODO: Figure it makes sense to have these types at all.
//  It might make more sense to implement these as two
//  individual subjects.
const (
	// shellCommand, command that will just wait for an
	// ack, and nothing of the output of the command are
	// delivered back in the reply ack message.
	// The message should contain the unique ID of the
	// command.
	Command MessageType = "command"
	// shellCommand, wait for and return the output
	// of the command in the ACK message. This means
	// that the command should be executed immediately
	// and that we should get the confirmation that it
	// was successful or not.
	Event MessageType = "event"
	// eventCommand, just wait for the ACK that the
	// message is received. What action happens on the
	// receiving side is up to the received to decide.
)

type Message struct {
	// The Unique ID of the message
	ID int `json:"id" yaml:"id"`
	// The actual data in the message
	// TODO: Change this to a slice instead...or maybe use an
	// interface type here to handle several data types ?
	Data []string `json:"data" yaml:"data"`
	// The type of the message being sent
	MessageType MessageType `json:"messageType" yaml:"messageType"`
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
	newMessagesCh chan []jsonFromFile
	// errorCh is used to report errors from a process
	// NB: Implementing this as an int to report for testing
	errorCh chan string
	// errorKernel
	errorKernel *errorKernel
}

// newServer will prepare and return a server type
func NewServer(brokerAddress string, nodeName string) (*server, error) {
	conn, err := nats.Connect(brokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	s := &server{
		nodeName:      nodeName,
		natsConn:      conn,
		processes:     make(map[subjectName]process),
		newMessagesCh: make(chan []jsonFromFile),
		errorCh:       make(chan string, 10),
	}

	// Start the error kernel that will do all the error handling
	// not done within a process.
	s.errorKernel = newErrorKernel()
	s.errorKernel.startErrorKernel(s.errorCh)

	return s, nil

}

func (s *server) PublisherStart() {
	// Start the checking the input file for new messages from operator.
	go getMessagesFromFile("./", "inmsg.txt", s.newMessagesCh)

	// Prepare and start a single process
	{
		sub := newSubject("ship1", "command", "shellcommand", "shell")
		proc := s.processPrepareNew(sub, s.errorCh)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawn(proc)
	}

	// Prepare and start a single process
	{
		sub := newSubject("ship2", "command", "shellcommand", "shell")
		proc := s.processPrepareNew(sub, s.errorCh)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawn(proc)
	}

	s.handleNewOperatorMessages()

	select {}

}

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
func (e *errorKernel) startErrorKernel(errorCh chan string) {
	// TODO: For now it will just print the error messages to the
	// console.
	go func() {

		for {
			er := <-errorCh
			log.Printf("*** ERROR_KERNEL: %#v, type=%T\n", er, er)
		}
	}()
}

// handleNewOperatorMessages will handle all the new operator messages
// given to the system, and route them to the correct subject queue.
func (s *server) handleNewOperatorMessages() {
	// Process the messages that have been received on the incomming
	// message pipe. Check and send if there are a specific subject
	// for it, and no subject exist throw an error.
	//
	// TODO: Later on the only thing that should be checked here is
	// that there is a node for the specific message, and the super-
	// visor should create the process with the wanted subject on both
	// the publishing and the receiving node. If there is no such node
	// an error should be generated and processed by the error-kernel.
	go func() {
		for v := range s.newMessagesCh {
			for _, vv := range v {

				m := vv.Message
				subjName := vv.Subject.name()
				fmt.Printf("** handleNewOperatorMessages: message: %v, ** subject: %#v\n", m, vv.Subject)
				_, ok := s.processes[subjName]
				if ok {
					log.Printf("info: found the specific subject: %v\n", subjName)
					// Put the message on the correct process's messageCh
					s.processes[subjName].subject.messageCh <- m
				} else {
					log.Printf("info: did not find that specific subject: %v\n", subjName)
					time.Sleep(time.Millisecond * 500)
					continue
				}
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
	MessageType MessageType `json:"messageType" yaml:"messageType"`
	// method, what is this message doing, etc. shellcommand, syslog, etc.
	Method string `json:"method" yaml:"method"`
	// domain is used to differentiate services. Like there can be more
	// logging services, but rarely more logging services for the same
	// thing. Domain is here used to differentiate the the services and
	// tell with one word what it is for.
	Domain string `json:"domain" yaml:"domain"`
	// messageCh is the channel for receiving new content to be sent
	messageCh chan Message
}

// newSubject will return a new variable of the type subject, and insert
// all the values given as arguments. It will also create the channel
// to receive new messages on the specific subject.
func newSubject(node string, messageType MessageType, method string, domain string) Subject {
	return Subject{
		Node:        node,
		MessageType: messageType,
		Method:      method,
		Domain:      domain,
		messageCh:   make(chan Message),
	}
}

type subjectName string

func (s Subject) name() subjectName {
	return subjectName(fmt.Sprintf("%s.%s.%s.%s", s.Node, s.MessageType, s.Method, s.Domain))
}

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
	errorCh chan string
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func (s *server) processPrepareNew(subject Subject, errCh chan string) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	s.lastProcessID++
	proc := process{
		messageID: 0,
		subject:   subject,
		node:      node(subject.Node),
		processID: s.lastProcessID,
		errorCh:   errCh,
		//messageCh: make(chan Message),
	}

	return proc
}

// spawnProcess will spawn a new process. It will give the process
// the next available ID, and also add the process to the processes
// map.
func (s *server) processSpawn(proc process) {
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
	for {
		// Wait and read the next message on the message channel
		m := <-proc.subject.messageCh
		m.ID = s.processes[proc.subject.name()].messageID
		messageDeliver(proc, m, s.natsConn)

		// Increment the counter for the next message to be sent.
		proc.messageID++
		s.processes[proc.subject.name()] = proc
		time.Sleep(time.Second * 1)

		// NB: simulate that we get an error, and that we can send that
		// out of the process and receive it in another thread.
		s.errorCh <- "received an error from process: " + fmt.Sprintf("ID=%v, subjectName=%v\n", proc.processID, proc.subject.name())
	}
}

func messageDeliver(proc process, message Message, natsConn *nats.Conn) {
	for {
		dataPayload, err := gobEncodePayload(message)
		if err != nil {
			log.Printf("error: createDataPayload: %v\n", err)
		}

		msg := &nats.Msg{
			Subject: string(proc.subject.name()),
			// Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "shellcommand"),
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
func gobEncodePayload(m Message) ([]byte, error) {
	var buf bytes.Buffer
	gobEnc := gob.NewEncoder(&buf)
	err := gobEnc.Encode(m)
	if err != nil {
		return nil, fmt.Errorf("error: gob.Enode failed: %v", err)
	}

	return buf.Bytes(), nil
}
