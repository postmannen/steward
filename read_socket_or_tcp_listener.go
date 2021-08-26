package steward

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

// readSocket will read the .sock file specified.
// It will take a channel of []byte as input, and it is in this
// channel the content of a file that has changed is returned.
func (s *server) readSocket() {
	// Loop, and wait for new connections.
	for {
		conn, err := s.StewardSocket.Accept()
		if err != nil {
			er := fmt.Errorf("error: failed to accept conn on socket: %v", err)
			sendErrorLogMessage(s.metrics, s.newMessagesCh, Node(s.nodeName), er)
		}

		go func(conn net.Conn) {
			defer conn.Close()

			var readBytes []byte

			for {
				b := make([]byte, 1500)
				_, err = conn.Read(b)
				if err != nil && err != io.EOF {
					er := fmt.Errorf("error: failed to read data from tcp listener: %v", err)
					sendErrorLogMessage(s.metrics, s.newMessagesCh, Node(s.nodeName), er)
					return
				}

				readBytes = append(readBytes, b...)

				if err == io.EOF {
					break
				}
			}

			readBytes = bytes.Trim(readBytes, "\x00")

			// unmarshal the JSON into a struct
			sams, err := s.convertBytesToSAMs(readBytes)
			if err != nil {
				er := fmt.Errorf("error: malformed json: %v", err)
				sendErrorLogMessage(s.metrics, s.newMessagesCh, Node(s.nodeName), er)
				return
			}

			for i := range sams {

				// Fill in the value for the FromNode field, so the receiver
				// can check this field to know where it came from.
				sams[i].Message.FromNode = Node(s.nodeName)
			}

			// Send the SAM struct to be picked up by the ring buffer.
			s.newMessagesCh <- sams

		}(conn)
	}
}

// readTCPListener wait and read messages delivered on the TCP
// port if started.
// It will take a channel of []byte as input, and it is in this
// channel the content of a file that has changed is returned.
func (s *server) readTCPListener(toRingbufferCh chan []subjectAndMessage) {
	ln, err := net.Listen("tcp", s.configuration.TCPListener)
	if err != nil {
		log.Printf("error: readTCPListener: failed to start tcp listener: %v\n", err)
		os.Exit(1)
	}
	// Loop, and wait for new connections.
	for {

		conn, err := ln.Accept()
		if err != nil {
			er := fmt.Errorf("error: failed to accept conn on socket: %v", err)
			sendErrorLogMessage(s.metrics, toRingbufferCh, Node(s.nodeName), er)
			continue
		}

		go func(conn net.Conn) {
			defer conn.Close()

			var readBytes []byte

			for {
				b := make([]byte, 1500)
				_, err = conn.Read(b)
				if err != nil && err != io.EOF {
					er := fmt.Errorf("error: failed to read data from tcp listener: %v", err)
					sendErrorLogMessage(s.metrics, toRingbufferCh, Node(s.nodeName), er)
					return
				}

				readBytes = append(readBytes, b...)

				if err == io.EOF {
					break
				}
			}

			readBytes = bytes.Trim(readBytes, "\x00")

			// unmarshal the JSON into a struct
			sam, err := s.convertBytesToSAMs(readBytes)
			if err != nil {
				er := fmt.Errorf("error: malformed json: %v", err)
				sendErrorLogMessage(s.metrics, toRingbufferCh, Node(s.nodeName), er)
				return
			}

			for i := range sam {

				// Fill in the value for the FromNode field, so the receiver
				// can check this field to know where it came from.
				sam[i].Message.FromNode = Node(s.nodeName)
			}

			// Send the SAM struct to be picked up by the ring buffer.
			toRingbufferCh <- sam

		}(conn)
	}
}

// TODO: Create the writer go routine for this socket.
func (s *server) writeStewSocket(toStewSocketCh []byte) {
	//s.StewSockListener
}

// The subject are made up of different parts of the message field.
// To make things easier and to avoid figuring out what the subject
// is in all places we've created the concept of subjectAndMessage
// (sam) where we get the subject for the message once, and use the
// sam structure with subject alongside the message instead.
type subjectAndMessage struct {
	Subject `json:"subject" yaml:"subject"`
	Message `json:"message" yaml:"message"`
}

// convertBytesToSAMs will range over the  byte representing a message given in
// json format. For each element found the Message type will be converted into
// a SubjectAndMessage type value and appended to a slice, and the slice is
// returned to the caller.
func (s *server) convertBytesToSAMs(b []byte) ([]subjectAndMessage, error) {
	MsgSlice := []Message{}

	err := json.Unmarshal(b, &MsgSlice)
	if err != nil {
		//fmt.Printf(" *** %v", string(b))
		return nil, fmt.Errorf("error: unmarshal of file failed: %#v", err)
	}

	// Check for toNode and toNodes field.
	MsgSlice = s.checkMessageToNodes(MsgSlice)
	s.metrics.promUserMessagesTotal.Add(float64(len(MsgSlice)))

	sam := []subjectAndMessage{}

	// Range over all the messages parsed from json, and create a subject for
	// each message.
	for _, m := range MsgSlice {
		sm, err := newSubjectAndMessage(m)
		if err != nil {
			log.Printf("error: jsonFromFileData: %v\n", err)
			continue
		}
		sam = append(sam, sm)
	}

	return sam, nil
}

// checkMessageToNodes will check that either toHost or toHosts are
// specified in the message. If not specified it will drop the message
// and send an error.
// if toNodes is specified, the original message will be used, and
// and an individual message will be created with a toNode field for
// each if the toNodes specified.
func (s *server) checkMessageToNodes(MsgSlice []Message) []Message {
	msgs := []Message{}

	for _, v := range MsgSlice {
		switch {
		// if toNode specified, we don't care about the toHosts.
		case v.ToNode != "":
			msgs = append(msgs, v)
			continue

		// if toNodes specified, we use the original message, and
		// create new node messages for each of the nodes specified.
		case len(v.ToNodes) != 0:
			fmt.Printf("\n * Found TonNodes: %#v\n\n", len(v.ToNodes))
			for _, n := range v.ToNodes {
				m := v
				// Set the toNodes field to nil since we're creating
				// an individual toNode message for each of the toNodes
				// found, and hence we no longer need that field.
				m.ToNodes = nil
				m.ToNode = n
				msgs = append(msgs, m)
			}
			continue

		// No toNode or toNodes specified. Drop the message by not appending it to
		// the slice since it is not valid.
		default:
			er := fmt.Errorf("error: no toNode or toNodes where specified in the message got'n, dropping message: %v", v)
			sendErrorLogMessage(s.metrics, s.newMessagesCh, Node(s.nodeName), er)
			continue
		}
	}

	return msgs
}

// newSubjectAndMessage will look up the correct values and value types to
// be used in a subject for a Message (sam), and return the a combined structure
// of type subjectAndMessage.
func newSubjectAndMessage(m Message) (subjectAndMessage, error) {
	// We need to create a tempory method type to look up the kind for the
	// real method for the message.
	var mt Method

	//fmt.Printf("-- \n getKind contains: %v\n\n", mt.getHandler(m.Method).getKind())

	tmpH := mt.getHandler(m.Method)
	if tmpH == nil {
		return subjectAndMessage{}, fmt.Errorf("error: method value did not exist in map")
	}

	sub := Subject{
		ToNode:         string(m.ToNode),
		CommandOrEvent: tmpH.getKind(),
		Method:         m.Method,
	}

	sam := subjectAndMessage{
		Subject: sub,
		Message: m,
	}

	return sam, nil
}
