// Package functionality tests for Steward.
//
// To turn on the normal logging functionality during tests, use:
// go test -logging=true
package steward

import (
	"flag"
	"io"
	"log"
	"net"
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

		StartSubREQCliCommand:  flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQnCliCommand: flagNodeSlice{OK: true, Values: []Node{"*"}},
		// StartSubREQnCliCommandCont:  flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQErrorLog:     flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQToFile:       flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQToFileAppend: flagNodeSlice{OK: true, Values: []Node{"*"}},
		StartSubREQHello:        flagNodeSlice{OK: true, Values: []Node{"*"}},
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
	// checkREQnCliCommandContTest(conf, t)
	// checkREQToConsoleTest(conf, t), NB: No tests will be made for console ouput.
	// checkREQToFileAppendTest(conf, t), NB: Already tested via others
	// checkREQToFileTest(conf, t), NB: Already tested via others
	checkREQHelloTest(conf, t)
	// checkREQErrorLogTest(conf, t)
	// checkREQPingTest(conf, t)
	// checkREQPongTest(conf, t)
	// checkREQHttpGetTest(conf, t)
	// checkREQTailFileTest(conf, t)
	// checkREQToSocketTest(conf, t)

	// ---------------------------------------

	s.Stop()

}

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
