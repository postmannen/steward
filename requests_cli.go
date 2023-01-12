package steward

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type methodREQCliCommand struct {
	event Event
}

func (m methodREQCliCommand) getKind() Event {
	return m.event
}

// handler to run a CLI command with timeout context. The handler will
// return the output of the command run back to the calling publisher
// as a new message.
func (m methodREQCliCommand) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- CLICommandREQUEST received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logDebug(inf, proc.configuration)

	msgForErrors := message
	msgForErrors.FileName = msgForErrors.FileName + ".error"

	// Execute the CLI command in it's own go routine, so we are able
	// to return immediately with an ack reply that the messag was
	// received, and we create a new message to send back to the calling
	// node for the out put of the actual command.
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		var a []string

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQCliCommand: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er, logWarning)
			newReplyMessage(proc, msgForErrors, []byte(er.Error()))

			return
		case len(message.MethodArgs) >= 0:
			a = message.MethodArgs[1:]
		}

		c := message.MethodArgs[0]

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			// Check if {{data}} is defined in the method arguments. If found put the
			// data payload there.
			var foundEnvData bool
			var envData string
			for i, v := range message.MethodArgs {
				if strings.Contains(v, "{{STEWARD_DATA}}") {
					foundEnvData = true
					// Replace the found env variable placeholder with an actual env variable
					message.MethodArgs[i] = strings.Replace(message.MethodArgs[i], "{{STEWARD_DATA}}", "$STEWARD_DATA", -1)

					// Put all the data which is a slice of string into a single
					// string so we can put it in a single env variable.
					envData = string(message.Data)
				}
			}

			cmd := exec.CommandContext(ctx, c, a...)

			// Check for the use of env variable for STEWARD_DATA, and set env if found.
			if foundEnvData {
				envData = fmt.Sprintf("STEWARD_DATA=%v", envData)
				cmd.Env = append(cmd.Env, envData)
			}

			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommand: cmd.Run failed : %v, methodArgs: %v, error_output: %v", err, message.MethodArgs, stderr.String())
				proc.errorKernel.errSend(proc, message, er, logWarning)
				newReplyMessage(proc, msgForErrors, []byte(er.Error()))
			}

			select {
			case outCh <- out.Bytes():
			case <-ctx.Done():
				return
			}
		}()

		select {
		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQCliCommand: method timed out: %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er, logWarning)
			newReplyMessage(proc, msgForErrors, []byte(er.Error()))
		case out := <-outCh:
			cancel()

			// NB: Not quite sure what is the best way to handle the below
			// isReply right now. Implementing as send to central for now.
			//
			// If this is this a reply message swap the toNode and fromNode
			// fields so the output of the command are sent to central node.
			if message.IsReply {
				message.ToNode, message.FromNode = message.FromNode, message.ToNode
			}

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQCliCommandCont struct {
	event Event
}

func (m methodREQCliCommandCont) getKind() Event {
	return m.event
}

// Handler to run REQCliCommandCont, which is the same as normal
// Cli command, but can be used when running a command that will take
// longer time and you want to send the output of the command continually
// back as it is generated, and not just when the command is finished.
func (m methodREQCliCommandCont) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- CLInCommandCont REQUEST received from: %v, containing: %v", message.FromNode, message.Data)
	proc.errorKernel.logDebug(inf, proc.configuration)

	msgForErrors := message
	msgForErrors.FileName = msgForErrors.FileName + ".error"

	// Execute the CLI command in it's own go routine, so we are able
	// to return immediately with an ack reply that the message was
	// received, and we create a new message to send back to the calling
	// node for the out put of the actual command.
	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		defer func() {
			// fmt.Printf(" * DONE *\n")
		}()

		var a []string

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQCliCommand: got <1 number methodArgs")
			proc.errorKernel.errSend(proc, message, er, logWarning)
			newReplyMessage(proc, msgForErrors, []byte(er.Error()))

			return
		case len(message.MethodArgs) >= 0:
			a = message.MethodArgs[1:]
		}

		c := message.MethodArgs[0]

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		// deadline, _ := ctx.Deadline()
		// fmt.Printf(" * DEBUG * deadline : %v\n", deadline)

		outCh := make(chan []byte)
		errCh := make(chan string)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			cmd := exec.CommandContext(ctx, c, a...)

			// Using cmd.StdoutPipe here so we are continuosly
			// able to read the out put of the command.
			outReader, err := cmd.StdoutPipe()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.StdoutPipe failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er, logWarning)
				newReplyMessage(proc, msgForErrors, []byte(er.Error()))
			}

			ErrorReader, err := cmd.StderrPipe()
			if err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.StderrPipe failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er, logWarning)
				newReplyMessage(proc, msgForErrors, []byte(er.Error()))
			}

			if err := cmd.Start(); err != nil {
				er := fmt.Errorf("error: methodREQCliCommandCont: cmd.Start failed : %v, methodArgs: %v", err, message.MethodArgs)
				proc.errorKernel.errSend(proc, message, er, logWarning)
				newReplyMessage(proc, msgForErrors, []byte(er.Error()))
			}

			go func() {
				scanner := bufio.NewScanner(ErrorReader)
				for scanner.Scan() {
					errCh <- scanner.Text()
				}
			}()

			go func() {
				scanner := bufio.NewScanner(outReader)
				for scanner.Scan() {
					outCh <- []byte(scanner.Text() + "\n")
				}
			}()

			// NB: sending cancel to command context, so processes are killed.
			// A github issue is filed on not killing all child processes when using pipes:
			// https://github.com/golang/go/issues/23019
			// TODO: Check in later if there are any progress on the issue.
			// When testing the problem seems to appear when using sudo, or tcpdump without
			// the -l option. So for now, don't use sudo, and remember to use -l with tcpdump
			// which makes stdout line buffered.

			<-ctx.Done()
			cancel()

			if err := cmd.Wait(); err != nil {
				er := fmt.Errorf("info: methodREQCliCommandCont: method timeout reached, canceled: methodArgs: %v, %v", message.MethodArgs, err)
				proc.errorKernel.errSend(proc, message, er, logWarning)
			}

		}()

		// Check if context timer or command output were received.
		for {
			select {
			case <-ctx.Done():
				cancel()
				er := fmt.Errorf("info: methodREQCliCommandCont: method timeout reached, canceling: methodArgs: %v", message.MethodArgs)
				proc.errorKernel.infoSend(proc, message, er)
				newReplyMessage(proc, msgForErrors, []byte(er.Error()))
				return
			case out := <-outCh:
				// fmt.Printf(" * out: %v\n", string(out))
				newReplyMessage(proc, message, out)
			case out := <-errCh:
				newReplyMessage(proc, message, []byte(out))
			}
		}
	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}
