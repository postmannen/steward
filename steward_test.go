// Package functionality tests for Steward.
//
// To turn on the normal logging functionality during tests, use:
// go test -logging=true
package steward

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
)

// Set the default logging functionality of the package to false.
var logging = flag.Bool("logging", false, "set to true to enable the normal logger of the package")

// Test the overall functionality of Steward.
// Starting up the server.
// Message passing.
// The different REQ types.
func TestStewardServer(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	// Start the nats-server message broker.
	startNatsServerTest(t)

	// Start Steward instance
	// ---------------------------------------
	// tempdir := t.TempDir()
	tempdir := "./tmp"
	conf := &Configuration{
		SocketFolder:          filepath.Join(tempdir, "tmp"),
		DatabaseFolder:        filepath.Join(tempdir, "var/lib"),
		SubscribersDataFolder: filepath.Join(tempdir, "data"),
		BrokerAddress:         "127.0.0.1:40222",
		NodeName:              "central",
		CentralNodeName:       "central",
		DefaultMessageRetries: 1,
		DefaultMessageTimeout: 3,

		StartSubREQCliCommand:      flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQnCliCommand:     flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQnCliCommandCont: flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQToConsole:       flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQToFileAppend:    flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQToFile:          flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQHello:           flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQErrorLog:        flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQHttpGet:         flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQTailFile:        flagNodeSlice{OK: true, Values: []Node{"*"}},
		// StartSubREQToSocket:		flagNodeSlice{OK: true, Values: []Node{"*"}},
	}
	s, err := NewServer(conf)
	if err != nil {
		t.Fatalf(" * failed: could not start the Steward instance %v\n", err)
	}
	s.Start()

	// Run the message tests
	//
	// ---------------------------------------

	checkREQOpCommandTest(conf, t)
	checkREQCliCommandTest(conf, t)
	checkREQnCliCommandTest(conf, t)
	checkREQnCliCommandContTest(conf, t)
	// checkREQToConsoleTest(conf, t), NB: No tests will be made for console ouput.
	// checkREQToFileAppendTest(conf, t), NB: Already tested via others
	// checkREQToFileTest(conf, t), NB: Already tested via others
	checkREQHelloTest(conf, t)
	checkREQErrorLogTest(conf, t)
	// checkREQPingTest(conf, t)
	// checkREQPongTest(conf, t)
	checkREQHttpGetTest(conf, t)
	checkREQTailFileTest(conf, t)
	// checkREQToSocketTest(conf, t)

	checkErrorKernelJSONtest(conf, t)

	// ---------------------------------------

	s.Stop()

}

// ----------------------------------------------------------------------------
// Check REQ types
// ----------------------------------------------------------------------------

// Testing op (operator) Commands.
func checkREQOpCommandTest(conf *Configuration, t *testing.T) {
	m := `[
		{
			"directory":"commands-executed",
			"fileExtension": ".result",
			"toNode": "central",
			"data": [],
			"method":"REQOpCommand",
			"operation":{
				"opCmd":"ps"
			},
			"replyMethod":"REQToFile",
			"ACKTimeout":3,
			"retries":3,
			"replyACKTimeout":3,
			"replyRetries":3,
			"MethodTimeout": 7
		}
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "commands-executed", "central", "central.REQOpCommand.result")
	findStringInFileTest("central.REQOpCommand.CommandACK", resultFile, conf, t)

}

// Sending of CLI Commands.
func checkREQCliCommandTest(conf *Configuration, t *testing.T) {
	m := `[
		{
			"directory":"commands-executed",
			"fileExtension":".result",
			"toNode": "central",
			"data": ["bash","-c","echo apekatt"],
			"replyMethod":"REQToFileAppend",
			"method":"REQCliCommand",
			"ACKTimeout":3,
			"retries":3,
			"methodTimeout": 10
		}
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "commands-executed", "central", "central.REQCliCommand.result")
	findStringInFileTest("apekatt", resultFile, conf, t)

}

// The non-sequential sending of CLI Commands.
func checkREQnCliCommandTest(conf *Configuration, t *testing.T) {
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

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "commands-executed", "central", "central.REQnCliCommand.result")
	findStringInFileTest("apekatt", resultFile, conf, t)

}

// The non-sequential sending of CLI Commands.
func checkREQnCliCommandContTest(conf *Configuration, t *testing.T) {
	m := `[
		{
			"directory":"commands-executed",
			"fileExtension":".result",
			"toNode": "central",
			"data": ["bash","-c","echo apekatt && sleep 5 && echo gris"],
			"replyMethod":"REQToFileAppend",
			"method":"REQnCliCommandCont",
			"ACKTimeout":3,
			"retries":3,
			"methodTimeout": 5
		}
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "commands-executed", "central", "central.REQnCliCommandCont.result")
	findStringInFileTest("apekatt", resultFile, conf, t)

}

// Sending of Hello.
func checkREQHelloTest(conf *Configuration, t *testing.T) {
	m := `[
		{
			"directory":"commands-executed",
			"fileExtension":".result",
			"toNode": "central",
			"data": [],
			"method":"REQHello"
		}
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "commands-executed", "central", "central.REQHello.result")
	findStringInFileTest("Received hello from", resultFile, conf, t)

}

func checkREQErrorLogTest(conf *Configuration, t *testing.T) {
	m := `[
		{
			"directory": "errorLog",
			"fileExtension":".result",
			"toNode": "errorCentral",
			"data": ["some error"],
			"method": "REQErrorLog"
		}
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "errorLog", "central", "errorCentral.REQErrorLog.result")
	findStringInFileTest("some error", resultFile, conf, t)

}

func checkREQHttpGetTest(conf *Configuration, t *testing.T) {
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

	m := `[
		{
			"directory": "httpget",
			"fileExtension":".result",
			"toNode": "central",
			"data": ["http://127.0.0.1:10080/"],
			"method": "REQHttpGet",
			"replyMethod":"REQToFile",
        	"ACKTimeout":5,
        	"methodTimeout": 5
		}
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "httpget", "central", "central.REQHttpGet.result")
	findStringInFileTest("web page content", resultFile, conf, t)

}

// ---

func checkREQTailFileTest(conf *Configuration, t *testing.T) {
	// // Create a file with some content.
	// fh, err := os.Create("test.file")
	// if err != nil {
	// 	t.Fatalf(" * failed: unable to open temporary file: %v\n", err)
	// }
	// defer fh.Close()
	//
	// for i := 1; i <= 10; i++ {
	// 	_, err = fh.Write([]byte("some file content"))
	// 	if err != nil {
	// 		t.Fatalf(" * failed: writing to temporary file: %v\n", err)
	// 	}
	// 	time.Sleep(time.Millisecond * 500)
	// }
	//
	// wd, err := os.Getwd()
	// if err != nil {
	// 	t.Fatalf(" * failed: getting current working directory: %v\n", err)
	// }
	//
	// file := filepath.Join(wd, "test.file")

	s := Message{
		Directory:     "tail-files",
		FileExtension: ".result",
		ToNode:        "central",
		Data:          []string{"/var/log/system.log"},
		Method:        REQTailFile,
		ACKTimeout:    3,
		Retries:       2,
		MethodTimeout: 10,
	}

	sJSON, err := json.Marshal(s)
	if err != nil {
		t.Fatalf(" * failed: json marshaling of message: %v: %v\n", sJSON, err)
	}

	writeToSocketTest(conf, string(sJSON), t)

	time.Sleep(time.Second * 5)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "tail-files", "central", "central.REQTailFile.result")
	findStringInFileTest("some file content", resultFile, conf, t)

}

// ------- Functionality tests.

// Check errorKernel
func checkErrorKernelJSONtest(conf *Configuration, t *testing.T) {
	m := `[
		{
			"directory": "some dir",
			"fileExtension":"someext",
			"toNode": "somenode",
			"data": ["some data"],
			"method": "REQErrorLog"
		missing brace here.....
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "errorLog", "central", "errorCentral.REQErrorLog.log")
	findStringInFileTest("error: malformed json", resultFile, conf, t)

}

// ----------------------------------------------------------------------------
// Check functionality
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Helper functions for tests
// ----------------------------------------------------------------------------

// Check if a file contains the given string.
func findStringInFileTest(want string, fileName string, conf *Configuration, t *testing.T) {
	// Wait n seconds for the results file to be created
	n := 5

	for i := 0; i <= n; i++ {
		_, err := os.Stat(fileName)
		if os.IsNotExist(err) {
			time.Sleep(time.Millisecond * 500)
			continue
		}

		if os.IsNotExist(err) && i >= n {
			t.Fatalf(" * failed: no result file created for request within the given time\n")
		}
	}

	fh, err := os.Open(fileName)
	if err != nil {
		t.Fatalf(" * failed: could not open result file: %v\n", err)
	}

	result, err := io.ReadAll(fh)
	if err != nil {
		t.Fatalf(" * failed: could not read result file: %v\n", err)
	}

	if !strings.Contains(string(result), want) {
		t.Fatalf(" * failed: did not find expexted word `%v` in result file ", want)
	}
}

// Write message to socket for testing purposes.
func writeToSocketTest(conf *Configuration, messageText string, t *testing.T) {
	socket, err := net.Dial("unix", filepath.Join(conf.SocketFolder, "steward.sock"))
	if err != nil {
		t.Fatalf(" * failed: could to open socket file for writing: %v\n", err)
	}
	defer socket.Close()

	_, err = socket.Write([]byte(messageText))
	if err != nil {
		t.Fatalf(" * failed: could not write to socket: %v\n", err)
	}

}

// Start up the nats-server message broker.
func startNatsServerTest(t *testing.T) {
	// Start up the nats-server message broker.
	nsOpt := &natsserver.Options{
		Host: "127.0.0.1",
		Port: 40222,
	}

	ns, err := natsserver.NewServer(nsOpt)
	if err != nil {
		t.Fatalf(" * failed: could not start the nats-server %v\n", err)
	}

	if err := natsserver.Run(ns); err != nil {
		natsserver.PrintAndDie(err.Error())
	}
}
