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

// server is the structure that will hold the state about spawned
// processes on a local instance.
type server struct {
	// Configuration options used for running the server
	configuration *Configuration
	// The nats connection to the broker
	natsConn *nats.Conn
	// TODO: sessions should probably hold a slice/map of processes ?
	processes map[processName]process
	// The last processID created
	lastProcessID int
	// The name of the node
	nodeName string
	// Mutex for locking when writing to the process map
	mu sync.Mutex
	// The channel where we put new messages read from file,
	// or some other process who wants to send something via the
	// system
	// We can than range this channel for new messages to process.
	newMessagesCh chan []subjectAndMessage
	// errorKernel is doing all the error handling like what to do if
	// an error occurs.
	// TODO: Will also send error messages to cental error subscriber.
	errorKernel *errorKernel
	// used to check if the methods specified in message is valid
	methodsAvailable MethodsAvailable
	// Map who holds the command and event types available.
	// Used to check if the commandOrEvent specified in message is valid
	commandOrEventAvailable CommandOrEventAvailable
	// metric exporter
	metrics *metrics
	// subscriberServices are where we find the services and the API to
	// use services needed by subscriber.
	// For example, this can be a service that knows
	// how to forward the data for a received message of type log to a
	// central logger.
	subscriberServices *subscriberServices
	// Is this the central error logger ?
	// collection of the publisher services and the types to control them
	publisherServices  *publisherServices
	centralErrorLogger bool
	// default message timeout in seconds. This can be overridden on the message level
	defaultMessageTimeout int
	// default amount of retries that will be done before a message is thrown away, and out of the system
	defaultMessageRetries int
}

// newServer will prepare and return a server type
func NewServer(c *Configuration) (*server, error) {
	conn, err := nats.Connect(c.BrokerAddress, nil)
	if err != nil {
		log.Printf("error: nats.Connect failed: %v\n", err)
	}

	var m Method
	var coe CommandOrEvent

	s := &server{
		configuration:           c,
		nodeName:                c.NodeName,
		natsConn:                conn,
		processes:               make(map[processName]process),
		newMessagesCh:           make(chan []subjectAndMessage),
		methodsAvailable:        m.GetMethodsAvailable(),
		commandOrEventAvailable: coe.GetCommandOrEventAvailable(),
		metrics:                 newMetrics(c.PromHostAndPort),
		subscriberServices:      newSubscriberServices(),
		publisherServices:       newPublisherServices(c.PublisherServiceSayhello),
		centralErrorLogger:      c.CentralErrorLogger,
		defaultMessageTimeout:   c.DefaultMessageTimeout,
		defaultMessageRetries:   c.DefaultMessageRetries,
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

	time.Sleep(time.Second * 2)
	s.printProcessesMap()

	// Start the processing of new messaging from an input channel.
	s.processNewMessages("./incommmingBuffer.db", s.newMessagesCh)

	select {}

}

func (s *server) printProcessesMap() {
	fmt.Println("--------------------------------------------------------------------------------------------")
	fmt.Printf("*** Output of processes map :\n")
	for _, v := range s.processes {
		fmt.Printf("*** - : %v\n", v)
	}

	s.metrics.metricsCh <- metricType{
		metric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "total_running_processes",
			Help: "The current number of total running processes",
		}),
		value: float64(len(s.processes)),
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
