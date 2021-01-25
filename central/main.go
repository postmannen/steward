// Same as 02 example, but using PublishMsg method instead of Request method
// for publishing.

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

type messageType int

const (
	// shellCommand, wait for and return the output
	// of the command in the ACK message. This means
	// that the command should be executed immediately
	// and that we should get the confirmation that it
	// was successful or not.
	shellCommand messageType = iota
	// eventCommand, just wait for the ACK that the
	// message is received. What action happens on the
	// receiving side is up to the received to decide.
	eventCommand messageType = iota
)

type Message struct {
	// The Unique ID of the message
	ID int
	// The actual data in the message
	// TODO: Change this to a slice instead...or maybe use an
	// interface type here to handle several data types ?
	Data string
	// The type of the message being sent
	MessageType messageType
}

func main() {
	// Create a connection to nats server, and publish a message.
	nc, err := nats.Connect("localhost", nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}
	defer nc.Close()

	// The SubscribeSync used in the subscriber, will get messages that
	// are sent after it started subscribing, so we start a publisher
	// that sends out a message every second.
	go func() {
		counter := 0

		for {
			m := Message{
				ID:   counter,
				Data: "ls",
			}

			var buf bytes.Buffer
			gobEnc := gob.NewEncoder(&buf)
			err := gobEnc.Encode(m)
			if err != nil {
				fmt.Printf("error: gob.Enode failed: %v\n", err)
			}

			//fmt.Printf("The gob encoded message right after encoding: %v\n", buf.Bytes())

			msg := &nats.Msg{
				Reply:   "subjectReply",
				Data:    buf.Bytes(),
				Subject: "subject1",
			}

			err = nc.PublishMsg(msg)
			if err != nil {
				log.Printf("error: publish failed: %v\n", err)
				continue
			}

			// Create a subscriber for the reply message.
			subReply, err := nc.SubscribeSync(msg.Reply)
			if err != nil {
				log.Printf("error: nc.SubscribeSync failed: %v\n", err)
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

			// Increment the counter for the next message to be sent.
			counter++
			time.Sleep(time.Second * 1)
		}
	}()

	select {}

}
