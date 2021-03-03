package steward

import (
	"fmt"
)

func (s *server) subscribersStart() {
	// Start a subscriber for CLICommand messages
	{
		fmt.Printf("Starting CLICommand subscriber: %#v\n", s.nodeName)
		sub := newSubject(CLICommand, CommandACK, s.nodeName)
		proc := newProcess(s, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"central", "ship2"})
		// fmt.Printf("*** %#v\n", proc)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for textLogging messages
	{
		fmt.Printf("Starting textlogging subscriber: %#v\n", s.nodeName)
		sub := newSubject(TextLogging, EventACK, s.nodeName)
		proc := newProcess(s, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"})
		// fmt.Printf("*** %#v\n", proc)
		go proc.spawnWorker(s)
	}

	// Start a subscriber for SayHello messages
	{
		fmt.Printf("Starting SayHello subscriber: %#v\n", s.nodeName)
		sub := newSubject(SayHello, EventNACK, s.nodeName)
		proc := newProcess(s, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"})
		// fmt.Printf("*** %#v\n", proc)
		go proc.spawnWorker(s)
	}

	if s.centralErrorLogger {
		// Start a subscriber for ErrorLog messages
		{
			fmt.Printf("Starting ErrorLog subscriber: %#v\n", s.nodeName)
			sub := newSubject(ErrorLog, EventNACK, "errorCentral")
			proc := newProcess(s, sub, s.errorKernel.errorCh, processKindSubscriber, []node{"*"})
			// fmt.Printf("*** %#v\n", proc)
			go proc.spawnWorker(s)
		}
	}
}
