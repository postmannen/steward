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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
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
	// REQTuiToConsole
	REQTuiToConsole Method = "REQTuiToConsole"
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
	// REQToFileNACK same as REQToFile but NACK.
	REQToFileNACK Method = "REQToFileNACK"
	// Read the source file to be copied to some node.
	REQCopyFileFrom Method = "REQCopyFileFrom"
	// Write the destination copied to some node.
	REQCopyFileTo Method = "REQCopyFileTo"
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
	// Http Get Scheduled
	// The second element of the MethodArgs slice holds the timer defined in seconds.
	REQHttpGetScheduled Method = "REQHttpGetScheduled"
	// Tail file
	REQTailFile Method = "REQTailFile"
	// Write to steward socket
	REQRelay Method = "REQRelay"
	// The method handler for the first step in a relay chain.
	REQRelayInitial Method = "REQRelayInitial"
	// REQNone is used when there should be no reply.
	REQNone Method = "REQNone"
	// REQPublicKey will get the public ed25519 key from a node.
	REQPublicKey Method = "REQPublicKey"
	// REQPublicKeysGet will get all the public keys from central.
	REQPublicKeysGet Method = "REQPublicKeysGet"
	// REQPublicKeysToNode will put all the public received from central.
	REQPublicKeysToNode Method = "REQPublicKeysToNode"
	// REQAuthPublicKeysAllow
	REQPublicKeysAllow Method = "REQPublicKeysAllow"
)

// The mapping of all the method constants specified, what type
// it references, and the kind if it is an Event or Command, and
// if it is ACK or NACK.
//  Allowed values for the Event field are:
//   - EventACK
//   - EventNack
func (m Method) GetMethodsAvailable() MethodsAvailable {

	ma := MethodsAvailable{
		Methodhandlers: map[Method]methodHandler{
			REQInitial: methodREQInitial{
				event: EventACK,
			},
			REQOpProcessList: methodREQOpProcessList{
				event: EventACK,
			},
			REQOpProcessStart: methodREQOpProcessStart{
				event: EventACK,
			},
			REQOpProcessStop: methodREQOpProcessStop{
				event: EventACK,
			},
			REQCliCommand: methodREQCliCommand{
				event: EventACK,
			},
			REQCliCommandCont: methodREQCliCommandCont{
				event: EventACK,
			},
			REQToConsole: methodREQToConsole{
				event: EventACK,
			},
			REQTuiToConsole: methodREQTuiToConsole{
				event: EventACK,
			},
			REQToFileAppend: methodREQToFileAppend{
				event: EventACK,
			},
			REQToFile: methodREQToFile{
				event: EventACK,
			},
			REQToFileNACK: methodREQToFile{
				event: EventNACK,
			},
			REQCopyFileFrom: methodREQCopyFileFrom{
				event: EventACK,
			},
			REQCopyFileTo: methodREQCopyFileTo{
				event: EventACK,
			},
			REQHello: methodREQHello{
				event: EventNACK,
			},
			REQErrorLog: methodREQErrorLog{
				event: EventACK,
			},
			REQPing: methodREQPing{
				event: EventACK,
			},
			REQPong: methodREQPong{
				event: EventACK,
			},
			REQHttpGet: methodREQHttpGet{
				event: EventACK,
			},
			REQHttpGetScheduled: methodREQHttpGetScheduled{
				event: EventACK,
			},
			REQTailFile: methodREQTailFile{
				event: EventACK,
			},
			REQRelay: methodREQRelay{
				event: EventACK,
			},
			REQRelayInitial: methodREQRelayInitial{
				event: EventACK,
			},
			REQPublicKey: methodREQPublicKey{
				event: EventACK,
			},
			REQPublicKeysGet: methodREQPublicKeysGet{
				event: EventNACK,
			},
			REQPublicKeysToNode: methodREQPublicKeysToNode{
				event: EventNACK,
			},
			REQPublicKeysAllow: methodREQPublicKeysAllow{
				event: EventACK,
			},
		},
	}

	return ma
}

// Reply methods. The slice generated here is primarily used within
// the Stew client for knowing what of the req types are generally
// used as reply methods.
func (m Method) GetReplyMethods() []Method {
	rm := []Method{REQToConsole, REQTuiToConsole, REQCliCommand, REQCliCommandCont, REQToFile, REQToFileAppend, REQNone}
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

// getContextForMethodTimeout, will return a context with cancel function
// with the timeout set to the method timeout in the message.
// If the value of timeout is set to -1, we don't want it to stop, so we
// return a context with a timeout set to 200 years.
func getContextForMethodTimeout(ctx context.Context, message Message) (context.Context, context.CancelFunc) {
	// If methodTimeout == -1, which means we don't want a timeout, set the
	// time out to 200 years.
	if message.MethodTimeout == -1 {
		return context.WithTimeout(ctx, time.Hour*time.Duration(8760*200))
	}

	return context.WithTimeout(ctx, time.Second*time.Duration(message.MethodTimeout))
}

// ----

// Initial parent method used to start other processes.
type methodREQInitial struct {
	event Event
}

func (m methodREQInitial) getKind() Event {
	return m.event
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

// newReplyMessage will create and send a reply message back to where
// the original provided message came from. The primary use of this
// function is to report back to a node who sent a message with the
// result of the request method of the original message.
//
// The method to use for the reply message when reporting back should
// be specified within a message in the  replyMethod field. We will
// pick up that value here, and use it as the method for the new
// request message. If no replyMethod is set we default to the
// REQToFileAppend method type.
//
// There will also be a copy of the original message put in the
// previousMessage field. For the copy of the original message the data
// field will be set to nil before the whole message is put in the
// previousMessage field so we don't copy around the original data in
// the reply response when it is not needed anymore.
func newReplyMessage(proc process, message Message, outData []byte) {
	// If REQNone is specified, we don't want to send a reply message
	// so we silently just return without sending anything.
	if message.ReplyMethod == "REQNone" {
		return
	}

	// If no replyMethod is set we default to writing to writing to
	// a log file.
	if message.ReplyMethod == "" {
		message.ReplyMethod = REQToFileAppend
	}

	// Make a copy of the message as it is right now to use
	// in the previous message field, but set the data field
	// to nil so we don't copy around the original data when
	// we don't need to for the reply message.
	thisMsg := message
	thisMsg.Data = nil

	// Create a new message for the reply, and put it on the
	// ringbuffer to be published.
	// TODO: Check that we still got all the fields present that are needed here.
	newMsg := Message{
		ToNode:        message.FromNode,
		FromNode:      message.ToNode,
		Data:          outData,
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
		PreviousMessage: &thisMsg,
	}

	sam, err := newSubjectAndMessage(newMsg)
	if err != nil {
		// In theory the system should drop the message before it reaches here.
		er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
		proc.errorKernel.errSend(proc, message, er)
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
	getKind() Event
}

// -----

// --- OpProcessList
type methodREQOpProcessList struct {
	event Event
}

func (m methodREQOpProcessList) getKind() Event {
	return m.event
}

// Handle Op Process List
func (m methodREQOpProcessList) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		out := []byte{}

		// Loop the the processes map, and find all that is active to
		// be returned in the reply message.

		proc.processes.active.mu.Lock()
		for _, pTmp := range proc.processes.active.procNames {
			s := fmt.Sprintf("%v, process: %v, id: %v, name: %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), pTmp.processKind, pTmp.processID, pTmp.subject.name())
			sb := []byte(s)
			out = append(out, sb...)

		}
		proc.processes.active.mu.Unlock()

		newReplyMessage(proc, message, out)
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// --- OpProcessStart

type methodREQOpProcessStart struct {
	event Event
}

func (m methodREQOpProcessStart) getKind() Event {
	return m.event
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
			proc.errorKernel.errSend(proc, message, er)
			return
		}

		m := message.MethodArgs[0]
		method := Method(m)
		tmpH := mt.getHandler(Method(method))
		if tmpH == nil {
			er := fmt.Errorf("error: OpProcessStart: no such request type defined: %v" + m)
			proc.errorKernel.errSend(proc, message, er)
			return
		}

		// Create the process and start it.
		sub := newSubject(method, proc.configuration.NodeName)
		procNew := newProcess(proc.ctx, proc.server, sub, processKindSubscriber, nil)
		go procNew.spawnWorker()

		txt := fmt.Sprintf("info: OpProcessStart: started id: %v, subject: %v: node: %v", procNew.processID, sub, message.ToNode)
		er := fmt.Errorf(txt)
		proc.errorKernel.errSend(proc, message, er)

		out = []byte(txt + "\n")
		newReplyMessage(proc, message, out)
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil

}

// --- OpProcessStop

type methodREQOpProcessStop struct {
	event Event
}

func (m methodREQOpProcessStop) getKind() Event {
	return m.event
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

		// --- Parse and check the method arguments given.
		// The Reason for also having the node as one of the arguments is
		// that publisher processes are named by the node they are sending the
		// message to. Subscriber processes names are named by the node name
		// they are running on.

		if v := len(message.MethodArgs); v != 3 {
			er := fmt.Errorf("error: methodREQOpProcessStop: got <4 number methodArgs, want: method,node,kind")
			proc.errorKernel.errSend(proc, message, er)
		}

		methodString := message.MethodArgs[0]
		node := message.MethodArgs[1]
		kind := message.MethodArgs[2]

		method := Method(methodString)
		tmpH := mt.getHandler(Method(method))
		if tmpH == nil {
			er := fmt.Errorf("error: OpProcessStop: no such request type defined: %v, check that the methodArgs are correct: " + methodString)
			proc.errorKernel.errSend(proc, message, er)
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
		proc.processes.active.mu.Lock()
		toStopProc, ok := proc.processes.active.procNames[processName]

		if ok {
			// Delete the process from the processes map
			delete(proc.processes.active.procNames, processName)
			// Stop started go routines that belong to the process.
			toStopProc.ctxCancel()
			// Stop subscribing for messages on the process's subject.
			err := toStopProc.natsSubscription.Unsubscribe()
			if err != nil {
				er := fmt.Errorf("error: methodREQOpStopProcess failed to stop nats.Subscription: %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er)
			}

			// Remove the prometheus label
			proc.metrics.promProcessesAllRunning.Delete(prometheus.Labels{"processName": string(processName)})

			txt := fmt.Sprintf("info: OpProcessStop: process stopped id: %v, method: %v on: %v", toStopProc.processID, sub, message.ToNode)
			er := fmt.Errorf(txt)
			proc.errorKernel.errSend(proc, message, er)

			out = []byte(txt + "\n")
			newReplyMessage(proc, message, out)

		} else {
			txt := fmt.Sprintf("error: OpProcessStop: did not find process to stop: %v on %v", sub, message.ToNode)
			er := fmt.Errorf(txt)
			proc.errorKernel.errSend(proc, message, er)

			out = []byte(txt + "\n")
			newReplyMessage(proc, message, out)
		}

		proc.processes.active.mu.Unlock()

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil

}

// ----

type methodREQToFileAppend struct {
	event Event
}

func (m methodREQToFileAppend) getKind() Event {
	return m.event
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
			proc.errorKernel.errSend(proc, message, er)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFileAppend.handler: failed to open file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er)
		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file : %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// -----

type methodREQToFile struct {
	event Event
}

func (m methodREQToFile) getKind() Event {
	return m.event
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
			proc.errorKernel.errSend(proc, message, er)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQToFile.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)

		return nil, err
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: file: %v, %v", file, err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQCopyFileFrom struct {
	event Event
}

func (m methodREQCopyFileFrom) getKind() Event {
	return m.event
}

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
func (m methodREQCopyFileFrom) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQCopyFileFrom: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		SrcFilePath := message.MethodArgs[0]
		DstNode := message.MethodArgs[1]
		DstFilePath := message.MethodArgs[2]

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		defer cancel()

		outCh := make(chan []byte)
		errCh := make(chan error)

		// Read the file, and put the result on the out channel to be sent when done reading.
		proc.processes.wg.Add(1)
		go copyFileFrom(ctx, &proc.processes.wg, SrcFilePath, errCh, outCh)

		// Wait here until we got the data to send, then create a new message
		// and send it.
		// Also checking the ctx.Done which calls Cancel will allow us to
		// kill all started go routines started by this message.
		select {
		case <-ctx.Done():
			er := fmt.Errorf("error: methodREQCopyFile: got <-ctx.Done(): %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)

			return
		case er := <-errCh:
			proc.errorKernel.errSend(proc, message, er)

			return
		case out := <-outCh:
			dstDir := filepath.Dir(DstFilePath)
			dstFile := filepath.Base(DstFilePath)

			// Prepare for sending a new message with the output

			// Copy the original message to get the defaults for timeouts etc,
			// and set new values for fields to change.
			msg := message
			msg.ToNode = Node(DstNode)
			//msg.Method = REQToFile
			msg.Method = REQCopyFileTo
			msg.Data = out
			msg.Directory = dstDir
			msg.FileName = dstFile

			// Create SAM and put the message on the send new message channel.

			sam, err := newSubjectAndMessage(msg)
			if err != nil {
				er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
				proc.errorKernel.errSend(proc, message, er)
			}

			proc.toRingbufferCh <- []subjectAndMessage{sam}

			replyData := fmt.Sprintf("info: succesfully read the file %v, and sent the content to %v\n", SrcFilePath, DstNode)

			newReplyMessage(proc, message, []byte(replyData))
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// copyFileFrom will read a file to be copied from the specified SrcFilePath.
// The result of be delivered on the provided outCh.
func copyFileFrom(ctx context.Context, wg *sync.WaitGroup, SrcFilePath string, errCh chan error, outCh chan []byte) {
	defer wg.Done()

	const natsMaxMsgSize = 1000000

	fi, err := os.Stat(SrcFilePath)

	// Check if the src file exists, and that it is not bigger than
	// the default limit used by nats which is 1MB.
	switch {
	case os.IsNotExist(err):
		errCh <- fmt.Errorf("error: methodREQCopyFile: src file not found: %v", SrcFilePath)
		return
	case fi.Size() > natsMaxMsgSize:
		errCh <- fmt.Errorf("error: methodREQCopyFile: src file to big. max size: %v", natsMaxMsgSize)
		return
	}

	fh, err := os.Open(SrcFilePath)
	if err != nil {
		errCh <- fmt.Errorf("error: methodREQCopyFile: failed to open file: %v, %v", SrcFilePath, err)
		return
	}

	b, err := io.ReadAll(fh)
	if err != nil {
		errCh <- fmt.Errorf("error: methodREQCopyFile: failed to read file: %v, %v", SrcFilePath, err)
		return
	}

	select {
	case outCh <- b:
		// fmt.Printf(" * DEBUG: after io.ReadAll: outCh <- b\n")
	case <-ctx.Done():
		return
	}
}

// ----

type methodREQCopyFileTo struct {
	event Event
}

func (m methodREQCopyFileTo) getKind() Event {
	return m.event
}

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
// Same as the REQToFile, but this requst type don't use the default data folder path
// for where to store files or add information about node names.
// This method also sends a msgReply back to the publisher if the method was done
// successfully, where REQToFile do not.
// This method will truncate and overwrite any existing files.
func (m methodREQCopyFileTo) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		defer cancel()

		// Put data that should be the result of the action done in the inner
		// go routine on the outCh.
		outCh := make(chan []byte)
		// Put errors from the inner go routine on the errCh.
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			// ---
			switch {
			case len(message.MethodArgs) < 3:
				er := fmt.Errorf("error: methodREQCopyFileTo: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
				proc.errorKernel.errSend(proc, message, er)

				return
			}

			// Pick up the values for the directory and filename for where
			// to store the file.
			DstFilePath := message.MethodArgs[2]
			dstDir := filepath.Dir(DstFilePath)
			dstFile := filepath.Base(DstFilePath)

			fileRealPath := path.Join(dstDir, dstFile)

			// Check if folder structure exist, if not create it.
			if _, err := os.Stat(dstDir); os.IsNotExist(err) {
				err := os.MkdirAll(dstDir, 0700)
				if err != nil {
					er := fmt.Errorf("failed to create toFile directory tree: subject:%v, folderTree: %v, %v", proc.subject, dstDir, err)
					errCh <- er
					return
				}

				{
					er := fmt.Errorf("info: MethodREQCopyFileTo: Creating folders %v", dstDir)
					proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
				}
			}

			// Open file and write data. Truncate and overwrite any existing files.
			file := filepath.Join(dstDir, dstFile)
			f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
			if err != nil {
				er := fmt.Errorf("failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
				errCh <- er
				return
			}
			defer f.Close()

			_, err = f.Write(message.Data)
			f.Sync()
			if err != nil {
				er := fmt.Errorf("failed to write to file: file: %v, error: %v", file, err)
				errCh <- er
			}

			// All went ok, send a signal to the outer select statement.
			outCh <- []byte(fileRealPath)

			// ---

		}()

		// Wait for messages received from the inner go routine.
		select {
		case <-ctx.Done():
			er := fmt.Errorf("error: methodREQCopyFileTo: got <-ctx.Done(): %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)
			return

		case err := <-errCh:
			er := fmt.Errorf("error: methodREQCopyFileTo: %v", err)
			proc.errorKernel.errSend(proc, message, er)
			return

		case out := <-outCh:
			replyData := fmt.Sprintf("info: succesfully created and wrote the file %v\n", out)
			newReplyMessage(proc, message, []byte(replyData))
			return
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----
type methodREQHello struct {
	event Event
}

func (m methodREQHello) getKind() Event {
	return m.event
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

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	//f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	f, err := os.OpenFile(file, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)

	if err != nil {
		er := fmt.Errorf("error: methodREQHello.handler: failed to open file: %v", err)
		return nil, er
	}
	defer f.Close()

	_, err = f.Write([]byte(data))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: %v", err)
		proc.errorKernel.errSend(proc, message, er)
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
	event Event
}

func (m methodREQErrorLog) getKind() Event {
	return m.event
}

// Handle the writing of error logs.
func (m methodREQErrorLog) handler(proc process, message Message, node string) ([]byte, error) {
	proc.metrics.promErrorMessagesReceivedTotal.Inc()

	// If it was a request type message we want to check what the initial messages
	// method, so we can use that in creating the file name to store the data.
	fileName, folderTree := selectFileNaming(message, proc)

	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(folderTree); os.IsNotExist(err) {
		err := os.MkdirAll(folderTree, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create errorLog directory tree %v: %v", folderTree, err)
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file and write data.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: methodREQErrorLog.handler: failed to open file: %v", err)
		return nil, er
	}
	defer f.Close()

	_, err = f.Write(message.Data)
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodEventTextLogging.handler: failed to write to file: %v", err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQPing struct {
	event Event
}

func (m methodREQPing) getKind() Event {
	return m.event
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
			proc.errorKernel.errSend(proc, message, er)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQPing.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)

		return nil, err
	}
	defer f.Close()

	// And write the data
	d := fmt.Sprintf("%v, ping received from %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.FromNode)
	_, err = f.Write([]byte(d))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodREQPing.handler: failed to write to file: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)
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
	event Event
}

func (m methodREQPong) getKind() Event {
	return m.event
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
			proc.errorKernel.errSend(proc, message, er)

			return nil, er
		}

		er := fmt.Errorf("info: Creating subscribers data folder at %v", folderTree)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
	}

	// Open file.
	file := filepath.Join(folderTree, fileName)
	f, err := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
	if err != nil {
		er := fmt.Errorf("error: methodREQPong.handler: failed to open file, check that you've specified a value for fileName in the message: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)

		return nil, err
	}
	defer f.Close()

	// And write the data
	d := fmt.Sprintf("%v, pong received from %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), message.PreviousMessage.ToNode)
	_, err = f.Write([]byte(d))
	f.Sync()
	if err != nil {
		er := fmt.Errorf("error: methodREQPong.handler: failed to write to file: directory: %v, fileName: %v, %v", message.Directory, message.FileName, err)
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQCliCommand struct {
	event Event
}

func (m methodREQCliCommand) getKind() Event {
	return m.event
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
func (m methodREQCliCommand) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- CLICommandREQUEST received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

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
			proc.errorKernel.errSend(proc, message, er)

			return
		case len(message.MethodArgs) >= 0:
			a = message.MethodArgs[1:]
		}

		c := message.MethodArgs[0]

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

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
					envData = string(message.Data)
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
				er := fmt.Errorf("error: methodREQCliCommand: cmd.Run failed : %v, methodArgs: %v, error_output: %v", err, message.MethodArgs, stderr.String())
				proc.errorKernel.errSend(proc, message, er)
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
			proc.errorKernel.errSend(proc, message, er)
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
	event Event
}

func (m methodREQToConsole) getKind() Event {
	return m.event
}

// Handler to write directly to console.
// This handler handles the writing to console both for TUI and shell clients.
func (m methodREQToConsole) handler(proc process, message Message, node string) ([]byte, error) {

	switch {
	case proc.configuration.EnableTUI:
		if proc.processes.tui.toConsoleCh != nil {
			proc.processes.tui.toConsoleCh <- message.Data
		} else {
			er := fmt.Errorf("error: no tui client started")
			proc.errorKernel.errSend(proc, message, er)
		}
	default:
		fmt.Fprintf(os.Stdout, "%v", string(message.Data))
		fmt.Println()
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQTuiToConsole struct {
	event Event
}

func (m methodREQTuiToConsole) getKind() Event {
	return m.event
}

// Handler to write directly to console.
// DEPRECATED
func (m methodREQTuiToConsole) handler(proc process, message Message, node string) ([]byte, error) {

	if proc.processes.tui.toConsoleCh != nil {
		proc.processes.tui.toConsoleCh <- message.Data
	} else {
		er := fmt.Errorf("error: no tui client started")
		proc.errorKernel.errSend(proc, message, er)
	}

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQHttpGet struct {
	event Event
}

func (m methodREQHttpGet) getKind() Event {
	return m.event
}

// handler to do a Http Get.
func (m methodREQHttpGet) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- REQHttpGet received from: %v, containing: %v", message.FromNode, message.Data)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQHttpGet: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		url := message.MethodArgs[0]

		client := http.Client{
			Timeout: time.Second * time.Duration(message.MethodTimeout),
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			er := fmt.Errorf("error: methodREQHttpGet: NewRequest failed: %v, bailing out: %v", err, message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)
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
				proc.errorKernel.errSend(proc, message, er)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				cancel()
				er := fmt.Errorf("error: methodREQHttpGet: not 200, were %#v, bailing out: %v", resp.StatusCode, message)
				proc.errorKernel.errSend(proc, message, er)
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				er := fmt.Errorf("error: methodREQHttpGet: io.ReadAll failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er)
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
			proc.errorKernel.errSend(proc, message, er)
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

type methodREQHttpGetScheduled struct {
	event Event
}

func (m methodREQHttpGetScheduled) getKind() Event {
	return m.event
}

// handler to do a Http Get Scheduled.
// The second element of the MethodArgs slice holds the timer defined in seconds.
func (m methodREQHttpGetScheduled) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- REQHttpGetScheduled received from: %v, containing: %v", message.FromNode, message.Data)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// --- Check and prepare the methodArgs

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQHttpGet: got <3 number methodArgs. Want URL, Schedule Interval in seconds, and the total time in minutes the scheduler should run for")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		url := message.MethodArgs[0]

		scheduleInterval, err := strconv.Atoi(message.MethodArgs[1])
		if err != nil {
			er := fmt.Errorf("error: methodREQHttpGetScheduled: schedule interval value is not a valid int number defined as a string value seconds: %v, bailing out: %v", err, message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)
			return
		}

		schedulerTotalTime, err := strconv.Atoi(message.MethodArgs[2])
		if err != nil {
			er := fmt.Errorf("error: methodREQHttpGetScheduled: scheduler total time value is not a valid int number defined as a string value minutes: %v, bailing out: %v", err, message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)
			return
		}

		// --- Prepare and start the scheduler.

		outCh := make(chan []byte)

		ticker := time.NewTicker(time.Second * time.Duration(scheduleInterval))

		// Prepare a context that will be for the schedule as a whole.
		// NB: Individual http get's will create their own context's
		// derived from this one.
		ctxScheduler, cancel := context.WithTimeout(proc.ctx, time.Minute*time.Duration(schedulerTotalTime))

		go func() {
			// Prepare the http request.
			client := http.Client{
				Timeout: time.Second * time.Duration(message.MethodTimeout),
			}

			for {

				select {
				case <-ticker.C:
					proc.processes.wg.Add(1)

					// Get a context with the timeout specified in message.MethodTimeout
					// for the individual http requests.
					ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

					req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
					if err != nil {
						er := fmt.Errorf("error: methodREQHttpGet: NewRequest failed: %v, error: %v", err, message.MethodArgs)
						proc.errorKernel.errSend(proc, message, er)
						cancel()
						return
					}

					// Run each individual http get in it's own go routine, and
					// deliver the result on the outCh.
					go func() {
						defer proc.processes.wg.Done()

						resp, err := client.Do(req)
						if err != nil {
							er := fmt.Errorf("error: methodREQHttpGet: client.Do failed: %v, error: %v", err, message.MethodArgs)
							proc.errorKernel.errSend(proc, message, er)
							return
						}
						defer resp.Body.Close()

						if resp.StatusCode != 200 {
							cancel()
							er := fmt.Errorf("error: methodREQHttpGet: not 200, were %#v, error: %v", resp.StatusCode, message)
							proc.errorKernel.errSend(proc, message, er)
							return
						}

						body, err := io.ReadAll(resp.Body)
						if err != nil {
							er := fmt.Errorf("error: methodREQHttpGet: io.ReadAll failed : %v, methodArgs: %v", err, message.MethodArgs)
							proc.errorKernel.errSend(proc, message, er)
						}

						out := body

						select {
						case outCh <- out:
						case <-ctx.Done():
							return
						case <-ctxScheduler.Done():
							// If the scheduler context is done then we also want to kill
							// all running http request.
							cancel()
							return
						}
					}()

				case <-ctxScheduler.Done():
					cancel()
					return

				}
			}
		}()

		for {
			select {
			case <-ctxScheduler.Done():
				// fmt.Printf(" * DEBUG: <-ctxScheduler.Done()\n")
				cancel()
				er := fmt.Errorf("error: methodREQHttpGet: schedule context timed out: %v", message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er)
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

// --- methodREQTailFile

type methodREQTailFile struct {
	event Event
}

func (m methodREQTailFile) getKind() Event {
	return m.event
}

// handler to run a tailing of files with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
func (m methodREQTailFile) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- TailFile REQUEST received from: %v, containing: %v", message.FromNode, message.Data)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQTailFile: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		fp := message.MethodArgs[0]

		// var ctx context.Context
		// var cancel context.CancelFunc

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		// Note: Replacing the 0 timeout with specific timeout.
		// if message.MethodTimeout != 0 {
		// 	ctx, cancel = context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))
		// } else {
		// 	ctx, cancel = context.WithCancel(proc.ctx)
		// }

		outCh := make(chan []byte)
		t, err := tail.TailFile(fp, tail.Config{Follow: true, Location: &tail.SeekInfo{
			Offset: 0,
			Whence: os.SEEK_END,
		}})
		if err != nil {
			er := fmt.Errorf("error: methodREQToTailFile: tailFile: %v", err)
			proc.errorKernel.errSend(proc, message, er)
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
				proc.errorKernel.infoSend(proc, message, er)

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
	event Event
}

func (m methodREQCliCommandCont) getKind() Event {
	return m.event
}

// Handler to run REQCliCommandCont, which is the same as normal
// Cli command, but can be used when running a command that will take
// longer time and you want to send the output of the command continually
// back as it is generated, and not just when the command is finished.
func (m methodREQCliCommandCont) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- CLInCommandCont REQUEST received from: %v, containing: %v", message.FromNode, message.Data)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	// Execute the CLI command in it's own go routine, so we are able
	// to return immediately with an ack reply that the message was
	// received, and we create a new message to send back to the calling
	// node for the out put of the actual command.
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		defer func() {
			// fmt.Printf(" * DONE *\n")
		}()

		var a []string

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQCliCommand: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er)

			return
		case len(message.MethodArgs) >= 0:
			a = message.MethodArgs[1:]
		}

		c := message.MethodArgs[0]

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		// deadline, _ := ctx.Deadline()
		// fmt.Printf(" * DEBUG * deadline : %v\n", deadline)

		outCh := make(chan []byte)
		errCh := make(chan string)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			cmd := exec.CommandContext(ctx, c, a...)

			// Using cmd.StdoutPipe here so we are continuosly
			// able to read the out put of the command.
			outReader, err := cmd.StdoutPipe()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.StdoutPipe failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er)
			}

			ErrorReader, err := cmd.StderrPipe()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.StderrPipe failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er)
			}

			if err := cmd.Start(); err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.Start failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er)
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
				er := fmt.Errorf("info: methodREQCliCommandCont: method timeout reached, canceled: methodArgs: %v, %v", message.MethodArgs, err)
				proc.errorKernel.errSend(proc, message, er)
			}

		}()

		// Check if context timer or command output were received.
		for {
			select {
			case <-ctx.Done():
				cancel()
				er := fmt.Errorf("info: methodREQCliCommandCont: method timeout reached, canceling: methodArgs: %v", message.MethodArgs)
				proc.errorKernel.infoSend(proc, message, er)
				return
			case out := <-outCh:
				// fmt.Printf(" * out: %v\n", string(out))
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

type methodREQRelayInitial struct {
	event Event
}

func (m methodREQRelayInitial) getKind() Event {
	return m.event
}

// Handler to relay messages via a host.
func (m methodREQRelayInitial) handler(proc process, message Message, node string) ([]byte, error) {
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		defer cancel()

		outCh := make(chan []byte)
		errCh := make(chan error)
		nothingCh := make(chan struct{}, 1)

		var out []byte

		// If the actual Method for the message is REQCopyFileFrom we need to
		// do the actual file reading here so we can fill the data field of the
		// message with the content of the file before relaying it.
		switch {
		case message.RelayOriginalMethod == REQCopyFileFrom:
			switch {
			case len(message.MethodArgs) < 3:
				er := fmt.Errorf("error: methodREQRelayInitial: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
				proc.errorKernel.errSend(proc, message, er)

				return
			}

			SrcFilePath := message.MethodArgs[0]
			//DstFilePath := message.MethodArgs[2]

			// Read the file, and put the result on the out channel to be sent when done reading.
			proc.processes.wg.Add(1)
			go copyFileFrom(ctx, &proc.processes.wg, SrcFilePath, errCh, outCh)

			// Since we now have read the source file we don't need the REQCopyFileFrom
			// request method anymore, so we change the original method of the message
			// so it will write the data after the relaying.
			//dstDir := filepath.Dir(DstFilePath)
			//dstFile := filepath.Base(DstFilePath)
			message.RelayOriginalMethod = REQCopyFileTo
			//message.FileName = dstFile
			//message.Directory = dstDir
		default:
			// No request type that need special handling if relayed, so we should signal that
			// there is nothing to do for the select below.
			// We need to do this signaling in it's own go routine here, so we don't block here
			// since the select below  is in the same function.
			go func() {
				nothingCh <- struct{}{}
			}()
		}

		select {
		case <-ctx.Done():
			er := fmt.Errorf("error: methodREQRelayInitial: CopyFromFile: got <-ctx.Done(): %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)

			return
		case er := <-errCh:
			proc.errorKernel.errSend(proc, message, er)

			return
		case <-nothingCh:
			// Do nothing.
		case out = <-outCh:

		}

		// relay the message to the actual host here by prefixing the the RelayToNode
		// to the subject.
		relayTo := fmt.Sprintf("%v.%v", message.RelayToNode, message.RelayOriginalViaNode)
		// message.ToNode = message.RelayOriginalViaNode
		message.ToNode = Node(relayTo)
		message.FromNode = Node(node)
		message.Method = REQRelay
		message.Data = out

		sam, err := newSubjectAndMessage(message)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
			proc.errorKernel.errSend(proc, message, er)
		}

		proc.toRingbufferCh <- []subjectAndMessage{sam}
	}()

	// Send back an ACK message.
	ackMsg := []byte("confirmed REQRelay from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQRelay struct {
	event Event
}

func (m methodREQRelay) getKind() Event {
	return m.event
}

// Handler to relay messages via a host.
func (m methodREQRelay) handler(proc process, message Message, node string) ([]byte, error) {
	// relay the message here to the actual host here.

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		message.ToNode = message.RelayToNode
		message.FromNode = Node(node)
		message.Method = message.RelayOriginalMethod

		sam, err := newSubjectAndMessage(message)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		select {
		case proc.toRingbufferCh <- []subjectAndMessage{sam}:
		case <-proc.ctx.Done():
		}
	}()

	// Send back an ACK message.
	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQPublicKey struct {
	event Event
}

func (m methodREQPublicKey) getKind() Event {
	return m.event
}

// Handler to get the public ed25519 key from a node.
func (m methodREQPublicKey) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.
	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			case outCh <- proc.nodeAuth.SignPublicKey:
			}
		}()

		select {
		// case proc.toRingbufferCh <- []subjectAndMessage{sam}:
		case <-ctx.Done():
		case out := <-outCh:

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, out)
		}
	}()

	// Send back an ACK message.
	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

type methodREQPublicKeysGet struct {
	event Event
}

func (m methodREQPublicKeysGet) getKind() Event {
	return m.event
}

// Handler to get all the public ed25519 keys from a central server.
func (m methodREQPublicKeysGet) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.

	// TODO:
	// - Since this is implemented as a NACK message we could implement a
	//   metric thats shows the last time a node did a key request.
	// - We could also implement a metrics on the receiver showing the last
	//   time a node had done an update.

	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			// TODO: Should we receive a hash of he current keys from the node here
			// to verify if we need to update or not ?
			case outCh <- []byte{}:
			}
		}()

		select {
		case <-ctx.Done():
		// case out := <-outCh:
		case <-outCh:
			proc.centralAuth.pki.nodesAcked.mu.Lock()
			// TODO: We should probably create a hash of the current map content,
			// store it alongside the KeyMap, and send both the KeyMap and hash
			// back. We can then later send that hash when asking for keys, compare
			// it with the current one for the KeyMap, and know if we need to send
			// and update back to the node who published the request to here.

			fmt.Printf(" <---- methodREQPublicKeysGet: received hash from NODE=%v, HASH=%v\n", message.FromNode, message.Data)

			fmt.Printf(" *     methodREQPublicKeysGet: marshalling new keys and hash to send: map=%v, hash=%v\n\n", proc.centralAuth.pki.nodesAcked.keysAndHash.Keys, proc.centralAuth.pki.nodesAcked.keysAndHash.Hash)

			b, err := json.Marshal(proc.centralAuth.pki.nodesAcked.keysAndHash)

			proc.centralAuth.pki.nodesAcked.mu.Unlock()

			if err != nil {
				er := fmt.Errorf("error: REQPublicKeysGet, failed to marshal keys map: %v", err)
				proc.errorKernel.errSend(proc, message, er)
			}
			fmt.Printf("\n ----> methodREQPublicKeysGet: SENDING KEYS TO NODE=%v\n", message.FromNode)
			newReplyMessage(proc, message, b)
		}
	}()

	// NB: We're not sending an ACK message for this request type.
	return nil, nil
}

// ----

type methodREQPublicKeysToNode struct {
	event Event
}

func (m methodREQPublicKeysToNode) getKind() Event {
	return m.event
}

// Handler to put the public key replies received from a central server.
func (m methodREQPublicKeysToNode) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.

	// TODO:
	// - Since this is implemented as a NACK message we could implement a
	//   metric thats shows the last time keys were updated.

	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			// TODO: Should we receive a hash of he current keys from the node here ?
			case outCh <- []byte{}:
			}
		}()

		select {
		// case proc.toRingbufferCh <- []subjectAndMessage{sam}:
		case <-ctx.Done():
		case <-outCh:

			proc.nodeAuth.publicKeys.mu.Lock()

			err := json.Unmarshal(message.Data, proc.nodeAuth.publicKeys.keysAndHash)
			fmt.Printf("\n <---- REQPublicKeysToNode: after unmarshal, nodeAuth keysAndhash contains: %+v\n\n", proc.nodeAuth.publicKeys.keysAndHash)

			proc.nodeAuth.publicKeys.mu.Unlock()

			if err != nil {
				er := fmt.Errorf("error: REQPublicKeysToNode : json unmarshal failed: %v, message: %v", err, message)
				proc.errorKernel.errSend(proc, message, er)
			}

			// TODO TOMORROW: The hash is not sent with the requests to get public keys, and
			// the reason is that the hash is not stored on the nodes ?
			// Idea: We need to also persist the hash on the receiving nodes. We can then load
			//  that key upon startup, and send it along when we do a public keys get.

			err = proc.nodeAuth.publicKeys.saveToFile()
			if err != nil {
				er := fmt.Errorf("error: REQPublicKeysToNode : save to file failed: %v, message: %v", err, message)
				proc.errorKernel.errSend(proc, message, er)
			}

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			// newReplyMessage(proc, message, out)
		}
	}()

	// Send back an ACK message.
	// ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return nil, nil
}

// ----

// TODO: We should also add a request method methodREQPublicKeysRevoke

type methodREQPublicKeysAllow struct {
	event Event
}

func (m methodREQPublicKeysAllow) getKind() Event {
	return m.event
}

// Handler to allow new public keys into the database on central auth.
// Nodes will send the public key in the REQHello messages. When they
// are recived on the central server they will be put into a temp key
// map, and we need to acknowledge them before they are moved into the
// main key map, and then allowed to be sent out to other nodes.
func (m methodREQPublicKeysAllow) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.
	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {
			// Normally we would do some logic here, where the result is passed to outCh when done,
			// so we can split up the working logic, and f.ex. sending a reply logic.
			// In this case this go func and the below select is not needed, but keeping it so the
			// structure is the same as the other handlers.
			select {
			case <-ctx.Done():
			case outCh <- []byte{}:
			}
		}()

		select {
		case <-ctx.Done():
		case <-outCh:
			proc.centralAuth.pki.nodeNotAckedPublicKeys.mu.Lock()
			defer proc.centralAuth.pki.nodeNotAckedPublicKeys.mu.Unlock()

			// Range over all the MethodArgs, where each element represents a node to allow,
			// and move the node from the notAcked map to the allowed map.
			for _, n := range message.MethodArgs {
				key, ok := proc.centralAuth.pki.nodeNotAckedPublicKeys.KeyMap[Node(n)]
				if ok {

					func() {
						proc.centralAuth.pki.nodesAcked.mu.Lock()
						defer proc.centralAuth.pki.nodesAcked.mu.Unlock()

						// Store/update the node and public key on the allowed pubKey map.
						proc.centralAuth.pki.nodesAcked.keysAndHash.Keys[Node(n)] = key
					}()

					// Add key to persistent storage.
					proc.centralAuth.pki.dbUpdatePublicKey(string(n), key)

					// Delete the key from the NotAcked map
					delete(proc.centralAuth.pki.nodeNotAckedPublicKeys.KeyMap, Node(n))

					er := fmt.Errorf("info: REQPublicKeysAllow : allowed new/updated public key for %v to allowed public key map", n)
					proc.errorKernel.infoSend(proc, message, er)
				}
			}

			// All new elements are now added, and we can create a new hash
			// representing the current keys in the allowed map.
			func() {
				proc.centralAuth.pki.nodesAcked.mu.Lock()
				defer proc.centralAuth.pki.nodesAcked.mu.Unlock()

				type NodesAndKeys struct {
					Node Node
					Key  []byte
				}

				// Create a slice of all the map keys, and its value.
				sortedNodesAndKeys := []NodesAndKeys{}

				// Range the map, and add each k/v to the sorted slice, to be sorted later.
				for k, v := range proc.centralAuth.pki.nodesAcked.keysAndHash.Keys {
					nk := NodesAndKeys{
						Node: k,
						Key:  v,
					}

					sortedNodesAndKeys = append(sortedNodesAndKeys, nk)
				}

				// sort the slice based on the node name.
				// Sort all the commands.
				sort.SliceStable(sortedNodesAndKeys, func(i, j int) bool {
					return sortedNodesAndKeys[i].Node < sortedNodesAndKeys[j].Node
				})

				// Then create a hash based on the sorted slice.

				b, err := cbor.Marshal(sortedNodesAndKeys)
				if err != nil {
					er := fmt.Errorf("error: methodREQPublicKeysAllow, failed to marshal slice, and will not update hash for public keys:  %v", err)
					proc.errorKernel.errSend(proc, message, er)
					log.Printf(" * DEBUG: %v\n", er)

					return
				}

				// Store the key in the key value map.
				hash := sha256.Sum256(b)
				proc.centralAuth.pki.nodesAcked.keysAndHash.Hash = hash

				// Store the key to the db for persistence.
				proc.centralAuth.pki.dbUpdateHash(hash[:])
				if err != nil {
					er := fmt.Errorf("error: methodREQPublicKeysAllow, failed to store the hash into the db:  %v", err)
					proc.errorKernel.errSend(proc, message, er)
					log.Printf(" * DEBUG: %v\n", er)

					return
				}

			}()

		}
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ----

// ---- Template that can be used for creating request methods

// func (m methodREQCopyFileTo) handler(proc process, message Message, node string) ([]byte, error) {
//
// 	proc.processes.wg.Add(1)
// 	go func() {
// 		defer proc.processes.wg.Done()
//
// 		ctx, cancel := context.WithTimeout(proc.ctx, time.Second*time.Duration(message.MethodTimeout))
// 		defer cancel()
//
// 		// Put data that should be the result of the action done in the inner
// 		// go routine on the outCh.
// 		outCh := make(chan []byte)
// 		// Put errors from the inner go routine on the errCh.
// 		errCh := make(chan error)
//
// 		proc.processes.wg.Add(1)
// 		go func() {
// 			defer proc.processes.wg.Done()
//
// 			// Do some work here....
//
// 		}()
//
// 		// Wait for messages received from the inner go routine.
// 		select {
// 		case <-ctx.Done():
// 			fmt.Printf(" ** DEBUG: got ctx.Done\n")
//
// 			er := fmt.Errorf("error: methodREQ...: got <-ctx.Done(): %v", message.MethodArgs)
// 			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
// 			return
//
// 		case er := <-errCh:
// 			sendErrorLogMessage(proc.configuration, proc.processes.metrics, proc.toRingbufferCh, proc.node, er)
// 			return
//
// 		case out := <-outCh:
// 			replyData := fmt.Sprintf("info: succesfully created and wrote the file %v\n", out)
// 			newReplyMessage(proc, message, []byte(replyData))
// 			return
// 		}
//
// 	}()
//
// 	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
// 	return ackMsg, nil
// }
