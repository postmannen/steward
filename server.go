// Notes:
package steward

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
)

type processName string

// Will return a process name made up of subjectName+processKind
func processNameGet(sn subjectName, pk processKind) processName {
	pn := fmt.Sprintf("%s_%s", sn, pk)
	return processName(pn)
}

// processes holds all the information about running processes
type processes struct {
	// The active spawned processes
	active map[processName]process
	// mutex to lock the map
	mu sync.RWMutex
	// The last processID created
	lastProcessID int
}

// newProcesses will prepare and return a *processes
func newProcesses() *processes {
	p := processes{
		active: make(map[processName]process),
	}

	return &p
}

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	// Configuration options used for running the server
	configuration *Configuration
	// The nats connection to the broker
	natsConn *nats.Conn
	// processes holds all the information about running processes
	processes *processes
	// The name of the node
	nodeName string
	// Mutex for locking when writing to the process map
	toRingbufferCh chan []subjectAndMessage
	// errorKernel is doing all the error handling like what to do if
	// an error occurs.
	errorKernel *errorKernel
	// metric exporter
	metrics *metrics
}

// newServer will prepare and return a server type
func NewServer(c *Configuration) (*server, error) {
	conn, err := nats.Connect(c.BrokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	s := &server{
		configuration:  c,
		nodeName:       c.NodeName,
		natsConn:       conn,
		processes:      newProcesses(),
		toRingbufferCh: make(chan []subjectAndMessage),
		metrics:        newMetrics(c.PromHostAndPort),
	}

	// Create the default data folder for where subscribers should
	// write it's data, check if data folder exist, and create it if needed.
	if _, err := os.Stat(c.SubscribersDataFolder); os.IsNotExist(err) {
		if c.SubscribersDataFolder == "" {
			return nil, fmt.Errorf("error: subscribersDataFolder value is empty, you need to provide the config or the flag value at startup %v: %v", c.SubscribersDataFolder, err)
		}
		err := os.Mkdir(c.SubscribersDataFolder, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create directory %v: %v", c.SubscribersDataFolder, err)
		}

		log.Printf("info: Creating subscribers data folder at %v\n", c.SubscribersDataFolder)
	}

	return s, nil

}

// Start will spawn up all the predefined subscriber processes.
// Spawning of publisher processes is done on the fly by checking
// if there is publisher process for a given message subject, and
// not exist it will spawn one.
func (s *server) Start() {
	// Start the error kernel that will do all the error handling
	// not done within a process.
	s.errorKernel = newErrorKernel()
	s.errorKernel.startErrorKernel(s.toRingbufferCh)

	// Start collecting the metrics
	go s.startMetrics()

	// Start the checking the input socket for new messages from operator.
	go s.readSocket(s.toRingbufferCh)

	// Start up the predefined subscribers.
	s.ProcessesStart()

	time.Sleep(time.Second * 1)
	s.printProcessesMap()

	// Start the processing of new messages from an input channel.
	s.routeMessagesToProcess("./incommmingBuffer.db", s.toRingbufferCh)

	select {}

}

func (s *server) printProcessesMap() {
	fmt.Println("--------------------------------------------------------------------------------------------")
	fmt.Printf("*** Output of processes map :\n")
	s.processes.mu.Lock()
	for _, v := range s.processes.active {
		fmt.Printf("* proc - : %v, id: %v, name: %v, allowed from: %v\n", v.processKind, v.processID, v.subject.name(), v.allowedReceivers)
	}
	s.processes.mu.Unlock()

	s.metrics.metricsCh <- metricType{
		metric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "total_running_processes",
			Help: "The current number of total running processes",
		}),
		value: float64(len(s.processes.active)),
	}

	fmt.Println("--------------------------------------------------------------------------------------------")
}

// sendErrorMessage will put the error message directly on the channel that is
// read by the nats publishing functions.
func sendErrorLogMessage(newMessagesCh chan<- []subjectAndMessage, FromNode node, theError error) {
	// NB: Adding log statement here for more visuality during development.
	log.Printf("%v\n", theError)
	sam := createErrorMsgContent(FromNode, theError)
	newMessagesCh <- []subjectAndMessage{sam}
}

// createErrorMsgContent will prepare a subject and message with the content
// of the error
func createErrorMsgContent(FromNode node, theError error) subjectAndMessage {
	// TESTING: Creating an error message to send to errorCentral
	fmt.Printf(" --- Sending error message to central !!!!!!!!!!!!!!!!!!!!!!!!!!!!\n")
	sam := subjectAndMessage{
		Subject: Subject{
			ToNode:         "errorCentral",
			CommandOrEvent: EventNACK,
			Method:         ErrorLog,
		},
		Message: Message{
			ToNode:   "errorCentral",
			FromNode: FromNode,
			Data:     []string{theError.Error()},
			Method:   ErrorLog,
		},
	}

	return sam
}

// routeMessagesToProcess takes a database name and an input channel as
// it's input arguments.
// The database will be used as the persistent store for the work queue
// which is implemented as a ring buffer.
// The input channel are where we read new messages to publish.
// Incomming messages will be routed to the correct subject process, where
// the handling of each nats subject is handled within it's own separate
// worker process.
// It will also handle the process of spawning more worker processes
// for publisher subjects if it does not exist.
func (s *server) routeMessagesToProcess(dbFileName string, newSAM chan []subjectAndMessage) {
	// Prepare and start a new ring buffer
	const bufferSize int = 1000
	rb := newringBuffer(bufferSize, dbFileName, node(s.nodeName), s.toRingbufferCh)
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
			if _, ok := methodsAvailable.CheckIfExists(sam.Message.Method); !ok {
				er := fmt.Errorf("error: routeMessagesToProcess: the method do not exist, message dropped: %v", sam.Message.Method)
				sendErrorLogMessage(s.toRingbufferCh, node(s.nodeName), er)
				continue
			}
			if !coeAvailable.CheckIfExists(sam.Subject.CommandOrEvent, sam.Subject) {
				er := fmt.Errorf("error: routeMessagesToProcess: the command or event do not exist, message dropped: %v", sam.Message.Method)
				sendErrorLogMessage(s.toRingbufferCh, node(s.nodeName), er)

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

			s.processes.mu.Lock()
			_, ok := s.processes.active[pn]
			s.processes.mu.Unlock()

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
				proc := newProcess(s.processes, s.toRingbufferCh, s.configuration, sub, s.errorKernel.errorCh, processKindPublisher, nil, nil)
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
