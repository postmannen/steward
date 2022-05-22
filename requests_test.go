package steward

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func newServerForTesting(t *testing.T) *server {
	//if !*logging {
	//	log.SetOutput(io.Discard)
	//}

	// Start Steward instance
	// ---------------------------------------
	// tempdir := t.TempDir()

	// Create the config to run a steward instance.
	tempdir := "./tmp"
	conf := &Configuration{
		SocketFolder:   filepath.Join(tempdir, "tmp"),
		DatabaseFolder: filepath.Join(tempdir, "var/lib"),
		//SubscribersDataFolder: filepath.Join(tempdir, "data"),
		SubscribersDataFolder: "./tmp/",
		ConfigFolder:          "./tmp/etc",
		BrokerAddress:         "127.0.0.1:42222",
		PromHostAndPort:       ":2112",
		NodeName:              "central",
		CentralNodeName:       "central",
		DefaultMessageRetries: 1,
		DefaultMessageTimeout: 3,
		EnableSocket:          true,
		// AllowEmptySignature:   true,
		EnableDebug: true,

		StartSubREQCliCommand:     true,
		StartSubREQCliCommandCont: true,
		StartSubREQToConsole:      true,
		StartSubREQToFileAppend:   true,
		StartSubREQToFile:         true,
		StartSubREQHello:          true,
		StartSubREQErrorLog:       true,
		StartSubREQHttpGet:        true,
		StartSubREQTailFile:       true,
		// StartSubREQToSocket:		flagNodeSlice{OK: true, Values: []Node{"*"}},
	}
	stewardServer, err := NewServer(conf, "test")
	if err != nil {
		t.Fatalf(" * failed: could not start the Steward instance %v\n", err)
	}

	return stewardServer
}

// Start up the nats-server message broker for testing purposes.
func newNatsServerForTesting(t *testing.T) *natsserver.Server {
	// Start up the nats-server message broker.
	nsOpt := &natsserver.Options{
		Host: "127.0.0.1",
		Port: 42222,
	}

	ns, err := natsserver.NewServer(nsOpt)
	if err != nil {
		t.Fatalf(" * failed: could not start the nats-server %v\n", err)
	}

	return ns
}

func TestRequest(t *testing.T) {
	type test struct {
		message Message
		want    []byte
	}

	ns := newNatsServerForTesting(t)
	if err := natsserver.Run(ns); err != nil {
		natsserver.PrintAndDie(err.Error())
	}
	defer ns.Shutdown()

	srv := newServerForTesting(t)
	srv.Start()
	defer srv.Stop()

	tests := []test{
		{message: Message{
			ToNode:        "central",
			FromNode:      "central",
			Method:        REQCliCommand,
			MethodArgs:    []string{"bash", "-c", "echo apekatt"},
			MethodTimeout: 5,
			ReplyMethod:   REQTest,
		}, want: []byte("apekatt"),
		},
	}

	for _, tt := range tests {

		sam, err := newSubjectAndMessage(tt.message)
		if err != nil {
			t.Fatalf("newSubjectAndMessage failed: %v\n", err)
		}

		srv.toRingBufferCh <- []subjectAndMessage{sam}

		result := <-srv.errorKernel.testCh
		resStr := string(result)
		resStr = strings.TrimSuffix(resStr, "\n")
		result = []byte(resStr)

		if !bytes.Equal(result, tt.want) {
			t.Fatalf("\n **** want: %v, got: %v\n", tt.want, result)
		}
	}
}
