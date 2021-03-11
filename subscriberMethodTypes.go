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

// ------------------------------------------------------------
// The constants that will be used throughout the system for
// when specifying what kind of Method to send or work with.
const (
	// Shell command to be executed via f.ex. bash
	CLICommand Method = "CLICommand"
	// Shell command to be executed via f.ex. bash
	CLICommandRequest Method = "CLICommandRequest"
	// Shell command to be executed via f.ex. bash
	// The NOSEQ method will process messages as they are recived,
	// and the reply back will be sent as soon as the process is
	// done. No order are preserved.
	CLICommandRequestNOSEQ Method = "CLICommandRequestNOSEQ"
	// Will generate a reply for a CLICommandRequest
	CLICommandReply Method = "CLICommandReply"
	// Send text logging to some host
	TextLogging Method = "TextLogging"
	// Send Hello I'm here message
	SayHello Method = "SayHello"
	// Error log methods to centralError
	ErrorLog Method = "ErrorLog"
	// Echo request will ask the subscriber for a
	// reply generated as a new message
	ECHORequest Method = "ECHORequest"
	// Will generate a reply for a ECHORequest
	ECHOReply Method = "ECHOReply"
)

// Method is used to specify the actual function/method that
// is represented in a typed manner.
type Method string

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
			CLICommand: methodSubscriberCLICommand{
				commandOrEvent: CommandACK,
			},
			CLICommandRequest: methodSubscriberCLICommandRequest{
				commandOrEvent: EventACK,
			},
			CLICommandRequestNOSEQ: methodSubscriberCLICommandRequestNOSEQ{
				commandOrEvent: EventACK,
			},
			CLICommandReply: methodSubscriberCLICommandReply{
				commandOrEvent: EventACK,
			},
			TextLogging: methodSubscriberTextLogging{
				commandOrEvent: EventACK,
			},
			SayHello: methodSubscriberSayHello{
				commandOrEvent: EventNACK,
			},
			ErrorLog: methodSubscriberErrorLog{
				commandOrEvent: EventACK,
			},
			ECHORequest: methodSubscriberEchoRequest{
				commandOrEvent: EventACK,
			},
			ECHOReply: methodSubscriberEchoReply{
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

type methodSubscriberCLICommand struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberCLICommand) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberCLICommand) handler(proc process, message Message, node string) ([]byte, error) {
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

	ackMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n%s---", node, message.ID, out))
	return ackMsg, nil
}

// -----

type methodSubscriberTextLogging struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberTextLogging) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberTextLogging) handler(proc process, message Message, node string) ([]byte, error) {
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

		//s.subscriberServices.logCh <- []byte(d)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// -----

type methodSubscriberSayHello struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberSayHello) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberSayHello) handler(proc process, message Message, node string) ([]byte, error) {

	log.Printf("<--- Received hello from %#v\n", message.FromNode)

	// send the message to the procFuncCh which is running alongside the process
	// and can hold registries and handle special things for an individual process.
	proc.procFuncCh <- message

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodSubscriberErrorLog struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberErrorLog) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberErrorLog) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- Received error from: %v, containing: %v", message.FromNode, message.Data)
	return nil, nil
}

// ---

type methodSubscriberEchoRequest struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberEchoRequest) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberEchoRequest) handler(proc process, message Message, node string) ([]byte, error) {
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

type methodSubscriberEchoReply struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberEchoReply) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberEchoReply) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- ECHO Reply received from: %v, containing: %v", message.FromNode, message.Data)

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- methodSubscriberCLICommandRequest

type methodSubscriberCLICommandRequest struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberCLICommandRequest) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberCLICommandRequest) handler(proc process, message Message, node string) ([]byte, error) {
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

// --- methodSubscriberCLICommandRequestNOSEQ

type methodSubscriberCLICommandRequestNOSEQ struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberCLICommandRequestNOSEQ) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// The NOSEQ method will process messages as they are recived,
// and the reply back will be sent as soon as the process is
// done. No order are preserved.
func (m methodSubscriberCLICommandRequestNOSEQ) handler(proc process, message Message, node string) ([]byte, error) {
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

type methodSubscriberCLICommandReply struct {
	commandOrEvent CommandOrEvent
}

func (m methodSubscriberCLICommandReply) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodSubscriberCLICommandReply) handler(proc process, message Message, node string) ([]byte, error) {
	fmt.Printf("### %v\n", message.Data)

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
