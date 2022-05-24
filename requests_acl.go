package steward

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ----

type methodREQAclRequestUpdate struct {
	event Event
}

func (m methodREQAclRequestUpdate) getKind() Event {
	return m.event
}

// Handler to get all acl's from a central server.
func (m methodREQAclRequestUpdate) handler(proc process, message Message, node string) ([]byte, error) {
	// Get a context with the timeout specified in message.MethodTimeout.
	ctx, _ := getContextForMethodTimeout(proc.ctx, message)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()
		outCh := make(chan []byte)

		go func() {

			select {
			case <-ctx.Done():
				// Done
			case outCh <- []byte{}:
			}
		}()

		select {
		case <-ctx.Done():
		// case out := <-outCh:
		case <-outCh:
			// Using a func here to set the scope of the lock, and then be able to
			// defer the unlock when leaving that scope.
			func() {
				proc.centralAuth.pki.nodesAcked.mu.Lock()
				defer proc.centralAuth.pki.nodesAcked.mu.Unlock()

				fmt.Printf(" <---- methodREQKeysRequestUpdate: received acl hash from NODE=%v, HASH=%v\n", message.FromNode, message.Data)

				// Check if the received hash is the same as the one currently active,
				// TODO: Replace this with checking the ACL hash for the node.
				if bytes.Equal(proc.centralAuth.pki.nodesAcked.keysAndHash.Hash[:], message.Data) {
					fmt.Printf("\n ------------ NODE AND CENTRAL ARE EQUAL, NOTHING TO DO, EXITING HANDLER\n\n")
					return
				}

				fmt.Printf("\n ------------ NODE AND CENTRAL WERE NOT EQUAL, PREPARING TO SEND NEW VERSION OF KEYS\n\n")

				fmt.Printf(" *     methodREQKeysRequestUpdate: marshalling new keys and hash to send: map=%v, hash=%v\n\n", proc.centralAuth.pki.nodesAcked.keysAndHash.Keys, proc.centralAuth.pki.nodesAcked.keysAndHash.Hash)

				b, err := json.Marshal(proc.centralAuth.pki.nodesAcked.keysAndHash)

				if err != nil {
					er := fmt.Errorf("error: methodREQKeysRequestUpdate, failed to marshal keys map: %v", err)
					proc.errorKernel.errSend(proc, message, er)
				}
				fmt.Printf("\n ----> methodREQKeysRequestUpdate: SENDING KEYS TO NODE=%v\n", message.FromNode)
				newReplyMessage(proc, message, b)
			}()
		}
	}()

	// NB: We're not sending an ACK message for this request type.
	return nil, nil
}

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

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 3:
				errCh <- fmt.Errorf("error: methodREQAclAddAccessList: got <3 number methodArgs, want 3")
				return
			}

			host := message.MethodArgs[0]
			source := message.MethodArgs[1]
			cmd := message.MethodArgs[2]

			proc.centralAuth.accessLists.aclAddCommand(Node(host), Node(source), command(cmd))

			outString := fmt.Sprintf("acl added: host=%v, source=%v, command=%v\n", host, source, cmd)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

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
	inf := fmt.Errorf("<--- methodREQAclDeleteCommand received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 3:
				errCh <- fmt.Errorf("error: methodREQAclDeleteCommand: got <3 number methodArgs, want 3")
				return
			}

			host := message.MethodArgs[0]
			source := message.MethodArgs[1]
			cmd := message.MethodArgs[2]

			proc.centralAuth.accessLists.aclDeleteCommand(Node(host), Node(source), command(cmd))

			outString := fmt.Sprintf("acl deleted: host=%v, source=%v, command=%v\n", host, source, cmd)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclDeleteCommand: method timed out: %v", message.MethodArgs)
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

type methodREQAclDeleteSource struct {
	event Event
}

func (m methodREQAclDeleteSource) getKind() Event {
	return m.event
}

func (m methodREQAclDeleteSource) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclDeleteSource received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 2:
				errCh <- fmt.Errorf("error: methodREQAclDeleteSource: got <2 number methodArgs, want 2")
				return
			}

			host := message.MethodArgs[0]
			source := message.MethodArgs[1]

			proc.centralAuth.accessLists.aclDeleteSource(Node(host), Node(source))

			outString := fmt.Sprintf("acl deleted: host=%v, source=%v\n", host, source)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclDeleteSource: method timed out: %v", message.MethodArgs)
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

type methodREQAclGroupNodesAddNode struct {
	event Event
}

func (m methodREQAclGroupNodesAddNode) getKind() Event {
	return m.event
}

func (m methodREQAclGroupNodesAddNode) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclGroupNodesAddNode received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 2:
				errCh <- fmt.Errorf("error: methodREQAclGroupNodesAddNode: got <2 number methodArgs, want 2")
				return
			}

			ng := message.MethodArgs[0]
			n := message.MethodArgs[1]

			proc.centralAuth.accessLists.groupNodesAddNode(nodeGroup(ng), Node(n))

			outString := fmt.Sprintf("added node to nodeGroup: nodeGroup=%v, node=%v\n", ng, n)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclGroupNodesAddNode: method timed out: %v", message.MethodArgs)
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

type methodREQAclGroupNodesDeleteNode struct {
	event Event
}

func (m methodREQAclGroupNodesDeleteNode) getKind() Event {
	return m.event
}

func (m methodREQAclGroupNodesDeleteNode) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclGroupNodesDeleteNode received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 2:
				errCh <- fmt.Errorf("error: methodREQAclGroupNodesDeleteNode: got <2 number methodArgs, want 2")
				return
			}

			ng := message.MethodArgs[0]
			n := message.MethodArgs[1]

			proc.centralAuth.accessLists.groupNodesDeleteNode(nodeGroup(ng), Node(n))

			outString := fmt.Sprintf("deleted node from nodeGroup: nodeGroup=%v, node=%v\n", ng, n)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclGroupNodesDeleteNode: method timed out: %v", message.MethodArgs)
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

type methodREQAclGroupNodesDeleteGroup struct {
	event Event
}

func (m methodREQAclGroupNodesDeleteGroup) getKind() Event {
	return m.event
}

func (m methodREQAclGroupNodesDeleteGroup) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclGroupNodesDeleteGroup received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 1:
				errCh <- fmt.Errorf("error: methodREQAclGroupNodesDeleteGroup: got <1 number methodArgs, want 1")
				return
			}

			ng := message.MethodArgs[0]

			proc.centralAuth.accessLists.groupNodesDeleteGroup(nodeGroup(ng))

			outString := fmt.Sprintf("deleted nodeGroup: nodeGroup=%v\n", ng)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclGroupNodesDeleteGroup: method timed out: %v", message.MethodArgs)
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

type methodREQAclGroupCommandsAddCommand struct {
	event Event
}

func (m methodREQAclGroupCommandsAddCommand) getKind() Event {
	return m.event
}

func (m methodREQAclGroupCommandsAddCommand) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclGroupCommandsAddCommand received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 2:
				errCh <- fmt.Errorf("error: methodREQAclGroupCommandsAddCommand: got <2 number methodArgs, want 1")
				return
			}

			cg := message.MethodArgs[0]
			c := message.MethodArgs[1]

			proc.centralAuth.accessLists.groupCommandsAddCommand(commandGroup(cg), command(c))

			outString := fmt.Sprintf("added command to commandGroup: commandGroup=%v, command=%v\n", cg, c)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclGroupCommandsAddCommand: method timed out: %v", message.MethodArgs)
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

type methodREQAclGroupCommandsDeleteCommand struct {
	event Event
}

func (m methodREQAclGroupCommandsDeleteCommand) getKind() Event {
	return m.event
}

func (m methodREQAclGroupCommandsDeleteCommand) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclGroupCommandsDeleteCommand received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 1:
				errCh <- fmt.Errorf("error: methodREQAclGroupCommandsDeleteCommand: got <1 number methodArgs, want 1")
				return
			}

			cg := message.MethodArgs[0]
			c := message.MethodArgs[1]

			proc.centralAuth.accessLists.groupCommandsDeleteCommand(commandGroup(cg), command(c))

			outString := fmt.Sprintf("deleted command from commandGroup: commandGroup=%v, command=%v\n", cg, c)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclGroupCommandsDeleteCommand: method timed out: %v", message.MethodArgs)
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

type methodREQAclGroupCommandsDeleteGroup struct {
	event Event
}

func (m methodREQAclGroupCommandsDeleteGroup) getKind() Event {
	return m.event
}

func (m methodREQAclGroupCommandsDeleteGroup) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclGroupCommandsDeleteGroup received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 1:
				errCh <- fmt.Errorf("error: methodREQAclGroupCommandsDeleteGroup: got <1 number methodArgs, want 1")
				return
			}

			cg := message.MethodArgs[0]

			proc.centralAuth.accessLists.groupCommandDeleteGroup(commandGroup(cg))

			outString := fmt.Sprintf("deleted commandGroup: commandGroup=%v\n", cg)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclGroupCommandsDeleteGroup: method timed out: %v", message.MethodArgs)
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

type methodREQAclExport struct {
	event Event
}

func (m methodREQAclExport) getKind() Event {
	return m.event
}

func (m methodREQAclExport) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclExport received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)
		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			out, err := proc.centralAuth.accessLists.exportACLs()
			if err != nil {
				errCh <- fmt.Errorf("error: methodREQAclExport failed: %v", err)
				return
			}

			// outString := fmt.Sprintf("Exported acls sent from: %v\n", message.FromNode)
			// out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclExport: method timed out: %v", message.MethodArgs)
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

type methodREQAclImport struct {
	event Event
}

func (m methodREQAclImport) getKind() Event {
	return m.event
}

func (m methodREQAclImport) handler(proc process, message Message, node string) ([]byte, error) {
	inf := fmt.Errorf("<--- methodREQAclImport received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)
		errCh := make(chan error)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

			switch {
			case len(message.MethodArgs) < 1:
				errCh <- fmt.Errorf("error: methodREQAclImport: got <1 number methodArgs, want 1")
				return
			}

			js := []byte(message.MethodArgs[0])
			err := proc.centralAuth.accessLists.importACLs(js)
			if err != nil {
				errCh <- fmt.Errorf("error: methodREQAclImport failed: %v", err)
				return
			}

			outString := fmt.Sprintf("Imported acl's sent from: %v\n", message.FromNode)
			out := []byte(outString)

			select {
			case outCh <- out:
			case <-ctx.Done():
				return
			}
		}()

		select {
		case err := <-errCh:
			proc.errorKernel.errSend(proc, message, err)

		case <-ctx.Done():
			cancel()
			er := fmt.Errorf("error: methodREQAclImport: method timed out")
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
