package steward

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func (p process) ProcessesStart() {

	// --- Subscriber services that can be started via flags

	{
		fmt.Printf("Starting REQOpCommand subscriber: %#v\n", p.node)
		sub := newSubject(REQOpCommand, string(p.node))
		proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, []node{"*"}, nil)
		go proc.spawnWorker(p.processes, p.natsConn)
	}

	// Start a subscriber for textLogging messages
	if p.configuration.StartSubREQTextToLogFile.OK {
		{
			fmt.Printf("Starting text logging subscriber: %#v\n", p.node)
			sub := newSubject(REQTextToLogFile, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQTextToLogFile.Values, nil)
			// fmt.Printf("*** %#v\n", proc)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for text to file messages
	if p.configuration.StartSubREQTextToFile.OK {
		{
			fmt.Printf("Starting text to file subscriber: %#v\n", p.node)
			sub := newSubject(REQTextToFile, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQTextToFile.Values, nil)
			// fmt.Printf("*** %#v\n", proc)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for Hello messages
	if p.configuration.StartSubREQHello.OK {
		{
			fmt.Printf("Starting Hello subscriber: %#v\n", p.node)
			sub := newSubject(REQHello, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQHello.Values, nil)
			proc.procFuncCh = make(chan Message)

			// The reason for running the say hello subscriber as a procFunc is that
			// a handler are not able to hold state, and we need to hold the state
			// of the nodes we've received hello's from in the sayHelloNodes map,
			// which is the information we pass along to generate metrics.
			proc.procFunc = func(ctx context.Context) error {
				sayHelloNodes := make(map[node]struct{})
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

					fmt.Printf("--- DEBUG : procFunc call:kind=%v, Subject=%v, toNode=%v\n", proc.processKind, proc.subject, proc.subject.ToNode)

					sayHelloNodes[m.FromNode] = struct{}{}

					// update the prometheus metrics
					proc.processes.metricsCh <- metricType{
						metric: prometheus.NewGauge(prometheus.GaugeOpts{
							Name: "hello_nodes",
							Help: "The current number of total nodes who have said hello",
						}),
						value: float64(len(sayHelloNodes)),
					}
				}
			}
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	if p.configuration.StartSubREQErrorLog.OK {
		// Start a subscriber for REQErrorLog messages
		{
			fmt.Printf("Starting REQErrorLog subscriber: %#v\n", p.node)
			sub := newSubject(REQErrorLog, "errorCentral")
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQErrorLog.Values, nil)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for Ping Request messages
	if p.configuration.StartSubREQPing.OK {
		{
			fmt.Printf("Starting Ping Request subscriber: %#v\n", p.node)
			sub := newSubject(REQPing, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQPing.Values, nil)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for REQPong messages
	if p.configuration.StartSubREQPong.OK {
		{
			fmt.Printf("Starting Pong subscriber: %#v\n", p.node)
			sub := newSubject(REQPong, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQPong.Values, nil)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for REQCliCommand messages
	if p.configuration.StartSubREQCliCommand.OK {
		{
			fmt.Printf("Starting CLICommand Request subscriber: %#v\n", p.node)
			sub := newSubject(REQCliCommand, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQCliCommand.Values, nil)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for Not In Order Cli Command Request messages
	if p.configuration.StartSubREQnCliCommand.OK {
		{
			fmt.Printf("Starting CLICommand Not Sequential Request subscriber: %#v\n", p.node)
			sub := newSubject(REQnCliCommand, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQnCliCommand.Values, nil)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// Start a subscriber for CLICommandReply messages
	if p.configuration.StartSubREQTextToConsole.OK {
		{
			fmt.Printf("Starting Text To Console subscriber: %#v\n", p.node)
			sub := newSubject(REQTextToConsole, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQTextToConsole.Values, nil)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}

	// --- Publisher services that can be started via flags

	// --------- Testing with publisher ------------
	// Define a process of kind publisher with subject for SayHello to central,
	// and register a procFunc with the process that will handle the actual
	// sending of say hello.
	if p.configuration.StartPubREQHello != 0 {
		fmt.Printf("Starting Hello Publisher: %#v\n", p.node)

		sub := newSubject(REQHello, p.configuration.CentralNodeName)
		proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindPublisher, []node{}, nil)

		// Define the procFunc to be used for the process.
		proc.procFunc = procFunc(
			func(ctx context.Context) error {
				ticker := time.NewTicker(time.Second * time.Duration(p.configuration.StartPubREQHello))
				for {
					fmt.Printf("--- DEBUG : procFunc call:kind=%v, Subject=%v, toNode=%v\n", proc.processKind, proc.subject, proc.subject.ToNode)

					d := fmt.Sprintf("Hello from %v\n", p.node)

					m := Message{
						ToNode:   "central",
						FromNode: node(p.node),
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
						fmt.Printf(" ** DEBUG: got <- ctx.Done\n")
						er := fmt.Errorf("info: stopped handleFunc for: %v", proc.subject.name())
						sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
						return nil
					}
				}
			})
		go proc.spawnWorker(p.processes, p.natsConn)
	}

	// Start a subscriber for Http Get Requests
	if p.configuration.StartSubREQHttpGet.OK {
		{
			fmt.Printf("Starting Http Get subscriber: %#v\n", p.node)
			sub := newSubject(REQHttpGet, string(p.node))
			proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQHttpGet.Values, nil)
			// fmt.Printf("*** %#v\n", proc)
			go proc.spawnWorker(p.processes, p.natsConn)
		}
	}
}

type startups struct{}

func (s startups) REQHttpGet(p process) {
	{
		fmt.Printf("Starting Http Get subscriber: %#v\n", p.node)
		sub := newSubject(REQHttpGet, string(p.node))
		proc := newProcess(p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, p.configuration.StartSubREQHttpGet.Values, nil)
		// fmt.Printf("*** %#v\n", proc)
		go proc.spawnWorker(p.processes, p.natsConn)
	}
}
