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
	// modePublisher := flag.Bool("modePublisher", false, "set to true if it should be able to publish")
	// modeSubscriber := flag.Bool("modeSubscriber", false, "set to true if it should be able to subscribe")
	profilingPort := flag.String("profilingPort", "", "The number of the profiling port")
	flag.Parse()

	if *profilingPort != "" {
		// TODO REMOVE: Added for profiling

		go func() {
			http.ListenAndServe("localhost:"+*profilingPort, nil)
		}()

	}

	s, err := steward.NewServer(*brokerAddress, *nodeName)
	if err != nil {
		log.Printf("error: failed to connect to broker: %v\n", err)
		os.Exit(1)
	}

	// Start the messaging server
	go s.Start()

	//if *modeSubscriber {
	//	go s.RunSubscriber()
	//}

	select {}
}
