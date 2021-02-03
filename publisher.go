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

var mu sync.Mutex

type messageType int

// TODO: Figure it makes sense to have these types at all.
//  It might make more sense to implement these as two
//  individual subjects.
const (
	// shellCommand, command that will just wait for an
	// ack, and nothing of the output of the command are
	// delivered back in the reply ack message.
	// The message should contain the unique ID of the
	// command.
	commandReturnOutput messageType = iota
	// shellCommand, wait for and return the output
	// of the command in the ACK message. This means
	// that the command should be executed immediately
	// and that we should get the confirmation that it
	// was successful or not.
	eventReturnAck messageType = iota
	// eventCommand, just wait for the ACK that the
	// message is received. What action happens on the
	// receiving side is up to the received to decide.
)

type Message struct {
	// The Unique ID of the message
	ID int
	// The actual data in the message
	// TODO: Change this to a slice instead...or maybe use an
	// interface type here to handle several data types ?
	Data []string
	// The type of the message being sent
	MessageType messageType
}

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	natsConn *nats.Conn
	// TODO: sessions should probably hold a slice/map of processes ?
	processes map[subjectName]process
	// The last processID created
	lastProcessID int
	nodeName      string
}

// newServer will prepare and return a server type
func NewServer(brokerAddress string, nodeName string) (*server, error) {
	conn, err := nats.Connect(brokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	s := &server{
		nodeName:  nodeName,
		natsConn:  conn,
		processes: make(map[subjectName]process),
	}

	go func() {

		for {
			for k := range s.processes {
				select {
				case e := <-s.processes[k].errorCh:
					fmt.Printf("*** %v\n", e)
				default:
					time.Sleep(time.Millisecond * 100)
				}

			}
		}
	}()

	return s, nil

}

func (s *server) PublisherStart() {
	// start the checking of files for input messages
	fileReadCh := make((chan []byte))
	go getMessagesFromFile("./", "inmsg.txt", fileReadCh)

	// TODO: For now we just print content of the files read.
	// Replace this whit a broker function that will know how
	// send it on to the correct publisher.
	go func() {
		for b := range fileReadCh {
			// Check if there are new content read from file input
			fmt.Printf("received: %s\n", b)

		}
	}()

	// Prepare and start a single process
	{
		sub := subject{
			node:        "btship1",
			messageType: "command",
			method:      "shellcommand",
			domain:      "shell",
			messageCh:   make(chan Message),
		}
		proc := s.processPrepareNew(sub)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawn(proc)
	}

	// Prepare and start a single process
	{
		sub := subject{
			node:        "btship2",
			messageType: "command",
			method:      "shellcommand",
			domain:      "shell",
			messageCh:   make(chan Message),
		}
		proc := s.processPrepareNew(sub)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawn(proc)
	}

	// Simulate generating some commands to be sent as messages to nodes.
	go func() {
		for {
			m := Message{
				Data:        []string{"bash", "-c", "uname -a"},
				MessageType: eventReturnAck,
			}
			subjName := subjectName("btship1.command.shellcommand.shell")
			_, ok := s.processes[subjName]
			if ok {
				s.processes[subjName].subject.messageCh <- m
			} else {
				time.Sleep(time.Millisecond * 500)
				continue
			}
		}
	}()

	// // Simulate generating some commands to be sent as messages to nodes.
	// go func() {
	// 	for {
	// 		m := Message{
	// 			Data:        []string{"bash", "-c", "uname -a"},
	// 			MessageType: eventReturnAck,
	// 		}
	// 		subjName := subjectName("btship2.command.shellcommand.shell")
	// 		_, ok := s.processes[subjName]
	// 		if ok {
	// 			s.processes[subjName].subject.messageCh <- m
	// 		} else {
	// 			time.Sleep(time.Millisecond * 500)
	// 			continue
	// 		}
	// 	}
	// }()

	select {}

}

type node string

// subject contains the representation of a subject to be used with one
// specific process
type subject struct {
	// node, the name of the node
	node string
	// messageType, command/event
	messageType string
	// method, what is this message doing, etc. shellcommand, syslog, etc.
	method string
	// domain is used to differentiate services. Like there can be more
	// logging services, but rarely more logging services for the same
	// thing. Domain is here used to differentiate the the services and
	// tell with one word what it is for.
	domain string
	// messageCh is the channel for receiving new content to be sent
	messageCh chan Message
}

type subjectName string

func (s subject) name() subjectName {
	return subjectName(fmt.Sprintf("%s.%s.%s.%s", s.node, s.messageType, s.method, s.domain))
}

// process are represent the communication to one individual host
type process struct {
	messageID int
	// the subject used for the specific process. One process
	// can contain only one sender on a message bus, hence
	// also one subject
	subject subject
	// Put a node here to be able know the node a process is at.
	// NB: Might not be needed later on.
	node node
	// The processID for the current process
	processID int
	// errorCh is used to report errors from a process
	// NB: Implementing this as an int to report for testing
	errorCh chan string
	// messageCh are the channel where we put the message we want
	// a process to send
	//messageCh chan Message
}

// prepareNewProcess will set the the provided values and the default
// values for a process.
func (s *server) processPrepareNew(subject subject) process {
	// create the initial configuration for a sessions communicating with 1 host process.
	s.lastProcessID++
	proc := process{
		messageID: 0,
		subject:   subject,
		node:      node(subject.node),
		processID: s.lastProcessID,
		errorCh:   make(chan string),
		//messageCh: make(chan Message),
	}

	return proc
}

// spawnProcess will spawn a new process. It will give the process
// the next available ID, and also add the process to the processes
// map.
func (s *server) processSpawn(proc process) {
	mu.Lock()
	// We use the full name of the subject to identify a unique
	// process. We can do that since a process can only handle
	// one message queue.
	s.processes[proc.subject.name()] = proc
	mu.Unlock()

	// Loop creating one new message every second to simulate getting new
	// messages to deliver.
	//
	// TODO: I think it makes most sense that the messages would come to
	// here from some other message-pickup-process, and that process will
	// give the message to the correct publisher process. A channel that
	// is listened on in the for loop below could be used to receive the
	// messages from the message-pickup-process.
	for {
		// m := getMessageToDeliver()
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
		s.processes[proc.subject.name()].errorCh <- "received an error from process: " + fmt.Sprintf("ID=%v, subjectName=%v\n", proc.processID, proc.subject.name())

		//fmt.Printf("%#v\n", s.processes[proc.node])
	}
}

// get MessageToDeliver will pick up the next message to be created.
// TODO: read this from local file or rest or....?
func getMessageToDeliver() Message {
	return Message{
		Data:        []string{"bash", "-c", "uname -a"},
		MessageType: eventReturnAck,
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
		fmt.Printf("publisher: received: %s\n", msgReply.Data)
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
