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
	err := c.CheckFlags()
	if err != nil {
		log.Printf("%v\n", err)
		return
	}

	// Start profiling if profiling port is specified
	if c.ProfilingPort != "" {
		go func() {
			http.ListenAndServe("localhost:"+c.ProfilingPort, nil)
		}()

	}

	s, err := steward.NewServer(c)
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	go s.Start()

	select {}
}
