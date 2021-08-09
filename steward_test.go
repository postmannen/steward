package steward

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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

	// Start Steward instance
	// ---------------------------------------
	tempdir := "./tmp"

	conf := &Configuration{
		SocketFolder:            filepath.Join(tempdir, "tmp"),
		DatabaseFolder:          filepath.Join(tempdir, "var/lib"),
		SubscribersDataFolder:   filepath.Join(tempdir, "data"),
		BrokerAddress:           "127.0.0.1:40222",
		NodeName:                "central",
		CentralNodeName:         "central",
		DefaultMessageRetries:   1,
		DefaultMessageTimeout:   3,
		StartSubREQnCliCommand:  flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQErrorLog:     flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQToFileAppend: flagNodeSlice{OK: true, Values: []Node{"*"}},
	}
	s, err := NewServer(conf)
	if err != nil {
		t.Fatalf("error: failed to start nats-server %v\n", err)
	}

	s.Start()

	// Messaging tests.
	//
	// Write to socket.
	// ---------------------------------------
	want := "apekatt"

	socket, err := net.Dial("unix", filepath.Join(conf.SocketFolder, "steward.sock"))
	if err != nil {
		t.Fatalf("error: failed to open socket file for writing: %v\n", err)
	}
	defer socket.Close()

	m := `[
		{
			"directory":"commands-executed",
			"fileExtension":".result",
			"toNode": "central",
			"data": ["bash","-c","echo apekatt"],
			"replyMethod":"REQToFileAppend",
			"method":"REQnCliCommand",
			"ACKTimeout":3,
			"retries":3,
			"methodTimeout": 10
		}
	]`

	_, err = socket.Write([]byte(m))
	if err != nil {
		t.Fatalf("error: failed to write to socket: %v\n", err)
	}

	// Wait a couple of seconds for the request to go through..
	time.Sleep(time.Second * 2)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "commands-executed", "central", "central.REQnCliCommand.result")
	fh, err := os.Open(resultFile)
	if err != nil {
		t.Fatalf("error: failed open result file: %v\n", err)
	}

	result, err := io.ReadAll(fh)
	if err != nil {
		t.Fatalf("error: failed read result file: %v\n", err)
	}

	if strings.Contains(string(result), want) {
		t.Fatalf("error: did not find expexted word `%v` in file ", want)
	}

	// ---------------------------------------

	s.Stop()

	// Shutdown services.
	ns.Shutdown()
}
