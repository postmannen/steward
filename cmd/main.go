package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	_ "net/http/pprof"

	"github.com/RaaLabs/steward"
)

func main() {
	nodeName := flag.String("node", "0", "some unique string to identify this Edge unit")
	brokerAddress := flag.String("brokerAddress", "0", "the address of the message broker")
	profilingPort := flag.String("profilingPort", "", "The number of the profiling port")
	promHostAndPort := flag.String("promHostAndPort", ":2112", "host and port for prometheus listener, e.g. localhost:2112")
	centralErrorLogger := flag.Bool("centralErrorLogger", false, "set to true if this is the node that should receive the error log's from other nodes")
	defaultMessageTimeout := flag.Int("defaultMessageTimeout", 10, "default message timeout in seconds. This can be overridden on the message level")
	defaultMessageRetries := flag.Int("defaultMessageRetries", 0, "default amount of retries that will be done before a message is thrown away, and out of the system")
	publisherServiceSayhello := flag.Int("publisherServiceSayhello", 0, "Make the current node send hello messages to central at given interval in seconds")
	flag.Parse()

	// Start profiling if profiling port is specified
	if *profilingPort != "" {
		go func() {
			http.ListenAndServe("localhost:"+*profilingPort, nil)
		}()

	}

	s, err := steward.NewServer(*brokerAddress, *nodeName, *promHostAndPort, *centralErrorLogger, *defaultMessageTimeout, *defaultMessageRetries, *publisherServiceSayhello)
	if err != nil {
		log.Printf("error: failed to connect to broker: %v\n", err)
		os.Exit(1)
	}

	// Start the messaging server
	go s.Start()

	select {}
}
