package steward

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-playground/validator/v10"
)

// // centralAuth
// type centralAuth struct {
// 	authorization *authorization
// }
//
// // newCentralAuth
// func newCentralAuth() *centralAuth {
// 	c := centralAuth{
// 		authorization: newAuthorization(),
// 	}
//
// 	return &c
// }

// --------------------------------------

type accessLists struct {
	// Holds the editable structures for ACL handling.
	schemaMain *schemaMain
	// Holds the generated based on the editable structures for ACL handling.
	schemaGenerated *schemaGenerated
	validator       *validator.Validate
	errorKernel     *errorKernel
	configuration   *Configuration
}

func newAccessLists(errorKernel *errorKernel, configuration *Configuration) *accessLists {
	a := accessLists{
		schemaMain:      newSchemaMain(),
		schemaGenerated: newSchemaGenerated(),
		validator:       validator.New(),
		errorKernel:     errorKernel,
		configuration:   configuration,
	}

	return &a
}

// type node string
type command string
type nodeGroup string
type commandGroup string

// schemaMain is the structure that holds the user editable parts for creating ACL's.
type schemaMain struct {
	ACLMap          map[Node]map[Node]map[command]struct{}
	NodeGroupMap    map[nodeGroup]map[Node]struct{}
	CommandGroupMap map[commandGroup]map[command]struct{}
	mu              sync.Mutex
}

func newSchemaMain() *schemaMain {
	s := schemaMain{
		ACLMap:          make(map[Node]map[Node]map[command]struct{}),
		NodeGroupMap:    make(map[nodeGroup]map[Node]struct{}),
		CommandGroupMap: make(map[commandGroup]map[command]struct{}),
	}
	return &s
}

// schemaGenerated is the structure that holds all the generated ACL's
// to be sent to nodes.
// The ACL's here are generated from the schemaMain.ACLMap.
type schemaGenerated struct {
	ACLsToConvert    map[Node]map[Node]map[command]struct{}
	GeneratedACLsMap map[Node]HostACLsSerializedWithHash
	mu               sync.Mutex
}

func newSchemaGenerated() *schemaGenerated {
	s := schemaGenerated{
		ACLsToConvert:    map[Node]map[Node]map[command]struct{}{},
		GeneratedACLsMap: make(map[Node]HostACLsSerializedWithHash),
	}
	return &s
}

// HostACLsSerializedWithHash holds the serialized representation node specific ACL's in the authSchema.
// There is also a sha256 hash of the data.
type HostACLsSerializedWithHash struct {
	// data is all the ACL's for a specific node serialized serialized into cbor.
	Data []byte
	// hash is the sha256 hash of the ACL's.
	// With maps the order are not guaranteed, so A sorted appearance
	// of the ACL map for a host node is used when creating the hash,
	// so the hash stays the same unless the ACL is changed.
	Hash [32]byte
}

// commandAsSlice will convert the given argument into a slice representation.
// If the argument is a group, then all the members of that group will be expanded into
// the slice.
// If the argument is not a group kind of value, then only a slice with that single
// value is returned.
func (a *accessLists) nodeAsSlice(n Node) []Node {
	nodes := []Node{}

	// Check if we are given a nodeGroup variable, and if we are, get all the
	// nodes for that group.
	if strings.HasPrefix(string(n), "grp_nodes_") {
		for nd := range a.schemaMain.NodeGroupMap[nodeGroup(n)] {
			nodes = append(nodes, nd)
		}
	} else {
		// No group found meaning a single node was given as an argument.
		nodes = []Node{n}
	}

	return nodes
}

// commandAsSlice will convert the given argument into a slice representation.
// If the argument is a group, then all the members of that group will be expanded into
// the slice.
// If the argument is not a group kind of value, then only a slice with that single
// value is returned.
func (a *accessLists) commandAsSlice(c command) []command {
	commands := []command{}

	// Check if we are given a nodeGroup variable, and if we are, get all the
	// nodes for that group.
	if strings.HasPrefix(string(c), "grp_commands_") {
		for cmd := range a.schemaMain.CommandGroupMap[commandGroup(c)] {
			commands = append(commands, cmd)
		}
	} else {
		// No group found meaning a single node was given as an argument, so we
		// just put the single node given as the only value in the slice.
		commands = []command{c}
	}

	return commands
}

// aclAddCommand will add a command for a fromNode.
// If the node or the fromNode do not exist they will be created.
// The json encoded schema for a node and the hash of those data
// will also be generated.
func (a *accessLists) aclAddCommand(host Node, source Node, cmd command) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()

	// Check if node exists in map.
	if _, ok := a.schemaMain.ACLMap[host]; !ok {
		// log.Printf("info: did not find node=%v in map, creating map[fromnode]map[command]struct{}\n", n)
		a.schemaMain.ACLMap[host] = make(map[Node]map[command]struct{})
	}

	// Check if also source node exists in map
	if _, ok := a.schemaMain.ACLMap[host][source]; !ok {
		// log.Printf("info: did not find node=%v in map, creating map[fromnode]map[command]struct{}\n", fn)
		a.schemaMain.ACLMap[host][source] = make(map[command]struct{})
	}

	a.schemaMain.ACLMap[host][source][cmd] = struct{}{}
	// err := a.generateJSONForHostOrGroup(n)
	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: addCommandForFromNode: %v", err)
		log.Printf("%v\n", er)
	}

	// fmt.Printf(" * DEBUG: aclNodeFromnodeCommandAdd: a.schemaMain.ACLMap=%v\n", a.schemaMain.ACLMap)

}

// aclDeleteCommand will delete the specified command from the fromnode.
func (a *accessLists) aclDeleteCommand(host Node, source Node, cmd command) error {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()

	// Check if node exists in map.
	if _, ok := a.schemaMain.ACLMap[host]; !ok {
		return fmt.Errorf("authSchema: no such node=%v to delete on in schema exists", host)
	}

	if _, ok := a.schemaMain.ACLMap[host][source]; !ok {
		return fmt.Errorf("authSchema: no such fromnode=%v to delete on in schema for node=%v exists", source, host)
	}

	if _, ok := a.schemaMain.ACLMap[host][source][cmd]; !ok {
		return fmt.Errorf("authSchema: no such command=%v from fromnode=%v to delete on in schema for node=%v exists", cmd, source, host)
	}

	delete(a.schemaMain.ACLMap[host][source], cmd)

	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: aclNodeFromNodeCommandDelete: %v", err)
		log.Printf("%v\n", er)
	}

	return nil
}

// aclDeleteSource will delete specified source node and all commands specified for it.
func (a *accessLists) aclDeleteSource(host Node, source Node) error {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()

	// Check if node exists in map.
	if _, ok := a.schemaMain.ACLMap[host]; !ok {
		return fmt.Errorf("authSchema: no such node=%v to delete on in schema exists", host)
	}

	if _, ok := a.schemaMain.ACLMap[host][source]; !ok {
		return fmt.Errorf("authSchema: no such fromnode=%v to delete on in schema for node=%v exists", source, host)
	}

	delete(a.schemaMain.ACLMap[host], source)

	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: aclNodeFromnodeDelete: %v", err)
		log.Printf("%v\n", er)
	}

	return nil
}

// generateACLsForAllNodes will generate a json encoded representation of the node specific
// map values of authSchema, along with a hash of the data.
//
// Will range over all the host elements defined in the ACL, create a new authParser for each one,
// and run a small state machine on each element to create the final ACL result to be used at host
// nodes.
// The result will be written to the schemaGenerated.ACLsToConvert map.
func (a *accessLists) generateACLsForAllNodes() error {
	a.schemaGenerated.mu.Lock()
	defer a.schemaGenerated.mu.Unlock()

	a.schemaGenerated.ACLsToConvert = make(map[Node]map[Node]map[command]struct{})

	// Rangle all ACL's. Both for single hosts, and group of hosts.
	// ACL's that are for a group of hosts will be generated split
	// out in it's indivial host name, and that current ACL will
	// be added to the individual host in the ACLsToConvert map to
	// built a complete picture of what the ACL's looks like for each
	// individual hosts.
	for n := range a.schemaMain.ACLMap {
		//a.schemaGenerated.ACLsToConvert = make(map[node]map[node]map[command]struct{})
		ap := newAuthParser(n, a)
		ap.parse()
	}

	inf := fmt.Errorf("generateACLsFor all nodes, ACLsToConvert contains: %#v", a.schemaGenerated.ACLsToConvert)
	a.errorKernel.logConsoleOnlyIfDebug(inf, a.configuration)

	// ACLsToConvert got the complete picture of what ACL's that
	// are defined for each individual host node.
	// Range this map, and generate a JSON representation of all
	// the ACL's each host.
	func() {
		// If the map to generate from map is empty we want to also set the generatedACLsMap
		// to empty so we can make sure that no more generated ACL's exists to be distributed.
		if len(a.schemaGenerated.ACLsToConvert) == 0 {
			a.schemaGenerated.GeneratedACLsMap = make(map[Node]HostACLsSerializedWithHash)

		}

		for n, m := range a.schemaGenerated.ACLsToConvert {
			//fmt.Printf("\n ################ DEBUG: RANGE in generate: n=%v, m=%v\n", n, m)

			// cbor marshal the data of the ACL map to store for the host node.
			cb, err := cbor.Marshal(m)
			if err != nil {
				er := fmt.Errorf("error: failed to generate json for host in schemaGenerated: %v", err)
				log.Printf("%v\n", er)
				os.Exit(1)
			}

			// Create the hash for the data for the host node.
			hash := func() [32]byte {
				sns := a.nodeMapToSlice(n)

				b, err := cbor.Marshal(sns)
				if err != nil {
					err := fmt.Errorf("error: authSchema, json for hash:  %v", err)
					log.Printf("%v\n", err)
					return [32]byte{}
				}

				hash := sha256.Sum256(b)
				return hash
			}()

			// Store both the cbor marshaled data and the hash in a structure.
			hostSerialized := HostACLsSerializedWithHash{
				Data: cb,
				Hash: hash,
			}

			// and then store the cbor encoded data and the hash in the generated map.
			a.schemaGenerated.GeneratedACLsMap[n] = hostSerialized

		}
	}()

	inf = fmt.Errorf("generateACLsFor all nodes, GeneratedACLsMap contains: %#v", a.schemaGenerated.GeneratedACLsMap)
	a.errorKernel.logConsoleOnlyIfDebug(inf, a.configuration)

	return nil
}

// sourceNode is used to convert the ACL map structure of a host into a slice,
// and we then use the slice representation of the ACL to create the hash for
// a specific host node.
type sourceNode struct {
	HostNode       Node
	SourceCommands []sourceNodeCommands
}

// sourceNodeCommand is used to convert the ACL map structure of a host into a slice,
// and we then use the slice representation of the ACL to create the hash for
// a specific host node.
type sourceNodeCommands struct {
	Source   Node
	Commands []command
}

// nodeMapToSlice will return a sourceNode structure, with the map sourceNode part
// of the map converted into a slice. Both the from node, and the commands
// defined for each sourceNode are sorted.
// This function is used when creating the hash of the nodeMap since we can not
// guarantee the order of a hash map, but we can with a slice.
func (a *accessLists) nodeMapToSlice(host Node) sourceNode {
	srcNodes := sourceNode{
		HostNode: host,
	}

	for sn, commandMap := range a.schemaGenerated.ACLsToConvert[host] {
		srcC := sourceNodeCommands{
			Source: sn,
		}

		for cmd := range commandMap {
			srcC.Commands = append(srcC.Commands, cmd)
		}

		// Sort all the commands.
		sort.SliceStable(srcC.Commands, func(i, j int) bool {
			return srcC.Commands[i] < srcC.Commands[j]
		})

		srcNodes.SourceCommands = append(srcNodes.SourceCommands, srcC)
	}

	// Sort all the source nodes.
	sort.SliceStable(srcNodes.SourceCommands, func(i, j int) bool {
		return srcNodes.SourceCommands[i].Source < srcNodes.SourceCommands[j].Source
	})

	// fmt.Printf(" * nodeMapToSlice: fromNodes: %#v\n", fns)

	return srcNodes
}

// groupNodesAddNode adds a node to a group. If the group does
// not exist it will be created.
func (a *accessLists) groupNodesAddNode(ng nodeGroup, n Node) {
	err := a.validator.Var(ng, "startswith=grp_nodes_")
	if err != nil {
		log.Printf("error: group name do not start with grp_nodes_: %v\n", err)
		return
	}

	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.NodeGroupMap[ng]; !ok {
		a.schemaMain.NodeGroupMap[ng] = make(map[Node]struct{})
	}

	a.schemaMain.NodeGroupMap[ng][n] = struct{}{}

	// fmt.Printf(" * groupNodesAddNode: After adding to group node looks like: %+v\n", a.schemaMain.NodeGroupMap)

	err = a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: groupNodesAddNode: %v", err)
		log.Printf("%v\n", er)
	}

}

// groupNodesDeleteNode deletes a node from a group in the map.
func (a *accessLists) groupNodesDeleteNode(ng nodeGroup, n Node) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.NodeGroupMap[ng][n]; !ok {
		log.Printf("info: no such node with name=%v found in group=%v\n", ng, n)
		return
	}

	delete(a.schemaMain.NodeGroupMap[ng], n)

	//fmt.Printf(" * After deleting nodeGroup map looks like: %+v\n", a.schemaMain.NodeGroupMap)

	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: groupNodesDeleteNode: %v", err)
		log.Printf("%v\n", er)
	}

}

// groupNodesDeleteGroup deletes a nodeGroup from map.
func (a *accessLists) groupNodesDeleteGroup(ng nodeGroup) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.NodeGroupMap[ng]; !ok {
		log.Printf("info: no such group found: %v\n", ng)
		return
	}

	delete(a.schemaMain.NodeGroupMap, ng)

	//fmt.Printf(" * After deleting nodeGroup map looks like: %+v\n", a.schemaMain.NodeGroupMap)

	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: groupNodesDeleteGroup: %v", err)
		log.Printf("%v\n", er)
	}

}

// -----

// groupCommandsAddCommand adds a command to a group. If the group does
// not exist it will be created.
func (a *accessLists) groupCommandsAddCommand(cg commandGroup, c command) {
	err := a.validator.Var(cg, "startswith=grp_commands_")
	if err != nil {
		log.Printf("error: group name do not start with grp_commands_ : %v\n", err)
		return
	}

	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.CommandGroupMap[cg]; !ok {
		a.schemaMain.CommandGroupMap[cg] = make(map[command]struct{})
	}

	a.schemaMain.CommandGroupMap[cg][c] = struct{}{}

	//fmt.Printf(" * groupCommandsAddCommand: After adding command=%v to command group=%v map looks like: %+v\n", c, cg, a.schemaMain.CommandGroupMap)

	err = a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: groupCommandsAddCommand: %v", err)
		log.Printf("%v\n", er)
	}

}

// groupCommandsDeleteCommand deletes a command from a group in the map.
func (a *accessLists) groupCommandsDeleteCommand(cg commandGroup, c command) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.CommandGroupMap[cg][c]; !ok {
		log.Printf("info: no such command with name=%v found in group=%v\n", c, cg)
		return
	}

	delete(a.schemaMain.CommandGroupMap[cg], c)

	//fmt.Printf(" * After deleting command=%v from group=%v map looks like: %+v\n", c, cg, a.schemaMain.CommandGroupMap)

	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: groupCommandsDeleteCommand: %v", err)
		log.Printf("%v\n", er)
	}

}

// groupCommandDeleteGroup deletes a commandGroup map.
func (a *accessLists) groupCommandDeleteGroup(cg commandGroup) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.CommandGroupMap[cg]; !ok {
		log.Printf("info: no such group found: %v\n", cg)
		return
	}

	delete(a.schemaMain.CommandGroupMap, cg)

	//fmt.Printf(" * After deleting commandGroup=%v map looks like: %+v\n", cg, a.schemaMain.CommandGroupMap)

	err := a.generateACLsForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: groupCommandDeleteGroup: %v", err)
		log.Printf("%v\n", er)
	}

}

// exportACLs will export the current content of the main ACLMap in JSON format.
func (a *accessLists) exportACLs() ([]byte, error) {

	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()

	js, err := json.Marshal(a.schemaMain.ACLMap)
	if err != nil {
		return nil, fmt.Errorf("error: failed to marshal schemaMain.ACLMap: %v", err)

	}

	return js, nil

}

// importACLs will import and replace all current ACL's with the ACL's provided as input.
func (a *accessLists) importACLs(js []byte) error {

	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()

	m := make(map[Node]map[Node]map[command]struct{})

	err := json.Unmarshal(js, &m)
	if err != nil {
		return fmt.Errorf("error: failed to unmarshal into ACLMap: %v", err)
	}

	a.schemaMain.ACLMap = m

	return nil

}
