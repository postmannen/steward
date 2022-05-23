// Package functionality tests for Steward.
//
// To turn on the normal logging functionality during tests, use:
// go test -logging=true
package steward

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

// Set the default logging functionality of the package to false.
var logging = flag.Bool("logging", false, "set to true to enable the normal logger of the package")

// Test the overall functionality of Steward.
// Starting up the server.
// Message passing.
// The different REQ types.
func TestStewardServer(t *testing.T) {
	// if !*logging {
	// 	log.SetOutput(io.Discard)
	// }

	t.Logf("--------------- 1\n")

	ns := newNatsServerForTesting(t, 40222)
	if err := natsserver.Run(ns); err != nil {
		natsserver.PrintAndDie(err.Error())
	}
	defer ns.Shutdown()

	t.Logf("--------------- 2\n")

	tempDir := t.TempDir()
	srv, conf := newServerForTesting(t, "127.0.0.1:40222", tempDir)
	srv.Start()
	defer srv.Stop()

	t.Logf("--------------- 3\n")

	// Run the message tests
	//
	// ---------------------------------------

	type testFunc func(*server, *Configuration, *testing.T, string) error

	// Specify all the test funcs to run in the slice.
	funcs := []testFunc{
		checkREQErrorLogTest,
		checkREQTailFileTest,
		checkErrorKernelMalformedJSONtest,
		checkMetricValuesTest,
	}

	for _, f := range funcs {
		err := f(srv, conf, t, tempDir)
		if err != nil {
			t.Errorf("%v\n", err)
		}
	}

}

// ----------------------------------------------------------------------------
// Check REQ types
// ----------------------------------------------------------------------------

// Check the error logger type.
func checkREQErrorLogTest(stewardServer *server, conf *Configuration, t *testing.T, tempDir string) error {
	m := Message{
		ToNode: "somenode",
	}

	p := newProcess(stewardServer.ctx, stewardServer, Subject{}, processKindSubscriber, nil)

	stewardServer.errorKernel.errSend(p, m, fmt.Errorf("some error"))

	resultFile := filepath.Join(conf.SubscribersDataFolder, "errorLog", "errorCentral", "error.log")
	_, err := findStringInFileTest("some error", resultFile, conf, t)
	if err != nil {
		return fmt.Errorf(" \U0001F631  [FAILED]	: checkREQErrorLogTest: %v", err)
	}

	t.Logf(" \U0001f600 [SUCCESS]	: checkREQErrorLogTest\n")
	return nil
}

// Check the tailing of files type.
func checkREQTailFileTest(stewardServer *server, conf *Configuration, t *testing.T, tmpDir string) error {
	// Create a file with some content.
	fp := filepath.Join(tmpDir, "test.file")
	fh, err := os.OpenFile(fp, os.O_APPEND|os.O_RDWR|os.O_CREATE|os.O_SYNC, 0600)
	if err != nil {
		return fmt.Errorf(" * failed: unable to open temporary file: %v", err)
	}
	defer fh.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Write content to the file with specified intervals.
	go func() {
		for i := 1; i <= 10; i++ {
			_, err = fh.Write([]byte("some file content\n"))
			if err != nil {
				fmt.Printf(" * failed: writing to temporary file: %v\n", err)
			}
			fh.Sync()
			time.Sleep(time.Millisecond * 500)

			// Check if we've received a done, else default to continuing.
			select {
			case <-ctx.Done():
				return
			default:
				// no done received, we're continuing.
			}

		}
	}()

	s := `[
		{
			"directory": "tail-files",
			"fileName": "fileName.result",
			"toNode": "central",
			"methodArgs": ["` + fp + `"],
			"method":"REQTailFile",
			"ACKTimeout":5,
			"retries":3,
			"methodTimeout": 10
		}
	]`

	writeToSocketTest(conf, s, t)

	// time.Sleep(time.Second * 5)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "tail-files", "central", "fileName.result")

	// Wait n times for result file to be created.
	n := 5
	for i := 0; i <= n; i++ {
		_, err := os.Stat(resultFile)
		if os.IsNotExist(err) {
			time.Sleep(time.Millisecond * 1000)
			continue
		}

		if os.IsNotExist(err) && i >= n {
			cancel()
			return fmt.Errorf(" \U0001F631  [FAILED]	: checkREQTailFileTest:  no result file created for request within the given time")
		}
	}

	cancel()

	_, err = findStringInFileTest("some file content", resultFile, conf, t)
	if err != nil {
		return fmt.Errorf(" \U0001F631  [FAILED]	: checkREQTailFileTest: %v", err)
	}

	t.Logf(" \U0001f600 [SUCCESS]	: checkREQTailFileTest\n")
	return nil
}

// ----------------------------------------------------------------------------
// Functionality checks
// ----------------------------------------------------------------------------

// Check errorKernel
func checkErrorKernelMalformedJSONtest(stewardServer *server, conf *Configuration, t *testing.T, tempDir string) error {

	// JSON message with error, missing brace.
	m := `[
		{
			"directory": "some dir",
			"fileName":"someext",
			"toNode": "somenode",
			"data": ["some data"],
			"method": "REQErrorLog"
		missing brace here.....
	]`

	writeToSocketTest(conf, m, t)

	resultFile := filepath.Join(conf.SubscribersDataFolder, "errorLog", "errorCentral", "error.log")

	// Wait n times for error file to be created.
	n := 5
	for i := 0; i <= n; i++ {
		_, err := os.Stat(resultFile)
		if os.IsNotExist(err) {
			time.Sleep(time.Millisecond * 1000)
			continue
		}

		if os.IsNotExist(err) && i >= n {
			return fmt.Errorf(" \U0001F631  [FAILED]	: checkErrorKernelMalformedJSONtest: no result file created for request within the given time")
		}
	}

	// Start checking if the result file is being updated.
	chUpdated := make(chan bool)
	go checkFileUpdated(resultFile, chUpdated)

	// We wait 5 seconds for an update, or else we fail.
	ticker := time.NewTicker(time.Second * 5)

	for {
		select {
		case <-chUpdated:
			// We got an update, so we continue to check if we find the string we're
			// looking for.
			found, err := findStringInFileTest("error: malformed json", resultFile, conf, t)
			if !found && err != nil {
				return fmt.Errorf(" \U0001F631  [FAILED]	: checkErrorKernelMalformedJSONtest: %v", err)
			}

			if !found && err == nil {
				continue
			}

			if found {
				t.Logf(" \U0001f600 [SUCCESS]	: checkErrorKernelMalformedJSONtest")
				return nil
			}
		case <-ticker.C:
			return fmt.Errorf(" * failed: did not get an update in the errorKernel log file")
		}
	}
}

func checkMetricValuesTest(stewardServer *server, conf *Configuration, t *testing.T, tempDir string) error {
	mfs, err := stewardServer.metrics.promRegistry.Gather()
	if err != nil {
		return fmt.Errorf("error: promRegistry.gathering: %v", mfs)
	}

	if len(mfs) <= 0 {
		return fmt.Errorf("error: promRegistry.gathering: did not find any metric families: %v", mfs)
	}

	found := false
	for _, mf := range mfs {
		if mf.GetName() == "steward_processes_total" {
			found = true

			m := mf.GetMetric()

			if m[0].Gauge.GetValue() <= 0 {
				return fmt.Errorf("error: promRegistry.gathering: did not find any running processes in metric for processes_total : %v", m[0].Gauge.GetValue())
			}
		}
	}

	if !found {
		return fmt.Errorf("error: promRegistry.gathering: did not find specified metric processes_total")
	}

	t.Logf(" \U0001f600 [SUCCESS]	: checkMetricValuesTest")

	return nil
}

// ----------------------------------------------------------------------------
// Helper functions for tests
// ----------------------------------------------------------------------------

// Check if file are getting updated with new content.
func checkFileUpdated(fileRealPath string, fileUpdated chan bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Failed fsnotify.NewWatcher")
		return
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		//Give a true value to updated so it reads the file the first time.
		fileUpdated <- true
		for {
			select {
			case event := <-watcher.Events:
				log.Println("event:", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("modified file:", event.Name)
					//testing with an update chan to get updates
					fileUpdated <- true
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(fileRealPath)
	if err != nil {
		log.Fatal(err)
	}
	<-done
}

// Check if a file contains the given string.
func findStringInFileTest(want string, fileName string, conf *Configuration, t *testing.T) (bool, error) {
	// Wait n seconds for the results file to be created
	n := 5

	for i := 0; i <= n; i++ {
		_, err := os.Stat(fileName)
		if os.IsNotExist(err) {
			time.Sleep(time.Millisecond * 500)
			continue
		}

		if os.IsNotExist(err) && i >= n {
			return false, fmt.Errorf(" * failed: no result file created for request within the given time\n")
		}
	}

	fh, err := os.Open(fileName)
	if err != nil {
		return false, fmt.Errorf(" * failed: could not open result file: %v", err)
	}

	result, err := io.ReadAll(fh)
	if err != nil {
		return false, fmt.Errorf(" * failed: could not read result file: %v", err)
	}

	found := strings.Contains(string(result), want)
	if !found {
		return false, nil
	}

	return true, nil
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
