package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	_ "net/http/pprof"

	"github.com/RaaLabs/steward"
)

// Use ldflags to set version
// env CONFIGFOLDER=./etc/ go run -ldflags "-X main.version=v0.1.10" --race ./cmd/steward/.
// or
// env GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=v0.1.10" -o steward
var version string

func main() {
	// defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()
	// defer profile.Start(profile.TraceProfile, profile.ProfilePath(".")).Stop()
	// defer profile.Start(profile.MemProfile, profile.MemProfileRate(1)).Stop()

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

	s, err := steward.NewServer(c, version)
	if err != nil {
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	// Start up the server
	go s.Start()

	// Wait for ctrl+c to stop the server.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	// Block and wait for CTRL+C
	sig := <-sigCh
	log.Printf("Got exit signal, terminating all processes, %v\n", sig)

	// Adding a safety function here so we can make sure that all processes
	// are stopped after a given time if the context cancelation hangs.
	go func() {
		time.Sleep(time.Second * 10)
		log.Printf("error: doing a non graceful shutdown of all processes..\n")
		os.Exit(1)
	}()

	// Stop all processes.
	s.Stop()
}
