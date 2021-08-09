package steward

import (
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func TestSteward(t *testing.T) {
	// Start up the nats-server message broker.
	nsOpt := &natsserver.Options{
		Host: "127.0.0.1",
		Port: 40222,
	}

	ns, err := natsserver.NewServer(nsOpt)
	if err != nil {
		t.Fatalf("error: failed to start nats-server %v\n", err)
	}

	if err := natsserver.Run(ns); err != nil {
		natsserver.PrintAndDie(err.Error())
	}

	// Start Steward instance for testing
	tempdir := t.TempDir()

	conf := &Configuration{
		SocketFolder:          filepath.Join(tempdir, "tmp"),
		DatabaseFolder:        filepath.Join(tempdir, "var/lib"),
		SubscribersDataFolder: filepath.Join(tempdir, "data"),
		BrokerAddress:         "127.0.0.1:40222",
		NodeName:              "central",
		CentralNodeName:       "central",
		DefaultMessageRetries: 1,
		DefaultMessageTimeout: 3,
	}
	s, err := NewServer(conf)
	if err != nil {
		t.Fatalf("error: failed to start nats-server %v\n", err)
	}

	s.Start()

	s.Stop()

	// Shutdown services
	ns.Shutdown()

	time.Sleep(time.Second * 5)

}
