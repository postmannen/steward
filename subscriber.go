package steward

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os/exec"

	"github.com/nats-io/nats.go"
)

// RunSubscriber will start a subscribing process.
// TODO: Right now the only thing a subscriber can do is ro receive commands,
// check if there are more things a subscriber should be able to do.
func (s *server) RunSubscriber() {
	{
		fmt.Printf("nodeName: %#v\n", s.nodeName)
		sub := newSubject(s.nodeName, "command", "shellcommand")
		proc := s.processPrepareNew(sub, s.errorCh, processKindSubscriber)
		// fmt.Printf("*** %#v\n", proc)
		go s.processSpawnWorker(proc)
	}

	// subject := fmt.Sprintf("%s.%s.%s", s.nodeName, "command", "shellcommand")
	//
	// // Subscribe will start up a Go routine under the hood calling the
	// // callback function specified when a new message is received.
	// _, err := s.natsConn.Subscribe(subject, func(msg *nats.Msg) {
	// 	// We start one handler per message received by using go routines here.
	// 	// This is for being able to reply back the current publisher who sent
	// 	// the message.
	// 	go handler(s.natsConn, s.nodeName, msg)
	// })
	// if err != nil {
	// 	log.Printf("error: Subscribe failed: %v\n", err)
	// }

	select {}
}

// handler will deserialize the message when a new message is received,
// check the MessageType field in the message to decide what kind of
// message it and then it will check how to handle that message type,
// and handle it.
// This handler function should be started in it's own go routine,so
// one individual handler is started per message received so we can keep
// the state of the message being processed, and then reply back to the
// correct sending process's reply, meaning so we ACK back to the correct
// publisher.
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

	//fmt.Printf("%v\n", msg)
	// TODO: Maybe the handling of the errors within the subscriber
	// should also involve the error-kernel to report back centrally
	// that there was a problem like missing method to handle a specific
	// method etc.
	switch {
	case message.MessageType == "Command":
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
	case message.MessageType == "Event":
		fmt.Printf("info: the event type is not implemented yet\n")

		// Send a confirmation message back to the publisher
		natsConn.Publish(msg.Reply, []byte("confirmed from: "+node+": "+fmt.Sprint(message.ID)))
	default:
		log.Printf("info: did not find that specific type of command: %#v\n", message.MessageType)
	}
}
