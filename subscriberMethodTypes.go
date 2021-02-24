// NB:
// When adding new constants for the Method or CommandOrEvent
// types, make sure to also add them to the map
// <Method/CommandOrEvent>Available since the this will be used
// to check if the message values are valid later on.

package steward

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/prometheus/client_golang/prometheus"
)

// ------------------------------------------------------------

// Method is used to specify the actual function/method that
// is represented in a typed manner.
type Method string

func (m Method) GetMethodsAvailable() MethodsAvailable {
	ma := MethodsAvailable{
		topics: map[Method]methodHandler{
			ShellCommand: methodCommandShellCommand{},
			TextLogging:  methodEventTextLogging{},
			SayHello:     methodEventSayHello{},
			ErrorLog:     methodEventErrorLog{},
		},
	}

	return ma
}

const (
	// Shell command to be executed via f.ex. bash
	ShellCommand Method = "ShellCommand"
	// Send text logging to some host
	TextLogging Method = "TextLogging"
	// Send Hello I'm here message
	SayHello Method = "SayHello"
	// Error log methods to centralError
	ErrorLog Method = "ErrorLog"
)

type MethodsAvailable struct {
	topics map[Method]methodHandler
}

// Check if exists will check if the Method is defined. If true the bool
// value will be set to true, and the methodHandler function for that type
// will be returned.
func (ma MethodsAvailable) CheckIfExists(m Method) (methodHandler, bool) {
	mFunc, ok := ma.topics[m]
	if ok {
		// fmt.Printf("******THE TOPIC EXISTS: %v******\n", m)
		return mFunc, true
	} else {
		// fmt.Printf("******THE TOPIC DO NOT EXIST: %v******\n", m)
		return nil, false
	}
}

// ------------------------------------------------------------
// Subscriber method handlers
// ------------------------------------------------------------

type methodHandler interface {
	handler(server *server, message Message, node string) ([]byte, error)
}

type methodCommandShellCommand struct{}

func (m methodCommandShellCommand) handler(s *server, message Message, node string) ([]byte, error) {
	// Since the command to execute is at the first position in the
	// slice we need to slice it out. The arguments are at the
	// remaining positions.
	c := message.Data[0]
	a := message.Data[1:]
	cmd := exec.Command(c, a...)
	//cmd.Stdout = os.Stdout
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("error: execution of command failed: %v\n", err)
	}

	outMsg := []byte(fmt.Sprintf("confirmed from node: %v: messageID: %v\n---\n%s---", node, message.ID, out))
	return outMsg, nil
}

// -----

type methodEventTextLogging struct{}

func (m methodEventTextLogging) handler(s *server, message Message, node string) ([]byte, error) {
	for _, d := range message.Data {
		s.subscriberServices.logCh <- []byte(d)
	}

	outMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return outMsg, nil
}

// -----

type methodEventSayHello struct{}

func (m methodEventSayHello) handler(s *server, message Message, node string) ([]byte, error) {
	log.Printf("################## Received hello from %v ##################\n", message.FromNode)
	// Since the handler is only called to handle a specific type of message we need
	// to store it elsewhere, and choice for now is under s.metrics.sayHelloNodes
	s.subscriberServices.sayHelloNodes[message.FromNode] = struct{}{}

	// update the prometheus metrics
	s.metrics.metricsCh <- metricType{
		metric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "hello_nodes",
			Help: "The current number of total nodes who have said hello",
		}),
		value: float64(len(s.subscriberServices.sayHelloNodes)),
	}
	outMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return outMsg, nil
}

// ---

type methodEventErrorLog struct{}

func (m methodEventErrorLog) handler(s *server, message Message, node string) ([]byte, error) {
	log.Printf("----------------------------------------------------------------------------..\n")
	log.Printf("Received error from: %v, containing: %v", message.FromNode, message.Data)
	log.Printf("----------------------------------------------------------------------------..\n")
	return nil, nil
}
