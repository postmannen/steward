package steward

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func (p process) ProcessesStart() {

	// --- Subscriber services that can be started via flags

	// Allways start an REQOpCommand subscriber
	{
		log.Printf("Starting REQOpCommand subscriber: %#v\n", p.node)
		sub := newSubject(REQOpCommand, string(p.node))
		proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, []Node{Node(p.configuration.CentralNodeName)}, nil)
		go proc.spawnWorker(p.processes, p.natsConn)
	}

	// Start a subscriber for textLogging messages
	if p.configuration.StartSubREQToFileAppend.OK {
		p.startup.subREQToFileAppend(p)
	}

	// Start a subscriber for text to file messages
	if p.configuration.StartSubREQToFile.OK {
		p.startup.subREQToFile(p)
	}

	// Start a subscriber for Hello messages
	if p.configuration.StartSubREQHello.OK {
		p.startup.subREQHello(p)
	}

	if p.configuration.StartSubREQErrorLog.OK {
		// Start a subscriber for REQErrorLog messages
		p.startup.subREQErrorLog(p)
	}

	// Start a subscriber for Ping Request messages
	if p.configuration.StartSubREQPing.OK {
		p.startup.subREQPing(p)
	}

	// Start a subscriber for REQPong messages
	if p.configuration.StartSubREQPong.OK {
		p.startup.subREQPong(p)
	}

	// Start a subscriber for REQCliCommand messages
	if p.configuration.StartSubREQCliCommand.OK {
		p.startup.subREQCliCommand(p)
	}

	// Start a subscriber for Not In Order Cli Command Request messages
	if p.configuration.StartSubREQnCliCommand.OK {
		p.startup.subREQnCliCommand(p)
	}

	// Start a subscriber for CLICommandReply messages
	if p.configuration.StartSubREQToConsole.OK {
		p.startup.subREQToConsole(p)
	}

	if p.configuration.StartPubREQHello != 0 {
		p.startup.pubREQHello(p)
	}

	// Start a subscriber for Http Get Requests
	if p.configuration.StartSubREQHttpGet.OK {
		p.startup.subREQHttpGet(p)
	}

	if p.configuration.StartSubREQTailFile.OK {
		p.startup.subREQTailFile(p)
	}

	p.startup.subREQToSocket(p)
}

// ---------------------------------------------------------------------------------------

type startup struct{}

func (s startup) subREQHttpGet(p process) {

	log.Printf("Starting Http Get subscriber: %#v\n", p.node)
	sub := newSubject(REQHttpGet, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQHttpGet.Values, nil)
	// fmt.Printf("*** %#v\n", proc)
	go proc.spawnWorker(p.processes, p.natsConn)

}

func (s startup) pubREQHello(p process) {
	log.Printf("Starting Hello Publisher: %#v\n", p.node)

	sub := newSubject(REQHello, p.configuration.CentralNodeName)
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindPublisher, []Node{}, nil)

	// Define the procFunc to be used for the process.
	proc.procFunc = procFunc(
		func(ctx context.Context) error {
			ticker := time.NewTicker(time.Second * time.Duration(p.configuration.StartPubREQHello))
			for {
				// fmt.Printf("--- DEBUG : procFunc call:kind=%v, Subject=%v, toNode=%v\n", proc.processKind, proc.subject, proc.subject.ToNode)

				d := fmt.Sprintf("Hello from %v\n", p.node)

				m := Message{
					ToNode:   Node(p.configuration.CentralNodeName),
					FromNode: Node(p.node),
					Data:     []string{d},
					Method:   REQHello,
				}

				sam, err := newSAM(m)
				if err != nil {
					// In theory the system should drop the message before it reaches here.
					log.Printf("error: ProcessesStart: %v\n", err)
				}
				proc.toRingbufferCh <- []subjectAndMessage{sam}

				select {
				case <-ticker.C:
				case <-ctx.Done():
					er := fmt.Errorf("info: stopped handleFunc for: %v", proc.subject.name())
					sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
					return nil
				}
			}
		})
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToConsole(p process) {
	log.Printf("Starting Text To Console subscriber: %#v\n", p.node)
	sub := newSubject(REQToConsole, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQToConsole.Values, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQnCliCommand(p process) {
	log.Printf("Starting CLICommand Not Sequential Request subscriber: %#v\n", p.node)
	sub := newSubject(REQnCliCommand, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQnCliCommand.Values, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCliCommand(p process) {
	log.Printf("Starting CLICommand Request subscriber: %#v\n", p.node)
	sub := newSubject(REQCliCommand, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQCliCommand.Values, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQPong(p process) {
	log.Printf("Starting Pong subscriber: %#v\n", p.node)
	sub := newSubject(REQPong, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQPong.Values, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQPing(p process) {
	log.Printf("Starting Ping Request subscriber: %#v\n", p.node)
	sub := newSubject(REQPing, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQPing.Values, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQErrorLog(p process) {
	log.Printf("Starting REQErrorLog subscriber: %#v\n", p.node)
	sub := newSubject(REQErrorLog, "errorCentral")
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQErrorLog.Values, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQHello(p process) {
	log.Printf("Starting Hello subscriber: %#v\n", p.node)
	sub := newSubject(REQHello, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQHello.Values, nil)
	proc.procFuncCh = make(chan Message)

	// The reason for running the say hello subscriber as a procFunc is that
	// a handler are not able to hold state, and we need to hold the state
	// of the nodes we've received hello's from in the sayHelloNodes map,
	// which is the information we pass along to generate metrics.
	proc.procFunc = func(ctx context.Context) error {
		sayHelloNodes := make(map[Node]struct{})

		promHelloNodes := promauto.NewGauge(prometheus.GaugeOpts{
			Name: "hello_nodes_total",
			Help: "The current number of total nodes who have said hello",
		})

		promHelloNodesNameVec := promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "hello_node_last_hello",
			Help: "Name of the nodes who have said hello",
		}, []string{"nodeName"},
		)

		for {
			// Receive a copy of the message sent from the method handler.
			var m Message

			select {
			case m = <-proc.procFuncCh:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: %v", proc.subject.name())
				sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				return nil
			}

			// Add an entry for the node in the map
			sayHelloNodes[m.FromNode] = struct{}{}

			// update the prometheus metrics
			promHelloNodes.Set(float64(len(sayHelloNodes)))
			promHelloNodesNameVec.With(prometheus.Labels{"nodeName": string(m.FromNode)}).SetToCurrentTime()

		}
	}
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToFile(p process) {
	log.Printf("Starting text to file subscriber: %#v\n", p.node)
	sub := newSubject(REQToFile, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQToFile.Values, nil)
	// fmt.Printf("*** %#v\n", proc)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToFileAppend(p process) {
	log.Printf("Starting text logging subscriber: %#v\n", p.node)
	sub := newSubject(REQToFileAppend, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQToFileAppend.Values, nil)
	// fmt.Printf("*** %#v\n", proc)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQTailFile(p process) {
	log.Printf("Starting tail log files subscriber: %#v\n", p.node)
	sub := newSubject(REQTailFile, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQTailFile.Values, nil)
	// fmt.Printf("*** %#v\n", proc)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToSocket(p process) {
	log.Printf("Starting write to socket subscriber: %#v\n", p.node)
	sub := newSubject(REQToSocket, string(p.node))
	proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, []Node{"*"}, nil)
	// fmt.Printf("*** %#v\n", proc)
	go proc.spawnWorker(p.processes, p.natsConn)
}
