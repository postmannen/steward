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

func processNameGet(sn subjectName, pk processKind) processName {
	pn := fmt.Sprintf("%s_%s", sn, pk)
	return processName(pn)
}

// processes holds all the information about running processes
type processes struct {
	// The active spawned processes
	active map[processName]process
	// mutex to lock the map
	mu sync.Mutex
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
	newMessagesCh chan []subjectAndMessage
	// errorKernel is doing all the error handling like what to do if
	// an error occurs.
	// TODO: Will also send error messages to cental error subscriber.
	errorKernel *errorKernel
	// metric exporter
	metrics *metrics
	// Is this the central error logger ?
	// collection of the publisher services and the types to control them
	publisherServices  *publisherServices
	centralErrorLogger bool
}

// newServer will prepare and return a server type
func NewServer(c *Configuration) (*server, error) {
	conn, err := nats.Connect(c.BrokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	s := &server{
		configuration:      c,
		nodeName:           c.NodeName,
		natsConn:           conn,
		processes:          newProcesses(),
		newMessagesCh:      make(chan []subjectAndMessage),
		metrics:            newMetrics(c.PromHostAndPort),
		publisherServices:  newPublisherServices(c.PublisherServiceSayhello),
		centralErrorLogger: c.CentralErrorLogger,
	}

	// Create the default data folder for where subscribers should
	// write it's data if needed.

	// Check if data folder exist, and create it if needed.
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
	s.errorKernel.startErrorKernel(s.newMessagesCh)

	// Start collecting the metrics
	go s.startMetrics()

	// Start the checking the input file for new messages from operator.
	go s.getMessagesFromFile("./", "inmsg.txt", s.newMessagesCh)

	// if enabled, start the sayHello I'm here service at the given interval
	if s.publisherServices.sayHelloPublisher.interval != 0 {
		go s.publisherServices.sayHelloPublisher.start(s.newMessagesCh, node(s.nodeName))
	}

	// Start up the predefined subscribers.
	// TODO: What to subscribe on should be handled via flags, or config
	// files.
	s.subscribersStart()

	time.Sleep(time.Second * 1)
	s.printProcessesMap()

	// Start the processing of new messaging from an input channel.
	s.processNewMessages("./incommmingBuffer.db", s.newMessagesCh)

	select {}

}

func (s *server) printProcessesMap() {
	fmt.Println("--------------------------------------------------------------------------------------------")
	fmt.Printf("*** Output of processes map :\n")
	s.processes.mu.Lock()
	for _, v := range s.processes.active {
		fmt.Printf("* proc - : id: %v, name: %v, allowed from: %v\n", v.processID, v.subject.name(), v.allowedReceivers)
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
	// --- Testing
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
