package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/nats-io/nats.go"
)

type messageType int

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
	Data []string
	// The type of the message being sent
	MessageType messageType
}

func main() {
	node := flag.String("node", "0", "some unique string to identify this Edge unit")
	flag.Parse()

	// Create a connection to nats server, and publish a message.
	natsConn, err := nats.Connect("localhost", nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}
	defer natsConn.Close()

	// Create a channel to put the data received in the subscriber callback
	// function
	reqMsgCh := make(chan Message)

	// Subscribe will start up a Go routine under the hood calling the
	// callback function specified when a new message is received.
	_, err = natsConn.Subscribe(*node, listenForMessage(natsConn, reqMsgCh, *node))
	if err != nil {
		fmt.Printf("error: Subscribe failed: %v\n", err)
	}

	// Do some further processing of the actual data we received in the
	// subscriber callback function.
	for {
		msg := <-reqMsgCh
		fmt.Printf("%v\n", msg)
		switch msg.MessageType {
		case eventReturnAck:
			c := msg.Data[0]
			a := msg.Data[1:]
			cmd := exec.Command(c, a...)
			cmd.Stdout = os.Stdout
			err := cmd.Start()
			if err != nil {
				fmt.Printf("error: execution of command failed: %v\n", err)
			}
		}

	}
}

// Listen for message will send an ACK message back to the sender,
// and put the received incomming message on the reqMsg channel
// for further processing.
func listenForMessage(natsConn *nats.Conn, reqMsgCh chan Message, node string) func(req *nats.Msg) {
	return func(req *nats.Msg) {
		message := Message{}

		// Create a buffer to decode the gob encoded binary data back
		// to it's original structure.
		buf := bytes.NewBuffer(req.Data)
		gobDec := gob.NewDecoder(buf)
		err := gobDec.Decode(&message)
		if err != nil {
			fmt.Printf("error: gob decoding failed: %v\n", err)
		}

		// Put the data recived on the channel for further processing
		reqMsgCh <- message

		// Send a confirmation message back to the publisher
		natsConn.Publish(req.Reply, []byte("confirmed from: "+node+": "+fmt.Sprint(message.ID)))
	}
}
