package main

import (
	"log"
	"net/http"
	"os"

	_ "net/http/pprof"

	"github.com/RaaLabs/steward"
)

func main() {
	c := steward.NewConfiguration()
	c.CheckFlags()

	// Start profiling if profiling port is specified
	if c.ProfilingPort != "" {
		go func() {
			http.ListenAndServe("localhost:"+c.ProfilingPort, nil)
		}()

	}

	s, err := steward.NewServer(c)
	if err != nil {
		log.Printf("error: failed to connect to broker: %v\n", err)
		os.Exit(1)
	}

	// TODO: Add a context
	// Start the messaging server
	go s.Start()

	select {}
}
