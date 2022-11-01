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
	// toRingBufferCh are the channel where new messages in a bulk
	// format (slice) are put into the system.
	//
	// In general the ringbuffer will read this
	// channel, unfold each slice, and put single messages on the buffer.
	toRingBufferCh chan []subjectAndMessage
	// directSAMSCh
	directSAMSCh chan []subjectAndMessage
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

	// nodeAuth holds all the signatures, the public keys and other components
	// related to authentication on an individual node.
	nodeAuth *nodeAuth
	// helloRegister is a register of all the nodes that have sent hello messages
	// to the central server
	helloRegister *helloRegister
	// holds the logic for the central auth services
	centralAuth *centralAuth
}

// newServer will prepare and return a server type
func NewServer(configuration *Configuration, version string) (*server, error) {
	// Set up the main background context.
	ctx, cancel := context.WithCancel(context.Background())

	metrics := newMetrics(configuration.PromHostAndPort)

	// Start the error kernel that will do all the error handling
	// that is not done within a process.
	errorKernel := newErrorKernel(ctx, metrics)

	var opt nats.Option

	if configuration.RootCAPath != "" {
		opt = nats.RootCAs(configuration.RootCAPath)
	}

	if configuration.NkeySeedFile != "" {
		var err error

		opt, err = nats.NkeyOptionFromSeed(configuration.NkeySeedFile)
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
		conn, err = nats.Connect(configuration.BrokerAddress,
			opt,
			//nats.FlusherTimeout(time.Second*10),
			nats.MaxReconnects(-1),
			nats.ReconnectJitter(time.Duration(configuration.NatsReconnectJitter)*time.Millisecond, time.Duration(configuration.NatsReconnectJitterTLS)*time.Second),
			nats.Timeout(time.Second*time.Duration(configuration.NatsConnOptTimeout)),
		)
		// If no servers where available, we loop and retry until succesful.
		if err != nil {
			log.Printf("error: could not connect, waiting %v seconds, and retrying: %v\n", configuration.NatsConnectRetryInterval, err)
			time.Sleep(time.Duration(time.Second * time.Duration(configuration.NatsConnectRetryInterval)))
			continue
		}

		break
	}

	log.Printf(" * conn.Opts.ReconnectJitterTLS: %v\n", conn.Opts.ReconnectJitterTLS)
	log.Printf(" * conn.Opts.ReconnectJitter: %v\n", conn.Opts.ReconnectJitter)

	var stewardSocket net.Listener
	var err error

	// Open the steward socket file, and start the listener if enabled.
	if configuration.EnableSocket {
		stewardSocket, err = createSocket(configuration.SocketFolder, "steward.sock")
		if err != nil {
			cancel()
			return nil, err
		}
	}

	// Create the tui client structure if enabled.
	var tuiClient *tui
	if configuration.EnableTUI {
		tuiClient, err = newTui(Node(configuration.NodeName))
		if err != nil {
			cancel()
			return nil, err
		}

	}

	//var nodeAuth *nodeAuth
	//if configuration.EnableSignatureCheck {
	nodeAuth := newNodeAuth(configuration, errorKernel)
	// fmt.Printf(" * DEBUG: newServer: signatures contains: %+v\n", signatures)
	//}

	//var centralAuth *centralAuth
	//if configuration.IsCentralAuth {
	centralAuth := newCentralAuth(configuration, errorKernel)
	//}

	s := server{
		ctx:            ctx,
		cancel:         cancel,
		configuration:  configuration,
		nodeName:       configuration.NodeName,
		natsConn:       conn,
		StewardSocket:  stewardSocket,
		toRingBufferCh: make(chan []subjectAndMessage),
		directSAMSCh:   make(chan []subjectAndMessage),
		metrics:        metrics,
		version:        version,
		tui:            tuiClient,
		errorKernel:    errorKernel,
		nodeAuth:       nodeAuth,
		helloRegister:  newHelloRegister(),
		centralAuth:    centralAuth,
	}

	s.processes = newProcesses(ctx, &s)

	// Create the default data folder for where subscribers should
	// write it's data, check if data folder exist, and create it if needed.
	if _, err := os.Stat(configuration.SubscribersDataFolder); os.IsNotExist(err) {
		if configuration.SubscribersDataFolder == "" {
			return nil, fmt.Errorf("error: subscribersDataFolder value is empty, you need to provide the config or the flag value at startup %v: %v", configuration.SubscribersDataFolder, err)
		}
		err := os.Mkdir(configuration.SubscribersDataFolder, 0700)
		if err != nil {
			return nil, fmt.Errorf("error: failed to create data folder directory %v: %v", configuration.SubscribersDataFolder, err)
		}

		er := fmt.Errorf("info: creating subscribers data folder at %v", configuration.SubscribersDataFolder)
		s.errorKernel.logConsoleOnlyIfDebug(er, s.configuration)
	}

	return &s, nil

}

// helloRegister is a register of all the nodes that have sent hello messages.
type helloRegister struct {
}

func newHelloRegister() *helloRegister {
	h := helloRegister{}

	return &h
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
		err := s.errorKernel.start(s.toRingBufferCh)
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
	s.processInitial = newProcess(context.TODO(), s, sub, "", nil)
	// Start all wanted subscriber processes.
	s.processes.Start(s.processInitial)

	time.Sleep(time.Second * 1)
	s.processes.printProcessesMap()

	// Start exposing the the data folder via HTTP if flag is set.
	if s.configuration.ExposeDataFolder != "" {
		log.Printf("info: Starting expose of data folder via HTTP\n")
		go s.exposeDataFolder(s.ctx)
	}

	if s.configuration.EnableTUI {
		go func() {
			err := s.tui.Start(s.ctx, s.toRingBufferCh)
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

	// Start reading the channel for injecting direct messages that should
	// not be sent via the message broker.
	s.directSAMSChRead()

	// Check and enable read the messages specified in the startup folder.
	s.readStartupFolder()

}

// directSAMSChRead for injecting messages directly in to the local system
// without sending them via the message broker.
func (s *server) directSAMSChRead() {
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				log.Printf("info: stopped the directSAMSCh reader\n\n")
				return
			case sams := <-s.directSAMSCh:
				// fmt.Printf(" * DEBUG: directSAMSChRead: <- sams = %v\n", sams)
				// Range over all the sams, find the process, check if the method exists, and
				// handle the message by starting the correct method handler.
				for i := range sams {
					processName := processNameGet(sams[i].Subject.name(), processKindSubscriber)

					s.processes.active.mu.Lock()
					p := s.processes.active.procNames[processName]
					s.processes.active.mu.Unlock()

					mh, ok := p.methodsAvailable.CheckIfExists(sams[i].Message.Method)
					if !ok {
						er := fmt.Errorf("error: subscriberHandler: method type not available: %v", p.subject.Event)
						p.errorKernel.errSend(p, sams[i].Message, er)
						continue
					}

					p.handler = mh.handler

					go executeHandler(p, sams[i].Message, s.nodeName)
				}
			}
		}
	}()
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

// samDBValueAndDelivered Contains the sam value as it is used in the
// state DB, and also a delivered function to be called when this message
// is picked up, so we can control if messages gets stale at some point.
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

	s.ringBuffer = newringBuffer(s.ctx, s.metrics, s.configuration, bufferSize, dbFileName, Node(s.nodeName), s.toRingBufferCh, samValueBucket, indexValueBucket, s.errorKernel, s.processInitial)

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
		for sams := range s.toRingBufferCh {
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
					var proc process
					switch {
					case m.IsSubPublishedMsg:
						proc = newSubProcess(s.ctx, s, sub, processKindPublisher, nil)
					default:
						proc = newProcess(s.ctx, s, sub, processKindPublisher, nil)
					}

					proc.spawnWorker()
					er := fmt.Errorf("info: processNewMessages: new process started, subject: %v, processID: %v", subjName, proc.processID)
					s.errorKernel.logConsoleOnlyIfDebug(er, s.configuration)

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
