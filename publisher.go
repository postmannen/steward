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
	processes map[node]process
	// The last processID created
	lastProcessID int
	thisNodeName  string
}

// newServer will prepare and return a server type
func NewServer(brokerAddress string, nodeName string) (*server, error) {
	conn, err := nats.Connect(brokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	return &server{
		thisNodeName: nodeName,
		natsConn:     conn,
		processes:    make(map[node]process),
	}, nil
}

func (s *server) RunPublisher() {
	proc := s.prepareNewProcess("btship1")
	// fmt.Printf("*** %#v\n", proc)
	go s.spawnProcess(proc)

	proc = s.prepareNewProcess("btship2")
	// fmt.Printf("*** %#v\n", proc)
	go s.spawnProcess(proc)

	// start the error handling
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

	select {}

}

type node string

// process are represent the communication to one individual host
type process struct {
	messageID int
	subject   string
	// Put a node here to be able know the node a process is at.
	// NB: Might not be needed later on.
	node node
	// The processID for the current process
	processID int
	// errorCh is used to report errors from a process
	// NB: Implementing this as an int to report for testing
	errorCh chan string
}

func (s *server) prepareNewProcess(nodeName string) process {
	// create the initial configuration for a sessions communicating with 1 host.
	s.lastProcessID++
	proc := process{
		messageID: 0,
		node:      node(nodeName),
		processID: s.lastProcessID,
		errorCh:   make(chan string),
	}

	return proc
}

// spawnProcess will spawn a new process
func (s *server) spawnProcess(proc process) {
	mu.Lock()
	s.processes[proc.node] = proc
	mu.Unlock()

	// Loop creating one new message every second to simulate getting new
	// messages to deliver.
	for {
		m := getMessageToDeliver()
		m.ID = s.processes[proc.node].messageID
		messageDeliver(proc, m, s.natsConn)

		// Increment the counter for the next message to be sent.
		proc.messageID++
		s.processes[proc.node] = proc
		time.Sleep(time.Second * 1)

		// simulate that we get an error, and that we can send that
		// out of the process and receive it in another thread.
		// s.processes[proc.node].errorCh <- "received an error from process: " + fmt.Sprintf("%v\n", proc.processID)

		//fmt.Printf("%#v\n", s.processes[proc.node])
	}
}

// get MessageToDeliver will pick up the next message to be created.
// TODO: read this from local file or rest or....?
func getMessageToDeliver() Message {
	return Message{
		Data:        []string{"uname", "-a"},
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
			Subject: fmt.Sprintf("%s.%s.%s", proc.node, "command", "shellcommand"),
			// Structure of the reply message are:
			// reply.<nodename>.<message type>.<method>
			Reply: "reply." + string(proc.node) + "command.shellcommand",
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
			log.Printf("error: subRepl.NextMsg failed for node=%v pid=%v: %v\n", proc.node, proc.processID, err)
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
