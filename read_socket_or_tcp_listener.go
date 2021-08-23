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
func (s *server) readSocket(toRingbufferCh chan []subjectAndMessage) {

	// Loop, and wait for new connections.
	for {
		conn, err := s.StewardSocket.Accept()
		if err != nil {
			er := fmt.Errorf("error: failed to accept conn on socket: %v", err)
			sendErrorLogMessage(toRingbufferCh, Node(s.nodeName), er)
		}

		b := make([]byte, 65535)
		_, err = conn.Read(b)
		if err != nil {
			er := fmt.Errorf("error: failed to read data from socket: %v", err)
			sendErrorLogMessage(toRingbufferCh, Node(s.nodeName), er)
			continue
		}

		b = bytes.Trim(b, "\x00")

		// unmarshal the JSON into a struct
		sam, err := convertBytesToSAM(b)
		if err != nil {
			er := fmt.Errorf("error: malformed json: %v", err)
			sendErrorLogMessage(toRingbufferCh, Node(s.nodeName), er)
			continue
		}

		for i := range sam {

			// Fill in the value for the FromNode field, so the receiver
			// can check this field to know where it came from.
			sam[i].Message.FromNode = Node(s.nodeName)
		}

		// Send the SAM struct to be picked up by the ring buffer.
		toRingbufferCh <- sam

		conn.Close()
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
			sendErrorLogMessage(toRingbufferCh, Node(s.nodeName), er)
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
					sendErrorLogMessage(toRingbufferCh, Node(s.nodeName), er)
					return
				}

				readBytes = append(readBytes, b...)

				if err == io.EOF {
					break
				}
			}

			readBytes = bytes.Trim(readBytes, "\x00")

			// unmarshal the JSON into a struct
			sam, err := convertBytesToSAM(readBytes)
			if err != nil {
				er := fmt.Errorf("error: malformed json: %v", err)
				sendErrorLogMessage(toRingbufferCh, Node(s.nodeName), er)
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

// convertBytesToSAM will range over the  byte representing a message given in
// json format. For each element found the Message type will be converted into
// a SubjectAndMessage type value and appended to a slice, and the slice is
// returned to the caller.
func convertBytesToSAM(b []byte) ([]subjectAndMessage, error) {
	MsgSlice := []Message{}

	err := json.Unmarshal(b, &MsgSlice)
	if err != nil {
		return nil, fmt.Errorf("error: unmarshal of file failed: %#v", err)
	}

	sam := []subjectAndMessage{}

	// Range over all the messages parsed from json, and create a subject for
	// each message.
	for _, m := range MsgSlice {
		sm, err := newSAM(m)
		if err != nil {
			log.Printf("error: jsonFromFileData: %v\n", err)
			continue
		}
		sam = append(sam, sm)
	}

	return sam, nil
}

// newSAM will look up the correct values and value types to
// be used in a subject for a Message (sam), and return the a combined structure
// of type subjectAndMessage.
func newSAM(m Message) (subjectAndMessage, error) {
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

	sm := subjectAndMessage{
		Subject: sub,
		Message: m,
	}

	return sm, nil
}
