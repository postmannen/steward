package steward

import (
	"log"
	"time"
)

// processNewMessages takes a database name and an input channel as
// it's input arguments.
// The database will be used as the persistent store for the work queue
// which is implemented as a ring buffer.
// The input channel are where we read new messages to publish.
// Incomming messages will be routed to the correct subject process, where
// the handling of each nats subject is handled within it's own separate
// worker process.
// It will also handle the process of spawning more worker processes
// for publisher subjects if it does not exist.
func (s *server) processNewMessages(dbFileName string, newSAM chan []subjectAndMessage) {
	// Prepare and start a new ring buffer
	const bufferSize int = 1000
	rb := newringBuffer(bufferSize, dbFileName)
	inCh := make(chan subjectAndMessage)
	ringBufferOutCh := make(chan samDBValue)
	// start the ringbuffer.
	rb.start(inCh, ringBufferOutCh, s.defaultMessageTimeout, s.defaultMessageRetries)

	// Start reading new fresh messages received on the incomming message
	// pipe/file requested, and fill them into the buffer.
	go func() {
		for samSlice := range newSAM {
			for _, sam := range samSlice {
				inCh <- sam
			}
		}
		close(inCh)
	}()

	// Process the messages that are in the ring buffer. Check and
	// send if there are a specific subject for it, and if no subject
	// exist throw an error.
	go func() {
		for samTmp := range ringBufferOutCh {
			sam := samTmp.Data
			// Check if the format of the message is correct.
			// TODO: Send a message to the error kernel here that
			// it was unable to process the message with the reason
			// why ?
			if _, ok := s.methodsAvailable.CheckIfExists(sam.Message.Method); !ok {
				log.Printf("error: the method do not exist: %v\n", sam.Message.Method)
				continue
			}
			if !s.commandOrEventAvailable.CheckIfExists(sam.Message.CommandOrEvent) {
				log.Printf("error: the command or evnt do not exist: %v\n", sam.Message.CommandOrEvent)
				continue
			}

		redo:
			// Adding a label here so we are able to redo the sending
			// of the last message if a process with specified subject
			// is not present. The process will then be created, and
			// the code will loop back to the redo: label.

			m := sam.Message
			subjName := sam.Subject.name()
			// DEBUG: fmt.Printf("** handleNewOperatorMessages: message: %v, ** subject: %#v\n", m, sam.Subject)
			pn := processNameGet(subjName, processKindPublisher)
			_, ok := s.processes[pn]

			// Are there already a process for that subject, put the
			// message on that processes incomming message channel.
			if ok {
				log.Printf("info: found the specific subject: %v\n", subjName)
				s.processes[pn].subject.messageCh <- m

				// If no process to handle the specific subject exist,
				// the we create and spawn one.
			} else {
				// If a publisher process do not exist for the given subject, create it, and
				// by using the goto at the end redo the process for this specific message.
				log.Printf("info: did not find that specific subject, starting new process for subject: %v\n", subjName)

				sub := newSubject(sam.Subject.Method, sam.Subject.CommandOrEvent, sam.Subject.ToNode)
				proc := s.processPrepareNew(sub, s.errorKernel.errorCh, processKindPublisher, nil)
				// fmt.Printf("*** %#v\n", proc)
				go s.spawnWorkerProcess(proc)

				time.Sleep(time.Millisecond * 500)
				s.printProcessesMap()
				// Now when the process is spawned we jump back to the redo: label,
				// and send the message to that new process.
				goto redo
			}
		}
	}()
}

func (s *server) publishMessages(proc process) {
	for {
		// Wait and read the next message on the message channel
		m := <-proc.subject.messageCh
		pn := processNameGet(proc.subject.name(), processKindPublisher)
		m.ID = s.processes[pn].messageID
		s.messageDeliverNats(proc, m)
		m.done <- struct{}{}

		// Increment the counter for the next message to be sent.
		proc.messageID++
		s.processes[pn] = proc
		time.Sleep(time.Second * 1)

		// NB: simulate that we get an error, and that we can send that
		// out of the process and receive it in another thread.
		ep := errProcess{
			infoText:      "process failed",
			process:       proc,
			message:       m,
			errorActionCh: make(chan errorAction),
		}
		s.errorKernel.errorCh <- ep

		// Wait for the response action back from the error kernel, and
		// decide what to do. Should we continue, quit, or .... ?
		switch <-ep.errorActionCh {
		case errActionContinue:
			log.Printf("The errAction was continue...so we're continuing\n")
		}
	}
}
