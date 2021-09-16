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
	// The node to send the message to.
	ToNode Node `json:"toNode" yaml:"toNode"`
	// ToNodes to specify several hosts to send message to in the
	// form of an slice/array.
	ToNodes []Node `json:"toNodes,omitempty" yaml:"toNodes,omitempty"`
	// The Unique ID of the message
	ID int `json:"id" yaml:"id"`
	// The actual data in the message. This is typically where we
	// specify the cli commands to execute on a node, and this is
	// also the field where we put the returned data in a reply
	// message.
	Data []string `json:"data" yaml:"data"`
	// Method, what request type to use, like REQCliCommand, REQHttpGet..
	Method Method `json:"method" yaml:"method"`
	// Additional arguments that might be needed when executing the
	// method. Can be f.ex. an ip address if it is a tcp sender, or the
	// shell command to execute in a cli session.
	// TODO:
	MethodArgs []string `json:"methodArgs" yaml:"methodArgs"`
	// ReplyMethod, is the method to use for the reply message.
	// By default the reply method will be set to log to file, but
	// you can override it setting your own here.
	ReplyMethod Method `json:"replyMethod" yaml:"replyMethod"`
	// Additional arguments that might be needed when executing the reply
	// method. Can be f.ex. an ip address if it is a tcp sender, or the
	// shell command to execute in a cli session.
	// TODO:
	ReplyMethodArgs []string `json:"replyMethodArgs" yaml:"replyMethodArgs"`
	// From what node the message originated
	FromNode Node
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
	// FileName is used to be able to set a wanted name
	// on a file being saved as the result of data being handled
	// by a method handler.
	FileName string `json:"fileName" yaml:"fileName"`
	// operation are used to give an opCmd and opArg's.
	Operation Operation `json:"operation"`
	// PreviousMessage are used for example if a reply message is
	// generated and we also need a copy of  the details of the the
	// initial request message.
	PreviousMessage *Message

	// done is used to signal when a message is fully processed.
	// This is used for signaling back to the ringbuffer that we are
	// done with processing a message, and the message can be removed
	// from the ringbuffer and into the time series log.
	done chan struct{}
}

// ---

// operation are used to specify opCmd and opArg's.
type Operation struct {
	OpCmd string          `json:"opCmd"`
	OpArg json.RawMessage `json:"opArg"`
}

// ---

// gobEncodePayload will encode the message structure into gob
// binary format before putting it into a nats message.
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

// Node is the type definition for the node who receive or send a message.
type Node string

// subject contains the representation of a subject to be used with one
// specific process
type Subject struct {
	// node, the name of the node to receive the message.
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
	coe, ok := ma.Methodhandlers[method]
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

// Return a value of the subjectName for the subject as used with nats subject.
func (s Subject) name() subjectName {
	return subjectName(fmt.Sprintf("%s.%s.%s", s.ToNode, s.Method, s.CommandOrEvent))
}
