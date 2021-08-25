// Notes:
package steward

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type processName string

// Will return a process name made up of subjectName+processKind
func processNameGet(sn subjectName, pk processKind) processName {
	pn := fmt.Sprintf("%s_%s", sn, pk)
	return processName(pn)
}

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	// The main background context
	ctx context.Context
	// The CancelFunc for the main context
	cancel context.CancelFunc
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
	// newMessagesCh are the channel where new messages to be handled
	// by the system are put.
	newMessagesCh chan []subjectAndMessage
	// errorKernel is doing all the error handling like what to do if
	// an error occurs.
	errorKernel *errorKernel
	// metric exporter
	metrics *metrics
}

// newServer will prepare and return a server type
func NewServer(c *Configuration) (*server, error) {
	// Set up the main background context.
	ctx, cancel := context.WithCancel(context.Background())

	var opt nats.Option

	if c.RootCAPath != "" {
		opt = nats.RootCAs(c.RootCAPath)
	}

	if c.NkeySeedFile != "" {
		var err error

		opt, err = nats.NkeyOptionFromSeed(c.NkeySeedFile)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("error: failed to read nkey seed file: %v", err)
		}
	}

	var conn *nats.Conn
	const connRetryWait = 5

	// Connect to the nats server, and retry until succesful.
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

	// Just as an extra check we eventually delete any existing Steward socket files if found.
	socketFilepath := filepath.Join(c.SocketFolder, "steward.sock")
	if _, err := os.Stat(socketFilepath); !os.IsNotExist(err) {
		err = os.Remove(socketFilepath)
		if err != nil {
			er := fmt.Errorf("error: could not delete sock file: %v", err)
			cancel()
			return nil, er
		}
	}

	// Open the socket.
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

	// Just as an extra check we eventually delete any existing Stew socket files if found.
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
		ctx:           ctx,
		cancel:        cancel,
		configuration: c,
		nodeName:      c.NodeName,
		natsConn:      conn,
		StewardSocket: nl,
		StewSocket:    stewNL,
		processes:     newProcesses(ctx, metrics),
		newMessagesCh: make(chan []subjectAndMessage),
		metrics:       metrics,
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
// if it does not exist it will spawn one.
func (s *server) Start() {
	// Start the error kernel that will do all the error handling
	// that is not done within a process.
	s.errorKernel = newErrorKernel(s.ctx)

	go func() {
		err := s.errorKernel.start(s.newMessagesCh)
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
	go s.readSocket()

	// Check if we should start the tcp listener fro new messages from operator.
	if s.configuration.TCPListener != "" {
		go s.readTCPListener(s.newMessagesCh)
	}

	// Start up the predefined subscribers.
	//
	// Since all the logic to handle processes are tied to the process
	// struct, we need to create an initial process to start the rest.
	//
	// NB: The context of the initial process are set in processes.Start.
	sub := newSubject(REQInitial, s.nodeName)
	p := newProcess(context.TODO(), s.metrics, s.natsConn, s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, "", []Node{}, nil)
	// Start all wanted subscriber processes.
	s.processes.Start(p)

	time.Sleep(time.Second * 1)
	s.processes.printProcessesMap()

	// Start exposing the the data folder via HTTP if flag is set.
	if s.configuration.ExposeDataFolder != "" {
		log.Printf("info: Starting expose of data folder via HTTP\n")
		go s.exposeDataFolder(s.ctx)
	}

	// Start the processing of new messages from an input channel.
	s.routeMessagesToProcess("./incomingBuffer.db")

}

// Will stop all processes started during startup.
func (s *server) Stop() {
	// Stop the started pub/sub message processes.
	s.processes.Stop()
	log.Printf("info: stopped all subscribers\n")

	// Stop the errorKernel.
	s.errorKernel.stop()
	log.Printf("info: stopped the errorKernel\n")

	// Stop the main context.
	s.cancel()
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
			Directory: "errorLog",
			ToNode:    "errorCentral",
			FromNode:  FromNode,
			FileName:  "error.log",
			Data:      []string{er},
			Method:    REQErrorLog,
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

// routeMessagesToProcess takes a database name it's input argument.
// The database will be used as the persistent k/v store for the work
// queue which is implemented as a ring buffer.
// The newMessagesCh are where we get new messages to publish.
// Incomming messages will be routed to the correct subject process, where
// the handling of each nats subject is handled within it's own separate
// worker process.
// It will also handle the process of spawning more worker processes
// for publisher subjects if it does not exist.
func (s *server) routeMessagesToProcess(dbFileName string) {
	// Prepare and start a new ring buffer
	const bufferSize int = 1000
	rb := newringBuffer(s.metrics, *s.configuration, bufferSize, dbFileName, Node(s.nodeName), s.newMessagesCh)
	// TODO:
	ringBufferInCh := make(chan subjectAndMessage)
	ringBufferOutCh := make(chan samDBValueAndDelivered)
	// start the ringbuffer.
	rb.start(ringBufferInCh, ringBufferOutCh, s.configuration.DefaultMessageTimeout, s.configuration.DefaultMessageRetries)

	// Start reading new fresh messages received on the incomming message
	// pipe/file requested, and fill them into the buffer.
	go func() {
		for sams := range s.newMessagesCh {
			for _, sam := range sams {
				ringBufferInCh <- sam
			}
		}
		close(ringBufferInCh)
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
				sendErrorLogMessage(s.newMessagesCh, Node(s.nodeName), er)
				continue
			}
			if !coeAvailable.CheckIfExists(sam.Subject.CommandOrEvent, sam.Subject) {
				er := fmt.Errorf("error: routeMessagesToProcess: the command or event do not exist, message dropped: %v", sam.Message.Method)
				sendErrorLogMessage(s.newMessagesCh, Node(s.nodeName), er)

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
				proc := newProcess(s.ctx, s.metrics, s.natsConn, s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindPublisher, nil, nil)
				// fmt.Printf("*** %#v\n", proc)
				proc.spawnWorker(s.processes, s.natsConn)

				// Now when the process is spawned we jump back to the redo: label,
				// and send the message to that new process.
				goto redo
			}
		}
	}()
}

func (s *server) exposeDataFolder(ctx context.Context) {
	fileHandler := func(w http.ResponseWriter, r *http.Request) {
		// w.Header().Set("Content-Type", "text/html")
		http.FileServer(http.Dir(s.configuration.SubscribersDataFolder)).ServeHTTP(w, r)
	}

	//create a file server, and serve the files found in ./
	//fd := http.FileServer(http.Dir(s.configuration.SubscribersDataFolder))
	http.HandleFunc("/", fileHandler)

	// we create a net.Listen type to use later with the http.Serve function.
	nl, err := net.Listen("tcp", s.configuration.ExposeDataFolder)
	if err != nil {
		log.Println("error: starting net.Listen: ", err)
	}

	// start the web server with http.Serve instead of the usual http.ListenAndServe
	err = http.Serve(nl, nil)
	if err != nil {
		log.Printf("Error: failed to start web server: %v\n", err)
	}
	os.Exit(1)

}
