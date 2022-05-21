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
	inf := fmt.Errorf("<--- methodREQAclDeleteCommand received from: %v, containing: %v", message.FromNode, message.MethodArgs)
	proc.errorKernel.logConsoleOnlyIfDebug(inf, proc.configuration)

	proc.processes.wg.Add(1)
	go func() {
		defer proc.processes.wg.Done()

		switch {
		case len(message.MethodArgs) < 3:
			er := fmt.Errorf("error: methodREQAclDeleteCommand: got <3 number methodArgs, want 3")
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

			outString := fmt.Sprintf("acl deleted: host=%v, source=%v, command=%v\n", host, source, cmd)
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

		switch {
		case len(message.MethodArgs) < 2:
			er := fmt.Errorf("error: methodREQAclDeleteSource: got <2 number methodArgs, want 2")
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

		switch {
		case len(message.MethodArgs) < 2:
			er := fmt.Errorf("error: methodREQAclGroupNodesAddNode: got <2 number methodArgs, want 2")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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

		switch {
		case len(message.MethodArgs) < 2:
			er := fmt.Errorf("error: methodREQAclGroupNodesDeleteNode: got <2 number methodArgs, want 2")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQAclGroupNodesDeleteGroup: got <1 number methodArgs, want 1")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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

		switch {
		case len(message.MethodArgs) < 2:
			er := fmt.Errorf("error: methodREQAclGroupCommandsAddCommand: got <2 number methodArgs, want 1")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQAclGroupCommandsDeleteCommand: got <1 number methodArgs, want 1")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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

		switch {
		case len(message.MethodArgs) < 1:
			er := fmt.Errorf("error: methodREQAclGroupCommandsDeleteGroup: got <1 number methodArgs, want 1")
			proc.errorKernel.errSend(proc, message, er)

			return
		}

		// Get a context with the timeout specified in message.MethodTimeout.
		ctx, cancel := getContextForMethodTimeout(proc.ctx, message)

		outCh := make(chan []byte)

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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

		// switch {
		// case len(message.MethodArgs) < 1:
		// 	er := fmt.Errorf("error: methodREQAclImport: got <1 number methodArgs, want 1")
		// 	proc.errorKernel.errSend(proc, message, er)
		//
		// 	return
		// }

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

		switch {
		case len(message.MethodArgs) < 1:
			errCh <- fmt.Errorf("error: methodREQAclImport: got <1 number methodArgs, want 1")
			return
		}

		proc.processes.wg.Add(1)
		go func() {
			defer proc.processes.wg.Done()

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
