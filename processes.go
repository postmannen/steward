package steward

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// processes holds all the information about running processes
type processes struct {
	// The main context for subscriber processes.
	ctx context.Context
	// cancel func to send cancel signal to the subscriber processes context.
	cancel context.CancelFunc
	// The active spawned processes
	active procsMap
	// mutex to lock the map
	// mu sync.RWMutex
	// The last processID created
	lastProcessID int
	// The instance global prometheus registry.
	metrics *metrics
	// Waitgroup to keep track of all the processes started
	wg sync.WaitGroup
}

// newProcesses will prepare and return a *processes which
// is map containing all the currently running processes.
func newProcesses(ctx context.Context, metrics *metrics) *processes {
	p := processes{
		active: *newProcsMap(),
	}

	// Prepare the parent context for the subscribers.
	ctx, cancel := context.WithCancel(ctx)

	// // Start the processes map.
	// go func() {
	// 	p.active.run(ctx)
	// }()

	p.ctx = ctx
	p.cancel = cancel

	p.metrics = metrics

	return &p
}

// ----------------------

type procsMap struct {
	procNames map[processName]process
	mu        sync.Mutex
}

func newProcsMap() *procsMap {
	cM := procsMap{
		procNames: make(map[processName]process),
	}
	return &cM
}

// ----------------------

// Start all the subscriber processes.
// Takes an initial process as it's input. All processes
// will be tied to this single process's context.
func (p *processes) Start(proc process) {
	// Set the context for the initial process.
	proc.ctx = p.ctx

	// --- Subscriber services that can be started via flags

	{
		log.Printf("Starting REQOpProcessList subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessList, string(proc.node))
		proc := newProcess(proc.ctx, p.metrics, proc.natsConn, p, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go proc.spawnWorker(proc.processes, proc.natsConn)
	}

	{
		log.Printf("Starting REQOpProcessStart subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessStart, string(proc.node))
		proc := newProcess(proc.ctx, p.metrics, proc.natsConn, p, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go proc.spawnWorker(proc.processes, proc.natsConn)
	}

	{
		log.Printf("Starting REQOpProcessStop subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessStop, string(proc.node))
		proc := newProcess(proc.ctx, p.metrics, proc.natsConn, p, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go proc.spawnWorker(proc.processes, proc.natsConn)
	}

	// Start a subscriber for textLogging messages
	if proc.configuration.StartSubREQToFileAppend {
		proc.startup.subREQToFileAppend(proc)
	}

	// Start a subscriber for text to file messages
	if proc.configuration.StartSubREQToFile {
		proc.startup.subREQToFile(proc)
	}

	// Start a subscriber for reading file to copy
	if proc.configuration.StartSubREQCopyFileFrom {
		proc.startup.subREQCopyFileFrom(proc)
	}

	// Start a subscriber for writing copied file to disk
	if proc.configuration.StartSubREQCopyFileTo {
		proc.startup.subREQCopyFileTo(proc)
	}

	// Start a subscriber for Hello messages
	if proc.configuration.StartSubREQHello {
		proc.startup.subREQHello(proc)
	}

	if proc.configuration.StartSubREQErrorLog {
		// Start a subscriber for REQErrorLog messages
		proc.startup.subREQErrorLog(proc)
	}

	// Start a subscriber for Ping Request messages
	if proc.configuration.StartSubREQPing {
		proc.startup.subREQPing(proc)
	}

	// Start a subscriber for REQPong messages
	if proc.configuration.StartSubREQPong {
		proc.startup.subREQPong(proc)
	}

	// Start a subscriber for REQCliCommand messages
	if proc.configuration.StartSubREQCliCommand {
		proc.startup.subREQCliCommand(proc)
	}

	// Start a subscriber for CLICommandReply messages
	if proc.configuration.StartSubREQToConsole {
		proc.startup.subREQToConsole(proc)
	}

	if proc.configuration.StartPubREQHello != 0 {
		proc.startup.pubREQHello(proc)
	}

	// Start a subscriber for Http Get Requests
	if proc.configuration.StartSubREQHttpGet {
		proc.startup.subREQHttpGet(proc)
	}

	if proc.configuration.StartSubREQTailFile {
		proc.startup.subREQTailFile(proc)
	}

	if proc.configuration.StartSubREQCliCommandCont {
		proc.startup.subREQCliCommandCont(proc)
	}

	if proc.configuration.StartSubREQRelay {
		proc.startup.subREQRelay(proc)
	}

	proc.startup.subREQRelayInitial(proc)

	proc.startup.subREQToSocket(proc)
}

// Stop all subscriber processes.
func (p *processes) Stop() {
	log.Printf("info: canceling all subscriber processes...\n")
	p.cancel()
	p.wg.Wait()
	log.Printf("info: done canceling all subscriber processes.\n")

}

// ---------------------------------------------------------------------------------------

// Startup holds all the startup methods for subscribers.
type startup struct {
	metrics *metrics
}

func newStartup(metrics *metrics) *startup {
	s := startup{metrics: metrics}

	return &s
}

func (s startup) subREQHttpGet(p process) {

	log.Printf("Starting Http Get subscriber: %#v\n", p.node)
	sub := newSubject(REQHttpGet, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)

}

func (s startup) pubREQHello(p process) {
	log.Printf("Starting Hello Publisher: %#v\n", p.node)

	sub := newSubject(REQHello, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindPublisher, nil)

	// Define the procFunc to be used for the process.
	proc.procFunc = procFunc(
		func(ctx context.Context) error {
			ticker := time.NewTicker(time.Second * time.Duration(p.configuration.StartPubREQHello))
			for {

				d := fmt.Sprintf("Hello from %v\n", p.node)

				m := Message{
					FileName:   "hello.log",
					Directory:  "hello-messages",
					ToNode:     Node(p.configuration.CentralNodeName),
					FromNode:   Node(p.node),
					Data:       []string{d},
					Method:     REQHello,
					ACKTimeout: 10,
					Retries:    1,
				}

				sam, err := newSubjectAndMessage(m)
				if err != nil {
					// In theory the system should drop the message before it reaches here.
					sendErrorLogMessage(proc.configuration, s.metrics, proc.toRingbufferCh, Node(proc.node), err)
					log.Printf("error: ProcessesStart: %v\n", err)
				}
				proc.toRingbufferCh <- []subjectAndMessage{sam}

				select {
				case <-ticker.C:
				case <-ctx.Done():
					er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
					// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
					log.Printf("%v\n", er)
					return nil
				}
			}
		})
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToConsole(p process) {
	log.Printf("Starting Text To Console subscriber: %#v\n", p.node)
	sub := newSubject(REQToConsole, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCliCommand(p process) {
	log.Printf("Starting CLICommand Request subscriber: %#v\n", p.node)
	sub := newSubject(REQCliCommand, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQPong(p process) {
	log.Printf("Starting Pong subscriber: %#v\n", p.node)
	sub := newSubject(REQPong, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQPing(p process) {
	log.Printf("Starting Ping Request subscriber: %#v\n", p.node)
	sub := newSubject(REQPing, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQErrorLog(p process) {
	log.Printf("Starting REQErrorLog subscriber: %#v\n", p.node)
	sub := newSubject(REQErrorLog, "errorCentral")
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

// subREQHello is the handler that is triggered when we are receiving a hello
// message. To keep the state of all the hello's received from nodes we need
// to also start a procFunc that will live as a go routine tied to this process,
// where the procFunc will receive messages from the handler when a message is
// received, the handler will deliver the message to the procFunc on the
// proc.procFuncCh, and we can then read that message from the procFuncCh in
// the procFunc running.
func (s startup) subREQHello(p process) {
	log.Printf("Starting Hello subscriber: %#v\n", p.node)
	sub := newSubject(REQHello, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	proc.procFuncCh = make(chan Message)

	// The reason for running the say hello subscriber as a procFunc is that
	// a handler are not able to hold state, and we need to hold the state
	// of the nodes we've received hello's from in the sayHelloNodes map,
	// which is the information we pass along to generate metrics.
	proc.procFunc = func(ctx context.Context) error {
		sayHelloNodes := make(map[Node]struct{})

		for {
			// Receive a copy of the message sent from the method handler.
			var m Message

			select {
			case m = <-proc.procFuncCh:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: subscriber %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return nil
			}

			// Add an entry for the node in the map
			sayHelloNodes[m.FromNode] = struct{}{}

			// update the prometheus metrics
			s.metrics.promHelloNodesTotal.Set(float64(len(sayHelloNodes)))
			s.metrics.promHelloNodesContactLast.With(prometheus.Labels{"nodeName": string(m.FromNode)}).SetToCurrentTime()

		}
	}
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToFile(p process) {
	log.Printf("Starting text to file subscriber: %#v\n", p.node)
	sub := newSubject(REQToFile, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCopyFileFrom(p process) {
	log.Printf("Starting copy file from subscriber: %#v\n", p.node)
	sub := newSubject(REQCopyFileFrom, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCopyFileTo(p process) {
	log.Printf("Starting copy file to subscriber: %#v\n", p.node)
	sub := newSubject(REQCopyFileTo, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToFileAppend(p process) {
	log.Printf("Starting text logging subscriber: %#v\n", p.node)
	sub := newSubject(REQToFileAppend, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQTailFile(p process) {
	log.Printf("Starting tail log files subscriber: %#v\n", p.node)
	sub := newSubject(REQTailFile, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCliCommandCont(p process) {
	log.Printf("Starting cli command with continous delivery: %#v\n", p.node)
	sub := newSubject(REQCliCommandCont, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQRelay(p process) {
	nodeWithRelay := fmt.Sprintf("*.%v", p.node)
	log.Printf("Starting Relay: %#v\n", nodeWithRelay)
	sub := newSubject(REQRelay, string(nodeWithRelay))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQRelayInitial(p process) {
	log.Printf("Starting Relay Initial: %#v\n", p.node)
	sub := newSubject(REQRelayInitial, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToSocket(p process) {
	log.Printf("Starting write to socket subscriber: %#v\n", p.node)
	sub := newSubject(REQToSocket, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

// ---------------------------------------------------------------

// Print the content of the processes map.
func (p *processes) printProcessesMap() {
	log.Printf("*** Output of processes map :\n")

	{
		p.active.mu.Lock()

		for pName, proc := range p.active.procNames {
			log.Printf("* proc - pub/sub: %v, procName in map: %v , id: %v, subject: %v\n", proc.processKind, pName, proc.processID, proc.subject.name())
		}

		p.metrics.promProcessesTotal.Set(float64(len(p.active.procNames)))

		p.active.mu.Unlock()
	}

}
