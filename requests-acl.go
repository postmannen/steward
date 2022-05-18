package steward

import (
	"fmt"
)

// ---

type methodREQAclAddCommand struct {
	event Event
}

func (m methodREQAclAddCommand) getKind() Event {
	return m.event
}

func (m methodREQAclAddCommand) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclAddCommand received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQAclAddAccessList: got <3 number methodArgs, want 3")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			host := message.MethodArgs[0]
			source := message.MethodArgs[1]
			cmd := message.MethodArgs[2]

			proc.centralAuth.accessLists.aclAddCommand(Node(host), Node(source), command(cmd))

			// Just print out for testing.
			proc.centralAuth.accessLists.schemaMain.mu.Lock()
			fmt.Printf("\n ---------- content of main acl map: %v-----------\n", proc.centralAuth.accessLists.schemaMain.ACLMap)
			proc.centralAuth.accessLists.schemaMain.mu.Unlock()

			proc.centralAuth.accessLists.schemaGenerated.mu.Lock()
			fmt.Printf("\n ---------- content of generated acl map: %v-----------\n", proc.centralAuth.accessLists.schemaGenerated.GeneratedACLsMap)
			proc.centralAuth.accessLists.schemaGenerated.mu.Unlock()

			outString := fmt.Sprintf("acl added: host=%v, source=%v, command=%v\n", host, source, cmd)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case <-ctx.Done():

			cancel()
			er := fmt.Errorf("error: methodREQAclAddAccessList: method timed out: %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)

		case out := <-outCh:

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---

type methodREQAclDeleteCommand struct {
	event Event
}

func (m methodREQAclDeleteCommand) getKind() Event {
	return m.event
}

func (m methodREQAclDeleteCommand) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclAddAccessList received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQAclAddAccessList: got <3 number methodArgs, want 3")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			host := message.MethodArgs[0]
			source := message.MethodArgs[1]
			cmd := message.MethodArgs[2]

			proc.centralAuth.accessLists.aclDeleteCommand(Node(host), Node(source), command(cmd))

			// Just print out for testing.
			proc.centralAuth.accessLists.schemaMain.mu.Lock()
			fmt.Printf("\n ---------- content of main acl map: %v-----------\n", proc.centralAuth.accessLists.schemaMain.ACLMap)
			proc.centralAuth.accessLists.schemaMain.mu.Unlock()

			proc.centralAuth.accessLists.schemaGenerated.mu.Lock()
			fmt.Printf("\n ---------- content of generated acl map: %v-----------\n", proc.centralAuth.accessLists.schemaGenerated.GeneratedACLsMap)
			proc.centralAuth.accessLists.schemaGenerated.mu.Unlock()

			outString := fmt.Sprintf("acl added: host=%v, source=%v, command=%v\n", host, source, cmd)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case <-ctx.Done():

			cancel()
			er := fmt.Errorf("error: methodREQAclAddAccessList: method timed out: %v", message.MethodArgs)
			proc.errorKernel.errSend(proc, message, er)

		case out := <-outCh:

			// Prepare and queue for sending a new message with the output
			// of the action executed.
			newReplyMessage(proc, message, out)
		}

	}()

	ackMsg := []byte("confirmed from: " + node + ": " + fmt.Sprint(message.ID))
	return ackMsg, nil
}

// ---
