package steward

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

func newServerForTesting(t *testing.T, addressAndPort string, testFolder string) (*server, *Configuration) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	// Start Steward instance
	// ---------------------------------------
	// tempdir := t.TempDir()

	// Create the config to run a steward instance.
	//tempdir := "./tmp"
	conf := newConfigurationDefaults()
	conf.BrokerAddress = addressAndPort
	conf.NodeName = "central"
	conf.CentralNodeName = "central"
	conf.ConfigFolder = testFolder
	conf.SubscribersDataFolder = testFolder
	conf.SocketFolder = testFolder
	conf.SubscribersDataFolder = testFolder
	conf.DatabaseFolder = testFolder
	conf.StartSubREQErrorLog = true

	stewardServer, err := NewServer(&conf, "test")
	if err != nil {
		t.Fatalf(" * failed: could not start the Steward instance %v\n", err)
	}

	return stewardServer, &conf
}

// Start up the nats-server message broker for testing purposes.
func newNatsServerForTesting(t *testing.T, port int) *natsserver.Server {
	// Start up the nats-server message broker.
	nsOpt := &natsserver.Options{
		Host: "127.0.0.1",
		Port: port,
	}

	ns, err := natsserver.NewServer(nsOpt)
	if err != nil {
		t.Fatalf(" * failed: could not start the nats-server %v\n", err)
	}

	return ns
}

// Write message to socket for testing purposes.
func writeMsgsToSocketTest(conf *Configuration, messages []Message, t *testing.T) {
	js, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("writeMsgsToSocketTest: %v\n ", err)
	}

	socket, err := net.Dial("unix", filepath.Join(conf.SocketFolder, "steward.sock"))
	if err != nil {
		t.Fatalf(" * failed: could to open socket file for writing: %v\n", err)
	}
	defer socket.Close()

	_, err = socket.Write(js)
	if err != nil {
		t.Fatalf(" * failed: could not write to socket: %v\n", err)
	}

}

func TestRequest(t *testing.T) {
	type containsOrEquals int
	const (
		REQTestContains containsOrEquals = iota
		REQTestEquals   containsOrEquals = iota
		fileContains    containsOrEquals = iota
	)

	type viaSocketOrCh int
	const (
		viaSocket viaSocketOrCh = iota
		viaCh     viaSocketOrCh = iota
	)

	type test struct {
		info    string
		message Message
		want    []byte
		containsOrEquals
		viaSocketOrCh
	}

	ns := newNatsServerForTesting(t, 42222)
	if err := natsserver.Run(ns); err != nil {
		natsserver.PrintAndDie(err.Error())
	}
	defer ns.Shutdown()

	// tempdir := t.TempDir()
	tempdir := "tmp"
	srv, conf := newServerForTesting(t, "127.0.0.1:42222", tempdir)
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
			info: "REQHello test",
			message: Message{
				ToNode:        "errorCentral",
				FromNode:      "errorCentral",
				Method:        REQErrorLog,
				MethodArgs:    []string{},
				MethodTimeout: 5,
				Data:          []byte("error data"),
				// ReplyMethod:   REQTest,
				Directory: "error_log",
				FileName:  "error.results",
			}, want: []byte("error data"),
			containsOrEquals: fileContains,
			viaSocketOrCh:    viaCh,
		},
		{
			info: "REQHello test",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQHello,
				MethodArgs:    []string{},
				MethodTimeout: 5,
				// ReplyMethod:   REQTest,
				Directory: "test",
				FileName:  "hello.results",
			}, want: []byte("Received hello from \"central\""),
			containsOrEquals: fileContains,
			viaSocketOrCh:    viaCh,
		},
		{
			info: "REQCliCommand test, echo gris",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQCliCommand,
				MethodArgs:    []string{"bash", "-c", "echo gris"},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("gris"),
			containsOrEquals: REQTestEquals,
			viaSocketOrCh:    viaCh,
		},
		{
			info: "REQCliCommand test via socket, echo sau",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQCliCommand,
				MethodArgs:    []string{"bash", "-c", "echo sau"},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("sau"),
			containsOrEquals: REQTestEquals,
			viaSocketOrCh:    viaSocket,
		},
		{
			info: "REQCliCommand test, echo sau, result in file",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQCliCommand,
				MethodArgs:    []string{"bash", "-c", "echo sau"},
				MethodTimeout: 5,
				ReplyMethod:   REQToFile,
				Directory:     "test",
				FileName:      "file1.result",
			}, want: []byte("sau"),
			containsOrEquals: fileContains,
			viaSocketOrCh:    viaCh,
		},
		{
			info: "REQCliCommand test, echo several, result in file continous",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQCliCommand,
				MethodArgs:    []string{"bash", "-c", "echo giraff && echo sau && echo apekatt"},
				MethodTimeout: 5,
				ReplyMethod:   REQToFile,
				Directory:     "test",
				FileName:      "file2.result",
			}, want: []byte("sau"),
			containsOrEquals: fileContains,
			viaSocketOrCh:    viaCh,
		},
		{
			info: "REQHttpGet test, localhost:10080",
			message: Message{
				ToNode:        "central",
				FromNode:      "central",
				Method:        REQHttpGet,
				MethodArgs:    []string{"http://localhost:10080"},
				MethodTimeout: 5,
				ReplyMethod:   REQTest,
			}, want: []byte("web page content"),
			containsOrEquals: REQTestContains,
			viaSocketOrCh:    viaCh,
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
			containsOrEquals: REQTestContains,
			viaSocketOrCh:    viaCh,
		},
	}

	// Range over the tests defined, and execute them, one at a time.
	for _, tt := range tests {
		switch tt.viaSocketOrCh {
		case viaCh:
			sam, err := newSubjectAndMessage(tt.message)
			if err != nil {
				t.Fatalf("newSubjectAndMessage failed: %v\n", err)
			}

			srv.toRingBufferCh <- []subjectAndMessage{sam}

		case viaSocket:
			msgs := []Message{tt.message}
			writeMsgsToSocketTest(conf, msgs, t)

		}

		switch tt.containsOrEquals {
		case REQTestEquals:
			result := <-srv.errorKernel.testCh
			resStr := string(result)
			resStr = strings.TrimSuffix(resStr, "\n")
			result = []byte(resStr)

			if !bytes.Equal(result, tt.want) {
				t.Fatalf(" \U0001F631  [FAILED]	:%v : want: %v, got: %v\n", tt.info, string(tt.want), string(result))
			}
			t.Logf(" \U0001f600 [SUCCESS]	: %v\n", tt.info)

		case REQTestContains:
			result := <-srv.errorKernel.testCh
			resStr := string(result)
			resStr = strings.TrimSuffix(resStr, "\n")
			result = []byte(resStr)

			if !strings.Contains(string(result), string(tt.want)) {
				t.Fatalf(" \U0001F631  [FAILED]	:%v : want: %v, got: %v\n", tt.info, string(tt.want), string(result))
			}
			t.Logf(" \U0001f600 [SUCCESS]	: %v\n", tt.info)

		case fileContains:
			resultFile := filepath.Join(conf.SubscribersDataFolder, tt.message.Directory, string(tt.message.FromNode), tt.message.FileName)

			found, err := findStringInFileTest(string(tt.want), resultFile, conf, t)
			if err != nil || found == false {
				t.Fatalf(" \U0001F631  [FAILED]	: %v: %v\n", tt.info, err)

			}

			t.Logf(" \U0001f600 [SUCCESS]	: %v\n", tt.info)
		}
	}
}
