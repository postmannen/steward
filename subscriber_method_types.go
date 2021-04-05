// The structure of how to add new method types to the system.
// -----------------------------------------------------------
// All methods need 3 things:
//  - A type definition
//  - The type needs a getKind method
//  - The type needs a handler method
// Overall structure example shown below.
//
// ---
// type methodCommandCLICommandRequest struct {
// 	commandOrEvent CommandOrEvent
// }
//
// func (m methodCommandCLICommandRequest) getKind() CommandOrEvent {
// 	return m.commandOrEvent
// }
//
// func (m methodCommandCLICommandRequest) handler(s *server, message Message, node string) ([]byte, error) {
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
	"context"
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
	// Command for client operation request of the system
	REQOpCommand Method = "REQOpCommand"
	// Execute a CLI command in for example bash or cmd.
	// This is an event type, where a message will be sent to a
	// node with the command to execute and an ACK will be replied
	// if it was delivered succesfully. The output of the command
	// ran will be delivered back to the node where it was initiated
	// as a new message.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	REQCliCommand Method = "REQCliCommand"
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
	REQnCliCommand Method = "REQnCliCommand"
	// Send text to be logged to the console.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	REQTextToConsole Method = "REQTextToConsole"
	// Send text logging to some host by appending the output to a
	// file, if the file do not exist we create it.
	// A file with the full subject+hostName will be created on
	// the receiving end.
	// The data field is a slice of strings where the values of the
	// slice will be written to the log file.
	REQTextToLogFile Method = "REQTextToLogFile"
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

	// Command, Used to make a request to perform an action
	// Event, Used to communicate that an action has been performed.
	ma := MethodsAvailable{
		methodhandlers: map[Method]methodHandler{
			REQOpCommand: methodREQOpCommand{
				commandOrEvent: CommandACK,
			},
			REQCliCommand: methodREQCliCommand{
				commandOrEvent: CommandACK,
			},
			REQnCliCommand: methodREQnCliCommand{
				commandOrEvent: CommandACK,
			},
			REQTextToConsole: methodREQTextToConsole{
				commandOrEvent: EventACK,
			},
			REQTextToLogFile: methodREQTextToLogFile{
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

type methodREQOpCommand struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQOpCommand) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// in the ack message.
func (m methodREQOpCommand) handler(proc process, message Message, node string) ([]byte, error) {
	go func() {
		out := []byte{}

		switch {

		case message.Data[0] == "ps":
			proc.processes.mu.Lock()
			// Loop the the processes map, and find all that is active to
			// be returned in the reply message.
			for _, v := range proc.processes.active {
				s := fmt.Sprintf("%v, proc: %v, id: %v, name: %v, allowed from: %s\n", time.Now().UTC(), v.processKind, v.processID, v.subject.name(), v.allowedReceivers)
				sb := []byte(s)
				out = append(out, sb...)
			}
			proc.processes.mu.Unlock()

		default:
			er := fmt.Errorf("error: no such OpCommand specified: " + message.Data[0])
			sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
			return
		}

		// Prepare and queue for sending a new message with the output
		// of the action executed.
		newReplyMessage(proc, message, REQTextToLogFile, out)
	}()

	ackMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n", node, message.ID))
	return ackMsg, nil
}

//--
// Create a new message for the reply containing the output of the
// action executed put in outData, and put it on the ringbuffer to
// be published.
func newReplyMessage(proc process, message Message, method Method, outData []byte) {
	//--
	// Create a new message for the reply, and put it on the
	// ringbuffer to be published.
	newMsg := Message{
		ToNode:  message.FromNode,
		Data:    []string{string(outData)},
		Method:  method,
		Timeout: message.RequestTimeout,
		Retries: message.RequestRetries,

		// Put in a copy of the initial request message, so we can use it's properties if
		// needed to for example create the file structure naming on the subscriber.
		PreviousMessage: &message,
	}

	nSAM, err := newSAM(newMsg)
	if err != nil {
		// In theory the system should drop the message before it reaches here.
		log.Printf("error: %v: %v\n", message.Method, err)
	}
	proc.toRingbufferCh <- []subjectAndMessage{nSAM}
	//--
}

type methodREQTextToLogFile struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQTextToLogFile) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQTextToLogFile) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fmt.Printf(" ** DEBUG: %v\n", message.PreviousMessage)
	var fileName string
	var folderTree string
	switch {
	case message.PreviousMessage == nil:
		// If this was a direct request there are no previous message to take
		// information from, so we use the one that are in the current mesage.
		fileName = fmt.Sprintf("%v.%v.log", message.ToNode, message.Method)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.Label, string(message.FromNode))
	case message.PreviousMessage.ToNode != "":
		fileName = fmt.Sprintf("%v.%v.log", message.PreviousMessage.ToNode, message.PreviousMessage.Method)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Label, string(message.PreviousMessage.ToNode))
	case message.PreviousMessage.ToNode == "":
		fileName = fmt.Sprintf("%v.%v.log", message.FromNode, message.Method)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Label, string(message.PreviousMessage.ToNode))
	}

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create directory %v: %v", folderTree, err)
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE, os.ModeAppend)
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

	// Prepare and queue for sending a new message with the output
	// of the action executed.
	newReplyMessage(proc, message, ECHOReply, []byte{})

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

type methodREQCliCommand struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQCliCommand) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
func (m methodREQCliCommand) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- CLICommandREQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	// Execute the CLI command in it's own go routine, so we are able
	// to return immediately with an ack reply that the messag was
	// received, and we create a new message to send back to the calling
	// node for the out put of the actual command.
	go func() {
		c := message.Data[0]
		a := message.Data[1:]

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(message.MethodTimeout))

		outCh := make(chan []byte)

		go func() {
			cmd := exec.CommandContext(ctx, c, a...)
			out, err := cmd.Output()
			if err != nil {
				log.Printf("error: %v\n", err)
			}
			outCh <- out
		}()

		select {
		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: method timed out %v", message)
			sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
		case out := <-outCh:
			cancel()

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, REQTextToLogFile, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- methodCLICommandRequestNOSEQ

type methodREQnCliCommand struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQnCliCommand) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
// The NOSEQ method will process messages as they are recived,
// and the reply back will be sent as soon as the process is
// done. No order are preserved.
func (m methodREQnCliCommand) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- CLICommand REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	// Execute the CLI command in it's own go routine, so we are able
	// to return immediately with an ack reply that the messag was
	// received, and we create a new message to send back to the calling
	// node for the out put of the actual command.
	go func() {
		c := message.Data[0]
		a := message.Data[1:]

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(message.MethodTimeout))

		outCh := make(chan []byte)

		go func() {
			cmd := exec.CommandContext(ctx, c, a...)
			out, err := cmd.Output()
			if err != nil {
				log.Printf("error: %v\n", err)
			}
			outCh <- out
		}()

		select {
		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: method timed out %v", message)
			sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
		case out := <-outCh:
			cancel()

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, REQTextToConsole, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQTextToConsole struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQTextToConsole) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQTextToConsole) handler(proc process, message Message, node string) ([]byte, error) {
	fmt.Printf("<--- methodCLICommandReply: %v\n", message.Data)

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
