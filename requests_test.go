package steward

import (
	"bytes"
	"net/http"
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
	//tempdir := "./tmp"
	conf := newConfigurationDefaults()
	conf.BrokerAddress = "127.0.0.1:42222"
	conf.NodeName = "central"
	conf.CentralNodeName = "central"
	conf.ConfigFolder = "tmp"
	conf.SubscribersDataFolder = "tmp"
	conf.SocketFolder = "tmp"
	conf.SubscribersDataFolder = "tmp"
	conf.DatabaseFolder = "tmp"

	stewardServer, err := NewServer(&conf, "test")
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
	type containsOrEquals int
	const contains containsOrEquals = 1
	const equals containsOrEquals = 2

	type test struct {
		info    string
		message Message
		want    []byte
		containsOrEquals
	}

	ns := newNatsServerForTesting(t)
	if err := natsserver.Run(ns); err != nil {
		natsserver.PrintAndDie(err.Error())
	}
	defer ns.Shutdown()

	srv := newServerForTesting(t)
	srv.Start()
	defer srv.Stop()

	// Web server for testing.
	{
		h := func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("web page content"))
		}
		http.HandleFunc("/", h)

		go func() {
			http.ListenAndServe(":10080", nil)
		}()
	}

	tests := []test{
		{
			info: "REQCliCommand test gris",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQCliCommand,
				MethodArgs:    []string{"bash", "-c", "echo gris"},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("gris"),
			containsOrEquals: equals,
		},
		{
			info: "REQCliCommand test sau",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQCliCommand,
				MethodArgs:    []string{"bash", "-c", "echo sau"},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("sau"),
			containsOrEquals: equals,
		},
		{
			info: "REQHttpGet test edgeos.raalabs.tech",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQHttpGet,
				MethodArgs:    []string{"http://localhost:10080"},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("web page content"),
			containsOrEquals: contains,
		},
		{
			info: "REQOpProcessList test",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQOpProcessList,
				MethodArgs:    []string{},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("central.REQHttpGet.EventACK"),
			containsOrEquals: contains,
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

		switch tt.containsOrEquals {

		case equals:
			if !bytes.Equal(result, tt.want) {
				t.Fatalf(" \U0001F631  [FAILED]	:%v : want: %v, got: %v\n", tt.info, string(tt.want), string(result))
			}
			t.Logf(" \U0001f600 [SUCCESS]	: %v\n", tt.info)

		case contains:
			if !strings.Contains(string(result), string(tt.want)) {
				t.Fatalf(" \U0001F631  [FAILED]	:%v : want: %v, got: %v\n", tt.info, string(tt.want), string(result))
			}
			t.Logf(" \U0001f600 [SUCCESS]	: %v\n", tt.info)
		}
	}
}
