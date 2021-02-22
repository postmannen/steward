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
	//isCentral := flag.Bool("isCentral", false, "used to indicate that this is the central master that will subscribe to error message subjects")
	flag.Parse()

	if *profilingPort != "" {
		// TODO REMOVE: Added for profiling

		go func() {
			http.ListenAndServe("localhost:"+*profilingPort, nil)
		}()

	}

	s, err := steward.NewServer(*brokerAddress, *nodeName, *promHostAndPort)
	if err != nil {
		log.Printf("error: failed to connect to broker: %v\n", err)
		os.Exit(1)
	}

	// Start the messaging server
	go s.Start()

	select {}
}
