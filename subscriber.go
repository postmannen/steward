package steward

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/nats-io/nats.go"
)

// RunSubscriber will start a subscribing process.
// TODO: Right now the only thing a subscriber can do is ro receive commands,
// check if there are more things a subscriber should be able to do.
func (s *server) RunSubscriber() {
	subject := fmt.Sprintf("%s.%s.%s.%s", s.nodeName, "command", "shellcommand", "shell")

	// Subscribe will start up a Go routine under the hood calling the
	// callback function specified when a new message is received.
	_, err := s.natsConn.Subscribe(subject, func(msg *nats.Msg) {
		go handler(s.natsConn, s.nodeName, msg)
	})
	if err != nil {
		log.Printf("error: Subscribe failed: %v\n", err)
	}

	// Do some further processing of the actual data we received in the
	// subscriber callback function.
	select {}
}

// Listen for message will send an ACK message back to the sender,
// and put the received incomming message on the reqMsg channel
// for further processing.
func handler(natsConn *nats.Conn, node string, msg *nats.Msg) {

	message := Message{}

	// Create a buffer to decode the gob encoded binary data back
	// to it's original structure.
	buf := bytes.NewBuffer(msg.Data)
	gobDec := gob.NewDecoder(buf)
	err := gobDec.Decode(&message)
	if err != nil {
		log.Printf("error: gob decoding failed: %v\n", err)
	}

	// ---------

	//fmt.Printf("%v\n", msg)
	switch message.MessageType {
	case "Command":
		// Since the command to execute is at the first position in the
		// slice we need to slice it out. The arguments are at the
		// remaining positions.
		c := message.Data[0]
		a := message.Data[1:]
		cmd := exec.Command(c, a...)
		//cmd.Stdout = os.Stdout
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("error: execution of command failed: %v\n", err)
		}
		fmt.Printf("%s", out)

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, []byte("confirmed from: "+node+": "+fmt.Sprintf("%v\n%s", message.ID, out)))
	case "Event":
		// Since the command to execute is at the first position in the
		// slice we need to slice it out. The arguments are at the
		// remaining positions.
		c := message.Data[0]
		a := message.Data[1:]
		cmd := exec.Command(c, a...)
		cmd.Stdout = os.Stdout
		err := cmd.Start()
		if err != nil {
			log.Printf("error: execution of command failed: %v\n", err)
		}

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, []byte("confirmed from: "+node+": "+fmt.Sprint(message.ID)))
	default:
		log.Printf("info: did not find that specific type of command: %#v\n", message.MessageType)
	}
	// ---------
}
