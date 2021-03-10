package steward

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func (s *server) ProcessesStart() {
	// Start a subscriber for CLICommand messages
	{
		fmt.Printf("Starting CLICommand subscriber: %#v\n", s.nodeName)
		sub := newSubject(CLICommand, CommandACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"central", "ship2"}, nil)
		// fmt.Printf("*** %#v\n", proc)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for textLogging messages
	{
		fmt.Printf("Starting textlogging subscriber: %#v\n", s.nodeName)
		sub := newSubject(TextLogging, EventACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		// fmt.Printf("*** %#v\n", proc)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for SayHello messages
	{
		fmt.Printf("Starting SayHello subscriber: %#v\n", s.nodeName)
		sub := newSubject(SayHello, EventNACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		proc.procFuncCh = make(chan Message)

		proc.procFunc = func() error {
			sayHelloNodes := make(map[node]struct{})
			for {
				//fmt.Printf("-- DEBUG 4.1: procFunc %v, procFuncCh %v\n\n", proc.procFunc, proc.procFuncCh)
				m := <-proc.procFuncCh
				fmt.Printf("--- DEBUG : procFunc call:kind=%v, Subject=%v, toNode=%v\n", proc.processKind, proc.subject, proc.subject.ToNode)
				sayHelloNodes[m.FromNode] = struct{}{}

				// update the prometheus metrics
				s.metrics.metricsCh <- metricType{
					metric: prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "hello_nodes",
						Help: "The current number of total nodes who have said hello",
					}),
					value: float64(len(sayHelloNodes)),
				}
			}
		}
		go proc.spawnWorker(s)
	}

	if s.centralErrorLogger {
		// Start a subscriber for ErrorLog messages
		{
			fmt.Printf("Starting ErrorLog subscriber: %#v\n", s.nodeName)
			sub := newSubject(ErrorLog, EventNACK, "errorCentral")
			proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
			go proc.spawnWorker(s)
		}
	}

	// --------- Testing with publisher ------------
	// Define a process of kind publisher with subject for SayHello to central,
	// and register a procFunc with the process that will handle the actual
	// sending of say hello.
	if s.configuration.PublisherServiceSayhello != 0 {
		fmt.Printf("Starting SayHello Publisher: %#v\n", s.nodeName)
		// TODO: Replace "central" name with variable below.
		sub := newSubject(SayHello, EventNACK, "central")
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindPublisher, []node{}, nil)

		proc.procFunc = func() error {
			for {
				fmt.Printf("--- DEBUG : procFunc call:kind=%v, Subject=%v, toNode=%v\n", proc.processKind, proc.subject, proc.subject.ToNode)

				d := fmt.Sprintf("Hello from %v\n", s.nodeName)

				m := Message{
					ToNode:   "central",
					FromNode: node(s.nodeName),
					Data:     []string{d},
					Method:   SayHello,
				}

				sam := createSAMfromMessage(m)
				proc.newMessagesCh <- []subjectAndMessage{sam}
				time.Sleep(time.Second * time.Duration(s.configuration.PublisherServiceSayhello))
			}
		}
		go proc.spawnWorker(s)
	}
}