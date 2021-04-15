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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hpcloud/tail"
	"github.com/prometheus/client_golang/prometheus"
)

// Method is used to specify the actual function/method that
// is represented in a typed manner.
type Method string

// ------------------------------------------------------------
// The constants that will be used throughout the system for
// when specifying what kind of Method to send or work with.
const (
	// Initial method used to start other processes.
	REQInitial Method = "REQInitial"
	// Command for client operation request of the system. The op
	// command to execute shall be given in the data field of the
	// message as string value. For example "ps".
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
	REQToConsole Method = "REQToConsole"
	// Send text logging to some host by appending the output to a
	// file, if the file do not exist we create it.
	// A file with the full subject+hostName will be created on
	// the receiving end.
	// The data field is a slice of strings where the values of the
	// slice will be written to the log file.
	REQToFileAppend Method = "REQToFileAppend"
	// Send text to some host by overwriting the existing content of
	// the fileoutput to a file. If the file do not exist we create it.
	// A file with the full subject+hostName will be created on
	// the receiving end.
	// The data field is a slice of strings where the values of the
	// slice will be written to the file.
	REQToFile Method = "REQToFile"
	// Send Hello I'm here message.
	REQHello Method = "REQHello"
	// Error log methods to centralError node.
	REQErrorLog Method = "REQErrorLog"
	// Echo request will ask the subscriber for a
	// reply generated as a new message, and sent back to where
	// the initial request was made.
	REQPing Method = "REQPing"
	// Will generate a reply for a ECHORequest
	REQPong Method = "REQPong"
	// Http Get
	REQHttpGet Method = "REQHttpGet"
	// Tail file
	REQTailFile Method = "REQTailFile"
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
			REQInitial: methodREQInitial{
				commandOrEvent: CommandACK,
			},
			REQOpCommand: methodREQOpCommand{
				commandOrEvent: CommandACK,
			},
			REQCliCommand: methodREQCliCommand{
				commandOrEvent: CommandACK,
			},
			REQnCliCommand: methodREQnCliCommand{
				commandOrEvent: CommandACK,
			},
			REQToConsole: methodREQToConsole{
				commandOrEvent: EventACK,
			},
			REQToFileAppend: methodREQToFileAppend{
				commandOrEvent: EventACK,
			},
			REQToFile: methodREQToFile{
				commandOrEvent: EventACK,
			},
			REQHello: methodREQHello{
				commandOrEvent: EventNACK,
			},
			REQErrorLog: methodREQErrorLog{
				commandOrEvent: EventACK,
			},
			REQPing: methodREQPing{
				commandOrEvent: EventACK,
			},
			REQPong: methodREQPong{
				commandOrEvent: EventACK,
			},
			REQHttpGet: methodREQHttpGet{
				commandOrEvent: EventACK,
			},
			REQTailFile: methodREQTailFile{
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

// ----

type methodREQInitial struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQInitial) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQInitial) handler(proc process, message Message, node string) ([]byte, error) {
	// proc.procFuncCh <- message
	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----
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

type OpCmdStartProc struct {
	Method       Method `json:"method"`
	AllowedNodes []node `json:"allowedNodes"`
}

type OpCmdStopProc struct {
	RecevingNode node        `json:"receivingNode"`
	Method       Method      `json:"method"`
	Kind         processKind `json:"kind"`
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// in the ack message.
func (m methodREQOpCommand) handler(proc process, message Message, nodeName string) ([]byte, error) {
	go func() {
		out := []byte{}

		// unmarshal the json.RawMessage field called OpArgs.
		//
		// Dst interface is the generic type to Unmarshal OpArgs into, and we will
		// set the type it should contain depending on the value specified in Cmd.
		var dst interface{}

		switch message.Operation.OpCmd {
		case "ps":
			proc.processes.mu.Lock()
			// Loop the the processes map, and find all that is active to
			// be returned in the reply message.
			for _, v := range proc.processes.active {
				s := fmt.Sprintf("%v, proc: %v, id: %v, name: %v, allowed from: %s\n", time.Now().UTC(), v.processKind, v.processID, v.subject.name(), v.allowedReceivers)
				sb := []byte(s)
				out = append(out, sb...)
			}
			proc.processes.mu.Unlock()

		case "startProc":
			// Set the interface type dst to &OpStart.
			dst = &OpCmdStartProc{}

			err := json.Unmarshal(message.Operation.OpArg, &dst)
			if err != nil {
				log.Printf("error: outer unmarshal: %v\n", err)
			}

			// Assert it into the correct non pointer value.
			arg := *dst.(*OpCmdStartProc)

			//fmt.Printf(" ** Content of Arg = %#v\n", arg)

			if len(arg.AllowedNodes) == 0 {
				er := fmt.Errorf("error: startProc: no allowed publisher nodes specified: %v" + fmt.Sprint(message))
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				return
			}

			if arg.Method == "" {
				er := fmt.Errorf("error: startProc: no method specified: %v" + fmt.Sprint(message))
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				return
			}

			// Create the process and start it.
			sub := newSubject(arg.Method, proc.configuration.NodeName)
			procNew := newProcess(proc.natsConn, proc.processes, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, arg.AllowedNodes, nil)
			go procNew.spawnWorker(proc.processes, proc.natsConn)

			er := fmt.Errorf("info: startProc: started %v on %v", sub, message.ToNode)
			sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)

		case "stopProc":
			// Set the interface type dst to &OpStart.
			dst = &OpCmdStopProc{}

			err := json.Unmarshal(message.Operation.OpArg, &dst)
			if err != nil {
				log.Printf("error: outer unmarshal: %v\n", err)
			}

			// Assert it into the correct non pointer value.
			arg := *dst.(*OpCmdStopProc)

			// Data layout: OPCommand, Method, publisher/subscriber, receivingNode
			//
			// The processes can be either publishers or subscribers. The subject name
			// are used within naming a process. Since the subject structure contains
			// the node name of the node that will receive this message we also need
			// specify it so we are able to delete the publisher processes, since a
			// publisher process will have the name of the node to receive the message,
			// and not just the local node name as with subscriber processes.
			// receive the message we need to specify
			// Process name example: ship2.REQToFileAppend.EventACK_subscriber

			sub := newSubject(arg.Method, string(arg.RecevingNode))
			processName := processNameGet(sub.name(), arg.Kind)
			// fmt.Printf(" ** DEBUG1: processName: %v\n", processName)

			proc.processes.mu.Lock()

			// for k, v := range proc.processes.active {
			// 	fmt.Printf(" ** DEBUG1.3: MAP: k = %v, v = %v\n", k, v.processKind)
			// }

			toStopProc, ok := proc.processes.active[processName]
			if ok {
				// Delete the process from the processes map
				delete(proc.processes.active, processName)
				// Stop started go routines that belong to the process.
				toStopProc.ctxCancel()
				// Stop subscribing for messages on the process's subject.
				err := toStopProc.natsSubscription.Unsubscribe()
				if err != nil {
					log.Printf(" ** Error: failed to stop *nats.Subscription: %v\n", err)
				}

				// Remove the prometheus label
				proc.processes.promProcessesVec.Delete(prometheus.Labels{"processName": string(processName)})

				er := fmt.Errorf("info: stopProc: stoped %v on %v", sub, message.ToNode)
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)

				newReplyMessage(proc, message, []byte(er.Error()))

			} else {
				er := fmt.Errorf("error: stopProc: did not find process to stop: %v on %v", sub, message.ToNode)
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)

				newReplyMessage(proc, message, []byte(er.Error()))
			}

			proc.processes.mu.Unlock()

		}

		newReplyMessage(proc, message, out)
	}()

	ackMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n", proc.node, message.ID))
	return ackMsg, nil
}

//--
// Create a new message for the reply containing the output of the
// action executed put in outData, and put it on the ringbuffer to
// be published.
// The method to use for the reply message should initially be
// specified within the first message as the replyMethod, and we will
// pick up that value here, and use it as the method for the new
// request message. If no replyMethod is set we default to the
// REQToFileAppend method type.
func newReplyMessage(proc process, message Message, outData []byte) {

	// If no replyMethod is set we default to writing to writing to
	// a log file.
	if message.ReplyMethod == "" {
		message.ReplyMethod = REQToFileAppend
	}
	//--
	// Create a new message for the reply, and put it on the
	// ringbuffer to be published.
	newMsg := Message{
		ToNode:     message.FromNode,
		Data:       []string{string(outData)},
		Method:     message.ReplyMethod,
		ACKTimeout: message.ReplyTimeout,
		Retries:    message.ReplyRetries,

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

type methodREQToFileAppend struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQToFileAppend) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQToFileAppend) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	var fileName string
	var folderTree string
	switch {
	case message.PreviousMessage == nil:
		// If this was a direct request there are no previous message to take
		// information from, so we use the one that are in the current mesage.
		fileName = fmt.Sprintf("%v.%v%v", message.ToNode, message.Method, message.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.Directory, string(message.FromNode))
	case message.PreviousMessage.ToNode != "":
		fileName = fmt.Sprintf("%v.%v%v", message.PreviousMessage.ToNode, message.PreviousMessage.Method, message.PreviousMessage.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
	case message.PreviousMessage.ToNode == "":
		fileName = fmt.Sprintf("%v.%v%v", message.FromNode, message.Method, message.PreviousMessage.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
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
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, os.ModeAppend)
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

type methodREQToFile struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQToFile) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQToFile) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	var fileName string
	var folderTree string
	switch {
	case message.PreviousMessage == nil:
		// If this was a direct request there are no previous message to take
		// information from, so we use the one that are in the current mesage.
		fileName = fmt.Sprintf("%v.%v%v", message.ToNode, message.Method, message.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.Directory, string(message.FromNode))
	case message.PreviousMessage.ToNode != "":
		fileName = fmt.Sprintf("%v.%v%v", message.PreviousMessage.ToNode, message.PreviousMessage.Method, message.PreviousMessage.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
	case message.PreviousMessage.ToNode == "":
		fileName = fmt.Sprintf("%v.%v%v", message.FromNode, message.Method, message.PreviousMessage.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
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
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
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

// ----
type methodREQHello struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQHello) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQHello) handler(proc process, message Message, node string) ([]byte, error) {

	log.Printf("<--- Received hello from %#v\n", message.FromNode)

	// send the message to the procFuncCh which is running alongside the process
	// and can hold registries and handle special things for an individual process.
	proc.procFuncCh <- message

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQErrorLog struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQErrorLog) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQErrorLog) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- Received error from: %v, containing: %v", message.FromNode, message.Data)

	// --

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	var fileName string
	var folderTree string
	switch {
	case message.PreviousMessage == nil:
		// If this was a direct request there are no previous message to take
		// information from, so we use the one that are in the current mesage.
		fileName = fmt.Sprintf("%v.%v%v", message.ToNode, message.Method, message.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.Directory, string(message.FromNode))
	case message.PreviousMessage.ToNode != "":
		fileName = fmt.Sprintf("%v.%v%v", message.PreviousMessage.ToNode, message.PreviousMessage.Method, message.PreviousMessage.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
	case message.PreviousMessage.ToNode == "":
		fileName = fmt.Sprintf("%v.%v%v", message.FromNode, message.Method, message.PreviousMessage.FileExtension)
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
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
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, os.ModeAppend)
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

	// --
}

// ---

type methodREQPing struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQPing) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQPing) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- PING REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	go func() {
		// Prepare and queue for sending a new message with the output
		// of the action executed.
		d := fmt.Sprintf("%v, ping reply sent from %v\n", time.Now().UTC(), message.ToNode)
		newReplyMessage(proc, message, []byte(d))
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQPong struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQPong) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQPong) handler(proc process, message Message, node string) ([]byte, error) {
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
			newReplyMessage(proc, message, out)
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
			newReplyMessage(proc, message, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQToConsole struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQToConsole) getKind() CommandOrEvent {
	return m.commandOrEvent
}

func (m methodREQToConsole) handler(proc process, message Message, node string) ([]byte, error) {
	fmt.Printf("<--- methodCLICommandReply: %v\n", message.Data)

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQHttpGet struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQHttpGet) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// handler to run a Http Get.
func (m methodREQHttpGet) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- REQHttpGet received from: %v, containing: %v", message.FromNode, message.Data)

	go func() {
		url := message.Data[0]

		client := http.Client{
			Timeout: time.Second * 5,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(message.MethodTimeout))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			er := fmt.Errorf("error: NewRequest failed: %v, bailing out: %v", err, message)
			sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
			cancel()
			return
		}

		outCh := make(chan []byte)

		go func() {
			resp, err := client.Do(req)
			if err != nil {
				er := fmt.Errorf("error: client.Do failed: %v, bailing out: %v", err, message)
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				cancel()
				er := fmt.Errorf("error: not 200, where %#v, bailing out: %v", resp.StatusCode, message)
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				return
			}

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("error: ReadAll failed: %v\n", err)
			}

			outCh <- b
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
			newReplyMessage(proc, message, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- methodREQTailFile

type methodREQTailFile struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQTailFile) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// handler to run a tailing of files with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
func (m methodREQTailFile) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- CLICommand REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	go func() {
		fp := message.Data[0]

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(message.MethodTimeout))

		outCh := make(chan []byte)
		t, err := tail.TailFile(fp, tail.Config{Follow: true})
		if err != nil {
			er := fmt.Errorf("error: tailFile: %v", err)
			log.Printf("%v\n", er)
			sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
		}

		go func() {
			for line := range t.Lines {
				outCh <- []byte(line.Text + "\n")
			}
		}()

		for {
			select {
			case <-ctx.Done():
				cancel()
				// Close the lines channel so we exit the reading lines
				// go routine.
				close(t.Lines)
				er := fmt.Errorf("error: method timed out %v", message)
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				return
			case out := <-outCh:
				// Prepare and queue for sending a new message with the output
				// of the action executed.
				newReplyMessage(proc, message, out)
			}
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
