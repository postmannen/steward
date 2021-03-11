// The structure of how to add new method types to the system.
// -----------------------------------------------------------
// All methods need 3 things:
//  - A type definition
//  - The type needs a getKind method
//  - The type needs a handler method
// Overall structure example shown below.
//
// ---
// type methodCommandCLICommand struct {
// 	commandOrEvent CommandOrEvent
// }
//
// func (m methodCommandCLICommand) getKind() CommandOrEvent {
// 	return m.commandOrEvent
// }
//
// func (m methodCommandCLICommand) handler(s *server, message Message, node string) ([]byte, error) {
//  ...
//  ...
// 	ackMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n%s---", node, message.ID, out))
// 	return ackMsg, nil
// }
//
// ---
// You also need to make a constant for the Method, and add
// that constant as the key in the map, where the value is
// the actual type you want to map it to with a handler method.
// You also specify if it is a Command or Event, and if it is
// ACK or NACK.
// Check out the existing code below for more examples.

package steward

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Method is used to specify the actual function/method that
// is represented in a typed manner.
type Method string

// ------------------------------------------------------------
// The constants that will be used throughout the system for
// when specifying what kind of Method to send or work with.
const (
	// Execute a CLI command in for example bash or cmd.
	// This is a command type, so the output of the command executed
	// will directly showed in the ACK message received.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	CLICommand Method = "CLICommand"
	// Execute a CLI command in for example bash or cmd.
	// This is an event type, where a message will be sent to a
	// node with the command to execute and an ACK will be replied
	// if it was delivered succesfully. The output of the command
	// ran will be delivered back to the node where it was initiated
	// as a new message.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	CLICommandRequest Method = "CLICommandRequest"
	// Execute a CLI command in for example bash or cmd.
	// This is an event type, where a message will be sent to a
	// node with the command to execute and an ACK will be replied
	// if it was delivered succesfully. The output of the command
	// ran will be delivered back to the node where it was initiated
	// as a new message.
	// The NOSEQ method will process messages as they are recived,
	// and the reply back will be sent as soon as the process is
	// done. No order are preserved.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	CLICommandRequestNOSEQ Method = "CLICommandRequestNOSEQ"
	// Will generate a reply for a CLICommandRequest.
	// This type is normally not used by the user when creating
	// a message. It is used in creating the reply message with
	// request messages. It is also used when defining a process to
	// start up for receiving the CLICommand request messages.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	CLICommandReply Method = "CLICommandReply"
	// Send text logging to some host.
	// A file with the full subject+hostName will be created on
	// the receiving end.
	// The data field is a slice of strings where the values of the
	// slice will be written to the log file.
	TextLogging Method = "TextLogging"
	// Send Hello I'm here message.
	SayHello Method = "SayHello"
	// Error log methods to centralError node.
	ErrorLog Method = "ErrorLog"
	// Echo request will ask the subscriber for a
	// reply generated as a new message, and sent back to where
	// the initial request was made.
	ECHORequest Method = "ECHORequest"
	// Will generate a reply for a ECHORequest
	ECHOReply Method = "ECHOReply"
)

// The mapping of all the method constants specified, what type
// it references, and the kind if it is an Event or Command, and
// if it is ACK or NACK.
//  Allowed values for the commandOrEvent field are:
//   - CommandACK
//   - CommandNACK
//   - EventACK
//   - EventNack
func (m Method) GetMethodsAvailable() MethodsAvailable {
	ma := MethodsAvailable{
		methodhandlers: map[Method]methodHandler{
			CLICommand: methodCLICommand{
				commandOrEvent: CommandACK,
			},
			CLICommandRequest: methodCLICommandRequest{
				commandOrEvent: EventACK,
			},
			CLICommandRequestNOSEQ: methodCLICommandRequestNOSEQ{
				commandOrEvent: EventACK,
			},
			CLICommandReply: methodCLICommandReply{
				commandOrEvent: EventACK,
			},
			TextLogging: methodTextLogging{
				commandOrEvent: EventACK,
			},
			SayHello: methodSayHello{
				commandOrEvent: EventNACK,
			},
			ErrorLog: methodErrorLog{
				commandOrEvent: EventACK,
			},
			ECHORequest: methodEchoRequest{
				commandOrEvent: EventACK,
			},
			ECHOReply: methodEchoReply{
				commandOrEvent: EventACK,
			},
		},
	}

	return ma
}

// getHandler will check the methodsAvailable map, and return the
// method handler for the method given
// as input argument.
func (m Method) getHandler(method Method) methodHandler {
	ma := m.GetMethodsAvailable()
	mh := ma.methodhandlers[method]

	return mh
}

// The structure that works as a reference for all the methods and if
// they are of the command or event type, and also if it is a ACK or
// NACK message.
type MethodsAvailable struct {
	methodhandlers map[Method]methodHandler
}

// Check if exists will check if the Method is defined. If true the bool
// value will be set to true, and the methodHandler function for that type
// will be returned.
func (ma MethodsAvailable) CheckIfExists(m Method) (methodHandler, bool) {
	mFunc, ok := ma.methodhandlers[m]
	if ok {
		// fmt.Printf("******THE TOPIC EXISTS: %v******\n", m)
		return mFunc, true
	} else {
		// fmt.Printf("******THE TOPIC DO NOT EXIST: %v******\n", m)
		return nil, false
	}
}

// ------------------------------------------------------------
// Subscriber method handlers
// ------------------------------------------------------------

type methodHandler interface {
	handler(proc process, message Message, node string) ([]byte, error)
	getKind() CommandOrEvent
}

// -----

type methodCLICommand struct {
	commandOrEvent CommandOrEvent
}

func (m methodCLICommand) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodCLICommand) handler(proc process, message Message, node string) ([]byte, error) {
	c := message.Data[0]
	a := message.Data[1:]
	cmd := exec.Command(c, a...)
	//cmd.Stdout = os.Stdout
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error: execution of command failed: %v\n", err)
	}

	ackMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n%s---", node, message.ID, out))
	return ackMsg, nil
}

// -----

type methodTextLogging struct {
	commandOrEvent CommandOrEvent
}

func (m methodTextLogging) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodTextLogging) handler(proc process, message Message, node string) ([]byte, error) {
	sub := Subject{
		ToNode:         string(message.ToNode),
		CommandOrEvent: proc.subject.CommandOrEvent,
		Method:         message.Method,
	}

	logFile := filepath.Join(proc.configuration.SubscribersDataFolder, string(sub.name())+"-"+string(message.FromNode))
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModeAppend)
	if err != nil {
		log.Printf("error: methodEventTextLogging.handler: failed to open file: %v\n", err)
		return nil, err
	}
	defer f.Close()

	for _, d := range message.Data {
		_, err := f.Write([]byte(d))
		f.Sync()
		if err != nil {
			log.Printf("error: methodEventTextLogging.handler: failed to write to file: %v\n", err)
		}
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// -----

type methodSayHello struct {
	commandOrEvent CommandOrEvent
}

func (m methodSayHello) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSayHello) handler(proc process, message Message, node string) ([]byte, error) {

	log.Printf("<--- Received hello from %#v\n", message.FromNode)

	// send the message to the procFuncCh which is running alongside the process
	// and can hold registries and handle special things for an individual process.
	proc.procFuncCh <- message

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodErrorLog struct {
	commandOrEvent CommandOrEvent
}

func (m methodErrorLog) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodErrorLog) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- Received error from: %v, containing: %v", message.FromNode, message.Data)
	return nil, nil
}

// ---

type methodEchoRequest struct {
	commandOrEvent CommandOrEvent
}

func (m methodEchoRequest) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodEchoRequest) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- ECHO REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	// Create a new message for the reply, and put it on the
	// ringbuffer to be published.
	newMsg := Message{
		ToNode:  message.FromNode,
		Data:    []string{""},
		Method:  ECHOReply,
		Timeout: 3,
		Retries: 3,
	}
	proc.newMessagesCh <- []subjectAndMessage{newSAM(newMsg)}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodEchoReply struct {
	commandOrEvent CommandOrEvent
}

func (m methodEchoReply) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodEchoReply) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- ECHO Reply received from: %v, containing: %v", message.FromNode, message.Data)

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodCLICommandRequest struct {
	commandOrEvent CommandOrEvent
}

func (m methodCLICommandRequest) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodCLICommandRequest) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- CLICommand REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	c := message.Data[0]
	a := message.Data[1:]
	cmd := exec.Command(c, a...)
	//cmd.Stdout = os.Stdout
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error: execution of command failed: %v\n", err)
	}

	time.Sleep(time.Second * time.Duration(message.MethodTimeout))

	// Create a new message for the reply, and put it on the
	// ringbuffer to be published.
	newMsg := Message{
		ToNode:  message.FromNode,
		Data:    []string{string(out)},
		Method:  CLICommandReply,
		Timeout: 3,
		Retries: 3,
	}
	fmt.Printf("** %#v\n", newMsg)
	proc.newMessagesCh <- []subjectAndMessage{newSAM(newMsg)}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- methodCLICommandRequestNOSEQ

type methodCLICommandRequestNOSEQ struct {
	commandOrEvent CommandOrEvent
}

func (m methodCLICommandRequestNOSEQ) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// The NOSEQ method will process messages as they are recived,
// and the reply back will be sent as soon as the process is
// done. No order are preserved.
func (m methodCLICommandRequestNOSEQ) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- CLICommand REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	go func() {

		c := message.Data[0]
		a := message.Data[1:]
		cmd := exec.Command(c, a...)
		//cmd.Stdout = os.Stdout
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("error: execution of command failed: %v\n", err)
		}

		time.Sleep(time.Second * time.Duration(message.MethodTimeout))

		// Create a new message for the reply, and put it on the
		// ringbuffer to be published.
		newMsg := Message{
			ToNode:  message.FromNode,
			Data:    []string{string(out)},
			Method:  CLICommandReply,
			Timeout: 3,
			Retries: 3,
		}
		fmt.Printf("** %#v\n", newMsg)
		proc.newMessagesCh <- []subjectAndMessage{newSAM(newMsg)}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodCLICommandReply struct {
	commandOrEvent CommandOrEvent
}

func (m methodCLICommandReply) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodCLICommandReply) handler(proc process, message Message, node string) ([]byte, error) {
	fmt.Printf("### %v\n", message.Data)

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
