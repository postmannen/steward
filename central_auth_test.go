package steward

import (
	"fmt"
	"io"
	"log"
	"sync"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestACLSingleNode(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	c.aclAddCommand("ship101", "admin", "HORSE")
	c.aclAddCommand("ship101", "admin", "PIG")

	// --- TESTS ---

	mapOfFromNodeCommands := make(map[Node]map[command]struct{})
	err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := mapOfFromNodeCommands["admin"]["HORSE"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	if _, ok := mapOfFromNodeCommands["admin"]["PIG"]; !ok {
		t.Fatal(" \U0001F631  [FAILED]: missing map entry")
	}

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLSingleNode")
}

func TestACLWithGroups(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	const (
		grp_nodes_operators      = "grp_nodes_operators"
		grp_nodes_ships          = "grp_nodes_ships"
		grp_commands_commandset1 = "grp_commands_commandset1"
	)

	c.groupNodesAddNode(grp_nodes_operators, "operator1")
	c.groupNodesAddNode(grp_nodes_operators, "operator2")

	c.groupNodesAddNode(grp_nodes_ships, "ship100")
	c.groupNodesAddNode(grp_nodes_ships, "ship101")

	c.groupCommandsAddCommand(grp_commands_commandset1, "dmesg")
	c.groupCommandsAddCommand(grp_commands_commandset1, "date")

	c.aclAddCommand(grp_nodes_ships, "admin", "useradd -m kongen")
	c.aclAddCommand("ship101", "admin", "HORSE")

	c.aclAddCommand(grp_nodes_ships, grp_nodes_operators, grp_commands_commandset1)

	mapOfFromNodeCommands := make(map[Node]map[command]struct{})
	err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
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

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLWithGroups")

}

func TestACLNodesGroupDeleteNode(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	const (
		grp_nodes_operators      = "grp_nodes_operators"
		grp_nodes_ships          = "grp_nodes_ships"
		grp_commands_commandset1 = "grp_commands_commandset1"
	)

	c.groupNodesAddNode(grp_nodes_operators, "operator1")
	c.groupNodesAddNode(grp_nodes_operators, "operator2")

	c.groupNodesAddNode(grp_nodes_ships, "ship100")
	c.groupNodesAddNode(grp_nodes_ships, "ship101")

	c.groupCommandsAddCommand(grp_commands_commandset1, "dmesg")
	c.groupCommandsAddCommand(grp_commands_commandset1, "date")

	c.aclAddCommand(grp_nodes_ships, "admin", "useradd -m kongen")
	c.aclAddCommand("ship101", "admin", "HORSE")

	c.aclAddCommand(grp_nodes_ships, grp_nodes_operators, grp_commands_commandset1)

	c.groupNodesDeleteNode(grp_nodes_ships, "ship101")

	// Check that we still got the data for ship100.
	{
		mapOfFromNodeCommands := make(map[Node]map[command]struct{})
		err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship100"].Data, &mapOfFromNodeCommands)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := mapOfFromNodeCommands["admin"]["useradd -m kongen"]; !ok {
			t.Fatal(" \U0001F631  [FAILED]: missing map entry")
		}
	}

	// Check that we don't have any data for ship101.
	{
		mapOfFromNodeCommands := make(map[Node]map[command]struct{})
		err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := mapOfFromNodeCommands["admin"]["useradd -m kongen"]; ok {
			t.Fatal(" \U0001F631  [FAILED]: missing map entry")
		}
	}

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLNodesGroupDeleteNode")

}

func TestGroupNodesDeleteGroup(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	const (
		grp_nodes_operators      = "grp_nodes_operators"
		grp_nodes_ships          = "grp_nodes_ships"
		grp_commands_commandset1 = "grp_commands_commandset1"
	)

	c.groupNodesAddNode(grp_nodes_operators, "operator1")
	c.groupNodesAddNode(grp_nodes_operators, "operator2")

	c.groupNodesAddNode(grp_nodes_ships, "ship100")
	c.groupNodesAddNode(grp_nodes_ships, "ship101")

	c.groupCommandsAddCommand(grp_commands_commandset1, "dmesg")
	c.groupCommandsAddCommand(grp_commands_commandset1, "date")

	c.aclAddCommand(grp_nodes_ships, "admin", "useradd -m kongen")
	c.aclAddCommand("ship101", "admin", "HORSE")

	c.aclAddCommand(grp_nodes_ships, grp_nodes_operators, grp_commands_commandset1)

	c.groupNodesDeleteGroup(grp_nodes_operators)

	// Check that we still got the data for other ACL's.
	{
		mapOfFromNodeCommands := make(map[Node]map[command]struct{})
		err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := mapOfFromNodeCommands["admin"]["HORSE"]; !ok {
			t.Fatal(" \U0001F631  [FAILED]: missing map entry")
		}
	}

	// Check that we don't have any data for grp_nodes_operators
	{
		mapOfFromNodeCommands := make(map[Node]map[command]struct{})
		err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := mapOfFromNodeCommands["admin"]["dmesg"]; ok {
			t.Fatal(" \U0001F631  [FAILED]: foud map entry")
		}
	}

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestGroupNodesDeleteGroup")

}

func TestGroupCommandDeleteGroup(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	const (
		grp_nodes_operators      = "grp_nodes_operators"
		grp_nodes_ships          = "grp_nodes_ships"
		grp_commands_commandset1 = "grp_commands_commandset1"
	)

	c.groupNodesAddNode(grp_nodes_operators, "operator1")
	c.groupNodesAddNode(grp_nodes_operators, "operator2")

	c.groupNodesAddNode(grp_nodes_ships, "ship100")
	c.groupNodesAddNode(grp_nodes_ships, "ship101")

	c.groupCommandsAddCommand(grp_commands_commandset1, "dmesg")
	c.groupCommandsAddCommand(grp_commands_commandset1, "date")

	c.aclAddCommand(grp_nodes_ships, "admin", "useradd -m kongen")
	c.aclAddCommand("ship101", "admin", "HORSE")

	c.aclAddCommand(grp_nodes_ships, grp_nodes_operators, grp_commands_commandset1)

	c.groupCommandDeleteGroup(grp_commands_commandset1)

	// Check that we still got the data for other ACL's.
	{
		mapOfFromNodeCommands := make(map[Node]map[command]struct{})
		err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := mapOfFromNodeCommands["admin"]["HORSE"]; !ok {
			t.Fatal(" \U0001F631  [FAILED]: missing map entry")
		}
	}

	// Check that we don't have any data for grp_nodes_operators
	{
		mapOfFromNodeCommands := make(map[Node]map[command]struct{})
		err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := mapOfFromNodeCommands["admin"]["dmesg"]; ok {
			t.Fatal(" \U0001F631  [FAILED]: foud map entry")
		}
	}

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestGroupCommandDeleteGroup")

}

func TestACLGenerated(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	c.aclAddCommand("ship101", "admin", "HORSE")

	c.groupNodesAddNode("grp_nodes_ships", "ship101")
	c.aclAddCommand("grp_nodes_ships", "admin", "HEN")

	c.groupCommandsAddCommand("grp_commands_test", "echo")
	c.groupCommandsAddCommand("grp_commands_test", "dmesg")
	c.aclAddCommand("grp_nodes_ships", "admin", "grp_commands_test")

	c.groupCommandsDeleteCommand("grp_commands_test", "echo")

	// --- TESTS ---

	mapOfFromNodeCommands := make(map[Node]map[command]struct{})
	err := cbor.Unmarshal(c.accessLists.schemaGenerated.GeneratedACLsMap["ship101"].Data, &mapOfFromNodeCommands)
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

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLGenerated")

}

func TestACLSchemaMainACLMap(t *testing.T) {
	if !*logging {
		log.SetOutput(io.Discard)
	}

	c := newCentralAuth(tstConf, &errorKernel{})

	//a.aclNodeFromnodeCommandAdd("ship101", "admin", "PIG")
	// fmt.Printf("---------------ADDING COMMAND-------------\n")
	c.aclAddCommand("ship0", "admin", "systemctl")
	c.aclAddCommand("ship1", "admin", "tcpdump")

	if _, ok := c.accessLists.schemaMain.ACLMap["ship0"]["admin"]["systemctl"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship0, admin, systemctl")
	}
	if _, ok := c.accessLists.schemaMain.ACLMap["ship1"]["admin"]["tcpdump"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------ADDING COMMAND-------------\n")
	c.groupNodesAddNode("grp_nodes_ships", "ship1")
	c.groupNodesAddNode("grp_nodes_ships", "ship2")
	c.aclAddCommand("grp_nodes_ships", "admin", "dmesg")

	if _, ok := c.accessLists.schemaMain.ACLMap["grp_nodes_ships"]["admin"]["dmesg"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------ADDING COMMAND-------------\n")
	c.aclAddCommand("ship2", "admin", "echo")

	if _, ok := c.accessLists.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------DELETING COMMAND grp_nodes_ships, admin, dmesg-------------\n")
	c.aclDeleteCommand("grp_nodes_ships", "admin", "dmesg")

	if _, ok := c.accessLists.schemaMain.ACLMap["grp_nodes_ships"]["admin"]["dmesg"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: found map entry: grp_nodes_ships, admin, dmesg")
	}
	// Check that the remaining are still ok.
	if _, ok := c.accessLists.schemaMain.ACLMap["ship0"]["admin"]["systemctl"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship0, admin, systemctl")
	}
	if _, ok := c.accessLists.schemaMain.ACLMap["ship1"]["admin"]["tcpdump"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	if _, ok := c.accessLists.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------DELETING COMMAND ship0, admin, systemctl-------------\n")
	c.aclDeleteCommand("ship0", "admin", "systemctl")

	if _, ok := c.accessLists.schemaMain.ACLMap["ship0"]["admin"]["systemctl"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship0, admin, systemctl")
	}
	// Check that the remaining are ok.
	if _, ok := c.accessLists.schemaMain.ACLMap["ship1"]["admin"]["tcpdump"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	if _, ok := c.accessLists.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	// fmt.Printf("---------------DELETING SOURCE ship1, admin-------------\n")
	c.aclDeleteSource("ship1", "admin")

	if _, ok := c.accessLists.schemaMain.ACLMap["ship1"]["admin"]; ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}
	// Check that the remaining are ok.
	if _, ok := c.accessLists.schemaMain.ACLMap["ship2"]["admin"]["echo"]; !ok {
		t.Fatalf(" \U0001F631  [FAILED]: missing map entry: ship1, admin, tcpdump")
	}

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLSchemaMainACLMap")

}

// Need to clean up from the other tests before this test is enabled
//
// func TestACLHash(t *testing.T) {
// 	if !*logging {
// 		log.SetOutput(io.Discard)
// 	}
//
// 	a := newAccessLists(&errorKernel{}, tstConf)
//
// 	a.aclAddCommand("ship101", "admin", "HORSE")
//
// 	a.groupNodesAddNode("grp_nodes_ships", "ship101")
// 	a.aclAddCommand("grp_nodes_ships", "admin", "HEN")
//
// 	hash := [32]uint8{0xa4, 0x99, 0xbd, 0xa3, 0x18, 0x26, 0x52, 0xc2, 0x92, 0x60, 0x23, 0x19, 0x3c, 0xa, 0x7, 0xa9, 0xb7, 0x77, 0x4f, 0x11, 0x34, 0xd5, 0x2d, 0xd1, 0x8d, 0xab, 0x6c, 0x4b, 0x2, 0xfa, 0x5c, 0x7a}
// 	value := a.schemaGenerated.GeneratedACLsMap["ship101"].Hash
// 	// fmt.Printf("%#v\n", a.schemaGenerated.GeneratedACLsMap["ship101"].Hash)
//
// 	if bytes.Equal(hash[:], value[:]) == false {
// 		t.Fatalf(" \U0001F631  [FAILED]: hash mismatch")
// 	}
//
// 	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLHash")
// }

func TestACLConcurrent(t *testing.T) {
	c := newCentralAuth(tstConf, &errorKernel{})

	// -----------General testing and creation of some data----------------

	// Start concurrent updating of the schema.
	var wg sync.WaitGroup
	for i := 0; i < 4000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.aclAddCommand("ship1", "operator2", "rm -rf")
			c.aclAddCommand("ship1", "operator1", "ls -lt")
			c.aclAddCommand("ship1", "operator1", "ls -lt")
			c.aclAddCommand("ship1", "operator2", "ls -l")
			c.aclAddCommand("ship3", "operator3", "ls -lt")
			c.aclAddCommand("ship3", "operator3", "vi /etc/hostname")
			c.aclDeleteCommand("ship3", "operator2", "ls -lt")
			c.aclDeleteSource("ship3", "operator3")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			// fmt.Println("----schemaMain------")
			c.accessLists.schemaMain.mu.Lock()
			for _, v := range c.accessLists.schemaMain.ACLMap {
				_ = fmt.Sprintf("%+v\n", v)
			}
			c.accessLists.schemaMain.mu.Unlock()

			// fmt.Println("----schemaGenerated------")
			c.accessLists.schemaGenerated.mu.Lock()
			for k, v := range c.accessLists.schemaGenerated.GeneratedACLsMap {
				_ = fmt.Sprintf("node: %v, NodeDataSerialized: %v\n", k, string(v.Data))
				_ = fmt.Sprintf("node: %v, Hash: %v\n", k, v.Hash)
			}
			c.accessLists.schemaGenerated.mu.Unlock()
		}()
	}
	wg.Wait()

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestACLConcurrent")

}

// Need to clean up from the other tests before this test is enabled
//
// func TestExportACLs(t *testing.T) {
// 	const (
// 		grp_nodes_operators      = "grp_nodes_operators"
// 		grp_nodes_ships          = "grp_nodes_ships"
// 		grp_commands_commandset1 = "grp_commands_commandset1"
// 	)
//
// 	a := newAccessLists(&errorKernel{}, tstConf)
//
// 	a.groupNodesAddNode(grp_nodes_operators, "operator1")
// 	a.groupNodesAddNode(grp_nodes_operators, "operator2")
//
// 	a.groupNodesAddNode(grp_nodes_ships, "ship100")
// 	a.groupNodesAddNode(grp_nodes_ships, "ship101")
//
// 	a.groupCommandsAddCommand(grp_commands_commandset1, "dmesg")
// 	a.groupCommandsAddCommand(grp_commands_commandset1, "date")
//
// 	a.aclAddCommand(grp_nodes_ships, "admin", "useradd -m kongen")
// 	a.aclAddCommand("ship101", "admin", "HORSE")
//
// 	a.aclAddCommand(grp_nodes_ships, grp_nodes_operators, grp_commands_commandset1)
//
// 	js, err := a.exportACLs()
// 	if err != nil {
// 		t.Fatalf("%v", err)
// 	}
//
// 	want := `{"grp_nodes_ships":{"admin":{"useradd -m kongen":{}},"grp_nodes_operators":{"grp_commands_commandset1":{}}},"ship101":{"admin":{"HORSE":{}}}}`
//
// 	fmt.Printf(" * GOT = %s\n", js)
// 	fmt.Printf(" * WANT = %v\n", want)
//
// 	if string(js) != string(want) {
// 		t.Fatalf(" \U0001F631  [FAILED]: export does not match with what we want\n")
// 	}
//
// 	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestExportACLs")
//
// }

func TestImportACLs(t *testing.T) {
	// js := `{"grp_nodes_ships":{"admin":{"useradd -m kongen":{}},"grp_nodes_operators":{"grp_commands_commandset1":{}}},"ship101":{"admin":{"HORSE":{}}}`

	js := []byte{0x7b, 0x22, 0x67, 0x72, 0x70, 0x5f, 0x6e, 0x6f, 0x64, 0x65, 0x73, 0x5f, 0x73, 0x68, 0x69, 0x70, 0x73, 0x22, 0x3a, 0x7b, 0x22, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x22, 0x3a, 0x7b, 0x22, 0x75, 0x73, 0x65, 0x72, 0x61, 0x64, 0x64, 0x20, 0x2d, 0x6d, 0x20, 0x6b, 0x6f, 0x6e, 0x67, 0x65, 0x6e, 0x22, 0x3a, 0x7b, 0x7d, 0x7d, 0x2c, 0x22, 0x67, 0x72, 0x70, 0x5f, 0x6e, 0x6f, 0x64, 0x65, 0x73, 0x5f, 0x6f, 0x70, 0x65, 0x72, 0x61, 0x74, 0x6f, 0x72, 0x73, 0x22, 0x3a, 0x7b, 0x22, 0x67, 0x72, 0x70, 0x5f, 0x63, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x73, 0x5f, 0x63, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x73, 0x65, 0x74, 0x31, 0x22, 0x3a, 0x7b, 0x7d, 0x7d, 0x7d, 0x2c, 0x22, 0x73, 0x68, 0x69, 0x70, 0x31, 0x30, 0x31, 0x22, 0x3a, 0x7b, 0x22, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x22, 0x3a, 0x7b, 0x22, 0x48, 0x4f, 0x52, 0x53, 0x45, 0x22, 0x3a, 0x7b, 0x7d, 0x7d, 0x7d, 0x7d}

	want := `map[grp_nodes_ships:map[admin:map[useradd -m kongen:{}] grp_nodes_operators:map[grp_commands_commandset1:{}]] ship101:map[admin:map[HORSE:{}]]]`

	c := newCentralAuth(tstConf, &errorKernel{})

	err := c.importACLs(js)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if fmt.Sprintf("%v", c.accessLists.schemaMain.ACLMap) != want {
		t.Fatalf("error: import does not match with what we want\n")
	}

	t.Logf(" \U0001f600 [SUCCESS]	: %v\n", "TestImportACLs")

}
