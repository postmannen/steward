// Notes:
package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

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
}

// server is the structure that will hold the state about spawned
// processes on a local instance.

type server struct {
	natsConn *nats.Conn
	// TODO: sessions should probably hold a slice/map of processes ?
	processes map[node]process
	// The last processID created
	lastProcessID int
}

// newServer will prepare and return a server type
func newServer(brokerAddress string) (*server, error) {
	return &server{
		processes: make(map[node]process),
	}, nil
}

func (s *server) Run() {
	proc := s.prepareNewProcess("btship1")
	go s.spawnProcess(proc)

	select {}
}

func (s *server) prepareNewProcess(nodeName string) process {
	// create the initial configuration for a sessions communicating with 1 host.
	s.lastProcessID++
	proc := process{
		messageID: 0,
		node:      node(nodeName),
		processID: s.lastProcessID,
	}

	return proc
}

// spawnProcess will spawn a new process
func (s *server) spawnProcess(proc process) {
	s.processes[proc.node] = proc

	// Loop creating one new message every second to simulate getting new
	// messages to deliver.
	for {
		m := getMessageToDeliver()
		m.ID = s.processes["btship1"].messageID
		messageDeliver("btship1", m, s.natsConn)

		// Increment the counter for the next message to be sent.
		proc.messageID++
		s.processes["btship1"] = proc
		time.Sleep(time.Second * 1)
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

func messageDeliver(edgeID string, message Message, natsConn *nats.Conn) {
	for {
		dataPayload, err := gobEncodePayload(message)
		if err != nil {
			log.Printf("error: createDataPayload: %v\n", err)
		}

		msg := &nats.Msg{
			Subject: edgeID,
			Reply:   "subjectReply",
			Data:    dataPayload,
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
			log.Printf("error: subRepl.NextMsg failed: %v\n", err)
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

func main() {
	s, err := newServer("localhost")
	if err != nil {
		log.Printf("error: failed to connect to broker: %v\n", err)
		os.Exit(1)
	}
	// Create a connection to nats server
	s.natsConn, err = nats.Connect("localhost", nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}
	defer s.natsConn.Close()

	s.Run()
}
