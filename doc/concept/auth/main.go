package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-playground/validator/v10"
)

// centralAuth
type centralAuth struct {
	authorization *authorization
}

// newCentralAuth
func newCentralAuth() *centralAuth {
	c := centralAuth{
		authorization: newAuthorization(),
	}

	return &c
}

// --------------------------------------

type authorization struct {
	authSchema *authSchema
}

func newAuthorization() *authorization {
	a := authorization{
		authSchema: newAuthSchema(),
	}

	return &a
}

// authSchema holds both the main schema to update by operators,
// and also the indvidual node generated data based on the main schema.
type authSchema struct {
	//         node      fromNode   commands
	schemaMain      *schemaMain
	schemaGenerated *schemaGenerated
	validator       *validator.Validate
}

type node string
type command string
type nodeGroup string
type commandGroup string

type schemaMain struct {
	ACLMap          map[node]map[node]map[command]struct{}
	NodeGroupMap    map[nodeGroup]map[node]struct{}
	CommandGroupMap map[commandGroup]map[command]struct{}
	mu              sync.Mutex
}

func newSchemaMain() *schemaMain {
	s := schemaMain{
		ACLMap:          make(map[node]map[node]map[command]struct{}),
		NodeGroupMap:    make(map[nodeGroup]map[node]struct{}),
		CommandGroupMap: make(map[commandGroup]map[command]struct{}),
	}
	return &s
}

type schemaGenerated struct {
	ACLsToConvert map[node]map[node]map[command]struct{}
	NodeMap       map[node]NodeDataWithHash
	mu            sync.Mutex
}

func newSchemaGenerated() *schemaGenerated {
	s := schemaGenerated{
		ACLsToConvert: map[node]map[node]map[command]struct{}{},
		NodeMap:       make(map[node]NodeDataWithHash),
	}
	return &s
}

func newAuthSchema() *authSchema {
	a := authSchema{
		schemaMain:      newSchemaMain(),
		schemaGenerated: newSchemaGenerated(),
		validator:       validator.New(),
	}

	return &a
}

// NodeDataWithHash is the serialized representation node specific value in the authSchema.
// There is also a sha256 hash of the data.
type NodeDataWithHash struct {
	// data is all the auth data for a specific node encoded in json.
	Data []byte
	// hash is the sha256 hash of the data.
	Hash [32]byte
}

func (a *authSchema) convToActualNodeSlice(n node) []node {
	nodes := []node{}

	// Check if we are given a nodeGroup variable, and if we are, get all the
	// nodes for that group.
	if strings.HasPrefix(string(n), "grp_nodes_") {
		for nd := range a.schemaMain.NodeGroupMap[nodeGroup(n)] {
			nodes = append(nodes, nd)
		}
	} else {
		// No group found meaning a single node was given as an argument.
		nodes = []node{n}
	}

	return nodes
}

// convertToActualCommandSlice will convert the given argument into a slice representation.
// If the argument is a group, then all the members of that group will be expanded into
// the slice.
// If the argument is not a group kind of value, then only a slice with that single
// value is returned.
func (a *authSchema) convertToActualCommandSlice(c command) []command {
	commands := []command{}

	// Check if we are given a nodeGroup variable, and if we are, get all the
	// nodes for that group.
	if strings.HasPrefix(string(c), "grp_cmds_") {
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

// aclAdd will add a command for a fromNode.
// If the node or the fromNode do not exist they will be created.
// The json encoded schema for a node and the hash of those data
// will also be generated.
func (a *authSchema) aclAdd(host node, source node, cmd command) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()

	// Check if node exists in map.
	if _, ok := a.schemaMain.ACLMap[host]; !ok {
		// log.Printf("info: did not find node=%v in map, creating map[fromnode]map[command]struct{}\n", n)
		a.schemaMain.ACLMap[host] = make(map[node]map[command]struct{})
	}

	// Check if also source node exists in map
	if _, ok := a.schemaMain.ACLMap[host][source]; !ok {
		// log.Printf("info: did not find node=%v in map, creating map[fromnode]map[command]struct{}\n", fn)
		a.schemaMain.ACLMap[host][source] = make(map[command]struct{})
	}

	a.schemaMain.ACLMap[host][source][cmd] = struct{}{}
	// err := a.generateJSONForHostOrGroup(n)
	err := a.generateJSONForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: addCommandForFromNode: %v", err)
		log.Printf("%v\n", er)
	}

	// fmt.Printf(" * DEBUG: aclNodeFromnodeCommandAdd: a.schemaMain.ACLMap=%v\n", a.schemaMain.ACLMap)

}

// aclDeleteCommand will delete the specified command from the fromnode.
func (a *authSchema) aclDeleteCommand(host node, source node, cmd command) error {
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

	err := a.generateJSONForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: aclNodeFromNodeCommandDelete: %v", err)
		log.Printf("%v\n", er)
	}

	return nil
}

// aclDeleteSource will delete specified source node and all commands specified for it.
func (a *authSchema) aclDeleteSource(host node, source node) error {
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

	err := a.generateJSONForAllNodes()
	if err != nil {
		er := fmt.Errorf("error: aclNodeFromnodeDelete: %v", err)
		log.Printf("%v\n", er)
	}

	return nil
}

// generateJSONForAllNodes will generate a json encoded representation of the node specific
// map values of authSchema, along with a hash of the data.
//
// Will range over all the host elements defined in the ACL, create a new authParser for each one,
// and run a small state machine on each element to create the final ACL result to be used at host
// nodes.
// The result will be written to the schemaGenerated.ACLsToConvert map.
func (a *authSchema) generateJSONForAllNodes() error {
	a.schemaGenerated.ACLsToConvert = make(map[node]map[node]map[command]struct{})

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

	// ACLsToConvert got the complete picture of what ACL's that
	// are defined for each individual host node.
	// Range this map, and generate a JSON representation of all
	// the ACL's each host.
	func() {
		for n, m := range a.schemaGenerated.ACLsToConvert {

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
			nd := NodeDataWithHash{
				Data: cb,
				Hash: hash,
			}

			// and then store the cbor encoded data and the hash in the generated map.
			a.schemaGenerated.NodeMap[n] = nd

		}
	}()

	return nil
}

type sourceNodes struct {
	Node           node
	SourceCommands []sourceCommands
}

type sourceCommands struct {
	Source   node
	Commands []command
}

// nodeMapToSlice will return a fromNode structure, with the map fromNode part
// of the map converted into a slice. Both the from node, and the commands
// defined for each fromNode are sorted.
// This function is used when creating the hash of the nodeMap since we can not
// guarantee the order of a hash map, but we can with a slice.
func (a *authSchema) nodeMapToSlice(host node) sourceNodes {
	srcNodes := sourceNodes{
		Node: host,
	}

	for sn, commandMap := range a.schemaGenerated.ACLsToConvert[host] {
		srcC := sourceCommands{
			Source: sn,
		}

		for cmd := range commandMap {
			srcC.Commands = append(srcC.Commands, cmd)
		}

		// sort.Strings(fnc.Commands)
		sort.SliceStable(srcC.Commands, func(i, j int) bool {
			return srcC.Commands[i] < srcC.Commands[j]
		})

		srcNodes.SourceCommands = append(srcNodes.SourceCommands, srcC)
	}

	sort.SliceStable(srcNodes.SourceCommands, func(i, j int) bool {
		return srcNodes.SourceCommands[i].Source < srcNodes.SourceCommands[j].Source
	})

	// fmt.Printf(" * nodeMapToSlice: fromNodes: %#v\n", fns)

	return srcNodes
}

// groupNodesAddNode adds a node to a group. If the group does
// not exist it will be created.
func (a *authSchema) groupNodesAddNode(ng nodeGroup, n node) {
	err := a.validator.Var(ng, "startswith=grp_nodes_")
	if err != nil {
		log.Printf("error: group name do not start with grp_nodes_: %v\n", err)
		return
	}

	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.NodeGroupMap[ng]; !ok {
		a.schemaMain.NodeGroupMap[ng] = make(map[node]struct{})
	}

	a.schemaMain.NodeGroupMap[ng][n] = struct{}{}

	// fmt.Printf(" * groupNodesAddNode: After adding to group node looks like: %+v\n", a.schemaMain.NodeGroupMap)

}

// groupNodesDeleteNode deletes a node from a group in the map.
func (a *authSchema) groupNodesDeleteNode(ng nodeGroup, n node) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.NodeGroupMap[ng][n]; !ok {
		log.Printf("info: no such node with name=%v found in group=%v\n", ng, n)
		return
	}

	delete(a.schemaMain.NodeGroupMap[ng], n)

	//fmt.Printf(" * After deleting nodeGroup map looks like: %+v\n", a.schemaMain.NodeGroupMap)

}

// groupNodesDeleteGroup deletes a nodeGroup from map.
func (a *authSchema) groupNodesDeleteGroup(ng nodeGroup) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.NodeGroupMap[ng]; !ok {
		log.Printf("info: no such group found: %v\n", ng)
		return
	}

	delete(a.schemaMain.NodeGroupMap, ng)

	//fmt.Printf(" * After deleting nodeGroup map looks like: %+v\n", a.schemaMain.NodeGroupMap)

}

// -----

// groupCommandsAddCommand adds a command to a group. If the group does
// not exist it will be created.
func (a *authSchema) groupCommandsAddCommand(cg commandGroup, c command) {
	err := a.validator.Var(cg, "startswith=grp_cmds_")
	if err != nil {
		log.Printf("error: group name do not start with grp_cmds_ : %v\n", err)
		return
	}

	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.CommandGroupMap[cg]; !ok {
		a.schemaMain.CommandGroupMap[cg] = make(map[command]struct{})
	}

	a.schemaMain.CommandGroupMap[cg][c] = struct{}{}

	//fmt.Printf(" * groupCommandsAddCommand: After adding command=%v to command group=%v map looks like: %+v\n", c, cg, a.schemaMain.CommandGroupMap)

}

// groupCommandsDeleteCommand deletes a command from a group in the map.
func (a *authSchema) groupCommandsDeleteCommand(cg commandGroup, c command) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.CommandGroupMap[cg][c]; !ok {
		log.Printf("info: no such command with name=%v found in group=%v\n", c, cg)
		return
	}

	delete(a.schemaMain.CommandGroupMap[cg], c)

	//fmt.Printf(" * After deleting command=%v from group=%v map looks like: %+v\n", c, cg, a.schemaMain.CommandGroupMap)

}

// groupCommandDeleteGroup deletes a commandGroup map.
func (a *authSchema) groupCommandDeleteGroup(cg commandGroup) {
	a.schemaMain.mu.Lock()
	defer a.schemaMain.mu.Unlock()
	if _, ok := a.schemaMain.CommandGroupMap[cg]; !ok {
		log.Printf("info: no such group found: %v\n", cg)
		return
	}

	delete(a.schemaMain.CommandGroupMap, cg)

	//fmt.Printf(" * After deleting commandGroup=%v map looks like: %+v\n", cg, a.schemaMain.CommandGroupMap)

}

// printMaps will print the auth maps for testing output.
func (c *centralAuth) printMaps() {
	{
		fmt.Println("\n-----------------PRINTING OUT MAPS------------------------")

		fmt.Println("----schemaMain------")
		c.authorization.authSchema.schemaMain.mu.Lock()
		for k, v := range c.authorization.authSchema.schemaMain.ACLMap {
			fmt.Printf("%v: %+v\n", k, v)
		}
		c.authorization.authSchema.schemaMain.mu.Unlock()

		fmt.Println("----schemaGenerated------")
		c.authorization.authSchema.schemaGenerated.mu.Lock()
		for k, v := range c.authorization.authSchema.schemaGenerated.NodeMap {
			fmt.Printf("node: %v, NodeDataSerialized: %v\n", k, string(v.Data))
			fmt.Printf("node: %v, Hash: %v\n", k, v.Hash)
		}
		c.authorization.authSchema.schemaGenerated.mu.Unlock()
	}
	fmt.Println("-----------------END OF PRINTING OUT MAPS------------------------")
	fmt.Println()
}

func main() {
	// c := newCentralAuth()

	// -----------General testing and creation of some data----------------

	// // Start concurrent updating of the schema.
	// var wg sync.WaitGroup
	// for i := 0; i < 1; i++ {
	// 	wg.Add(1)
	// 	go func() {
	// 		defer wg.Done()
	// 		c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship1", "operator2", "rm -rf")
	// 		c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship1", "operator1", "ls -lt")
	// 		c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship1", "operator1", "ls -lt")
	// 		c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship1", "operator2", "ls -l")
	// 		c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship3", "operator3", "ls -lt")
	// 		c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship3", "operator3", "vi /etc/hostname")
	// 		c.authorization.authSchema.aclNodeFromNodeCommandDelete("ship3", "operator2", "ls -lt")
	// 		c.authorization.authSchema.aclNodeFromnodeDelete("ship3", "operator3")
	// 	}()
	//
	// 	wg.Add(1)
	// 	go func() {
	// 		defer wg.Done()
	// 		fmt.Println("----schemaMain------")
	// 		c.authorization.authSchema.schemaMain.mu.Lock()
	// 		for _, v := range c.authorization.authSchema.schemaMain.NodeMap {
	// 			fmt.Printf("%+v\n", v)
	// 		}
	// 		c.authorization.authSchema.schemaMain.mu.Unlock()
	//
	// 		fmt.Println("----schemaGenerated------")
	// 		c.authorization.authSchema.schemaGenerated.mu.Lock()
	// 		for k, v := range c.authorization.authSchema.schemaGenerated.NodeMap {
	// 			fmt.Printf("node: %v, NodeDataSerialized: %v\n", k, string(v.Data))
	// 			fmt.Printf("node: %v, Hash: %v\n", k, v.Hash)
	// 		}
	// 		c.authorization.authSchema.schemaGenerated.mu.Unlock()
	// 	}()
	// }
	// wg.Wait()
	// c.printMaps()

	// // --------- Testing as needed for request types--------------
	// fmt.Println("--------- Testing as needed for request types--------------")
	//
	// // Get the hash for a node schema
	// {
	// 	c.authorization.authSchema.schemaGenerated.mu.Lock()
	// 	h := c.authorization.authSchema.schemaGenerated.NodeMap["ship1"].Hash
	// 	c.authorization.authSchema.schemaGenerated.mu.Unlock()
	//
	// 	fmt.Printf("testing: hash for ship1 = %v\n", h)
	// }

	// // Generate json to send to node in message with nodeMap and hash
	// {
	// 	var tmpData map[string]map[string]struct{}
	// 	nodeName := node("ship1")
	// 	err := json.Unmarshal(c.authorization.authSchema.schemaGenerated.NodeMap[nodeName].Data, &tmpData)
	// 	if err != nil {
	// 		log.Printf("error: failed to unmarshal schemaGenerated.NodeMap: %v\n", err)
	// 		os.Exit(1)
	// 	}
	//
	// 	fmt.Printf("\n * Data for %v: %#v\n", nodeName, tmpData)
	//
	// }

	// // Test node group.
	// {
	// 	fmt.Println("\n--------------Group testing----------------")
	// 	fmt.Println(" * ADDING NODES TO GROUPS")
	// 	c.authorization.authSchema.groupNodesAddNode("grp_nodes_GROUP_A", "ship1")
	// 	c.authorization.authSchema.groupNodesAddNode("grp_nodes_GROUP_A", "ship2")
	// 	c.authorization.authSchema.groupNodesAddNode("grp_nodes_GROUP_A", "ship3")
	// 	c.authorization.authSchema.groupNodesAddNode("grp_nodes_GROUP_B", "ship2")
	// 	c.authorization.authSchema.groupNodesAddNode("grp_nodes_GROUP_C", "ship3")
	//
	// 	fmt.Println("\n * Deleting  NODES FROM GROUPS")
	// 	c.authorization.authSchema.groupNodesDeleteNode("grp_nodes_GROUP_A", "ship2")
	//
	// 	fmt.Println("\n * Deleting  NODE GROUPS")
	// 	c.authorization.authSchema.groupNodesDeleteGroup("grp_nodes_GROUP_B")
	//
	// 	// -----
	//
	// 	fmt.Println("\n * ADDING COMMANDS TO COMMAND GROUPS")
	// 	c.authorization.authSchema.groupCommandsAddCommand("grp_cmds_C_GROUP_A", "echo apekatt")
	// 	c.authorization.authSchema.groupCommandsAddCommand("grp_cmds_C_GROUP_A", "ls -lt")
	// 	c.authorization.authSchema.groupCommandsAddCommand("grp_cmds_C_GROUP_A", "rm -rf")
	// 	c.authorization.authSchema.groupCommandsAddCommand("grp_cmds_C_GROUP_B", "df -ah")
	// 	c.authorization.authSchema.groupCommandsAddCommand("grp_cmds_C_GROUP_C", "cat /etc/hosts")
	//
	// 	fmt.Println("\n * Deleting  COMMANDS FROM GROUPS")
	// 	c.authorization.authSchema.groupCommandsDeleteCommand("grp_cmds_C_GROUP_A", "ls -lt")
	//
	// 	fmt.Println("\n * Deleting  COMMAND GROUP")
	// 	c.authorization.authSchema.groupCommandDeleteGroup("grp_cmds_C_GROUP_B")
	// }

	//{
	//	fmt.Println("\n --------- GROUP, add nodes to a group, and apply acl's --------")
	//	c.authorization.authSchema.groupNodesAddNode("grp_nodes_operators", "operator1")
	//	c.authorization.authSchema.groupNodesAddNode("grp_nodes_operators", "operator2")
	//
	//	c.authorization.authSchema.groupNodesAddNode("grp_nodes_group100", "ship100")
	//	c.authorization.authSchema.groupNodesAddNode("grp_nodes_group100", "ship101")
	//	c.authorization.authSchema.groupNodesAddNode("grp_nodes_group100", "ship102")
	//
	//	c.authorization.authSchema.groupCommandsAddCommand("grp_cmds_1", "dmesg")
	//
	//	// Also add a non group user
	//	c.authorization.authSchema.aclCommandAdd("grp_nodes_group100", "admin", "useradd -m kongen")
	//	// c.authorization.authSchema.aclNodeFromnodeCommandAdd("grp_nodes_group100", "operator1", "useradd -m dmesg")
	//	// c.authorization.authSchema.aclNodeFromnodeCommandAdd("grp_nodes_group100", "grp_nodes_operators", "dmesg")
	//	c.authorization.authSchema.aclCommandAdd("grp_nodes_group100", "grp_nodes_operators", "grp_cmds_1")
	//	fmt.Println("\n --------- END OF GROUP NODES ADD NODE TESTING ----------")
	//}

	//c.printMaps()

	// {
	// 	fmt.Println("\n --------- GROUP, on all nodes, delete the command for operator1 --------")
	// 	c.authorization.authSchema.aclNodeFromNodeCommandDelete("grp_nodes_group100", "operator1", "useradd -m operator")
	//
	// 	fmt.Println("\n --------- END ----------")
	// }
	//
	// c.printMaps()

	// {
	// 	fmt.Println("\n --------- GROUP, on all nodes, delete fromnode operator1 --------")
	// 	c.authorization.authSchema.aclNodeFromnodeDelete("grp_nodes_group100", "operator1")
	//
	// 	fmt.Println("\n --------- END OF DELETE A FROMNODE ----------")
	// }
	//
	// c.printMaps()

	//{
	//	fmt.Println("\n --------- GROUP, delete node from group --------")
	//	c.authorization.authSchema.groupNodesDeleteNode("grp_nodes_group100", "ship100")
	//	c.authorization.authSchema.generateJSONForAllNodes()
	//
	//	fmt.Println("\n --------- END OF DELETE A FROMNODE ----------")
	//}
	//
	//c.printMaps()

}
