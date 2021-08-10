// Notes:
package steward

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	active map[processName]map[int]process
	// mutex to lock the map
	mu sync.RWMutex
	// The last processID created
	lastProcessID int
	//
	promTotalProcesses prometheus.Gauge
	//
	promProcessesVec *prometheus.GaugeVec
}

// newProcesses will prepare and return a *processes which
// is map containing all the currently running processes.
func newProcesses(promRegistry *prometheus.Registry) *processes {
	p := processes{
		active: make(map[processName]map[int]process),
	}

	p.promTotalProcesses = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "total_running_processes",
		Help: "The current number of total running processes",
	})

	p.promProcessesVec = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "running_process",
		Help: "Name of the running process",
	}, []string{"processName"},
	)

	return &p
}

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	// The main background context
	ctx context.Context
	// The CancelFunc for the main context
	ctxCancelFunc context.CancelFunc
	// The context for the subscriber processes
	ctxSubscribers context.Context
	// The cancelFun for the subscribers
	ctxSubscribersCancelFunc context.CancelFunc
	// Configuration options used for running the server
	configuration *Configuration
	// The nats connection to the broker
	natsConn *nats.Conn
	// net listener for communicating via the steward socket
	StewardSocket net.Listener
	// net listener for the communication with Stew
	StewSocket net.Listener
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
	ctx, cancel := context.WithCancel(context.Background())

	var opt nats.Option
	if c.RootCAPath != "" {
		opt = nats.RootCAs(c.RootCAPath)
	}

	if c.NkeySeedFile != "" {
		var err error
		// fh, err := os.Open(c.NkeySeedFile)
		// if err != nil {
		// 	return nil, fmt.Errorf("error: failed to open nkey seed file: %v\n", err)
		// }
		// b, err := io.ReadAll(fh)
		// if err != nil {
		// 	return nil, fmt.Errorf("error: failed to read nkey seed file: %v\n", err)
		// }
		opt, err = nats.NkeyOptionFromSeed(c.NkeySeedFile)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("error: failed to read nkey seed file: %v", err)
		}
	}

	// Connect to the nats server, and retry until succesful.

	var conn *nats.Conn
	const connRetryWait = 5

	for {
		var err error
		// Setting MaxReconnects to -1 which equals unlimited.
		conn, err = nats.Connect(c.BrokerAddress, opt, nats.MaxReconnects(-1))
		// Nats use string types for errors, so we need to check the content of the error.
		// If no servers where available, we loop and retry until succesful.
		if err != nil {
			if strings.Contains(err.Error(), "no servers available") {
				log.Printf("error: could not connect, waiting 5 seconds, and retrying: %v\n", err)
				time.Sleep(time.Duration(time.Second * connRetryWait))
				continue
			}

			er := fmt.Errorf("error: nats.Connect failed: %v", err)
			cancel()
			return nil, er
		}

		break
	}
	// Prepare the connection to the  Steward socket file

	// Check if socket folder exists, if not create it
	if _, err := os.Stat(c.SocketFolder); os.IsNotExist(err) {
		err := os.MkdirAll(c.SocketFolder, 0700)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("error: failed to create socket folder directory %v: %v", c.SocketFolder, err)
		}
	}

	socketFilepath := filepath.Join(c.SocketFolder, "steward.sock")
	if _, err := os.Stat(socketFilepath); !os.IsNotExist(err) {
		err = os.Remove(socketFilepath)
		if err != nil {
			er := fmt.Errorf("error: could not delete sock file: %v", err)
			cancel()
			return nil, er
		}
	}

	nl, err := net.Listen("unix", socketFilepath)

	if err != nil {
		er := fmt.Errorf("error: failed to open socket: %v", err)
		cancel()
		return nil, er
	}

	// ---

	// Prepare the connection to the  Stew socket file

	// Check if socket folder exists, if not create it
	if _, err := os.Stat(c.SocketFolder); os.IsNotExist(err) {
		err := os.MkdirAll(c.SocketFolder, 0700)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("error: failed to create socket folder directory %v: %v", c.SocketFolder, err)
		}
	}

	stewSocketFilepath := filepath.Join(c.SocketFolder, "stew.sock")

	if _, err := os.Stat(stewSocketFilepath); !os.IsNotExist(err) {
		err = os.Remove(stewSocketFilepath)
		if err != nil {
			er := fmt.Errorf("error: could not delete stew.sock file: %v", err)
			cancel()
			return nil, er
		}
	}

	stewNL, err := net.Listen("unix", stewSocketFilepath)
	if err != nil {
		er := fmt.Errorf("error: failed to open stew socket: %v", err)
		cancel()
		return nil, er
	}

	// ---

	metrics := newMetrics(c.PromHostAndPort)

	s := &server{
		ctx:            ctx,
		ctxCancelFunc:  cancel,
		configuration:  c,
		nodeName:       c.NodeName,
		natsConn:       conn,
		StewardSocket:  nl,
		StewSocket:     stewNL,
		processes:      newProcesses(metrics.promRegistry),
		toRingbufferCh: make(chan []subjectAndMessage),
		metrics:        metrics,
	}

	// Create the default data folder for where subscribers should
	// write it's data, check if data folder exist, and create it if needed.
	if _, err := os.Stat(c.SubscribersDataFolder); os.IsNotExist(err) {
		if c.SubscribersDataFolder == "" {
			return nil, fmt.Errorf("error: subscribersDataFolder value is empty, you need to provide the config or the flag value at startup %v: %v", c.SubscribersDataFolder, err)
		}
		err := os.Mkdir(c.SubscribersDataFolder, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create data folder directory %v: %v", c.SubscribersDataFolder, err)
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
	// that is not done within a process.
	s.errorKernel = newErrorKernel(s.ctx)

	go func() {
		err := s.errorKernel.start(s.toRingbufferCh)
		if err != nil {
			log.Printf("%v\n", err)
		}
	}()

	// Start collecting the metrics
	go func() {
		err := s.metrics.start()
		if err != nil {
			log.Printf("%v\n", err)
			os.Exit(1)
		}
	}()

	// Start the checking the input socket for new messages from operator.
	go s.readSocket(s.toRingbufferCh)

	// Start up the predefined subscribers. Since all the logic to handle
	// processes are tied to the process struct, we need to create an
	// initial process to start the rest.
	s.ctxSubscribers, s.ctxSubscribersCancelFunc = context.WithCancel(s.ctx)
	sub := newSubject(REQInitial, s.nodeName)
	p := newProcess(s.ctxSubscribers, s.natsConn, s.processes, s.toRingbufferCh, s.configuration, sub, s.errorKernel.errorCh, "", []Node{}, nil)
	p.ProcessesStart(s.ctxSubscribers)

	time.Sleep(time.Second * 1)
	s.processes.printProcessesMap()

	// Start the processing of new messages from an input channel.
	s.routeMessagesToProcess("./incomingBuffer.db", s.toRingbufferCh)

}

// Will stop all processes started during startup.
func (s *server) Stop() {
	// TODO: Add done sync functionality within the
	// stop functions so we get a confirmation that
	// all processes actually are stopped.

	// Stop the started pub/sub message processes.
	s.ctxSubscribersCancelFunc()
	log.Printf("info: stopped all subscribers\n")

	// Stop the errorKernel.
	s.errorKernel.stop()
	log.Printf("info: stopped the errorKernel\n")

	// Stop the main context.
	s.ctxCancelFunc()
	log.Printf("info: stopped the main context\n")

	// Delete the socket file when the program exits.
	socketFilepath := filepath.Join(s.configuration.SocketFolder, "steward.sock")

	if _, err := os.Stat(socketFilepath); !os.IsNotExist(err) {
		err = os.Remove(socketFilepath)
		if err != nil {
			er := fmt.Errorf("error: could not delete sock file: %v", err)
			log.Printf("%v\n", er)
		}
	}

}

func (p *processes) printProcessesMap() {
	log.Printf("*** Output of processes map :\n")
	p.mu.Lock()
	for _, vSub := range p.active {
		for _, vID := range vSub {
			log.Printf("* proc - : %v, id: %v, name: %v, allowed from: %v\n", vID.processKind, vID.processID, vID.subject.name(), vID.allowedReceivers)
		}
	}
	p.mu.Unlock()

	p.promTotalProcesses.Set(float64(len(p.active)))

}

// sendErrorMessage will put the error message directly on the channel that is
// read by the nats publishing functions.
func sendErrorLogMessage(newMessagesCh chan<- []subjectAndMessage, FromNode Node, theError error) {
	// NB: Adding log statement here for more visuality during development.
	log.Printf("%v\n", theError)
	sam := createErrorMsgContent(FromNode, theError)
	newMessagesCh <- []subjectAndMessage{sam}
}

// createErrorMsgContent will prepare a subject and message with the content
// of the error
func createErrorMsgContent(FromNode Node, theError error) subjectAndMessage {
	// Add time stamp
	er := fmt.Sprintf("%v, %v\n", time.Now().UTC(), theError.Error())

	sam := subjectAndMessage{
		Subject: newSubject(REQErrorLog, "errorCentral"),
		Message: Message{
			Directory:     "errorLog",
			ToNode:        "errorCentral",
			FromNode:      FromNode,
			FileExtension: ".log",
			Data:          []string{er},
			Method:        REQErrorLog,
		},
	}

	return sam
}

// Contains the sam value as it is used in the state DB, and also a
// delivered function to be called when this message is picked up, so
// we can control if messages gets stale at some point.
type samDBValueAndDelivered struct {
	samDBValue samDBValue
	delivered  func()
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
	rb := newringBuffer(*s.configuration, bufferSize, dbFileName, Node(s.nodeName), s.toRingbufferCh)
	inCh := make(chan subjectAndMessage)
	ringBufferOutCh := make(chan samDBValueAndDelivered)
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
			samTmp.delivered()

			sam := samTmp.samDBValue.Data
			// Check if the format of the message is correct.
			if _, ok := methodsAvailable.CheckIfExists(sam.Message.Method); !ok {
				er := fmt.Errorf("error: routeMessagesToProcess: the method do not exist, message dropped: %v", sam.Message.Method)
				sendErrorLogMessage(s.toRingbufferCh, Node(s.nodeName), er)
				continue
			}
			if !coeAvailable.CheckIfExists(sam.Subject.CommandOrEvent, sam.Subject) {
				er := fmt.Errorf("error: routeMessagesToProcess: the command or event do not exist, message dropped: %v", sam.Message.Method)
				sendErrorLogMessage(s.toRingbufferCh, Node(s.nodeName), er)

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

			// Check if there is a map of type map[int]process registered
			// for the processName, and if it exists then return it.
			s.processes.mu.Lock()
			existingProcIDMap, ok := s.processes.active[pn]
			s.processes.mu.Unlock()

			// If found a map above, range it, and are there already a process
			// for that subject, put the message on that processes incomming
			// message channel.
			if ok {
				s.processes.mu.Lock()
				for _, existingProc := range existingProcIDMap {
					log.Printf("info: processNewMessages: found the specific subject: %v\n", subjName)
					existingProc.subject.messageCh <- m
				}
				s.processes.mu.Unlock()

				// If no process to handle the specific subject exist,
				// the we create and spawn one.
			} else {
				// If a publisher process do not exist for the given subject, create it, and
				// by using the goto at the end redo the process for this specific message.
				log.Printf("info: processNewMessages: did not find that specific subject, starting new process for subject: %v\n", subjName)

				sub := newSubject(sam.Subject.Method, sam.Subject.ToNode)
				proc := newProcess(s.ctx, s.natsConn, s.processes, s.toRingbufferCh, s.configuration, sub, s.errorKernel.errorCh, processKindPublisher, nil, nil)
				// fmt.Printf("*** %#v\n", proc)
				proc.spawnWorker(s.processes, s.natsConn)

				// Now when the process is spawned we jump back to the redo: label,
				// and send the message to that new process.
				goto redo
			}
		}
	}()
}
