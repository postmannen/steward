package steward

import (
	"log"
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
	rb.start(inCh, ringBufferOutCh, s.configuration.DefaultMessageTimeout, s.configuration.DefaultMessageRetries)

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

	var coe CommandOrEvent
	coeAvailable := coe.GetCommandOrEventAvailable()

	var method Method
	methodsAvailable := method.GetMethodsAvailable()

	go func() {
		for samTmp := range ringBufferOutCh {
			sam := samTmp.Data
			// Check if the format of the message is correct.
			// TODO: Send a message to the error kernel here that
			// it was unable to process the message with the reason
			// why ?
			if _, ok := methodsAvailable.CheckIfExists(sam.Message.Method); !ok {
				log.Printf("error: the method do not exist, message dropped: %v\n", sam.Message.Method)
				continue
			}
			if !coeAvailable.CheckIfExists(sam.Subject.CommandOrEvent, sam.Subject) {
				log.Printf("error: the command or event do not exist, message dropped: %v\n", sam.Subject.CommandOrEvent)
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
			_, ok := s.processes.active[pn]

			// Are there already a process for that subject, put the
			// message on that processes incomming message channel.
			if ok {
				log.Printf("info: processNewMessages: found the specific subject: %v\n", subjName)
				s.processes.active[pn].subject.messageCh <- m

				// If no process to handle the specific subject exist,
				// the we create and spawn one.
			} else {
				// If a publisher process do not exist for the given subject, create it, and
				// by using the goto at the end redo the process for this specific message.
				log.Printf("info: processNewMessages: did not find that specific subject, starting new process for subject: %v\n", subjName)

				sub := newSubject(sam.Subject.Method, sam.Subject.CommandOrEvent, sam.Subject.ToNode)
				proc := newProcess(s.processes, sub, s.errorKernel.errorCh, processKindPublisher, nil, nil)
				// fmt.Printf("*** %#v\n", proc)
				proc.spawnWorker(s)

				// REMOVED:
				//time.Sleep(time.Millisecond * 500)
				s.printProcessesMap()
				// Now when the process is spawned we jump back to the redo: label,
				// and send the message to that new process.
				goto redo
			}
		}
	}()
}
