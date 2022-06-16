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
	"strconv"

	"github.com/fxamacker/cbor/v2"
	"github.com/google/uuid"
)

type copyInitialData struct {
	UUID           string
	SrcMethod      Method
	SrcNode        Node
	DstMethod      Method
	DstNode        Node
	SrcFilePath    string
	DstDir         string
	DstFile        string
	SplitChunkSize int
	FileMode       fs.FileMode
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
//	 - loop and check for status "last", if last:
//     - build original file from chuncs.
//     - verify hash when file is built.
//     - dst->src, send status "done".
//
// - We should also be be able to resend a chunk, or restart the copying from where we left of if it seems to hang.
//
// dataStructure{
//	Data	[]bytes
//	Status	copyStatus
//  id		int
// }
//
// Create a new copy sync process to handle the actual file copying.
// We use the context already created based on the time out specified
// in the requestTimeout field of the message.
//
// -----------------------------------------------------
// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
func (m methodREQCopySrc) handler(proc process, message Message, node string) ([]byte, error) {

	var subProcessName string

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Set default split chunk size, will be replaced with value from
		// methodArgs[3] if defined.
		splitChunkSize := 2

		// Verify and check the methodArgs
		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQCopySrc: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
			proc.errorKernel.errSend(proc, message, er)
			return

		case len(message.MethodArgs) > 3:
			// Check if split chunk size was set, if not set default.
			var err error
			splitChunkSize, err = strconv.Atoi(message.MethodArgs[3])
			if err != nil {
				er := fmt.Errorf("error: methodREQCopySrc: ch")
				proc.errorKernel.errSend(proc, message, er)
			}
		}

		fmt.Printf("\n * DEBUG: IN THE BEGINNING: SPLITCHUNKSIZE: %v\n\n", splitChunkSize)

		SrcFilePath := message.MethodArgs[0]
		DstNode := message.MethodArgs[1]
		DstFilePath := message.MethodArgs[2]

		// Get a context with the timeout specified in message.MethodTimeout.
		// Since the subProc spawned will outlive this method here we do not
		// want to cancel this method. We care about the methodTimeout, but
		// we ignore the CancelFunc.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

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
			// errCh <- fmt.Errorf("error: methodREQCopyFile: failed to open file: %v, %v", SrcFilePath, err)
			log.Printf("error: copySrcSubProcFunc: failed to stat file: %v\n", err)
			return
		}

		fileMode := fileInfo.Mode()

		cia := copyInitialData{
			UUID:           uid.String(),
			SrcNode:        proc.node,
			SrcMethod:      m,
			DstNode:        Node(DstNode),
			DstMethod:      dstM,
			SrcFilePath:    SrcFilePath,
			DstDir:         dstDir,
			DstFile:        dstFile,
			SplitChunkSize: splitChunkSize,
			FileMode:       fileMode,
		}

		sub := newSubjectNoVerifyHandler(m, node)

		// Create a new sub process that will do the actual file copying.
		copySrcSubProc := newProcess(ctx, proc.server, sub, processKindSubscriber, nil)

		// Give the sub process a procFunc so we do the actual copying within a procFunc,
		// and not directly within the handler.
		copySrcSubProc.procFunc = copySrcSubProcFunc(copySrcSubProc, cia)

		// assign a handler to the sub process
		copySrcSubProc.handler = copySrcSubHandler(cia)

		// The process will be killed when the context expires.
		go copySrcSubProc.spawnWorker()

		// Send a message over the the node where the destination file will be written,
		// to also start up a sub process on the destination node.

		// Marshal the data payload to send the the dst.
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
			er := fmt.Errorf("error: methodREQCopySrc failed to cbor Marshal data: %v, message=%v", err, message)
			proc.errorKernel.errSend(proc, message, er)
			cancel()
		}

		proc.toRingbufferCh <- []subjectAndMessage{sam}

		replyData := fmt.Sprintf("info: succesfully initiated copy source process: procName=%v, srcNode=%v, srcPath=%v, dstNode=%v, dstPath=%v, starting sub process=%v for the actual copying\n", copySrcSubProc.processName, node, SrcFilePath, DstNode, DstFilePath, subProcessName)

		newReplyMessage(proc, message, []byte(replyData))

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
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

		// Get a context with the timeout specified in message.MethodTimeout.
		// Since the subProc spawned will outlive this method here we do not
		// want to cancel this method. We care about the methodTimeout, but
		// we ignore the CancelFunc.
		ctx, _ := getContextForMethodTimeout(proc.ctx, message)

		// Create a subject for one copy request
		sub := newSubjectNoVerifyHandler(cia.DstMethod, node)

		// Create a new sub process that will do the actual file copying.
		copyDstSubProc := newProcess(ctx, proc.server, sub, processKindSubscriber, nil)

		// Give the sub process a procFunc so we do the actual copying within a procFunc,
		// and not directly within the handler.
		copyDstSubProc.procFunc = copyDstSubProcFunc(copyDstSubProc, cia, message)

		// assign a handler to the sub process
		copyDstSubProc.handler = copyDstSubHandler(cia)

		// The process will be killed when the context expires.
		go copyDstSubProc.spawnWorker()

		fp := filepath.Join(cia.DstDir, cia.DstFile)
		replyData := fmt.Sprintf("info: succesfully initiated copy source process: procName=%v, srcNode=%v, dstPath=%v, starting sub process=%v for the actual copying\n", copyDstSubProc.processName, node, fp, subProcessName)

		newReplyMessage(proc, message, []byte(replyData))

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

func copySrcSubHandler(cia copyInitialData) func(process, Message, string) ([]byte, error) {
	h := func(proc process, message Message, node string) ([]byte, error) {

		// We should receive a ready message generated by the procFunc of Dst.

		select {
		case <-proc.ctx.Done():
			log.Printf(" * copySrcHandler ended: %v\n", proc.processName)
		case proc.procFuncCh <- message:
			log.Printf(" * copySrcHandler passing message over to procFunc: %v\n", proc.processName)
		}

		return nil, nil
	}

	return h
}

func copyDstSubHandler(cia copyInitialData) func(process, Message, string) ([]byte, error) {
	h := func(proc process, message Message, node string) ([]byte, error) {

		select {
		case <-proc.ctx.Done():
			log.Printf(" * copyDstHandler ended: %v\n", proc.processName)
		case proc.procFuncCh <- message:
			log.Printf(" * copySrcHandler passing message over to procFunc: %v\n", proc.processName)
		}

		return nil, nil
	}

	return h
}

type copyStatus int

const (
	copyReady      copyStatus = 1
	copyData       copyStatus = 2
	copyDone       copyStatus = 3
	copyResendLast copyStatus = 4
)

// copySubData is the structure being used between the src and dst while copying data.
type copySubData struct {
	CopyStatus  copyStatus
	CopyData    []byte
	ChunkNumber int
	Hash        [32]byte
}

func copySrcSubProcFunc(proc process, cia copyInitialData) func(context.Context, chan Message) error {
	pf := func(ctx context.Context, procFuncCh chan Message) error {

		var chunkNumber = 0
		var lastReadChunk []byte
		var resendRetries int

		// Initiate the file copier.

		fh, err := os.Open(cia.SrcFilePath)
		if err != nil {
			// errCh <- fmt.Errorf("error: methodREQCopyFile: failed to open file: %v, %v", SrcFilePath, err)
			log.Fatalf("error: copySrcSubProcFunc: failed to open file: %v\n", err)
			return nil
		}
		defer fh.Close()

		// Do action based on copyStatus received.
		for {
			fmt.Printf("\n * DEBUG: copySrcSubProcFunc: cia contains: %+v\n\n", cia)
			select {
			case <-ctx.Done():
				log.Printf(" * copySrcProcFunc ENDED: %v\n", proc.processName)
				return nil

			// Pick up the message recived by the copySrcSubHandler.
			case message := <-procFuncCh:
				var csa copySubData
				err := cbor.Unmarshal(message.Data, &csa)
				if err != nil {
					log.Fatalf("error: copySrcSubHandler: cbor unmarshal of csa failed: %v\n", err)
				}

				switch csa.CopyStatus {
				case copyReady:
					// We set the default status to copyData. If we get an io.EOF we change it to copyDone later.
					status := copyData

					log.Printf(" * RECEIVED in copySrcSubProcFunc from dst *  copyStatus=copyReady: %v\n\n", csa.CopyStatus)
					b := make([]byte, cia.SplitChunkSize)
					n, err := fh.Read(b)
					if err != nil && err != io.EOF {
						log.Printf("error: copySrcSubHandler: failed to read chuck from file: %v\n", err)
					}
					if err == io.EOF {
						status = copyDone
					}

					lastReadChunk = b[:n]

					// Create a hash of the bytes
					hash := sha256.Sum256(b[:n])

					chunkNumber++

					// Create message and send data to dst
					// fmt.Printf("**** DATA READ: %v\n", b)

					csa := copySubData{
						CopyStatus:  status,
						CopyData:    b[:n],
						ChunkNumber: chunkNumber,
						Hash:        hash,
					}

					csaSerialized, err := cbor.Marshal(csa)
					if err != nil {
						log.Fatalf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v\n", err)
					}

					// We want to send a message back to src that we are ready to start.
					fmt.Printf("\n\n\n ************** DEBUG: copyDstHandler sub process sending copyReady to:%v\n ", message.FromNode)
					msg := Message{
						ToNode:      cia.DstNode,
						FromNode:    cia.SrcNode,
						Method:      cia.DstMethod,
						ReplyMethod: REQNone,
						Data:        csaSerialized,
					}

					fmt.Printf("\n ***** DEBUG: copyDstSubProcFunc: cia.SrcMethod: %v\n\n ", cia.SrcMethod)

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						log.Fatalf("copyDstProcSubFunc: newSubjectAndMessage failed: %v\n", err)
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

					resendRetries = 0

					// Testing with contect canceling here.
					// proc.ctxCancel()

				case copyResendLast:
					if resendRetries > message.Retries {
						er := fmt.Errorf("error: %v: failed to resend the chunk for the %v time, giving up", cia.DstMethod, resendRetries)
						proc.errorKernel.errSend(proc, message, er)
						// NB: Should we call cancel here, or wait for the timeout ?
						proc.ctxCancel()
					}

					// HERE!
					b := lastReadChunk
					status := copyData

					// Create a hash of the bytes
					hash := sha256.Sum256(b)

					chunkNumber++

					// Create message and send data to dst
					fmt.Printf("**** DATA READ: %v\n", b)

					csa := copySubData{
						CopyStatus:  status,
						CopyData:    b,
						ChunkNumber: chunkNumber,
						Hash:        hash,
					}

					csaSerialized, err := cbor.Marshal(csa)
					if err != nil {
						log.Fatalf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v\n", err)
					}

					// We want to send a message back to src that we are ready to start.
					fmt.Printf("\n\n\n ************** DEBUG: copyDstHandler sub process sending copyReady to:%v\n ", message.FromNode)
					msg := Message{
						ToNode:      cia.DstNode,
						FromNode:    cia.SrcNode,
						Method:      cia.DstMethod,
						ReplyMethod: REQNone,
						Data:        csaSerialized,
					}

					fmt.Printf("\n ***** DEBUG: copyDstSubProcFunc: cia.SrcMethod: %v\n\n ", cia.SrcMethod)

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						log.Fatalf("copyDstProcSubFunc: newSubjectAndMessage failed: %v\n", err)
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

					resendRetries++

				default:
					// TODO: Any error logic here ?
					log.Fatalf("error: copySrcSubProcFunc: not valid copyStatus, exiting: %v\n", csa.CopyStatus)
				}
			}
		}

		//return nil
	}

	return pf
}

func copyDstSubProcFunc(proc process, cia copyInitialData, message Message) func(context.Context, chan Message) error {

	pf := func(ctx context.Context, procFuncCh chan Message) error {
		fmt.Printf("\n ******* WORKING IN copyDstSubProcFunc: %+v\n\n", cia)

		csa := copySubData{
			CopyStatus: copyReady,
		}

		csaSerialized, err := cbor.Marshal(csa)
		if err != nil {
			log.Fatalf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v\n", err)
		}

		// We want to send a message back to src that we are ready to start.
		{
			fmt.Printf("\n\n\n ************** DEBUG: copyDstHandler sub process sending copyReady to:%v\n ", message.FromNode)
			msg := Message{
				ToNode:      cia.SrcNode,
				FromNode:    cia.DstNode,
				Method:      cia.SrcMethod,
				ReplyMethod: REQNone,
				Data:        csaSerialized,
			}

			fmt.Printf("\n ***** DEBUG: copyDstSubProcFunc: cia.SrcMethod: %v\n\n ", cia.SrcMethod)

			sam, err := newSubjectAndMessage(msg)
			if err != nil {
				log.Fatalf("copyDstProcSubFunc: newSubjectAndMessage failed: %v\n", err)
			}

			proc.toRingbufferCh <- []subjectAndMessage{sam}
		}

		// Open a tmp folder for where to write the received chunks
		tmpFolder := filepath.Join(proc.configuration.SocketFolder, cia.DstFile+"-"+cia.UUID)
		os.Mkdir(tmpFolder, 0700)

		for {
			fmt.Printf("\n * DEBUG: copyDstSubProcFunc: cia contains: %+v\n\n", cia)
			select {
			case <-ctx.Done():
				log.Printf(" * copyDstProcFunc ended: %v\n", proc.processName)
				return nil
			case message := <-procFuncCh:
				var csa copySubData
				err := cbor.Unmarshal(message.Data, &csa)
				if err != nil {
					log.Fatalf("error: copySrcSubHandler: cbor unmarshal of csa failed: %v\n", err)
				}

				// Check if the hash matches. If it fails we set the status so we can
				// trigger the resend of the last message in the switch below.
				hash := sha256.Sum256(csa.CopyData)
				if hash != csa.Hash {
					log.Printf("error: copyDstSubProcFunc: hash of received message is not correct for: %v\n", cia.DstMethod)

					csa.CopyStatus = copyResendLast
				}

				fmt.Printf(" * DEBUG: Hash was verified OK\n")

				switch csa.CopyStatus {
				case copyData:
					// Write the data chunk to disk ?
					// fmt.Printf("\n * Received data: %s\n\n", csa.CopyData)

					func() {
						filePath := filepath.Join(tmpFolder, strconv.Itoa(csa.ChunkNumber)+"."+cia.UUID)
						fh, err := os.OpenFile(filePath, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
						if err != nil {
							log.Fatalf("error: copyDstSubProcFunc: open file failed: %v\n", err)
						}
						defer fh.Close()

						_, err = fh.Write(csa.CopyData)
						if err != nil {
							log.Fatalf("error: copyDstSubProcFunc: open file failed: %v\n", err)
						}
					}()

					// Prepare and send a ready message to src for the next chunk.
					csa := copySubData{
						CopyStatus: copyReady,
					}

					csaSer, err := cbor.Marshal(csa)
					if err != nil {
						log.Fatalf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v\n", err)
					}

					fmt.Printf("\n\n\n ************** DEBUG: copyDstHandler sub process sending copyReady to:%v\n ", message.FromNode)
					msg := Message{
						ToNode:      cia.SrcNode,
						FromNode:    cia.DstNode,
						Method:      cia.SrcMethod,
						ReplyMethod: REQNone,
						Data:        csaSer,
					}

					fmt.Printf("\n ***** DEBUG: copyDstSubProcFunc: cia.SrcMethod: %v\n\n ", cia.SrcMethod)

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						log.Fatalf("copyDstProcSubFunc: newSubjectAndMessage failed: %v\n", err)
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

				case copyResendLast:
					// The csa already contains copyStatus copyResendLast when reached here,
					// so we can just serialize csa, and send a message back to sourcde for
					// resend of the last message.
					csaSer, err := cbor.Marshal(csa)
					if err != nil {
						log.Fatalf("error: copyDstSubProcFunc: cbor marshal of csa failed: %v\n", err)
					}

					msg := Message{
						ToNode:      cia.SrcNode,
						FromNode:    cia.DstNode,
						Method:      cia.SrcMethod,
						ReplyMethod: REQNone,
						Data:        csaSer,
					}

					sam, err := newSubjectAndMessage(msg)
					if err != nil {
						log.Fatalf("copyDstProcSubFunc: newSubjectAndMessage failed: %v\n", err)
					}

					proc.toRingbufferCh <- []subjectAndMessage{sam}

				case copyDone:
					func() {

						// Open the main file that chunks files will be written into.
						filePath := filepath.Join(cia.DstDir, cia.DstFile)

						// Rename the file so we got a backup.
						backupOriginalFileName := filePath + ".bck"
						os.Rename(filePath, backupOriginalFileName)

						mainfh, err := os.OpenFile(filePath, os.O_TRUNC|os.O_RDWR|os.O_CREATE|os.O_SYNC, cia.FileMode)
						if err != nil {
							log.Fatalf("error: copyDstSubProcFunc: open file failed: %v\n", err)
						}
						defer mainfh.Close()

						// Walk the tmp transfer directory and combine all the chunks into one file.
						err = filepath.Walk(tmpFolder, func(path string, info os.FileInfo, err error) error {
							if err != nil {
								return err
							}

							if !info.IsDir() {
								fh, err := os.Open(path)
								if err != nil {
									return err
								}
								defer fh.Close()

								b := make([]byte, cia.SplitChunkSize)

								n, err := fh.Read(b)
								if err != nil {
									return err
								}

								log.Printf("info: copy: writing content of split chunk file=%v into=%v, size=%v\n", path, filePath, info.Size())
								_, err = mainfh.Write(b[:n])
								if err != nil {
									return err
								}

							}

							return nil
						})
						if err != nil {
							log.Printf("error: copyDstSubProcFunc: combining the split file chunks back to original file failed: %v\n", err)

							// Delete the file we've been trying to write to.
							os.Remove(filePath)
							// Rename the old file back to it's original name.
							os.Rename(backupOriginalFileName, filePath)

							return
						}

						// All ok, remove the backup file, and tmp folder.
						os.Remove(backupOriginalFileName)
						err = os.RemoveAll(tmpFolder)
						if err != nil {
							log.Fatalf("error: copyDstSubProcFunc: remove temp dir failed: %v\n", err)
						}

						log.Printf("info: copy: successfully wrote all split chunk files into file=%v\n", filePath)

					}()
				}
			}
		}

	}

	return pf
}
