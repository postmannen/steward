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
	// shellCommand, command that will just wait for an
	// ack, and nothing of the output of the command are
	// delivered back in the reply ack message.
	// The message should contain the unique ID of the
	// command.
	shellCommandReturnOutput messageType = iota
	// shellCommand, wait for and return the output
	// of the command in the ACK message. This means
	// that the command should be executed immediately
	// and that we should get the confirmation that it
	// was successful or not.
	shellCommandReturnAck messageType = iota
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
	Data []string
	// The type of the message being sent
	MessageType messageType
}

func main() {
	edgeID := "btship1"
	// Create a connection to nats server, and publish a message.
	natsConn, err := nats.Connect("localhost", nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}
	defer natsConn.Close()

	// There should be on IDCounter per Subject later on.
	IDCounter := 0

	for {
		m := Message{
			ID:          IDCounter,
			Data:        []string{"uname", "-a"},
			MessageType: shellCommandReturnAck,
		}

		// TODO: Have a channel here to return
		messageDeliver(edgeID, m, natsConn)

		// Increment the counter for the next message to be sent.
		IDCounter++
		time.Sleep(time.Second * 1)
	}

}

func messageDeliver(edgeID string, message Message, natsConn *nats.Conn) {
	for {
		dataPayload, err := createDataPayload(message)
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

func createDataPayload(m Message) ([]byte, error) {
	var buf bytes.Buffer
	gobEnc := gob.NewEncoder(&buf)
	err := gobEnc.Encode(m)
	if err != nil {
		return nil, fmt.Errorf("error: gob.Enode failed: %v", err)
	}

	return buf.Bytes(), nil
}
