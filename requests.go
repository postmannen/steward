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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	// Initial parent method used to start other processes.
	REQInitial Method = "REQInitial"
	// Command for client operation request of the system. The op
	// command to execute shall be given in the data field of the
	// message as string value. For example "ps".
	REQOpCommand Method = "REQOpCommand"
	// Get a list of all the running processes.
	REQOpProcessList Method = "REQOpProcessList"
	// Start up a process.
	REQOpProcessStart Method = "REQOpProcessStart"
	// Stop up a process.
	REQOpProcessStop Method = "REQOpProcessStop"
	// Execute a CLI command in for example bash or cmd.
	// This is an event type, where a message will be sent to a
	// node with the command to execute and an ACK will be replied
	// if it was delivered succesfully. The output of the command
	// ran will be delivered back to the node where it was initiated
	// as a new message.
	// The data field is a slice of strings where the first string
	// value should be the command, and the following the arguments.
	REQCliCommand Method = "REQCliCommand"
	// REQCliCommandCont same as normal Cli command, but can be used
	// when running a command that will take longer time and you want
	// to send the output of the command continually back as it is
	// generated, and not wait until the command is finished.
	REQCliCommandCont Method = "REQCliCommandCont"
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
	// Write to steward socket
	REQToSocket Method = "REQToSocket"
	// Send a message via a node
	REQRelay Method = "REQRelay"
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
	// Event, Used to communicate that something have happened.
	ma := MethodsAvailable{
		Methodhandlers: map[Method]methodHandler{
			REQInitial: methodREQInitial{
				commandOrEvent: CommandACK,
			},
			REQOpCommand: methodREQOpCommand{
				commandOrEvent: CommandACK,
			},
			REQOpProcessList: methodREQOpProcessList{
				commandOrEvent: CommandACK,
			},
			REQOpProcessStart: methodREQOpProcessStart{
				commandOrEvent: CommandACK,
			},
			REQOpProcessStop: methodREQOpProcessStop{
				commandOrEvent: CommandACK,
			},
			REQCliCommand: methodREQCliCommand{
				commandOrEvent: CommandACK,
			},
			REQCliCommandCont: methodREQCliCommandCont{
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
			REQToSocket: methodREQToSocket{
				commandOrEvent: EventACK,
			},
			REQRelay: methodREQRelay{
				commandOrEvent: EventACK,
			},
		},
	}

	return ma
}

// Reply methods. The slice generated here is primarily used within
// the Stew client for knowing what of the req types are generally
// used as reply methods.
func (m Method) GetReplyMethods() []Method {
	rm := []Method{REQToConsole, REQToFile, REQToFileAppend, REQToSocket}
	return rm
}

// getHandler will check the methodsAvailable map, and return the
// method handler for the method given
// as input argument.
func (m Method) getHandler(method Method) methodHandler {
	ma := m.GetMethodsAvailable()
	mh := ma.Methodhandlers[method]

	return mh
}

// The structure that works as a reference for all the methods and if
// they are of the command or event type, and also if it is a ACK or
// NACK message.

// ----

// Initial parent method used to start other processes.
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

// MethodsAvailable holds a map of all the different method types and the
// associated handler to that method type.
type MethodsAvailable struct {
	Methodhandlers map[Method]methodHandler
}

// Check if exists will check if the Method is defined. If true the bool
// value will be set to true, and the methodHandler function for that type
// will be returned.
func (ma MethodsAvailable) CheckIfExists(m Method) (methodHandler, bool) {
	mFunc, ok := ma.Methodhandlers[m]
	if ok {
		return mFunc, true
	} else {
		return nil, false
	}
}

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

	// Create a new message for the reply, and put it on the
	// ringbuffer to be published.
	newMsg := Message{
		ToNode:        message.FromNode,
		FromNode:      message.ToNode,
		Data:          []string{string(outData)},
		Method:        message.ReplyMethod,
		MethodArgs:    message.ReplyMethodArgs,
		MethodTimeout: message.ReplyMethodTimeout,
		IsReply:       true,
		ACKTimeout:    message.ReplyACKTimeout,
		Retries:       message.ReplyRetries,
		Directory:     message.Directory,
		FileName:      message.FileName,

		// Put in a copy of the initial request message, so we can use it's properties if
		// needed to for example create the file structure naming on the subscriber.
		PreviousMessage: &message,
	}

	sam, err := newSubjectAndMessage(newMsg)
	if err != nil {
		// In theory the system should drop the message before it reaches here.
		er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
	}
	proc.toRingbufferCh <- []subjectAndMessage{sam}
}

// selectFileNaming will figure out the correct naming of the file
// structure to use for the reply data.
// It will return the filename, and the tree structure for the folders
// to create.
func selectFileNaming(message Message, proc process) (string, string) {
	var fileName string
	var folderTree string

	switch {
	case message.PreviousMessage == nil:
		// If this was a direct request there are no previous message to take
		// information from, so we use the one that are in the current mesage.
		fileName = message.FileName
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.Directory, string(message.ToNode))
	case message.PreviousMessage.ToNode != "":
		fileName = message.PreviousMessage.FileName
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.PreviousMessage.ToNode))
	case message.PreviousMessage.ToNode == "":
		fileName = message.PreviousMessage.FileName
		folderTree = filepath.Join(proc.configuration.SubscribersDataFolder, message.PreviousMessage.Directory, string(message.FromNode))
	}

	return fileName, folderTree
}

// ------------------------------------------------------------
// Subscriber method handlers
// ------------------------------------------------------------

// The methodHandler interface.
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
	AllowedNodes []Node `json:"allowedNodes"`
}

type OpCmdStopProc struct {
	RecevingNode Node        `json:"receivingNode"`
	Method       Method      `json:"method"`
	Kind         processKind `json:"kind"`
	ID           int         `json:"id"`
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// in the ack message.
func (m methodREQOpCommand) handler(proc process, message Message, nodeName string) ([]byte, error) {
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		out := []byte{}

		// unmarshal the json.RawMessage field called OpArgs.
		//
		// Dst interface is the generic type to Unmarshal OpArgs into, and we will
		// set the type it should contain depending on the value specified in Cmd.
		var dst interface{}

		switch message.Operation.OpCmd {
		case "ps":
			// Loop the the processes map, and find all that is active to
			// be returned in the reply message.

			activeProcs := proc.processes.active.getAll()

			for _, idMap := range activeProcs {
				for _, v := range idMap.v {
					s := fmt.Sprintf("%v, proc: %v, id: %v, name: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), v.processKind, v.processID, v.subject.name())
					sb := []byte(s)
					out = append(out, sb...)
				}

			}

		case "startProc":
			// Set the empty interface type dst to &OpStart.
			dst = &OpCmdStartProc{}

			err := json.Unmarshal(message.Operation.OpArg, &dst)
			if err != nil {
				er := fmt.Errorf("error: methodREQOpCommand startProc json.Umarshal failed : %v, OpArg: %v", err, message.Operation.OpArg)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
			}

			// Assert it into the correct non pointer value.
			arg := *dst.(*OpCmdStartProc)

			if len(arg.AllowedNodes) == 0 {
				er := fmt.Errorf("error: startProc: no allowed publisher nodes specified: %v" + fmt.Sprint(message))
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return
			}

			if arg.Method == "" {
				er := fmt.Errorf("error: startProc: no method specified: %v" + fmt.Sprint(message))
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return
			}

			// Create the process and start it.
			sub := newSubject(arg.Method, proc.configuration.NodeName)
			procNew := newProcess(proc.ctx, proc.processes.metrics, proc.natsConn, proc.processes, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
			go procNew.spawnWorker(proc.processes, proc.natsConn)

			er := fmt.Errorf("info: startProc: started id: %v, subject: %v: node: %v", procNew.processID, sub, message.ToNode)
			sendInfoLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)

		case "stopProc":
			// Set the interface type dst to &OpStart.
			dst = &OpCmdStopProc{}

			err := json.Unmarshal(message.Operation.OpArg, &dst)
			if err != nil {
				er := fmt.Errorf("error: methodREQOpCommand stopProc json.Umarshal failed : %v, message: %v", err, message)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
			}

			// Assert it into the correct non pointer value.
			arg := *dst.(*OpCmdStopProc)

			// Based on the arg values received in the message we create a
			// processName structure as used in naming the real processes.
			// We can then use this processName to get the real values for the
			// actual process we want to stop.
			sub := newSubject(arg.Method, string(arg.RecevingNode))
			processName := processNameGet(sub.name(), arg.Kind)

			// Check if the message contains an id.
			err = func() error {
				if arg.ID == 0 {
					er := fmt.Errorf("error: stopProc: did not find process to stop: %v on %v", sub, message.ToNode)
					sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
					return er
				}
				return nil
			}()

			if err != nil {
				er := fmt.Errorf("error: stopProc: err was not nil: %v : %v on %v", err, sub, message.ToNode)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return
			}

			// Remove the process from the processes active map if found.
			p1 := proc.processes.active.get(processName)
			toStopProc, ok := p1.v[arg.ID]

			if ok {
				// Delete the process from the processes map
				proc.processes.active.del(keyValue{k: processName})
				// Stop started go routines that belong to the process.
				toStopProc.ctxCancel()
				// Stop subscribing for messages on the process's subject.
				err := toStopProc.natsSubscription.Unsubscribe()
				if err != nil {
					er := fmt.Errorf("error: methodREQOpCommand, toStopProc, failed to stop nats.Subscription: %v, message: %v", err, message)
					sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
					log.Printf("%v\n", er)
				}

				// Remove the prometheus label
				proc.processes.metrics.promProcessesAllRunning.Delete(prometheus.Labels{"processName": string(processName)})

				er := fmt.Errorf("info: stopProc: stopped %v on %v", sub, message.ToNode)
				sendInfoLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)

				newReplyMessage(proc, message, []byte(er.Error()))

			} else {
				er := fmt.Errorf("error: stopProc: methodREQOpCommand, did not find process to stop: %v on %v", sub, message.ToNode)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)

				newReplyMessage(proc, message, []byte(er.Error()))
			}
		}

		newReplyMessage(proc, message, out)
	}()

	ackMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n", proc.node, message.ID))
	return ackMsg, nil
}

// ---- New operations

// --- OpProcessList
type methodREQOpProcessList struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQOpProcessList) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// Handle Op Process List
func (m methodREQOpProcessList) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		out := []byte{}

		// Loop the the processes map, and find all that is active to
		// be returned in the reply message.
		procsAll := proc.processes.active.getAll()
		for _, idMap := range procsAll {
			for _, v := range idMap.v {
				s := fmt.Sprintf("%v, process: %v, id: %v, name: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), v.processKind, v.processID, v.subject.name())
				sb := []byte(s)
				out = append(out, sb...)
			}

		}

		newReplyMessage(proc, message, out)
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- OpProcessStart

type methodREQOpProcessStart struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQOpProcessStart) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// Handle Op Process Start
func (m methodREQOpProcessStart) handler(proc process, message Message, node string) ([]byte, error) {
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		var out []byte

		// We need to create a tempory method type to look up the kind for the
		// real method for the message.
		var mt Method

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQOpProcessStart: got <1 number methodArgs")
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		}

		m := message.MethodArgs[0]
		method := Method(m)
		tmpH := mt.getHandler(Method(method))
		if tmpH == nil {
			er := fmt.Errorf("error: OpProcessStart: no such request type defined: %v" + m)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			return
		}

		// Create the process and start it.
		sub := newSubject(method, proc.configuration.NodeName)
		procNew := newProcess(proc.ctx, proc.processes.metrics, proc.natsConn, proc.processes, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go procNew.spawnWorker(proc.processes, proc.natsConn)

		txt := fmt.Sprintf("info: OpProcessStart: started id: %v, subject: %v: node: %v", procNew.processID, sub, message.ToNode)
		er := fmt.Errorf(txt)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)

		// TODO: What should this look like ?
		out = []byte(txt + "\n")
		newReplyMessage(proc, message, out)
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil

}

// --- OpProcessStop

type methodREQOpProcessStop struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQOpProcessStop) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// RecevingNode Node        `json:"receivingNode"`
// Method       Method      `json:"method"`
// Kind         processKind `json:"kind"`
// ID           int         `json:"id"`

// Handle Op Process Start
func (m methodREQOpProcessStop) handler(proc process, message Message, node string) ([]byte, error) {
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		var out []byte

		// We need to create a tempory method type to use to look up the kind for the
		// real method for the message.
		var mt Method

		if v := len(message.MethodArgs); v != 4 {
			er := fmt.Errorf("error: OpProcessStop: methodArgs should contain 4 elements, found %v", v)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			return
		}

		// --- Parse and check the method arguments given.
		// The Reason for also having the node as one of the arguments is
		// that publisher processes are named by the node they are sending the
		// message to. Subscriber processes names are named by the node name
		// they are running on.

		switch {
		case len(message.MethodArgs) != 4:
			er := fmt.Errorf("error: methodREQCliCommand: got <4 number methodArgs, want: method,node,kind,id")
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		}

		methodString := message.MethodArgs[0]
		node := message.MethodArgs[1]
		kind := message.MethodArgs[2]
		idString := message.MethodArgs[3]

		method := Method(methodString)
		tmpH := mt.getHandler(Method(method))
		if tmpH == nil {
			er := fmt.Errorf("error: OpProcessStop: no such request type defined: %v, check that the methodArgs are correct: " + methodString)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			return
		}

		// Check if id is a valid number.
		id, err := strconv.Atoi(idString)
		if err != nil {
			er := fmt.Errorf("error: OpProcessStop: id: %v, is not a number, check that the methodArgs are correct: %v", idString, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		}

		// --- Find, and stop process if found

		// Based on the arg values received in the message we create a
		// processName structure as used in naming the real processes.
		// We can then use this processName to get the real values for the
		// actual process we want to stop.
		sub := newSubject(method, string(node))
		processName := processNameGet(sub.name(), processKind(kind))

		// Remove the process from the processes active map if found.
		p1 := proc.processes.active.get(processName)
		toStopProc, ok := p1.v[id]

		if ok {
			// Delete the process from the processes map
			proc.processes.active.del(keyValue{k: processName})
			// Stop started go routines that belong to the process.
			toStopProc.ctxCancel()
			// Stop subscribing for messages on the process's subject.
			err := toStopProc.natsSubscription.Unsubscribe()
			if err != nil {
				er := fmt.Errorf("error: methodREQOpStopProcess failed to stop nats.Subscription: %v, methodArgs: %v", err, message.MethodArgs)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
			}

			// Remove the prometheus label
			proc.processes.metrics.promProcessesAllRunning.Delete(prometheus.Labels{"processName": string(processName)})

			txt := fmt.Sprintf("info: OpProcessStop: process stopped id: %v, method: %v on: %v", id, sub, message.ToNode)
			er := fmt.Errorf(txt)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)

			out = []byte(txt + "\n")
			newReplyMessage(proc, message, out)

		} else {
			txt := fmt.Sprintf("error: OpProcessStop: did not find process to stop: %v on %v", sub, message.ToNode)
			er := fmt.Errorf(txt)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)

			out = []byte(txt + "\n")
			newReplyMessage(proc, message, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil

}

// ----

type methodREQToFileAppend struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQToFileAppend) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// Handle appending data to file.
func (m methodREQToFileAppend) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFileAppend: failed to create toFileAppend directory tree:%v, subject: %v, %v", folderTree, proc.subject, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFileAppend.handler: failed to open file: %v, %v", file, err)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
		return nil, err
	}
	defer f.Close()

	for _, d := range message.Data {
		_, err := f.Write([]byte(d))
		f.Sync()
		if err != nil {
			er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file : %v, %v", file, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
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

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
func (m methodREQToFile) handler(proc process, message Message, node string) ([]byte, error) {

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQToFile failed to create toFile directory tree: subject:%v, folderTree: %v, %v", proc.subject, folderTree, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)

			return nil, er
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFile.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
		return nil, err
	}
	defer f.Close()

	for _, d := range message.Data {
		_, err := f.Write([]byte(d))
		f.Sync()
		if err != nil {
			er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: file: %v, %v", file, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
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

// Handler for receiving hello messages.
func (m methodREQHello) handler(proc process, message Message, node string) ([]byte, error) {
	data := fmt.Sprintf("%v, Received hello from %#v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.FromNode)

	fileName := message.FileName
	folderTree := filepath.Join(proc.configuration.SubscribersDataFolder, message.Directory, string(message.FromNode))

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)

	if err != nil {
		log.Printf("error: methodREQHello.handler: failed to open file: %v\n", err)
		return nil, err
	}
	defer f.Close()

	_, err = f.Write([]byte(data))
	f.Sync()
	if err != nil {
		log.Printf("error: methodEventTextLogging.handler: failed to write to file: %v\n", err)
	}

	// --------------------------

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

// Handle the writing of error logs.
func (m methodREQErrorLog) handler(proc process, message Message, node string) ([]byte, error) {
	proc.processes.metrics.promErrorMessagesReceivedTotal.Inc()

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		log.Printf("error: methodREQErrorLog.handler: failed to open file: %v\n", err)
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

// ---

type methodREQPing struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQPing) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// Handle receving a ping.
func (m methodREQPing) handler(proc process, message Message, node string) ([]byte, error) {
	// Write to file that we received a ping

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQPing.handler: failed to create toFile directory tree: %v, %v", folderTree, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)

			return nil, er
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQPing.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
		return nil, err
	}
	defer f.Close()

	// And write the data
	d := fmt.Sprintf("%v, ping received from %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.FromNode)
	_, err = f.Write([]byte(d))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodREQPing.handler: failed to write to file: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
	}

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		newReplyMessage(proc, message, nil)
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

// Handle receiving a pong.
func (m methodREQPong) handler(proc process, message Message, node string) ([]byte, error) {
	// Write to file that we received a pong

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			er := fmt.Errorf("error: methodREQPong.handler: failed to create toFile directory tree %v: %v", folderTree, err)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)

			return nil, er
		}

		log.Printf("info: Creating subscribers data folder at %v\n", folderTree)
	}

	// Open file.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQPong.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
		return nil, err
	}
	defer f.Close()

	// And write the data
	d := fmt.Sprintf("%v, pong received from %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.PreviousMessage.ToNode)
	_, err = f.Write([]byte(d))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodREQPong.handler: failed to write to file: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		log.Printf("%v\n", er)
	}

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
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		var a []string

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQCliCommand: got <1 number methodArgs")
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		case len(message.MethodArgs) >= 0:
			a = message.MethodArgs[1:]
		}

		c := message.MethodArgs[0]

		ctx, cancel := context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			// Check if {{data}} is defined in the method arguments. If found put the
			// data payload there.
			var foundEnvData bool
			var envData string
			for i, v := range message.MethodArgs {
				if strings.Contains(v, "{{STEWARD_DATA}}") {
					foundEnvData = true
					// Replace the found env variable placeholder with an actual env variable
					message.MethodArgs[i] = strings.Replace(message.MethodArgs[i], "{{STEWARD_DATA}}", "$STEWARD_DATA", -1)

					// Put all the data which is a slice of string into a single
					// string so we can put it in a single env variable.
					envData = strings.Join(message.Data, "")
				}
			}

			cmd := exec.CommandContext(ctx, c, a...)

			// Check for the use of env variable for STEWARD_DATA, and set env if found.
			if foundEnvData {
				envData = fmt.Sprintf("STEWARD_DATA=%v", envData)
				cmd.Env = append(cmd.Env, envData)
			}

			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err != nil {
				if err != nil {
					log.Printf("error: failed cmd.Run: %v\n", err)
				}

				er := fmt.Errorf("error: methodREQCliCommand: cmd.Run : %v, methodArgs: %v, error_output: %v", err, message.MethodArgs, stderr.String())
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
			}
			select {
			case outCh <- out.Bytes():
			case <-ctx.Done():
				return
			}
		}()

		select {
		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQCliCommand: method timed out: %v", message.MethodArgs)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		case out := <-outCh:
			cancel()

			// NB: Not quite sure what is the best way to handle the below
			// isReply right now. Implementing as send to central for now.
			//
			// If this is this a reply message swap the toNode and fromNode
			// fields so the output of the command are sent to central node.
			if message.IsReply {
				message.ToNode, message.FromNode = message.FromNode, message.ToNode
			}

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

// Handler to write directly to console.
func (m methodREQToConsole) handler(proc process, message Message, node string) ([]byte, error) {

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

// handler to do a Http Get.
func (m methodREQHttpGet) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- REQHttpGet received from: %v, containing: %v", message.FromNode, message.Data)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQHttpGet: got <1 number methodArgs")
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		}

		url := message.MethodArgs[0]

		client := http.Client{
			Timeout: time.Second * 5,
		}

		ctx, cancel := context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			er := fmt.Errorf("error: methodREQHttpGet: NewRequest failed: %v, bailing out: %v", err, message.MethodArgs)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			cancel()
			return
		}

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			resp, err := client.Do(req)
			if err != nil {
				er := fmt.Errorf("error: methodREQHttpGet: client.Do failed: %v, bailing out: %v", err, message.MethodArgs)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				cancel()
				er := fmt.Errorf("error: methodREQHttpGet: not 200, where %#v, bailing out: %v", resp.StatusCode, message)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				er := fmt.Errorf("error: methodREQHttpGet: io.ReadAll failed : %v, methodArgs: %v", err, message.MethodArgs)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
			}

			out := body

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQHttpGet: method timed out: %v", message.MethodArgs)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
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
	log.Printf("<--- TailFile REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQTailFile: got <1 number methodArgs")
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		}

		fp := message.MethodArgs[0]

		var ctx context.Context
		var cancel context.CancelFunc

		if message.MethodTimeout != 0 {
			ctx, cancel = context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))
		} else {
			ctx, cancel = context.WithCancel(proc.ctx)
		}

		outCh := make(chan []byte)
		t, err := tail.TailFile(fp, tail.Config{Follow: true, Location: &tail.SeekInfo{
			Offset: 0,
			Whence: os.SEEK_END,
		}})
		if err != nil {
			er := fmt.Errorf("error: methodREQToTailFile: tailFile: %v", err)
			log.Printf("%v\n", er)
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
		}

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			for {
				select {
				case line := <-t.Lines:
					outCh <- []byte(line.Text + "\n")
				case <-ctx.Done():
					return
				}

			}
		}()

		for {
			select {
			case <-ctx.Done():
				cancel()
				// Close the lines channel so we exit the reading lines
				// go routine.
				// close(t.Lines)
				er := fmt.Errorf("info: method timeout reached REQTailFile, canceling: %v", message.MethodArgs)
				sendInfoLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)

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

// ---
type methodREQCliCommandCont struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQCliCommandCont) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// Handler to run REQCliCommandCont, which is the same as normal
// Cli command, but can be used when running a command that will take
// longer time and you want to send the output of the command continually
// back as it is generated, and not just when the command is finished.
func (m methodREQCliCommandCont) handler(proc process, message Message, node string) ([]byte, error) {
	log.Printf("<--- CLInCommandCont REQUEST received from: %v, containing: %v", message.FromNode, message.Data)

	// Execute the CLI command in it's own go routine, so we are able
	// to return immediately with an ack reply that the message was
	// received, and we create a new message to send back to the calling
	// node for the out put of the actual command.
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		var a []string

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQCliCommand: got <1 number methodArgs")
			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
			log.Printf("%v\n", er)
			return
		case len(message.MethodArgs) >= 0:
			a = message.MethodArgs[1:]
		}

		c := message.MethodArgs[0]

		ctx, cancel := context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))

		outCh := make(chan []byte)
		errCh := make(chan string)

		proc.processes.wg.Add(1)
		go func() {
			proc.processes.wg.Done()

			cmd := exec.CommandContext(ctx, c, a...)

			// Using cmd.StdoutPipe here so we are continuosly
			// able to read the out put of the command.
			outReader, err := cmd.StdoutPipe()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.StdoutPipe failed : %v, methodArgs: %v", err, message.MethodArgs)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("error: %v\n", er)
			}

			ErrorReader, err := cmd.StderrPipe()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.StderrPipe failed : %v, methodArgs: %v", err, message.MethodArgs)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
			}

			if err := cmd.Start(); err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.Start failed : %v, methodArgs: %v", err, message.MethodArgs)
				sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)

			}

			go func() {
				scanner := bufio.NewScanner(ErrorReader)
				for scanner.Scan() {
					errCh <- scanner.Text()
				}
			}()

			go func() {
				scanner := bufio.NewScanner(outReader)
				for scanner.Scan() {
					outCh <- []byte(scanner.Text() + "\n")
				}
			}()

			// NB: sending cancel to command context, so processes are killed.
			// A github issue is filed on not killing all child processes when using pipes:
			// https://github.com/golang/go/issues/23019
			// TODO: Check in later if there are any progress on the issue.
			// When testing the problem seems to appear when using sudo, or tcpdump without
			// the -l option. So for now, don't use sudo, and remember to use -l with tcpdump
			// which makes stdout line buffered.

			<-ctx.Done()
			cancel()

			if err := cmd.Wait(); err != nil {
				log.Printf(" --------------- * error: REQCliCommandCont: cmd.Wait: %v\n", err)
			}

		}()

		// Check if context timer or command output were received.
		for {
			select {
			case <-ctx.Done():
				cancel()
				er := fmt.Errorf("info: methodREQCliCommandCont: method timeout reached, canceling: methodArgs: %v", message.MethodArgs)
				sendInfoLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
				return
			case out := <-outCh:
				newReplyMessage(proc, message, out)
			case out := <-errCh:
				newReplyMessage(proc, message, []byte(out))
			}
		}
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQToSocket struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQToSocket) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// TODO:
// Handler to write to unix socket file.
func (m methodREQToSocket) handler(proc process, message Message, node string) ([]byte, error) {

	for _, d := range message.Data {
		// TODO: Write the data to the socket here.
		fmt.Printf("Info: Data to write to socket: %v\n", d)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQRelay struct {
	commandOrEvent CommandOrEvent
}

func (m methodREQRelay) getKind() CommandOrEvent {
	return m.commandOrEvent
}

// Handler to relay messages via a host.
func (m methodREQRelay) handler(proc process, message Message, node string) ([]byte, error) {
	// relay the message here to the actual host here.

	// Send back an ACK message.
	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----
