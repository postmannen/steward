package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"sync"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// Set the default logging functionality of the package to false.
var logging = flag.Bool("logging", true, "set to true to enable the normal logger of the package")

func TestACLSingleNode(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth()
	c.authorization.authSchema.aclAdd("ship101", "admin", "HORSE")
	c.authorization.authSchema.aclAdd("ship101", "admin", "PIG")

	// --- TESTS ---

	mapOfFromNodeCommands := make(map[node]map[command]struct{})
	err := cbor.Unmarshal(c.authorization.authSchema.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := mapOfFromNodeCommands["admin"]["HORSE"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["admin"]["PIG"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}
}

func TestACLWithGroups(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth()

	const (
		grp_nodes_operators      = "grp_nodes_operators"
		grp_nodes_ships          = "grp_nodes_ships"
		grp_commands_commandset1 = "grp_commands_commandset1"
	)

	c.authorization.authSchema.groupNodesAddNode(grp_nodes_operators, "operator1")
	c.authorization.authSchema.groupNodesAddNode(grp_nodes_operators, "operator2")

	c.authorization.authSchema.groupNodesAddNode(grp_nodes_ships, "ship100")
	c.authorization.authSchema.groupNodesAddNode(grp_nodes_ships, "ship101")

	c.authorization.authSchema.groupCommandsAddCommand(grp_commands_commandset1, "dmesg")
	c.authorization.authSchema.groupCommandsAddCommand(grp_commands_commandset1, "date")

	c.authorization.authSchema.aclAdd(grp_nodes_ships, "admin", "useradd -m kongen")
	c.authorization.authSchema.aclAdd("ship101", "admin", "HORSE")

	c.authorization.authSchema.aclAdd(grp_nodes_ships, grp_nodes_operators, grp_commands_commandset1)

	mapOfFromNodeCommands := make(map[node]map[command]struct{})
	err := cbor.Unmarshal(c.authorization.authSchema.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := mapOfFromNodeCommands["admin"]["useradd -m kongen"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["operator1"]["dmesg"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["operator1"]["date"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["operator2"]["dmesg"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["operator2"]["date"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["admin"]["HORSE"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

}

func TestACLGenerated(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth()

	c.authorization.authSchema.aclAdd("ship101", "admin", "HORSE")

	c.authorization.authSchema.groupNodesAddNode("grp_nodes_ships", "ship101")
	c.authorization.authSchema.aclAdd("grp_nodes_ships", "admin", "HEN")

	c.authorization.authSchema.groupCommandsAddCommand("grp_commands_test", "echo")
	c.authorization.authSchema.groupCommandsAddCommand("grp_commands_test", "dmesg")
	c.authorization.authSchema.aclAdd("grp_nodes_ships", "admin", "grp_commands_test")

	c.authorization.authSchema.groupCommandsDeleteCommand("grp_commands_test", "echo")

	// --- TESTS ---

	mapOfFromNodeCommands := make(map[node]map[command]struct{})
	err := cbor.Unmarshal(c.authorization.authSchema.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
	if err != nil {
		t.Fatal(err)
	}

	//if _, ok := mapOfFromNodeCommands["admin"]["PIG"]; !ok {
	//	t.Fatalf(" \U0001F631  [FAILED]: missing map entry: PIG: Content of Map: %v", mapOfFromNodeCommands)
	//}

	if _, ok := mapOfFromNodeCommands["admin"]["HORSE"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: HORSE: Content of Map: %v", mapOfFromNodeCommands)
	}

	if _, ok := mapOfFromNodeCommands["admin"]["HEN"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: HEN: Content of Map: %v", mapOfFromNodeCommands)
	}

	if _, ok := mapOfFromNodeCommands["admin"]["echo"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: should not contain map entry: echo: Content of Map: %v", mapOfFromNodeCommands)
	}

	if _, ok := mapOfFromNodeCommands["admin"]["dmesg"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: echo: Content of Map: %v", mapOfFromNodeCommands)

	}

}

func TestACLSchemaMainACLMap(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth()

	//c.authorization.authSchema.aclNodeFromnodeCommandAdd("ship101", "admin", "PIG")
	// fmt.Printf("---------------ADDING COMMAND-------------\n")
	c.authorization.authSchema.aclAdd("ship0", "admin", "systemctl")
	c.authorization.authSchema.aclAdd("ship1", "admin", "tcpdump")

	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship0"]["admin"]["systemctl"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship0, admin, systemctl")
	}
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship1"]["admin"]["tcpdump"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------ADDING COMMAND-------------\n")
	c.authorization.authSchema.groupNodesAddNode("grp_nodes_ships", "ship1")
	c.authorization.authSchema.groupNodesAddNode("grp_nodes_ships", "ship2")
	c.authorization.authSchema.aclAdd("grp_nodes_ships", "admin", "dmesg")

	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["grp_nodes_ships"]["admin"]["dmesg"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------ADDING COMMAND-------------\n")
	c.authorization.authSchema.aclAdd("ship2", "admin", "echo")

	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------DELETING COMMAND grp_nodes_ships, admin, dmesg-------------\n")
	c.authorization.authSchema.aclDeleteCommand("grp_nodes_ships", "admin", "dmesg")

	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["grp_nodes_ships"]["admin"]["dmesg"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: found map entry: grp_nodes_ships, admin, dmesg")
	}
	// Check that the remaining are still ok.
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship0"]["admin"]["systemctl"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship0, admin, systemctl")
	}
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship1"]["admin"]["tcpdump"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------DELETING COMMAND ship0, admin, systemctl-------------\n")
	c.authorization.authSchema.aclDeleteCommand("ship0", "admin", "systemctl")

	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship0"]["admin"]["systemctl"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship0, admin, systemctl")
	}
	// Check that the remaining are ok.
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship1"]["admin"]["tcpdump"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------DELETING SOURCE ship1, admin-------------\n")
	c.authorization.authSchema.aclDeleteSource("ship1", "admin")

	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship1"]["admin"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	// Check that the remaining are ok.
	if _, ok := c.authorization.authSchema.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	// --- TESTS ---
}

func TestACLHash(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth()

	c.authorization.authSchema.aclAdd("ship101", "admin", "HORSE")

	c.authorization.authSchema.groupNodesAddNode("grp_nodes_ships", "ship101")
	c.authorization.authSchema.aclAdd("grp_nodes_ships", "admin", "HEN")

	hash := [32]uint8{0xa4, 0x99, 0xbd, 0xa3, 0x18, 0x26, 0x52, 0xc2, 0x92, 0x60, 0x23, 0x19, 0x3c, 0xa, 0x7, 0xa9, 0xb7, 0x77, 0x4f, 0x11, 0x34, 0xd5, 0x2d, 0xd1, 0x8d, 0xab, 0x6c, 0x4b, 0x2, 0xfa, 0x5c, 0x7a}
	value := c.authorization.authSchema.schemaGenerated.GeneratedACLsMap["ship101"].Hash
	// fmt.Printf("%#v\n", c.authorization.authSchema.schemaGenerated.GeneratedACLsMap["ship101"].Hash)

	if bytes.Equal(hash[:], value[:]) == false {
		t.Fatalf(" \U0001F631  [FAILED]: hash mismatch")
	}
}

func TestACLConcurrent(t *testing.T) {
	c := newCentralAuth()

	// -----------General testing and creation of some data----------------

	// Start concurrent updating of the schema.
	var wg sync.WaitGroup
	for i := 0; i < 4000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.authorization.authSchema.aclAdd("ship1", "operator2", "rm -rf")
			c.authorization.authSchema.aclAdd("ship1", "operator1", "ls -lt")
			c.authorization.authSchema.aclAdd("ship1", "operator1", "ls -lt")
			c.authorization.authSchema.aclAdd("ship1", "operator2", "ls -l")
			c.authorization.authSchema.aclAdd("ship3", "operator3", "ls -lt")
			c.authorization.authSchema.aclAdd("ship3", "operator3", "vi /etc/hostname")
			c.authorization.authSchema.aclDeleteCommand("ship3", "operator2", "ls -lt")
			c.authorization.authSchema.aclDeleteSource("ship3", "operator3")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			// fmt.Println("----schemaMain------")
			c.authorization.authSchema.schemaMain.mu.Lock()
			for _, v := range c.authorization.authSchema.schemaMain.ACLMap {
				_ = fmt.Sprintf("%+v\n", v)
			}
			c.authorization.authSchema.schemaMain.mu.Unlock()

			// fmt.Println("----schemaGenerated------")
			c.authorization.authSchema.schemaGenerated.mu.Lock()
			for k, v := range c.authorization.authSchema.schemaGenerated.GeneratedACLsMap {
				_ = fmt.Sprintf("node: %v, NodeDataSerialized: %v\n", k, string(v.Data))
				_ = fmt.Sprintf("node: %v, Hash: %v\n", k, v.Hash)
			}
			c.authorization.authSchema.schemaGenerated.mu.Unlock()
		}()
	}
	wg.Wait()
}
