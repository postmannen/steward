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
		},
	}

	return ma
}

const (
	// Shell command to be executed via f.ex. bash
	ShellCommand Method = "shellCommand"
	// Send text logging to some host
	TextLogging Method = "textLogging"
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
		fmt.Printf("******THE TOPIC EXISTS: %v******\n", m)
		return mFunc, true
	} else {
		fmt.Printf("******THE TOPIC DO NOT EXIST: %v******\n", m)
		return nil, false
	}
}

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

type methodEventTextLogging struct{}

func (m methodEventTextLogging) handler(s *server, message Message, node string) ([]byte, error) {
	for _, d := range message.Data {
		s.logCh <- []byte(d)
	}

	outMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return outMsg, nil
}
