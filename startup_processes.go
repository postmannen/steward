package steward

import (
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func (s *server) ProcessesStart() {
	// Start a subscriber for CLICommand messages
	if s.configuration.StartSubCLICommand.OK {
		{
			fmt.Printf("Starting CLICommand subscriber: %#v\n", s.nodeName)
			sub := newSubject(CLICommand, CommandACK, s.nodeName)
			proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, s.configuration.StartSubCLICommand.Values, nil)
			// fmt.Printf("*** %#v\n", proc)
			go proc.spawnWorker(s)
		}
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
	if s.configuration.StartSubSayHello.OK {
		{
			fmt.Printf("Starting SayHello subscriber: %#v\n", s.nodeName)
			sub := newSubject(SayHello, EventNACK, s.nodeName)
			proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, s.configuration.StartSubSayHello.Values, nil)
			proc.procFuncCh = make(chan Message)

			proc.procFunc = func() error {
				sayHelloNodes := make(map[node]struct{})
				for {
					// Receive a copy of the message sent from the method handler.
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
	}

	if s.configuration.StartSubErrorLog.OK {
		// Start a subscriber for ErrorLog messages
		{
			fmt.Printf("Starting ErrorLog subscriber: %#v\n", s.nodeName)
			sub := newSubject(ErrorLog, EventNACK, "errorCentral")
			proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, s.configuration.StartSubErrorLog.Values, nil)
			go proc.spawnWorker(s)
		}
	}

	// --------- Testing with publisher ------------
	// Define a process of kind publisher with subject for SayHello to central,
	// and register a procFunc with the process that will handle the actual
	// sending of say hello.
	if s.configuration.PublisherServiceSayhello != 0 {
		fmt.Printf("Starting SayHello Publisher: %#v\n", s.nodeName)

		sub := newSubject(SayHello, EventNACK, s.configuration.CentralNodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindPublisher, []node{}, nil)

		// Define the procFun to be used for the process.
		proc.procFunc = procFunc(
			func() error {
				for {
					fmt.Printf("--- DEBUG : procFunc call:kind=%v, Subject=%v, toNode=%v\n", proc.processKind, proc.subject, proc.subject.ToNode)

					d := fmt.Sprintf("Hello from %v\n", s.nodeName)

					m := Message{
						ToNode:   "central",
						FromNode: node(s.nodeName),
						Data:     []string{d},
						Method:   SayHello,
					}

					sam, err := newSAM(m)
					if err != nil {
						// In theory the system should drop the message before it reaches here.
						log.Printf("error: ProcessesStart: %v\n", err)
					}
					proc.newMessagesCh <- []subjectAndMessage{sam}
					time.Sleep(time.Second * time.Duration(s.configuration.PublisherServiceSayhello))
				}
			})
		go proc.spawnWorker(s)
	}

	// Start a subscriber for ECHORequest messages
	{
		fmt.Printf("Starting Echo Request subscriber: %#v\n", s.nodeName)
		sub := newSubject(ECHORequest, EventACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for ECHOReply messages
	{
		fmt.Printf("Starting Echo Reply subscriber: %#v\n", s.nodeName)
		sub := newSubject(ECHOReply, EventACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for CLICommandRequest messages
	{
		fmt.Printf("Starting CLICommand Request subscriber: %#v\n", s.nodeName)
		sub := newSubject(CLICommandRequest, EventACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for CLICommandRequest messages
	{
		fmt.Printf("Starting CLICommand NOSEQ Request subscriber: %#v\n", s.nodeName)
		sub := newSubject(CLICommandRequestNOSEQ, EventACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for CLICommandReply messages
	{
		fmt.Printf("Starting CLICommand Reply subscriber: %#v\n", s.nodeName)
		sub := newSubject(CLICommandReply, EventACK, s.nodeName)
		proc := newProcess(s.processes, s.newMessagesCh, s.configuration, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"}, nil)
		go proc.spawnWorker(s)
	}
}
