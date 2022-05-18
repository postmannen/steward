package steward

import (
	"strings"
)

type authParser struct {
	currentHost node
	authSchema  *accessLists
	//ACLsToConvert map[node]map[node]map[command]struct{}
}

// newAuthParser returns a new authParser, with the current host node set.
func newAuthParser(n node, accessLists *accessLists) *authParser {
	a := authParser{
		currentHost: n,
		authSchema:  accessLists,
		//ACLsToConvert: make(map[node]map[node]map[command]struct{}),
	}
	return &a
}

type parseFn func() parseFn

// parse will parse one host or one host group.
func (a *authParser) parse() {
	fn := a.hostGroupOrSingle()
	for {
		fn = fn()
		if fn == nil {
			break
		}
	}

}

// hostGroupOrSingle checks if host grp or single node.
func (a *authParser) hostGroupOrSingle() parseFn {
	switch {
	case strings.HasPrefix(string(a.currentHost), "grp_nodes_"):
		// Is group
		return a.hostIsGroup
	default:
		// Is single node
		return a.hostIsNotGroup
	}
}

// hostIsGroup
func (a *authParser) hostIsGroup() parseFn {
	// fmt.Printf("%v is a grp type\n", a.currentHost)

	hosts := a.authSchema.nodeAsSlice(a.currentHost)

	for source, cmdMap := range a.authSchema.schemaMain.ACLMap[a.currentHost] {

		for cmd, emptyStruct := range cmdMap {
			cmdSlice := a.authSchema.commandAsSlice(cmd)

			// Expand eventual groups, so we use real fromNode nodenames in ACL for nodes.
			sourceNodes := a.authSchema.nodeAsSlice(source)
			for _, sourceNode := range sourceNodes {
				for _, host := range hosts {

					for _, cm := range cmdSlice {
						if a.authSchema.schemaGenerated.ACLsToConvert[host] == nil {
							a.authSchema.schemaGenerated.ACLsToConvert[host] = make(map[node]map[command]struct{})
						}
						if a.authSchema.schemaGenerated.ACLsToConvert[host][sourceNode] == nil {
							a.authSchema.schemaGenerated.ACLsToConvert[host][sourceNode] = make(map[command]struct{})
						}

						a.authSchema.schemaGenerated.ACLsToConvert[host][sourceNode][cm] = emptyStruct
					}
				}
			}
		}
	}

	// fmt.Printf(" * ACLsToConvert=%+v\n", a.authSchema.schemaGenerated.ACLsToConvert)
	// Done with host. Return nil will make the main loop take the next host in the main for loop.
	return nil
}

// hostIsNotGroup
func (a *authParser) hostIsNotGroup() parseFn {
	// fmt.Printf("%v is a single node type\n", a.currentHost)

	host := a.currentHost

	for source, cmdMap := range a.authSchema.schemaMain.ACLMap[a.currentHost] {

		for cmd, emptyStruct := range cmdMap {
			cmdSlice := a.authSchema.commandAsSlice(cmd)

			// Expand eventual groups, so we use real fromNode nodenames in ACL for nodes.
			sourceNodes := a.authSchema.nodeAsSlice(source)
			for _, sourceNode := range sourceNodes {

				for _, cm := range cmdSlice {
					if a.authSchema.schemaGenerated.ACLsToConvert[host] == nil {
						a.authSchema.schemaGenerated.ACLsToConvert[host] = make(map[node]map[command]struct{})
					}
					if a.authSchema.schemaGenerated.ACLsToConvert[host][sourceNode] == nil {
						a.authSchema.schemaGenerated.ACLsToConvert[host][sourceNode] = make(map[command]struct{})
					}

					a.authSchema.schemaGenerated.ACLsToConvert[host][sourceNode][cm] = emptyStruct
				}
			}
		}
	}

	// fmt.Printf(" * ACLsToConvert contains: %+v\n", a.authSchema.schemaGenerated.ACLsToConvert)

	// Done with host. Return nil will make the main loop take the next host in the main for loop.
	return nil
}
