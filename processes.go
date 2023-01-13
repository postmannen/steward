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
		er := fmt.Errorf("tarting REQOpProcessList subscriber: %#v", proc.node)
		p.errorKernel.logDebug(er, p.configuration)
		sub := newSubject(REQOpProcessList, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	{
		er := fmt.Errorf("starting REQOpProcessStart subscriber: %#v", proc.node)
		p.errorKernel.logDebug(er, p.configuration)
		sub := newSubject(REQOpProcessStart, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	{
		er := fmt.Errorf("starting REQOpProcessStop subscriber: %#v", proc.node)
		p.errorKernel.logDebug(er, p.configuration)
		sub := newSubject(REQOpProcessStop, string(proc.node))
		proc := newProcess(proc.ctx, p.server, sub, processKindSubscriber, nil)
		go proc.spawnWorker()
	}

	{
		er := fmt.Errorf("starting REQTest subscriber: %#v", proc.node)
		p.errorKernel.logDebug(er, p.configuration)
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

	er := fmt.Errorf("starting Http Get subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQHttpGet, string(p.node))
	proc := newProcess(p.ctx, p.processes.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()

}

func (s startup) subREQHttpGetScheduled(p process) {

	er := fmt.Errorf("starting Http Get Scheduled subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)

	sub := newSubject(REQHttpGetScheduled, string(p.node))
	proc := newProcess(p.ctx, p.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()

}

func (s startup) pubREQHello(p process) {
	er := fmt.Errorf("starting Hello Publisher: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)

	sub := newSubject(REQHello, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.server, sub, processKindPublisher, nil)
	proc.isLongRunningPublisher = true

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
				er := fmt.Errorf("error: ProcessesStart: %v", err)
				p.errorKernel.errSend(p, m, er, logError)
			}
			proc.toRingbufferCh <- []subjectAndMessage{sam}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				p.errorKernel.logDebug(er, p.configuration)
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
	er := fmt.Errorf("starting PublicKeysGet Publisher: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)

	sub := newSubject(REQKeysRequestUpdate, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.server, sub, processKindPublisher, nil)
	proc.isLongRunningPublisher = true

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
			p.errorKernel.logDebug(er, p.configuration)

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
				p.errorKernel.errSend(p, m, err, logError)
			}
			proc.toRingbufferCh <- []subjectAndMessage{sam}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				p.errorKernel.logDebug(er, p.configuration)
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
	er := fmt.Errorf("starting REQAclRequestUpdate Publisher: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)

	sub := newSubject(REQAclRequestUpdate, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.server, sub, processKindPublisher, nil)
	proc.isLongRunningPublisher = true

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
			p.errorKernel.logDebug(er, p.configuration)

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
				p.errorKernel.errSend(p, m, err, logError)
				log.Printf("error: ProcessesStart: %v\n", err)
			}
			proc.toRingbufferCh <- []subjectAndMessage{sam}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				er := fmt.Errorf("info: stopped handleFunc for: publisher %v", proc.subject.name())
				// sendErrorLogMessage(proc.toRingbufferCh, proc.node, er)
				p.errorKernel.logDebug(er, p.configuration)
				return nil
			}
		}
	}
	go proc.spawnWorker()
}

func (s startup) subREQKeysRequestUpdate(p process) {
	er := fmt.Errorf("starting Public keys request update subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQKeysRequestUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQKeysDeliverUpdate(p process) {
	er := fmt.Errorf("starting Public keys to Node subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQKeysDeliverUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQKeysAllow(p process) {
	er := fmt.Errorf("starting Public keys allow subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQKeysAllow, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQKeysDelete(p process) {
	er := fmt.Errorf("starting Public keys delete subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQKeysDelete, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclRequestUpdate(p process) {
	er := fmt.Errorf("starting Acl Request update subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclRequestUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclDeliverUpdate(p process) {
	er := fmt.Errorf("starting Acl deliver update subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclDeliverUpdate, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

// HERE!

func (s startup) subREQAclAddCommand(p process) {
	er := fmt.Errorf("starting Acl Add Command subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclAddCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclDeleteCommand(p process) {
	er := fmt.Errorf("starting Acl Delete Command subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclDeleteCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclDeleteSource(p process) {
	er := fmt.Errorf("starting Acl Delete Source subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclDeleteSource, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupNodesAddNode(p process) {
	er := fmt.Errorf("starting Acl Add node to nodeGroup subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclGroupNodesAddNode, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupNodesDeleteNode(p process) {
	er := fmt.Errorf("starting Acl Delete node from nodeGroup subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclGroupNodesDeleteNode, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupNodesDeleteGroup(p process) {
	er := fmt.Errorf("starting Acl Delete nodeGroup subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclGroupNodesDeleteGroup, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupCommandsAddCommand(p process) {
	er := fmt.Errorf("starting Acl add command to command group subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclGroupCommandsAddCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupCommandsDeleteCommand(p process) {
	er := fmt.Errorf("starting Acl delete command from command group subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclGroupCommandsDeleteCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclGroupCommandsDeleteGroup(p process) {
	er := fmt.Errorf("starting Acl delete command group subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclGroupCommandsDeleteGroup, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclExport(p process) {
	er := fmt.Errorf("starting Acl export subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclExport, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQAclImport(p process) {
	er := fmt.Errorf("starting Acl import subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQAclImport, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQToConsole(p process) {
	er := fmt.Errorf("starting Text To Console subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQToConsole, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQTuiToConsole(p process) {
	er := fmt.Errorf("starting Tui To Console subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQTuiToConsole, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQCliCommand(p process) {
	er := fmt.Errorf("starting CLICommand Request subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQCliCommand, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQPong(p process) {
	er := fmt.Errorf("starting Pong subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQPong, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQPing(p process) {
	er := fmt.Errorf("starting Ping Request subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQPing, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)
	go proc.spawnWorker()
}

func (s startup) subREQErrorLog(p process) {
	er := fmt.Errorf("starting REQErrorLog subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
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
	er := fmt.Errorf("starting Hello subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
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
				p.errorKernel.logDebug(er, p.configuration)
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
	er := fmt.Errorf("starting text to file subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQToFile, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQToFileNACK(p process) {
	er := fmt.Errorf("starting text to file subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQToFileNACK, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQCopySrc(p process) {
	er := fmt.Errorf("starting copy src subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQCopySrc, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQCopyDst(p process) {
	er := fmt.Errorf("starting copy dst subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQCopyDst, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQToFileAppend(p process) {
	er := fmt.Errorf("starting text logging subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQToFileAppend, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQTailFile(p process) {
	er := fmt.Errorf("starting tail log files subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQTailFile, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQCliCommandCont(p process) {
	er := fmt.Errorf("starting cli command with continous delivery: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQCliCommandCont, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

func (s startup) subREQPublicKey(p process) {
	er := fmt.Errorf("starting get Public Key subscriber: %#v", p.node)
	p.errorKernel.logDebug(er, p.configuration)
	sub := newSubject(REQPublicKey, string(p.node))
	proc := newProcess(p.ctx, s.server, sub, processKindSubscriber, nil)

	go proc.spawnWorker()
}

// ---------------------------------------------------------------

// Print the content of the processes map.
func (p *processes) printProcessesMap() {
	er := fmt.Errorf("output of processes map : ")
	p.errorKernel.logDebug(er, p.configuration)

	{
		p.active.mu.Lock()

		for pName, proc := range p.active.procNames {
			er := fmt.Errorf("info: proc - pub/sub: %v, procName in map: %v , id: %v, subject: %v", proc.processKind, pName, proc.processID, proc.subject.name())
			proc.errorKernel.logDebug(er, proc.configuration)
		}

		p.metrics.promProcessesTotal.Set(float64(len(p.active.procNames)))

		p.active.mu.Unlock()
	}

}
