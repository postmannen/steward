package steward

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"os/exec"

	"github.com/nats-io/nats.go"
)

func (s *server) RunSubscriber() {

	// Create a channel to put the data received in the subscriber callback
	// function
	reqMsgCh := make(chan Message)

	// Subscribe will start up a Go routine under the hood calling the
	// callback function specified when a new message is received.
	subject := fmt.Sprintf("%s.%s.%s", s.thisNodeName, "command", "shellcommand")
	_, err := s.natsConn.Subscribe(subject, listenForMessage(s.natsConn, reqMsgCh, s.thisNodeName))
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
