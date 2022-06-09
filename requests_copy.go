package steward

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
)

type methodREQCopySrc struct {
	event Event
}

func (m methodREQCopySrc) getKind() Event {
	return m.event
}

// Idea:
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
//   - src->dst, when last chunch is sent send status "last"
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

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQCopySrc: got <3 number methodArgs: want srcfilePath,dstNode,dstFilePath")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		SrcFilePath := message.MethodArgs[0]
		DstNode := message.MethodArgs[1]
		DstFilePath := message.MethodArgs[2]

		// Get a context with the timeout specified in message.MethodTimeout.
		// Since the subProc spawned will outlive this method here we do not
		// want to cancel this method. We care about the methodTimeout, but
		// we ignore the CancelFunc.
		ctx, _ := getContextForMethodTimeout(proc.ctx, message)

		// Create a subject for one copy request
		uid := uuid.New()
		subProcessName = fmt.Sprintf("copySrc_%v", uid.String())

		m := Method(subProcessName)
		sub := newSubjectNoVerifyHandler(m, node)

		// Create a new sub process that will do the actual file copying.
		copySrcSubProc := newProcess(ctx, proc.server, sub, processKindSubscriber, nil)
		// Give the sub process a procFunc so we do the actual copying within a procFunc,
		// and not directly within the handler.
		copySrcSubProc.procFunc = copySrcProcFunc()
		// The process will be killed when the context expires.
		go copySrcSubProc.spawnWorker()

		replyData := fmt.Sprintf("info: succesfully initiated copy: srcNode=%v, srcPath=%v, dstNode=%v, dstPath=%v, starting sub process=%v for the actual copying\n", node, SrcFilePath, DstNode, DstFilePath, subProcessName)
		newReplyMessage(proc, message, []byte(replyData))

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

func copySrcProcFunc() func(context.Context, chan Message) error {
	pf := func(ctx context.Context, procFuncCh chan Message) error {

		select {
		case <-ctx.Done():
			log.Printf(" * copySrcProcFunc ended\n")
		}

		return nil
	}

	return pf
}

// ----

type methodREQCopyDst struct {
	event Event
}

func (m methodREQCopyDst) getKind() Event {
	return m.event
}

// Handle writing to a file. Will truncate any existing data if the file did already
// exist.
// Same as the REQToFile, but this requst type don't use the default data folder path
// for where to store files or add information about node names.
// This method also sends a msgReply back to the publisher if the method was done
// successfully, where REQToFile do not.
// This method will truncate and overwrite any existing files.
func (m methodREQCopyDst) handler(proc process, message Message, node string) ([]byte, error) {

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
