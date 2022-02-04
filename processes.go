package steward

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// processes holds all the information about running processes
type processes struct {
	// The main context for subscriber processes.
	ctx context.Context
	// cancel func to send cancel signal to the subscriber processes context.
	cancel context.CancelFunc
	// The active spawned processes
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

	// Full path to the signing keys folder
	SignKeyFolder string
	// Full path to private signing key.
	SignKeyPrivateKeyPath string
	// Full path to public signing key.
	SignKeyPublicKeyPath string

	// allowedSignatures  holds all the signatures,
	// and the public keys
	allowedSignatures allowedSignatures

	// private key for ed25519 signing.
	SignPrivateKey []byte
	// public key for ed25519 signing.
	SignPublicKey []byte
}

// newProcesses will prepare and return a *processes which
// is map containing all the currently running processes.
func newProcesses(ctx context.Context, metrics *metrics, tui *tui, errorKernel *errorKernel, configuration *Configuration) *processes {
	p := processes{
		active:        *newProcsMap(),
		tui:           tui,
		errorKernel:   errorKernel,
		configuration: configuration,
		allowedSignatures: allowedSignatures{
			signatures: make(map[string]struct{}),
		},
	}

	// Prepare the parent context for the subscribers.
	ctx, cancel := context.WithCancel(ctx)

	// // Start the processes map.
	// go func() {
	// 	p.active.run(ctx)
	// }()

	p.ctx = ctx
	p.cancel = cancel

	p.metrics = metrics

	// Set the signing key paths.
	p.SignKeyFolder = filepath.Join(p.configuration.ConfigFolder, "signing")
	p.SignKeyPrivateKeyPath = filepath.Join(p.SignKeyFolder, "private.key")
	p.SignKeyPublicKeyPath = filepath.Join(p.SignKeyFolder, "public.key")

	return &p
}

// ----------------------

// allowedSignatures is the structure for reading and writing from
// the signatures map. It holds a mutex to use when interacting with
// the map.
type allowedSignatures struct {
	// signatures is a map for holding all the allowed signatures.
	signatures map[string]struct{}
	mu         sync.Mutex
}

// loadSigningKeys will try to load the ed25519 signing keys. If the
// files are not found new keys will be generated and written to disk.
func (p *processes) loadSigningKeys(initProc process) error {
	// Check if folder structure exist, if not create it.
	if _, err := os.Stat(p.SignKeyFolder); os.IsNotExist(err) {
		err := os.MkdirAll(p.SignKeyFolder, 0700)
		if err != nil {
			er := fmt.Errorf("error: failed to create directory for signing keys : %v", err)
			return er
		}

	}

	// Check if there already are any keys in the etc folder.
	foundKey := false

	if _, err := os.Stat(p.SignKeyPublicKeyPath); !os.IsNotExist(err) {
		foundKey = true
	}
	if _, err := os.Stat(p.SignKeyPrivateKeyPath); !os.IsNotExist(err) {
		foundKey = true
	}

	// If no keys where found generete a new pair, load them into the
	// processes struct fields, and write them to disk.
	if !foundKey {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			er := fmt.Errorf("error: failed to generate ed25519 keys for signing: %v", err)
			return er
		}
		pubB64string := base64.RawStdEncoding.EncodeToString(pub)
		privB64string := base64.RawStdEncoding.EncodeToString(priv)

		// Write public key to file.
		err = p.writeSigningKey(p.SignKeyPublicKeyPath, pubB64string)
		if err != nil {
			return err
		}

		// Write private key to file.
		err = p.writeSigningKey(p.SignKeyPrivateKeyPath, privB64string)
		if err != nil {
			return err
		}

		// Also store the keys in the processes structure so we can
		// reference them from there when we need them.
		p.SignPublicKey = pub
		p.SignPrivateKey = priv

		er := fmt.Errorf("info: no signing keys found, generating new keys")
		p.errorKernel.errSend(initProc, Message{}, er)

		// We got the new generated keys now, so we can return.
		return nil
	}

	// Key files found, load them into the processes struct fields.
	pubKey, _, err := p.readKeyFile(p.SignKeyPublicKeyPath)
	if err != nil {
		return err
	}
	p.SignPublicKey = pubKey

	privKey, _, err := p.readKeyFile(p.SignKeyPrivateKeyPath)
	if err != nil {
		return err
	}
	p.SignPublicKey = pubKey
	p.SignPrivateKey = privKey

	return nil
}

// readKeyFile will take the path of a key file as input, read the base64
// encoded data, decode the data. It will return the raw data as []byte,
// the base64 encoded data, and any eventual error.
func (p *processes) readKeyFile(keyFile string) (ed2519key []byte, b64Key []byte, err error) {
	fh, err := os.Open(keyFile)
	if err != nil {
		er := fmt.Errorf("error: failed to open key file: %v", err)
		return nil, nil, er
	}
	defer fh.Close()

	b, err := ioutil.ReadAll(fh)
	if err != nil {
		er := fmt.Errorf("error: failed to read key file: %v", err)
		return nil, nil, er
	}

	key, err := base64.RawStdEncoding.DecodeString(string(b))
	if err != nil {
		er := fmt.Errorf("error: failed to base64 decode key data: %v", err)
		return nil, nil, er
	}

	return key, b, nil
}

// writeSigningKey will write the base64 encoded signing key to file.
func (p *processes) writeSigningKey(realPath string, keyB64 string) error {
	fh, err := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		er := fmt.Errorf("error: failed to open key file for writing: %v", err)
		return er
	}
	defer fh.Close()

	_, err = fh.Write([]byte(keyB64))
	if err != nil {
		er := fmt.Errorf("error: failed to write key to file: %v", err)
		return er
	}

	return nil
}

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
		proc := newProcess(proc.ctx, p.metrics, proc.natsConn, p, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go proc.spawnWorker(proc.processes, proc.natsConn)
	}

	{
		log.Printf("Starting REQOpProcessStart subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessStart, string(proc.node))
		proc := newProcess(proc.ctx, p.metrics, proc.natsConn, p, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go proc.spawnWorker(proc.processes, proc.natsConn)
	}

	{
		log.Printf("Starting REQOpProcessStop subscriber: %#v\n", proc.node)
		sub := newSubject(REQOpProcessStop, string(proc.node))
		proc := newProcess(proc.ctx, p.metrics, proc.natsConn, p, proc.toRingbufferCh, proc.configuration, sub, proc.errorCh, processKindSubscriber, nil)
		go proc.spawnWorker(proc.processes, proc.natsConn)
	}

	// Start a subscriber for textLogging messages
	if proc.configuration.StartSubREQToFileAppend {
		proc.startup.subREQToFileAppend(proc)
	}

	// Start a subscriber for text to file messages
	if proc.configuration.StartSubREQToFile {
		proc.startup.subREQToFile(proc)
	}

	// Start a subscriber for reading file to copy
	if proc.configuration.StartSubREQCopyFileFrom {
		proc.startup.subREQCopyFileFrom(proc)
	}

	// Start a subscriber for writing copied file to disk
	if proc.configuration.StartSubREQCopyFileTo {
		proc.startup.subREQCopyFileTo(proc)
	}

	// Start a subscriber for Hello messages
	if proc.configuration.StartSubREQHello {
		proc.startup.subREQHello(proc)
	}

	if proc.configuration.StartSubREQErrorLog {
		// Start a subscriber for REQErrorLog messages
		proc.startup.subREQErrorLog(proc)
	}

	// Start a subscriber for Ping Request messages
	if proc.configuration.StartSubREQPing {
		proc.startup.subREQPing(proc)
	}

	// Start a subscriber for REQPong messages
	if proc.configuration.StartSubREQPong {
		proc.startup.subREQPong(proc)
	}

	// Start a subscriber for REQCliCommand messages
	if proc.configuration.StartSubREQCliCommand {
		proc.startup.subREQCliCommand(proc)
	}

	// Start a subscriber for CLICommandReply messages
	if proc.configuration.StartSubREQToConsole {
		proc.startup.subREQToConsole(proc)
	}

	// Start a subscriber for CLICommandReply messages
	if proc.configuration.EnableTUI {
		proc.startup.subREQTuiToConsole(proc)
	}

	if proc.configuration.StartPubREQHello != 0 {
		proc.startup.pubREQHello(proc)
	}

	// Start a subscriber for Http Get Requests
	if proc.configuration.StartSubREQHttpGet {
		proc.startup.subREQHttpGet(proc)
	}

	if proc.configuration.StartSubREQTailFile {
		proc.startup.subREQTailFile(proc)
	}

	if proc.configuration.StartSubREQCliCommandCont {
		proc.startup.subREQCliCommandCont(proc)
	}

	if proc.configuration.StartSubREQRelay {
		proc.startup.subREQRelay(proc)
	}

	proc.startup.subREQRelayInitial(proc)

	proc.startup.subREQToSocket(proc)
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
	metrics *metrics
}

func newStartup(metrics *metrics) *startup {
	s := startup{metrics: metrics}

	return &s
}

func (s startup) subREQHttpGet(p process) {

	log.Printf("Starting Http Get subscriber: %#v\n", p.node)
	sub := newSubject(REQHttpGet, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)

}

func (s startup) pubREQHello(p process) {
	log.Printf("Starting Hello Publisher: %#v\n", p.node)

	sub := newSubject(REQHello, p.configuration.CentralNodeName)
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindPublisher, nil)

	// Define the procFunc to be used for the process.
	proc.procFunc = procFunc(
		func(ctx context.Context, procFuncCh chan Message) error {
			ticker := time.NewTicker(time.Second * time.Duration(p.configuration.StartPubREQHello))
			for {

				d := fmt.Sprintf("Hello from %v\n", p.node)

				m := Message{
					FileName:   "hello.log",
					Directory:  "hello-messages",
					ToNode:     Node(p.configuration.CentralNodeName),
					FromNode:   Node(p.node),
					Data:       []byte(d),
					Method:     REQHello,
					ACKTimeout: 10,
					Retries:    1,
				}

				sam, err := newSubjectAndMessage(m)
				if err != nil {
					// In theory the system should drop the message before it reaches here.
					p.processes.errorKernel.errSend(p, m, err)
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
		})
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToConsole(p process) {
	log.Printf("Starting Text To Console subscriber: %#v\n", p.node)
	sub := newSubject(REQToConsole, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQTuiToConsole(p process) {
	log.Printf("Starting Tui To Console subscriber: %#v\n", p.node)
	sub := newSubject(REQTuiToConsole, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCliCommand(p process) {
	log.Printf("Starting CLICommand Request subscriber: %#v\n", p.node)
	sub := newSubject(REQCliCommand, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQPong(p process) {
	log.Printf("Starting Pong subscriber: %#v\n", p.node)
	sub := newSubject(REQPong, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQPing(p process) {
	log.Printf("Starting Ping Request subscriber: %#v\n", p.node)
	sub := newSubject(REQPing, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQErrorLog(p process) {
	log.Printf("Starting REQErrorLog subscriber: %#v\n", p.node)
	sub := newSubject(REQErrorLog, "errorCentral")
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)
	go proc.spawnWorker(p.processes, p.natsConn)
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
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	// The reason for running the say hello subscriber as a procFunc is that
	// a handler are not able to hold state, and we need to hold the state
	// of the nodes we've received hello's from in the sayHelloNodes map,
	// which is the information we pass along to generate metrics.
	proc.procFunc = func(ctx context.Context, procFuncCh chan Message) error {
		sayHelloNodes := make(map[Node]struct{})

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

			// Add an entry for the node in the map
			sayHelloNodes[m.FromNode] = struct{}{}

			// update the prometheus metrics
			s.metrics.promHelloNodesTotal.Set(float64(len(sayHelloNodes)))
			s.metrics.promHelloNodesContactLast.With(prometheus.Labels{"nodeName": string(m.FromNode)}).SetToCurrentTime()

		}
	}
	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToFile(p process) {
	log.Printf("Starting text to file subscriber: %#v\n", p.node)
	sub := newSubject(REQToFile, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCopyFileFrom(p process) {
	log.Printf("Starting copy file from subscriber: %#v\n", p.node)
	sub := newSubject(REQCopyFileFrom, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCopyFileTo(p process) {
	log.Printf("Starting copy file to subscriber: %#v\n", p.node)
	sub := newSubject(REQCopyFileTo, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToFileAppend(p process) {
	log.Printf("Starting text logging subscriber: %#v\n", p.node)
	sub := newSubject(REQToFileAppend, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQTailFile(p process) {
	log.Printf("Starting tail log files subscriber: %#v\n", p.node)
	sub := newSubject(REQTailFile, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQCliCommandCont(p process) {
	log.Printf("Starting cli command with continous delivery: %#v\n", p.node)
	sub := newSubject(REQCliCommandCont, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQRelay(p process) {
	nodeWithRelay := fmt.Sprintf("*.%v", p.node)
	log.Printf("Starting Relay: %#v\n", nodeWithRelay)
	sub := newSubject(REQRelay, string(nodeWithRelay))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQRelayInitial(p process) {
	log.Printf("Starting Relay Initial: %#v\n", p.node)
	sub := newSubject(REQRelayInitial, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

func (s startup) subREQToSocket(p process) {
	log.Printf("Starting write to socket subscriber: %#v\n", p.node)
	sub := newSubject(REQToSocket, string(p.node))
	proc := newProcess(p.ctx, s.metrics, p.natsConn, p.processes, p.toRingbufferCh, p.configuration, sub, p.errorCh, processKindSubscriber, nil)

	go proc.spawnWorker(p.processes, p.natsConn)
}

// ---------------------------------------------------------------

// Print the content of the processes map.
func (p *processes) printProcessesMap() {
	log.Printf("*** Output of processes map :\n")

	{
		p.active.mu.Lock()

		for pName, proc := range p.active.procNames {
			log.Printf("* proc - pub/sub: %v, procName in map: %v , id: %v, subject: %v\n", proc.processKind, pName, proc.processID, proc.subject.name())
		}

		p.metrics.promProcessesTotal.Set(float64(len(p.active.procNames)))

		p.active.mu.Unlock()
	}

}
