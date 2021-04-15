package steward

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// --- Message

type Message struct {
	// The node to send the message to
	ToNode node `json:"toNode" yaml:"toNode"`
	// The Unique ID of the message
	ID int `json:"id" yaml:"id"`
	// The actual data in the message
	Data []string `json:"data" yaml:"data"`
	// Method, what is this message doing, etc. CLI, syslog, etc.
	Method Method `json:"method" yaml:"method"`
	// ReplyMethod, is the method to use for the reply message.
	// By default the reply method will be set to log to file, but
	// you can override it setting your own here.
	ReplyMethod Method `json:"replyMethod" yaml:"replyMethod"`
	// From what node the message originated
	FromNode node
	// ACKTimeout for waiting for an ack message
	ACKTimeout int `json:"ACKTimeout" yaml:"ACKTimeout"`
	// Resend retries
	Retries int `json:"retries" yaml:"retries"`
	// The ACK timeout of the new message created via a request event.
	ReplyACKTimeout int `json:"replyACKTimeout" yaml:"replyACKTimeout"`
	// The retries of the new message created via a request event.
	ReplyRetries int `json:"replyRetries" yaml:"replyRetries"`
	// Timeout for long a process should be allowed to operate
	MethodTimeout int `json:"methodTimeout" yaml:"methodTimeout"`
	// Directory is a string that can be used to create the
	//directory structure when saving the result of some method.
	// For example "syslog","metrics", or "metrics/mysensor"
	// The type is typically used in the handler of a method.
	Directory string `json:"directory" yaml:"directory"`
	// FileExtension is used to be able to set a wanted extension
	// on a file being saved as the result of data being handled
	// by a method handler.
	FileExtension string `json:"fileExtension" yaml:"fileExtension"`
	// operation are used to give an opCmd and opArg's.
	Operation Operation `json:"operation"`
	// PreviousMessage are used for example if a reply message is
	// generated and we also need a copy of  thedetails of the the
	// initial request message.
	PreviousMessage *Message

	// done is used to signal when a message is fully processed.
	// This is used for signaling back to the ringbuffer that we are
	// done with processing a message, and the message can be removed
	// from the ringbuffer and into the time series log.
	done chan struct{}
}

// ---

type Operation struct {
	OpCmd string          `json:"opCmd"`
	OpArg json.RawMessage `json:"opArg"`
}

// ---

// gobEncodePayload will encode the message structure into gob
// binary format.
func gobEncodeMessage(m Message) ([]byte, error) {
	var buf bytes.Buffer
	gobEnc := gob.NewEncoder(&buf)
	err := gobEnc.Encode(m)
	if err != nil {
		return nil, fmt.Errorf("error: gob.Encode failed: %v", err)
	}

	return buf.Bytes(), nil
}

// --- Subject

type node string

// subject contains the representation of a subject to be used with one
// specific process
type Subject struct {
	// node, the name of the node
	ToNode string `json:"node" yaml:"toNode"`
	// messageType, command/event
	CommandOrEvent CommandOrEvent `json:"commandOrEvent" yaml:"commandOrEvent"`
	// method, what is this message doing, etc. CLICommand, Syslog, etc.
	Method Method `json:"method" yaml:"method"`
	// messageCh is used by publisher kind processes to read new messages
	// to be published. The content on this channel have been routed here
	// from routeMessagesToPublish in *server.
	// This channel is only used for publishing processes.
	messageCh chan Message
}

// newSubject will return a new variable of the type subject, and insert
// all the values given as arguments. It will also create the channel
// to receive new messages on the specific subject.
func newSubject(method Method, node string) Subject {
	// Get the CommandOrEvent type for the Method.
	ma := method.GetMethodsAvailable()
	coe, ok := ma.methodhandlers[method]
	if !ok {
		log.Printf("error: no CommandOrEvent type specified for the method: %v\n", method)
		os.Exit(1)
	}

	return Subject{
		ToNode:         node,
		CommandOrEvent: coe.getKind(),
		Method:         method,
		messageCh:      make(chan Message),
	}
}

// subjectName is the complete representation of a subject
type subjectName string

func (s Subject) name() subjectName {
	return subjectName(fmt.Sprintf("%s.%s.%s", s.ToNode, s.Method, s.CommandOrEvent))
}
