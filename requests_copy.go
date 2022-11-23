package steward

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
)

type copyInitialData struct {
	UUID             string
	SrcMethod        Method
	SrcNode          Node
	DstMethod        Method
	DstNode          Node
	SrcFilePath      string
	DstDir           string
	DstFile          string
	SplitChunkSize   int
	MaxTotalCopyTime int
	FileMode         fs.FileMode
	FolderPermission uint64
}

type methodREQCopySrc struct {
	event Event
}

func (m methodREQCopySrc) getKind() Event {
	return m.event
}

// methodREQCopySrc are handles the initial and first part of
// the message flow for a copy to destination request.
// It's main role is to start up a sub process for the destination
// in which all the actual file copying is done.
//
// Initialization, Source:
// - Use the REQCopySrc method to handle the initial request from the user.
// - Spawn a REQCopySrc_uid subscriber to receive sync messages from destination.
// - Send the uid, and full-file hash to the destination in a REQCopyDst message.
//
// Initialization, Destination:
// - Spawn a REQCopyDst-uid from the uid we got from source.
// --------------------------------------------------------------------------------------
//
// All below happens in the From-uid and To-uid methods until the copying is done.
//
// - dst->src, dst sends a REQCopySrc-uid message with status "ready" file receiving to src.
// - src receives the message and start reading the file:
// - src, creates hash of the complete file.
// - src, reads the file in chunks, and create a hash of each chunk.
//   - src->dst, send chunk when read.
//   - dst->src, wait for status "ready" indicating the chuck was transfered.
//   - Loop and read new chunc.
//   - src->dst, when last chunch is sent send status back that we are ready for the next message.
//   - src->dst, if failure send status "error", abort file copying and clean up on both src and dst.
//
// - dst, read and store each chunch to tmp folder and verify hash.
//   - dst->src, send status "ready" to src when chunch is stored.
//   - loop and check for status "last", if last:
//   - build original file from chuncs.
//   - verify hash when file is built.
//   - dst->src, send status "done".
//
// - We should also be be able to resend a chunk, or restart the copying from where we left of if it seems to hang.
//
//	dataStructure{
//		Data	[]bytes
//		Status	copyStatus
//	 id		int
//	}
//
// Create a new copy sync process to handle the actual file copying.
// We use the context already created based on the time out specified
// in the requestTimeout field of the message.
//
// -----------------------------------------------------
// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
func (m methodREQCopySrc) handler(proc process, message Message, node string) ([]byte, error) {

	// If the toNode field is not the same as nodeName of the receiving node
	// we should forward the message to that specified toNode. This will allow
	// us to initiate a file copy from another node to this node.
	if message.ToNode != proc.node {
		sam, err := newSubjectAndMessage(message)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage failed: %v, message=%v", err, message)
			proc.errorKernel.errSend(proc, message, er)
		}

		proc.toRingbufferCh <- []subjectAndMessage{sam}

		return nil, fmt.Errorf("info: the copy message was forwarded to %v message, %v", message.ToNode, message)
	}

	var subProcessName string

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Set default split chunk size, will be replaced with value from
		// methodArgs[3] if defined.
		splitChunkSize := 100000
		// Set default max total copy time, will be replaced with value from
		// methodArgs[4] if defined.
		maxTotalCopyTime := message.MethodTimeout
		// Default permission of destination folder if we need to create it.
		// The value will be replaced
		folderPermission := uint64(0755)

		er := fmt.Errorf("info: before switch: FolderPermission defined in message for socket: %04o", folderPermission)
		proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
		// Verify and check the methodArgs

		if len(message.MethodArgs) < 3 {
			er := fmt.Errorf("error: methodREQCopySrc: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
			proc.errorKernel.errSend(proc, message, er)
			return
		}

		if len(message.MethodArgs) > 3 {
			// Check if split chunk size was set, if not keep default.
			var err error
			splitChunkSize, err = strconv.Atoi(message.MethodArgs[3])
			if err != nil {
				er := fmt.Errorf("error: methodREQCopySrc: unble to convert splitChunkSize into int value: %v", err)
				proc.errorKernel.errSend(proc, message, er)
			}
		}

		if len(message.MethodArgs) > 4 {
			// Check if maxTotalCopyTime was set, if not keep default.
			var err error
			maxTotalCopyTime, err = strconv.Atoi(message.MethodArgs[4])
			if err != nil {
				er := fmt.Errorf("error: methodREQCopySrc: unble to convert maxTotalCopyTime into int value: %v", err)
				proc.errorKernel.errSend(proc, message, er)
			}
		}

		if len(message.MethodArgs) > 5 && message.MethodArgs[5] != "" {
			// Check if file permissions were set, if not use default.
			var err error
			folderPermission, err = strconv.ParseUint(message.MethodArgs[5], 8, 32)
			if err != nil {
				log.Printf("%v\n", err)
			}

			er := fmt.Errorf("info: FolderPermission defined in message for socket: %v, converted = %v", message.MethodArgs[5], folderPermission)
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
			if err != nil {
				er := fmt.Errorf("error: methodREQCopySrc: unable to convert folderPermission into int value: %v", err)
				proc.errorKernel.errSend(proc, message, er)
			}
		}

		SrcFilePath := message.MethodArgs[0]
		DstNode := message.MethodArgs[1]
		DstFilePath := message.MethodArgs[2]

		// Create a child context to use with the procFunc with timeout set to the max allowed total copy time
		// specified in the message.
		var ctx context.Context
		var cancel context.CancelFunc
		func() {
			ctx, cancel = context.WithTimeout(proc.ctx, time.Second*time.Duration(maxTotalCopyTime))
		}()

		// Create a subject for one copy request
		uid := uuid.New()
		subProcessName = fmt.Sprintf("REQSUBCopySrc.%v", uid.String())

		dstDir := filepath.Dir(DstFilePath)
		dstFile := filepath.Base(DstFilePath)
		m := Method(subProcessName)

		// Also choosing to create the naming for the dst method here so
		// we can have all the information in the cia from the beginning
		// at both ends.
		dstSubProcessName := fmt.Sprintf("REQSUBCopyDst.%v", uid.String())
		dstM := Method(dstSubProcessName)

		// Get the file permissions
		fileInfo, err := os.Stat(SrcFilePath)
		if err != nil {
			// errCh <- fmt.Errorf("error: methodREQCopySrc: failed to open file: %v, %v", SrcFilePath, err)
			er := fmt.Errorf("error: copySrcSubProcFunc: failed to stat file: %v", err)
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
			return
		}

		fileMode := fileInfo.Mode()

		cia := copyInitialData{
			UUID:             uid.String(),
			SrcNode:          proc.node,
			SrcMethod:        m,
			DstNode:          Node(DstNode),
			DstMethod:        dstM,
			SrcFilePath:      SrcFilePath,
			DstDir:           dstDir,
			DstFile:          dstFile,
			SplitChunkSize:   splitChunkSize,
			MaxTotalCopyTime: maxTotalCopyTime,
			FileMode:         fileMode,
			FolderPermission: folderPermission,
		}

		sub := newSubjectNoVerifyHandler(m, node)

		// Create a new sub process that will do the actual file copying.

		copySrcSubProc := newSubProcess(ctx, proc.server, sub, processKindSubscriber, nil)

		// Give the sub process a procFunc so we do the actual copying within a procFunc,
		// and not directly within the handler.
		copySrcSubProc.procFunc = copySrcSubProcFunc(copySrcSubProc, cia, cancel, message)

		// assign a handler to the sub process
		copySrcSubProc.handler = copySrcSubHandler(cia)

		// The process will be killed when the context expires.
		go copySrcSubProc.spawnWorker()

		// Send a message over the the node where the destination file will be written,
		// to also start up a sub process on the destination node.

		// Marshal the data payload to send to the dst.
		cb, err := cbor.Marshal(cia)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage : %v, message: %v", err, message)
			proc.errorKernel.errSend(proc, message, er)
			cancel()
		}

		msg := message
		msg.ToNode = Node(DstNode)
		//msg.Method = REQToFile
		msg.Method = REQCopyDst
		msg.Data = cb
		// msg.Directory = dstDir
		// msg.FileName = dstFile

		sam, err := newSubjectAndMessage(msg)
		if err != nil {
			er := fmt.Errorf("error: newSubjectAndMessage failed: %v, message=%v", err, message)
			proc.errorKernel.errSend(proc, message, er)
			cancel()
		}

		proc.toRingbufferCh <- []subjectAndMessage{sam}

		replyData := fmt.Sprintf("info: succesfully initiated copy source process: procName=%v, srcNode=%v, srcPath=%v, dstNode=%v, dstPath=%v, starting sub process=%v for the actual copying", copySrcSubProc.processName, node, SrcFilePath, DstNode, DstFilePath, subProcessName)

		newReplyMessage(proc, message, []byte(replyData))

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// newSubProcess is a wrapper around newProcess which sets the isSubProcess value to true.
func newSubProcess(ctx context.Context, server *server, subject Subject, processKind processKind, procFunc func() error) process {
	p := newProcess(ctx, server, subject, processKind, procFunc)
	p.isSubProcess = true

	return p
}

// ----

type methodREQCopyDst struct {
	event Event
}

func (m methodREQCopyDst) getKind() Event {
	return m.event
}

// methodREQCopyDst are handles the initial and first part of
// the message flow for a copy to destination request.
// It's main role is to start up a sub process for the destination
// in which all the actual file copying is done.
func (m methodREQCopyDst) handler(proc process, message Message, node string) ([]byte, error) {
	var subProcessName string

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get the status message sent from source.
		var cia copyInitialData
		err := cbor.Unmarshal(message.Data, &cia)
		if err != nil {
			er := fmt.Errorf("error: methodREQCopyDst: failed to cbor Unmarshal data: %v, message=%v", err, message)
			proc.errorKernel.errSend(proc, message, er)
			return
		}

		// Create a child context to use with the procFunc with timeout set to the max allowed total copy time
		// specified in the message.
		var ctx context.Context
		var cancel context.CancelFunc
		func() {
			ctx, cancel = context.WithTimeout(proc.ctx, time.Second*time.Duration(cia.MaxTotalCopyTime))
		}()

		// Create a subject for one copy request
		sub := newSubjectNoVerifyHandler(cia.DstMethod, node)

		// Create a new sub process that will do the actual file copying.
		copyDstSubProc := newSubProcess(ctx, proc.server, sub, processKindSubscriber, nil)

		// Give the sub process a procFunc so we do the actual copying within a procFunc,
		// and not directly within the handler.
		copyDstSubProc.procFunc = copyDstSubProcFunc(copyDstSubProc, cia, message, cancel)

		// assign a handler to the sub process
		copyDstSubProc.handler = copyDstSubHandler(cia)

		// The process will be killed when the context expires.
		go copyDstSubProc.spawnWorker()

		fp := filepath.Join(cia.DstDir, cia.DstFile)
		replyData := fmt.Sprintf("info: succesfully initiated copy source process: procName=%v, srcNode=%v, dstPath=%v, starting sub process=%v for the actual copying", copyDstSubProc.processName, node, fp, subProcessName)

		newReplyMessage(proc, message, []byte(replyData))

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

func copySrcSubHandler(cia copyInitialData) func(process, Message, string) ([]byte, error) {
	h := func(proc process, message Message, node string) ([]byte, error) {

		// We should receive a ready message generated by the procFunc of Dst,
		// and any messages received we directly route into the procFunc.

		select {
		case <-proc.ctx.Done():
			er := fmt.Errorf(" * copySrcHandler ended: %v", proc.processName)
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
		case proc.procFuncCh <- message:
			er := fmt.Errorf(" * copySrcHandler passing message over to procFunc: %v", proc.processName)
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
		}

		return nil, nil
	}

	return h
}

func copyDstSubHandler(cia copyInitialData) func(process, Message, string) ([]byte, error) {
	h := func(proc process, message Message, node string) ([]byte, error) {

		select {
		case <-proc.ctx.Done():
			er := fmt.Errorf(" * copyDstHandler ended: %v", proc.processName)
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
		case proc.procFuncCh <- message:
			er := fmt.Errorf(" * copySrcHandler passing message over to procFunc: %v", proc.processName)
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
		}

		return nil, nil
	}

	return h
}

type copyStatus int

const (
	copyReady      copyStatus = 1
	copyData       copyStatus = 2
	copySrcDone    copyStatus = 3
	copyResendLast copyStatus = 4
	copyDstDone    copyStatus = 5
)

// copySubData is the structure being used between the src and dst while copying data.
type copySubData struct {
	CopyStatus  copyStatus
	CopyData    []byte
	ChunkNumber int
	Hash        [32]byte
}

func copySrcSubProcFunc(proc process, cia copyInitialData, cancel context.CancelFunc, initialMessage Message) func(context.Context, chan Message) error {
	pf := func(ctx context.Context, procFuncCh chan Message) error {

		// We want to be able to send the reply message when the copying is done,
		// and also for any eventual errors within the subProcFunc. We want to
		// write these to the same place as the the reply message for the initial
		// request, but we append .sub and .error to be able to write them to
		// individual files.
		msgForSubReplies := initialMessage
		msgForSubErrors := initialMessage
		msgForSubReplies.FileName = msgForSubReplies.FileName + ".copyreply"
		msgForSubErrors.FileName = msgForSubErrors.FileName + ".copyerror"

		var chunkNumber = 0
		var lastReadChunk []byte
		var resendRetries int

		// Initiate the file copier.

		fh, err := os.Open(cia.SrcFilePath)
		if err != nil {
			er := fmt.Errorf("error: copySrcSubProcFunc: failed to open file: %v", err)
			proc.errorKernel.errSend(proc, Message{}, er)
			newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
			return er
		}
		defer fh.Close()

		// Do action based on copyStatus received.
		for {
			select {
			case <-ctx.Done():
				er := fmt.Errorf(" info: canceling copySrcProcFunc : %v", proc.processName)
				proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
				return nil

			// Pick up the message recived by the copySrcSubHandler.
			case message := <-procFuncCh:
				var csa copySubData
				err := cbor.Unmarshal(message.Data, &csa)
				if err != nil {
					er := fmt.Errorf("error: copySrcSubHandler: cbor unmarshal of csa failed: %v", err)
					proc.errorKernel.errSend(proc, message, er)
					newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
					return er
				}

				switch csa.CopyStatus {
				case copyReady:
					err := func() error {
						// We set the default status to copyData. If we get an io.EOF we change it to copyDone later.
						status := copyData

						b := make([]byte, cia.SplitChunkSize)
						n, err := fh.Read(b)
						if err != nil && err != io.EOF {
							er := fmt.Errorf("error: copySrcSubHandler: failed to read chunk from file: %v", err)
							proc.errorKernel.errSend(proc, message, er)
							newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
							return er
						}
						if err == io.EOF {
							status = copySrcDone
						}

						lastReadChunk = make([]byte, len(b[:n]))
						copy(lastReadChunk, b[:n])
						//lastReadChunk = b[:n]

						// Create a hash of the bytes.
						hash := sha256.Sum256(b[:n])

						chunkNumber++

						// Create message and send data to dst.

						csa := copySubData{
							CopyStatus:  status,
							CopyData:    b[:n],
							ChunkNumber: chunkNumber,
							Hash:        hash,
						}

						csaSerialized, err := cbor.Marshal(csa)
						if err != nil {
							er := fmt.Errorf("error: copySrcSubProcFunc: cbor marshal of csa failed: %v", err)
							proc.errorKernel.errSend(proc, message, er)
							newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
							return er
						}

						// We want to send a message back to src that we are ready to start.
						msg := Message{
							ToNode:            cia.DstNode,
							FromNode:          cia.SrcNode,
							Method:            cia.DstMethod,
							ReplyMethod:       REQNone,
							Data:              csaSerialized,
							IsSubPublishedMsg: true,
						}

						sam, err := newSubjectAndMessage(msg)
						if err != nil {
							er := fmt.Errorf("copySrcProcSubFunc: newSubjectAndMessage failed: %v", err)
							proc.errorKernel.errSend(proc, message, er)
							newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
							return er
						}

						proc.toRingbufferCh <- []subjectAndMessage{sam}

						resendRetries = 0

						// Testing with contect canceling here.
						// proc.ctxCancel()

						return nil
					}()

					if err != nil {
						return err
					}

				case copyResendLast:
					if resendRetries > message.Retries {
						er := fmt.Errorf("error: %v: failed to resend the chunk for the %v time, giving up", cia.DstMethod, resendRetries)
						proc.errorKernel.errSend(proc, message, er)
						newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
						// NB: Should we call cancel here, or wait for the timeout ?
						proc.ctxCancel()
					}

					b := lastReadChunk
					status := copyData

					// Create a hash of the bytes
					hash := sha256.Sum256(b)

					chunkNumber++

					// Create message and send data to dst

					csa := copySubData{
						CopyStatus:  status,
						CopyData:    b,
						ChunkNumber: chunkNumber,
						Hash:        hash,
					}

					csaSerialized, err := cbor.Marshal(csa)
					if err != nil {
						er := fmt.Errorf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v", err)
						proc.errorKernel.errSend(proc, message, er)
						newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
						return er
					}

					// We want to send a message back to src that we are ready to start.
					msg := Message{
						ToNode:            cia.DstNode,
						FromNode:          cia.SrcNode,
						Method:            cia.DstMethod,
						ReplyMethod:       REQNone,
						Data:              csaSerialized,
						IsSubPublishedMsg: true,
					}

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						er := fmt.Errorf("copyDstProcSubFunc: newSubjectAndMessage failed: %v", err)
						proc.errorKernel.errSend(proc, message, er)
						newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
						return er
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

					resendRetries++

				case copyDstDone:
					newReplyMessage(proc, msgForSubReplies, []byte("copyDstDone"))

					cancel()
					return nil

				default:
					er := fmt.Errorf("error: copySrcSubProcFunc: not valid copyStatus, exiting: %v", csa.CopyStatus)
					proc.errorKernel.errSend(proc, message, er)
					newReplyMessage(proc, msgForSubErrors, []byte(er.Error()))
					return er
				}
			}
		}

		//return nil
	}

	return pf
}

func copyDstSubProcFunc(proc process, cia copyInitialData, message Message, cancel context.CancelFunc) func(context.Context, chan Message) error {

	pf := func(ctx context.Context, procFuncCh chan Message) error {

		csa := copySubData{
			CopyStatus: copyReady,
		}

		csaSerialized, err := cbor.Marshal(csa)
		if err != nil {
			er := fmt.Errorf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v", err)
			proc.errorKernel.errSend(proc, message, er)
			return er
		}

		// We want to send a message back to src that we are ready to start.
		{
			msg := Message{
				ToNode:            cia.SrcNode,
				FromNode:          cia.DstNode,
				Method:            cia.SrcMethod,
				ReplyMethod:       REQNone,
				Data:              csaSerialized,
				IsSubPublishedMsg: true,
			}

			sam, err := newSubjectAndMessage(msg)
			if err != nil {
				er := fmt.Errorf("copyDstProcSubFunc: newSubjectAndMessage failed: %v", err)
				proc.errorKernel.errSend(proc, message, er)
				return er
			}

			proc.toRingbufferCh <- []subjectAndMessage{sam}
		}

		// Open a tmp folder for where to write the received chunks
		tmpFolder := filepath.Join(proc.configuration.SocketFolder, cia.DstFile+"-"+cia.UUID)
		err = os.Mkdir(tmpFolder, 0700)
		if err != nil {
			er := fmt.Errorf("copyDstProcSubFunc: create tmp folder for copying failed: %v", err)
			proc.errorKernel.errSend(proc, message, er)
			return er
		}

		for {
			select {
			case <-ctx.Done():
				er := fmt.Errorf(" * copyDstProcFunc ended: %v", proc.processName)
				proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
				return nil
			case message := <-procFuncCh:
				var csa copySubData
				err := cbor.Unmarshal(message.Data, &csa)
				if err != nil {
					er := fmt.Errorf("error: copySrcSubHandler: cbor unmarshal of csa failed: %v", err)
					proc.errorKernel.errSend(proc, message, er)
					return er
				}

				// Check if the hash matches. If it fails we set the status so we can
				// trigger the resend of the last message in the switch below.
				hash := sha256.Sum256(csa.CopyData)
				if hash != csa.Hash {
					er := fmt.Errorf("error: copyDstSubProcFunc: hash of received message is not correct for: %v", cia.DstMethod)
					proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)

					csa.CopyStatus = copyResendLast
				}

				switch csa.CopyStatus {
				case copyData:
					err := func() error {
						filePath := filepath.Join(tmpFolder, strconv.Itoa(csa.ChunkNumber)+"."+cia.UUID)
						fh, err := os.OpenFile(filePath, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
						if err != nil {
							er := fmt.Errorf("error: copyDstSubProcFunc: open destination chunk file for writing failed: %v", err)
							return er
						}
						defer fh.Close()

						_, err = fh.Write(csa.CopyData)
						if err != nil {
							er := fmt.Errorf("error: copyDstSubProcFunc: writing to chunk file failed: %v", err)
							return er
						}

						return nil
					}()

					if err != nil {
						proc.errorKernel.errSend(proc, message, err)
						return err
					}

					// Prepare and send a ready message to src for the next chunk.
					csa := copySubData{
						CopyStatus: copyReady,
					}

					csaSer, err := cbor.Marshal(csa)
					if err != nil {
						er := fmt.Errorf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v", err)
						proc.errorKernel.errSend(proc, message, er)
						return er
					}

					msg := Message{
						ToNode:            cia.SrcNode,
						FromNode:          cia.DstNode,
						Method:            cia.SrcMethod,
						ReplyMethod:       REQNone,
						Data:              csaSer,
						IsSubPublishedMsg: true,
					}

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						er := fmt.Errorf("copyDstProcSubFunc: newSubjectAndMessage failed: %v", err)
						proc.errorKernel.errSend(proc, message, er)
						return er
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

				case copyResendLast:
					// The csa already contains copyStatus copyResendLast when reached here,
					// so we can just serialize csa, and send a message back to sourcde for
					// resend of the last message.
					csaSer, err := cbor.Marshal(csa)
					if err != nil {
						er := fmt.Errorf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v", err)
						proc.errorKernel.errSend(proc, message, er)
						return er
					}

					msg := Message{
						ToNode:            cia.SrcNode,
						FromNode:          cia.DstNode,
						Method:            cia.SrcMethod,
						ReplyMethod:       REQNone,
						Data:              csaSer,
						IsSubPublishedMsg: true,
					}

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						er := fmt.Errorf("copyDstProcSubFunc: newSubjectAndMessage failed: %v", err)
						proc.errorKernel.errSend(proc, message, er)
						return er
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

				case copySrcDone:
					err := func() error {

						// Open the main file that chunks files will be written into.
						filePath := filepath.Join(cia.DstDir, cia.DstFile)

						// HERE:
						er := fmt.Errorf("info: Before creating folder: cia.FolderPermission: %04o", cia.FolderPermission)
						proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)

						if _, err := os.Stat(cia.DstDir); os.IsNotExist(err) {
							// TODO: Add option to set permission here ???
							err := os.MkdirAll(cia.DstDir, fs.FileMode(cia.FolderPermission))
							if err != nil {
								return fmt.Errorf("error: failed to create destination directory for file copying %v: %v", cia.DstDir, err)
							}
							er := fmt.Errorf("info: Created folder: with cia.FolderPermission: %04o", cia.FolderPermission)
							proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
						}

						// Rename the file so we got a backup.
						backupOriginalFileName := filePath + ".bck"
						os.Rename(filePath, backupOriginalFileName)

						mainfh, err := os.OpenFile(filePath, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, cia.FileMode)
						if err != nil {
							er := fmt.Errorf("error: copyDstSubProcFunc: open final destination file failed: %v", err)
							proc.errorKernel.errSend(proc, message, er)
							return er
						}
						defer mainfh.Close()

						type fInfo struct {
							name string
							dir  string
							nr   int
						}

						files := []fInfo{}

						// Walk the tmp transfer directory and combine all the chunks into one file.
						err = filepath.Walk(tmpFolder, func(path string, info os.FileInfo, err error) error {
							if err != nil {
								return err
							}

							if !info.IsDir() {
								fi := fInfo{}
								fi.name = filepath.Base(path)
								fi.dir = filepath.Dir(path)

								sp := strings.Split(fi.name, ".")
								nr, err := strconv.Atoi(sp[0])
								if err != nil {
									return err
								}

								fi.nr = nr

								files = append(files, fi)

							}

							return nil
						})

						// Sort all the source nodes.
						sort.SliceStable(files, func(i, j int) bool {
							return files[i].nr < files[j].nr
						})

						if err != nil {
							er := fmt.Errorf("error: copyDstSubProcFunc: creation of slice of chunk paths failed: %v", err)
							proc.errorKernel.errSend(proc, message, er)
							return er
						}

						err = func() error {
							for _, fInfo := range files {
								fp := filepath.Join(fInfo.dir, fInfo.name)
								fh, err := os.Open(fp)
								if err != nil {
									return err
								}
								defer fh.Close()

								b := make([]byte, cia.SplitChunkSize)

								n, err := fh.Read(b)
								if err != nil {
									return err
								}

								_, err = mainfh.Write(b[:n])
								if err != nil {
									return err
								}

							}

							return nil
						}()

						if err != nil {
							er := fmt.Errorf("error: copyDstSubProcFunc: write to final destination file failed: %v", err)
							proc.errorKernel.errSend(proc, message, er)
						}

						// Remove the backup file, and tmp folder.
						os.Remove(backupOriginalFileName)
						err = os.RemoveAll(tmpFolder)
						if err != nil {
							er := fmt.Errorf("error: copyDstSubProcFunc: remove temp dir failed: %v", err)
							proc.errorKernel.errSend(proc, message, er)
							return er
						}

						er = fmt.Errorf("info: copy: successfully wrote all split chunk files into file=%v", filePath)
						proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)

						// Signal back to src that we are done, so it can cancel the process.
						{
							csa := copySubData{
								CopyStatus: copyDstDone,
							}

							csaSerialized, err := cbor.Marshal(csa)
							if err != nil {
								er := fmt.Errorf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v", err)
								proc.errorKernel.errSend(proc, message, er)
								return er
							}

							// We want to send a message back to src that we are ready to start.
							msg := Message{
								ToNode:            cia.SrcNode,
								FromNode:          cia.DstNode,
								Method:            cia.SrcMethod,
								ReplyMethod:       REQNone,
								Data:              csaSerialized,
								IsSubPublishedMsg: true,
							}

							sam, err := newSubjectAndMessage(msg)
							if err != nil {
								er := fmt.Errorf("copyDstProcSubFunc: newSubjectAndMessage failed: %v", err)
								proc.errorKernel.errSend(proc, message, er)
								return er
							}

							proc.toRingbufferCh <- []subjectAndMessage{sam}
						}

						cancel()

						return nil
					}()

					if err != nil {
						return err
					}
				}
			}
		}

	}

	return pf
}
