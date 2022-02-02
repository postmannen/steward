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
	// processes holds all the information about running processes
	processes *processes
	// The name of the node
	nodeName string
	// ringBufferBulkInCh are the channel where new messages in a bulk
	// format (slice) are put into the system.
	//
	// In general the ringbuffer will read this
	// channel, unfold each slice, and put single messages on the buffer.
	ringBufferBulkInCh chan []subjectAndMessage
	// errorKernel is doing all the error handling like what to do if
	// an error occurs.
	errorKernel *errorKernel
	// Ring buffer
	ringBuffer *ringBuffer
	// metric exporter
	metrics *metrics
	// Version of package
	version string
	// tui client
	tui *tui
	// processInitial is the initial process that all other processes are tied to.
	processInitial process
}

// newServer will prepare and return a server type
func NewServer(c *Configuration, version string) (*server, error) {
	// Set up the main background context.
	ctx, cancel := context.WithCancel(context.Background())

	metrics := newMetrics(c.PromHostAndPort)

	// Start the error kernel that will do all the error handling
	// that is not done within a process.
	errorKernel := newErrorKernel(ctx, metrics)

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

	// Connect to the nats server, and retry until succesful.
	for {
		var err error
		// Setting MaxReconnects to -1 which equals unlimited.
		conn, err = nats.Connect(c.BrokerAddress,
			opt,
			nats.MaxReconnects(-1),
			nats.ReconnectJitter(time.Duration(c.NatsReconnectJitter)*time.Millisecond, time.Duration(c.NatsReconnectJitterTLS)*time.Second),
			nats.Timeout(time.Second*time.Duration(c.NatsConnOptTimeout)),
		)
		// If no servers where available, we loop and retry until succesful.
		if err != nil {
			log.Printf("error: could not connect, waiting %v seconds, and retrying: %v\n", c.NatsConnectRetryInterval, err)
			time.Sleep(time.Duration(time.Second * time.Duration(c.NatsConnectRetryInterval)))
			continue
		}

		break
	}

	log.Printf(" * conn.Opts.ReconnectJitterTLS: %v\n", conn.Opts.ReconnectJitterTLS)
	log.Printf(" * conn.Opts.ReconnectJitter: %v\n", conn.Opts.ReconnectJitter)

	var stewardSocket net.Listener
	var err error

	// Open the steward socket file, and start the listener if enabled.
	if c.EnableSocket {
		stewardSocket, err = createSocket(c.SocketFolder, "steward.sock")
		if err != nil {
			cancel()
			return nil, err
		}
	}

	// Create the tui client structure if enabled.
	var tuiClient *tui
	if c.EnableTUI {
		tuiClient, err = newTui(Node(c.NodeName))
		if err != nil {
			cancel()
			return nil, err
		}

	}

	s := &server{
		ctx:                ctx,
		cancel:             cancel,
		configuration:      c,
		nodeName:           c.NodeName,
		natsConn:           conn,
		StewardSocket:      stewardSocket,
		processes:          newProcesses(ctx, metrics, tuiClient, errorKernel, c),
		ringBufferBulkInCh: make(chan []subjectAndMessage),
		metrics:            metrics,
		version:            version,
		tui:                tuiClient,
		errorKernel:        errorKernel,
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

// create socket will create a socket file, and return the net.Listener to
// communicate with that socket.
func createSocket(socketFolder string, socketFileName string) (net.Listener, error) {
	// Check if socket folder exists, if not create it
	if _, err := os.Stat(socketFolder); os.IsNotExist(err) {
		err := os.MkdirAll(socketFolder, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create socket folder directory %v: %v", socketFolder, err)
		}
	}

	// Just as an extra check we eventually delete any existing Steward socket files if found.
	socketFilepath := filepath.Join(socketFolder, socketFileName)
	if _, err := os.Stat(socketFilepath); !os.IsNotExist(err) {
		err = os.Remove(socketFilepath)
		if err != nil {
			er := fmt.Errorf("error: could not delete sock file: %v", err)
			return nil, er
		}
	}

	// Open the socket.
	nl, err := net.Listen("unix", socketFilepath)
	if err != nil {
		er := fmt.Errorf("error: failed to open socket: %v", err)
		return nil, er
	}

	return nl, nil
}

// Start will spawn up all the predefined subscriber processes.
// Spawning of publisher processes is done on the fly by checking
// if there is publisher process for a given message subject, and
// if it does not exist it will spawn one.
func (s *server) Start() {
	log.Printf("Starting steward, version=%+v\n", s.version)
	s.metrics.promVersion.With(prometheus.Labels{"version": string(s.version)})

	go func() {
		err := s.errorKernel.start(s.ringBufferBulkInCh)
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
	if s.configuration.EnableSocket {
		go s.readSocket()
	}

	// Check if we should start the tcp listener for new messages from operator.
	if s.configuration.TCPListener != "" {
		go s.readTCPListener()
	}

	// Check if we should start the http listener for new messages from operator.
	if s.configuration.HTTPListener != "" {
		go s.readHttpListener()
	}

	// Start up the predefined subscribers.
	//
	// Since all the logic to handle processes are tied to the process
	// struct, we need to create an initial process to start the rest.
	//
	// NB: The context of the initial process are set in processes.Start.
	sub := newSubject(REQInitial, s.nodeName)
	s.processInitial = newProcess(context.TODO(), s.metrics, s.natsConn, s.processes, s.ringBufferBulkInCh, s.configuration, sub, s.errorKernel.errorCh, "", nil)
	// Start all wanted subscriber processes.
	s.processes.Start(s.processInitial)

	// We need the initial process to be able to send error messages so
	// we have to load the signing keys here.
	err := s.processes.loadSigningKeys(s.processInitial)
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	time.Sleep(time.Second * 1)
	s.processes.printProcessesMap()

	// Start exposing the the data folder via HTTP if flag is set.
	if s.configuration.ExposeDataFolder != "" {
		log.Printf("info: Starting expose of data folder via HTTP\n")
		go s.exposeDataFolder(s.ctx)
	}

	if s.configuration.EnableTUI {
		go func() {
			err := s.tui.Start(s.ctx, s.ringBufferBulkInCh)
			if err != nil {
				log.Printf("%v\n", err)
				os.Exit(1)
			}
		}()
	}

	// Start the processing of new messages from an input channel.
	// NB: We might need to create a sub context for the ringbuffer here
	// so we can cancel this context last, and not use the server.
	s.routeMessagesToProcess("./incomingBuffer.db")

	// Check and enable read the messages specified in the startup folder.
	s.readStartupFolder()

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

	// Delete the steward socket file when the program exits.
	socketFilepath := filepath.Join(s.configuration.SocketFolder, "steward.sock")

	if _, err := os.Stat(socketFilepath); !os.IsNotExist(err) {
		err = os.Remove(socketFilepath)
		if err != nil {
			er := fmt.Errorf("error: could not delete sock file: %v", err)
			log.Printf("%v\n", er)
		}
	}

}

// sendInfoMessage will put the error message directly on the channel that is
// read by the nats publishing functions.
//
// DEPRECATED:
// func sendInfoLogMessage(conf *Configuration, metrics *metrics, ringBufferBulkInCh chan<- []subjectAndMessage, FromNode Node, theError error) {
// 	// NB: Adding log statement here for more visuality during development.
// 	log.Printf("%v\n", theError)
// 	sam := createErrorMsgContent(conf, FromNode, theError)
// 	ringBufferBulkInCh <- []subjectAndMessage{sam}
//
// 	metrics.promInfoMessagesSentTotal.Inc()
// }

// createErrorMsgContent will prepare a subject and message with the content
// of the error
func createErrorMsgContent(conf *Configuration, FromNode Node, theError error) subjectAndMessage {
	// Add time stamp
	er := fmt.Sprintf("%v, node: %v, %v\n", time.Now().Format("Mon Jan _2 15:04:05 2006"), FromNode, theError.Error())

	sam := subjectAndMessage{
		Subject: newSubject(REQErrorLog, "errorCentral"),
		Message: Message{
			Directory:  "errorLog",
			ToNode:     "errorCentral",
			FromNode:   FromNode,
			FileName:   "error.log",
			Data:       []byte(er),
			Method:     REQErrorLog,
			ACKTimeout: conf.ErrorMessageTimeout,
			Retries:    conf.ErrorMessageRetries,
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
// The ringBufferInCh are where we get new messages to publish.
// Incomming messages will be routed to the correct subject process, where
// the handling of each nats subject is handled within it's own separate
// worker process.
// It will also handle the process of spawning more worker processes
// for publisher subjects if it does not exist.
func (s *server) routeMessagesToProcess(dbFileName string) {
	// Prepare and start a new ring buffer
	var bufferSize int = s.configuration.RingBufferSize
	const samValueBucket string = "samValueBucket"
	const indexValueBucket string = "indexValueBucket"

	s.ringBuffer = newringBuffer(s.ctx, s.metrics, s.configuration, bufferSize, dbFileName, Node(s.nodeName), s.ringBufferBulkInCh, samValueBucket, indexValueBucket, s.errorKernel, s.processInitial)

	ringBufferInCh := make(chan subjectAndMessage)
	ringBufferOutCh := make(chan samDBValueAndDelivered)
	// start the ringbuffer.
	s.ringBuffer.start(s.ctx, ringBufferInCh, ringBufferOutCh)

	// Start reading new fresh messages received on the incomming message
	// pipe/file requested, and fill them into the buffer.
	// Since the new messages comming into the system is a []subjectAndMessage
	// we loop here, unfold the slice, and put single subjectAndMessages's on
	// the channel to the ringbuffer.
	go func() {
		for sams := range s.ringBufferBulkInCh {
			for _, sam := range sams {
				ringBufferInCh <- sam
			}
		}
		close(ringBufferInCh)
	}()

	// Process the messages that are in the ring buffer. Check and
	// send if there are a specific subject for it, and if no subject
	// exist throw an error.

	var event Event
	eventAvailable := event.CheckEventAvailable()

	var method Method
	methodsAvailable := method.GetMethodsAvailable()

	go func() {
		for samDBVal := range ringBufferOutCh {
			// Signal back to the ringbuffer that message have been picked up.
			samDBVal.delivered()

			sam := samDBVal.samDBValue.Data
			// Check if the format of the message is correct.
			if _, ok := methodsAvailable.CheckIfExists(sam.Message.Method); !ok {
				er := fmt.Errorf("error: routeMessagesToProcess: the method do not exist, message dropped: %v", sam.Message.Method)
				s.errorKernel.errSend(s.processInitial, sam.Message, er)
				continue
			}
			if !eventAvailable.CheckIfExists(sam.Subject.Event, sam.Subject) {
				er := fmt.Errorf("error: routeMessagesToProcess: the event type do not exist, message dropped: %v", sam.Message.Method)
				s.errorKernel.errSend(s.processInitial, sam.Message, er)

				continue
			}

			for {
				// Looping here so we are able to redo the sending
				// of the last message if a process for the specified subject
				// is not present. The process will then be created, and
				// the code will loop back here.

				m := sam.Message

				// Check if it is a relay message
				if m.RelayViaNode != "" && m.RelayViaNode != Node(s.nodeName) {

					// Keep the original values.
					m.RelayFromNode = m.FromNode
					m.RelayToNode = m.ToNode
					m.RelayOriginalViaNode = m.RelayViaNode
					m.RelayOriginalMethod = m.Method

					// Convert it to a relay initial message.
					m.Method = REQRelayInitial
					// Set the toNode of the message to this host, so we send
					// it to ourselves again and pick it up with the subscriber
					// for the REQReplyInitial handler method.
					m.ToNode = Node(s.nodeName)

					// We are now done with the initial checking for if the new
					// message is a relay message, so we empty the viaNode field
					// so we don't end in an endless loop here.
					// The value is stored in RelayOriginalViaNode for later use.
					m.RelayViaNode = ""

					sam.Subject = newSubject(REQRelayInitial, string(s.nodeName))
				}

				subjName := sam.Subject.name()
				pn := processNameGet(subjName, processKindPublisher)

				// Check if there is a map of type map[int]process registered
				// for the processName, and if it exists then return it.
				s.processes.active.mu.Lock()
				proc, ok := s.processes.active.procNames[pn]
				s.processes.active.mu.Unlock()

				// If found a map above, range it, and are there already a process
				// for that subject, put the message on that processes incomming
				// message channel.
				if ok {
					// We have found the process to route the message to, deliver it.
					proc.subject.messageCh <- m

					break
				} else {
					// If a publisher process do not exist for the given subject, create it.
					// log.Printf("info: processNewMessages: did not find that specific subject, starting new process for subject: %v\n", subjName)

					sub := newSubject(sam.Subject.Method, sam.Subject.ToNode)
					proc := newProcess(s.ctx, s.metrics, s.natsConn, s.processes, s.ringBufferBulkInCh, s.configuration, sub, s.errorKernel.errorCh, processKindPublisher, nil)

					proc.spawnWorker(s.processes, s.natsConn)
					// log.Printf("info: processNewMessages: new process started, subject: %v, processID: %v\n", subjName, proc.processID)

					// Now when the process is spawned we continue,
					// and send the message to that new process.
					continue
				}
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
