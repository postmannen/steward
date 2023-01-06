package steward

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// processes holds all the information about running processes
type processes struct {
	// mutex for processes
	mu sync.Mutex
	// The main context for subscriber processes.
	ctx context.Context
	// cancel func to send cancel signal to the subscriber processes context.
	cancel context.CancelFunc
	// The active spawned processes
	// server
	server *server
	active procsMap
	// mutex to lock the map
	// mu sync.RWMutex
	// The last processID created
	lastProcessID int
	// The instance global prometheus registry.
	metrics *metrics
	// Waitgroup to keep track of all the processes started.
	wg sync.WaitGroup
	// tui
	tui *tui
	// errorKernel
	errorKernel *errorKernel
	// configuration
	configuration *Configuration

	// Signatures
	nodeAuth *nodeAuth
}

// newProcesses will prepare and return a *processes which
// is map containing all the currently running processes.
func newProcesses(ctx context.Context, server *server) *processes {
	p := processes{
		server:        server,
		active:        *newProcsMap(),
		tui:           server.tui,
		errorKernel:   server.errorKernel,
		configuration: server.configuration,
		nodeAuth:      server.nodeAuth,
		metrics:       server.metrics,
	}

	// Prepare the parent context for the subscribers.
	ctx, cancel := context.WithCancel(ctx)

	// // Start the processes map.
	// go func() {
	// 	p.active.run(ctx)
	// }()

	p.ctx = ctx
	p.cancel = cancel

	return &p
}

// ----------------------

// ----------------------

type procsMap struct {
	procNames map[processName]process
	mu        sync.Mutex
}

func newProcsMap() *procsMap {
	cM := procsMap{
		procNames: make(map[processName]process),
	}
	return &cM
}

// ----------------------

// Start all the subscriber processes.
// Takes an initial process as it's input. All processes
// will be tied to this single process's context.
func (p *processes) Start(proc process) {
	// Set the context for the initial process.
	proc.ctx = p.ctx

	// --- Subscriber services that can be started via flags

	{
		log.Printf("Starting REQOpProcessList subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessList, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	{
		log.Printf("Starting REQOpProcessStart subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessStart, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	{
		log.Printf("Starting REQOpProcessStop subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessStop, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	{
		log.Printf("Starting REQTest subscriber: %#v\n", proc.node)
		sub := newSubject(REQTest, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	if proc.configuration.StartSubREQToFileAppend {
		proc.startup.subREQToFileAppend(proc)
	}

	if proc.configuration.StartSubREQToFile {
		proc.startup.subREQToFile(proc)
	}

	if proc.configuration.StartSubREQToFileNACK {
		proc.startup.subREQToFileNACK(proc)
	}

	if proc.configuration.StartSubREQCopySrc {
		proc.startup.subREQCopySrc(proc)
	}

	if proc.configuration.StartSubREQCopyDst {
		proc.startup.subREQCopyDst(proc)
	}

	if proc.configuration.StartSubREQHello {
		proc.startup.subREQHello(proc)
	}

	if proc.configuration.IsCentralErrorLogger {
		proc.startup.subREQErrorLog(proc)
	}

	if proc.configuration.StartSubREQPing {
		proc.startup.subREQPing(proc)
	}

	if proc.configuration.StartSubREQPong {
		proc.startup.subREQPong(proc)
	}

	if proc.configuration.StartSubREQCliCommand {
		proc.startup.subREQCliCommand(proc)
	}

	if proc.configuration.StartSubREQToConsole {
		proc.startup.subREQToConsole(proc)
	}

	if proc.configuration.EnableTUI {
		proc.startup.subREQTuiToConsole(proc)
	}

	if proc.configuration.StartPubREQHello != 0 {
		proc.startup.pubREQHello(proc)
	}

	if proc.configuration.EnableKeyUpdates {
		proc.startup.pubREQKeysRequestUpdate(proc)
		proc.startup.subREQKeysDeliverUpdate(proc)
	}

	if proc.configuration.EnableAclUpdates {
		proc.startup.pubREQAclRequestUpdate(proc)
		proc.startup.subREQAclDeliverUpdate(proc)
	}

	if proc.configuration.IsCentralAuth {
		proc.startup.subREQKeysRequestUpdate(proc)
		proc.startup.subREQKeysAllow(proc)
		proc.startup.subREQKeysDelete(proc)

		proc.startup.subREQAclRequestUpdate(proc)

		proc.startup.subREQAclAddCommand(proc)
		proc.startup.subREQAclDeleteCommand(proc)
		proc.startup.subREQAclDeleteSource(proc)
		proc.startup.subREQAclGroupNodesAddNode(proc)
		proc.startup.subREQAclGroupNodesDeleteNode(proc)
		proc.startup.subREQAclGroupNodesDeleteGroup(proc)
		proc.startup.subREQAclGroupCommandsAddCommand(proc)
		proc.startup.subREQAclGroupCommandsDeleteCommand(proc)
		proc.startup.subREQAclGroupCommandsDeleteGroup(proc)
		proc.startup.subREQAclExport(proc)
		proc.startup.subREQAclImport(proc)
	}

	// Moved this together with proc.configuration.StartPubREQKeysRequestUpdate since they belong together.
	// if proc.configuration.StartSubREQKeysDeliverUpdate {
	// 	proc.startup.subREQKeysDeliverUpdate(proc)
	// }

	if proc.configuration.StartSubREQHttpGet {
		proc.startup.subREQHttpGet(proc)
	}

	if proc.configuration.StartSubREQHttpGetScheduled {
		proc.startup.subREQHttpGetScheduled(proc)
	}

	if proc.configuration.StartSubREQTailFile {
		proc.startup.subREQTailFile(proc)
	}

	if proc.configuration.StartSubREQCliCommandCont {
		proc.startup.subREQCliCommandCont(proc)
	}

	proc.startup.subREQPublicKey(proc)
}

// Stop all subscriber processes.
func (p *processes) Stop() {
	log.Printf("info: canceling all subscriber processes...\n")
	p.cancel()
	p.wg.Wait()
	log.Printf("info: done canceling all subscriber processes.\n")

}

// ---------------------------------------------------------------------------------------

// Startup holds all the startup methods for subscribers.
type startup struct {
	server      *server
	centralAuth *centralAuth
	metrics     *metrics
}

func newStartup(server *server) *startup {
	s := startup{
		server:      server,
		centralAuth: server.centralAuth,
		metrics:     server.metrics,
	}

	return &s
}

func (s startup) subREQHttpGet(p process) {

	log.Printf("Starting Http Get subscriber: %#v\n", p.node)
	sub := newSubject(REQHttpGet, string(p.node))
	proc := newProcess(p.ctx, p.processes.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()

}

func (s startup) subREQHttpGetScheduled(p process) {

	log.Printf("Starting Http Get Scheduled subscriber: %#v\n", p.node)
	sub := newSubject(REQHttpGetScheduled, string(p.node))
	proc := newProcess(p.ctx, p.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()

}

func (s startup) pubREQHello(p process) {
	log.Printf("Starting Hello Publisher: %#v\n", p.node)

	sub := newSubject(REQHello, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.server, sub, processKindPublisher, nil)

	// Define the procFunc to be used for the process.
	proc.procFunc = func(ctx context.Context, procFuncCh chan Message) error {
		ticker := time.NewTicker(time.Second * time.Duration(p.configuration.StartPubREQHello))
		defer ticker.Stop()
		for {

			// d := fmt.Sprintf("Hello from %v\n", p.node)
			// Send the ed25519 public key used for signing as the payload of the message.
			d := s.server.nodeAuth.SignPublicKey

			m := Message{
				FileName:   "hello.log",
				Directory:  "hello-messages",
				ToNode:     Node(p.configuration.CentralNodeName),
				FromNode:   Node(p.node),
				Data:       []byte(d),
				Method:     REQHello,
				ACKTimeout: proc.configuration.DefaultMessageTimeout,
				Retries:    1,
			}

			sam, err := newSubjectAndMessage(m)
			if err != nil {
				// In theory the system should drop the message before it reaches here.
				p.errorKernel.errSend(p, m, err)
				log.Printf("error: ProcessesStart: %v\n", err)
			}
			proc.toRingbufferCh <- []subjectAndMessage{sam}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return nil
			}
		}
	}
	go proc.spawnWorker()
}

// pubREQKeysRequestUpdate defines the startup of a publisher that will send REQREQKeysRequestUpdate
// to central server and ask for publics keys, and to get them deliver back with a request
// of type pubREQKeysDeliverUpdate.
func (s startup) pubREQKeysRequestUpdate(p process) {
	log.Printf("Starting PublicKeysGet Publisher: %#v\n", p.node)

	sub := newSubject(REQKeysRequestUpdate, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.server, sub, processKindPublisher, nil)

	// Define the procFunc to be used for the process.
	proc.procFunc = func(ctx context.Context, procFuncCh chan Message) error {
		ticker := time.NewTicker(time.Second * time.Duration(p.configuration.REQKeysRequestUpdateInterval))
		defer ticker.Stop()
		for {

			// Send a message with the hash of the currently stored keys,
			// so we would know on the subscriber at central if it should send
			// and update with new keys back.

			proc.nodeAuth.publicKeys.mu.Lock()
			er := fmt.Errorf(" ----> publisher REQKeysRequestUpdate: sending our current hash: %v", []byte(proc.nodeAuth.publicKeys.keysAndHash.Hash[:]))
			p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

			m := Message{
				FileName:    "publickeysget.log",
				Directory:   "publickeysget",
				ToNode:      Node(p.configuration.CentralNodeName),
				FromNode:    Node(p.node),
				Data:        []byte(proc.nodeAuth.publicKeys.keysAndHash.Hash[:]),
				Method:      REQKeysRequestUpdate,
				ReplyMethod: REQKeysDeliverUpdate,
				ACKTimeout:  proc.configuration.DefaultMessageTimeout,
				Retries:     1,
			}
			proc.nodeAuth.publicKeys.mu.Unlock()

			sam, err := newSubjectAndMessage(m)
			if err != nil {
				// In theory the system should drop the message before it reaches here.
				p.errorKernel.errSend(p, m, err)
				log.Printf("error: ProcessesStart: %v\n", err)
			}
			proc.toRingbufferCh <- []subjectAndMessage{sam}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return nil
			}
		}
	}
	go proc.spawnWorker()
}

// pubREQAclRequestUpdate defines the startup of a publisher that will send REQREQKeysRequestUpdate
// to central server and ask for publics keys, and to get them deliver back with a request
// of type pubREQKeysDeliverUpdate.
func (s startup) pubREQAclRequestUpdate(p process) {
	log.Printf("Starting REQAclRequestUpdate Publisher: %#v\n", p.node)

	sub := newSubject(REQAclRequestUpdate, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.server, sub, processKindPublisher, nil)

	// Define the procFunc to be used for the process.
	proc.procFunc = func(ctx context.Context, procFuncCh chan Message) error {
		ticker := time.NewTicker(time.Second * time.Duration(p.configuration.REQAclRequestUpdateInterval))
		defer ticker.Stop()
		for {

			// Send a message with the hash of the currently stored acl's,
			// so we would know for the subscriber at central if it should send
			// and update with new keys back.

			proc.nodeAuth.nodeAcl.mu.Lock()
			er := fmt.Errorf(" ----> publisher REQAclRequestUpdate: sending our current hash: %v", []byte(proc.nodeAuth.nodeAcl.aclAndHash.Hash[:]))
			p.errorKernel.logConsoleOnlyIfDebug(er, p.configuration)

			m := Message{
				FileName:    "aclRequestUpdate.log",
				Directory:   "aclRequestUpdate",
				ToNode:      Node(p.configuration.CentralNodeName),
				FromNode:    Node(p.node),
				Data:        []byte(proc.nodeAuth.nodeAcl.aclAndHash.Hash[:]),
				Method:      REQAclRequestUpdate,
				ReplyMethod: REQAclDeliverUpdate,
				ACKTimeout:  proc.configuration.DefaultMessageTimeout,
				Retries:     1,
			}
			proc.nodeAuth.nodeAcl.mu.Unlock()

			sam, err := newSubjectAndMessage(m)
			if err != nil {
				// In theory the system should drop the message before it reaches here.
				p.errorKernel.errSend(p, m, err)
				log.Printf("error: ProcessesStart: %v\n", err)
			}
			proc.toRingbufferCh <- []subjectAndMessage{sam}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return nil
			}
		}
	}
	go proc.spawnWorker()
}

func (s startup) subREQKeysRequestUpdate(p process) {
	log.Printf("Starting Public keys request update subscriber: %#v\n", p.node)
	sub := newSubject(REQKeysRequestUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQKeysDeliverUpdate(p process) {
	log.Printf("Starting Public keys to Node subscriber: %#v\n", p.node)
	sub := newSubject(REQKeysDeliverUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQKeysAllow(p process) {
	log.Printf("Starting Public keys allow subscriber: %#v\n", p.node)
	sub := newSubject(REQKeysAllow, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQKeysDelete(p process) {
	log.Printf("Starting Public keys delete subscriber: %#v\n", p.node)
	sub := newSubject(REQKeysDelete, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclRequestUpdate(p process) {
	log.Printf("Starting Acl Request update subscriber: %#v\n", p.node)
	sub := newSubject(REQAclRequestUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclDeliverUpdate(p process) {
	log.Printf("Starting Acl deliver update subscriber: %#v\n", p.node)
	sub := newSubject(REQAclDeliverUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

// HERE!

func (s startup) subREQAclAddCommand(p process) {
	log.Printf("Starting Acl Add Command subscriber: %#v\n", p.node)
	sub := newSubject(REQAclAddCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclDeleteCommand(p process) {
	log.Printf("Starting Acl Delete Command subscriber: %#v\n", p.node)
	sub := newSubject(REQAclDeleteCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclDeleteSource(p process) {
	log.Printf("Starting Acl Delete Source subscriber: %#v\n", p.node)
	sub := newSubject(REQAclDeleteSource, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupNodesAddNode(p process) {
	log.Printf("Starting Acl Add node to nodeGroup subscriber: %#v\n", p.node)
	sub := newSubject(REQAclGroupNodesAddNode, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupNodesDeleteNode(p process) {
	log.Printf("Starting Acl Delete node from nodeGroup subscriber: %#v\n", p.node)
	sub := newSubject(REQAclGroupNodesDeleteNode, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupNodesDeleteGroup(p process) {
	log.Printf("Starting Acl Delete nodeGroup subscriber: %#v\n", p.node)
	sub := newSubject(REQAclGroupNodesDeleteGroup, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupCommandsAddCommand(p process) {
	log.Printf("Starting Acl add command to command group subscriber: %#v\n", p.node)
	sub := newSubject(REQAclGroupCommandsAddCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupCommandsDeleteCommand(p process) {
	log.Printf("Starting Acl delete command from command group subscriber: %#v\n", p.node)
	sub := newSubject(REQAclGroupCommandsDeleteCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupCommandsDeleteGroup(p process) {
	log.Printf("Starting Acl delete command group subscriber: %#v\n", p.node)
	sub := newSubject(REQAclGroupCommandsDeleteGroup, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclExport(p process) {
	log.Printf("Starting Acl export subscriber: %#v\n", p.node)
	sub := newSubject(REQAclExport, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclImport(p process) {
	log.Printf("Starting Acl import subscriber: %#v\n", p.node)
	sub := newSubject(REQAclImport, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQToConsole(p process) {
	log.Printf("Starting Text To Console subscriber: %#v\n", p.node)
	sub := newSubject(REQToConsole, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQTuiToConsole(p process) {
	log.Printf("Starting Tui To Console subscriber: %#v\n", p.node)
	sub := newSubject(REQTuiToConsole, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQCliCommand(p process) {
	log.Printf("Starting CLICommand Request subscriber: %#v\n", p.node)
	sub := newSubject(REQCliCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQPong(p process) {
	log.Printf("Starting Pong subscriber: %#v\n", p.node)
	sub := newSubject(REQPong, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQPing(p process) {
	log.Printf("Starting Ping Request subscriber: %#v\n", p.node)
	sub := newSubject(REQPing, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQErrorLog(p process) {
	log.Printf("Starting REQErrorLog subscriber: %#v\n", p.node)
	sub := newSubject(REQErrorLog, "errorCentral")
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

// subREQHello is the handler that is triggered when we are receiving a hello
// message. To keep the state of all the hello's received from nodes we need
// to also start a procFunc that will live as a go routine tied to this process,
// where the procFunc will receive messages from the handler when a message is
// received, the handler will deliver the message to the procFunc on the
// proc.procFuncCh, and we can then read that message from the procFuncCh in
// the procFunc running.
func (s startup) subREQHello(p process) {
	log.Printf("Starting Hello subscriber: %#v\n", p.node)
	sub := newSubject(REQHello, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	// The reason for running the say hello subscriber as a procFunc is that
	// a handler are not able to hold state, and we need to hold the state
	// of the nodes we've received hello's from in the sayHelloNodes map,
	// which is the information we pass along to generate metrics.
	proc.procFunc = func(ctx context.Context, procFuncCh chan Message) error {
		// sayHelloNodes := make(map[Node]struct{})

		for {
			// Receive a copy of the message sent from the method handler.
			var m Message

			select {
			case m = <-procFuncCh:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: subscriber %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				log.Printf("%v\n", er)
				return nil
			}

			s.centralAuth.addPublicKey(proc, m)

			// update the prometheus metrics

			s.server.centralAuth.pki.nodesAcked.mu.Lock()
			mapLen := len(s.server.centralAuth.pki.nodesAcked.keysAndHash.Keys)
			s.server.centralAuth.pki.nodesAcked.mu.Unlock()
			s.metrics.promHelloNodesTotal.Set(float64(mapLen))
			s.metrics.promHelloNodesContactLast.With(prometheus.Labels{"nodeName": string(m.FromNode)}).SetToCurrentTime()

		}
	}
	go proc.spawnWorker()
}

func (s startup) subREQToFile(p process) {
	log.Printf("Starting text to file subscriber: %#v\n", p.node)
	sub := newSubject(REQToFile, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQToFileNACK(p process) {
	log.Printf("Starting text to file subscriber: %#v\n", p.node)
	sub := newSubject(REQToFileNACK, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQCopySrc(p process) {
	log.Printf("Starting copy src subscriber: %#v\n", p.node)
	sub := newSubject(REQCopySrc, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQCopyDst(p process) {
	log.Printf("Starting copy dst subscriber: %#v\n", p.node)
	sub := newSubject(REQCopyDst, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQToFileAppend(p process) {
	log.Printf("Starting text logging subscriber: %#v\n", p.node)
	sub := newSubject(REQToFileAppend, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQTailFile(p process) {
	log.Printf("Starting tail log files subscriber: %#v\n", p.node)
	sub := newSubject(REQTailFile, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQCliCommandCont(p process) {
	log.Printf("Starting cli command with continous delivery: %#v\n", p.node)
	sub := newSubject(REQCliCommandCont, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQPublicKey(p process) {
	log.Printf("Starting get Public Key subscriber: %#v\n", p.node)
	sub := newSubject(REQPublicKey, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

// ---------------------------------------------------------------

// Print the content of the processes map.
func (p *processes) printProcessesMap() {
	log.Printf("*** Output of processes map :\n")

	{
		p.active.mu.Lock()

		for pName, proc := range p.active.procNames {
			er := fmt.Errorf("info: proc - pub/sub: %v, procName in map: %v , id: %v, subject: %v", proc.processKind, pName, proc.processID, proc.subject.name())
			proc.errorKernel.logConsoleOnlyIfDebug(er, proc.configuration)
		}

		p.metrics.promProcessesTotal.Set(float64(len(p.active.procNames)))

		p.active.mu.Unlock()
	}

}
