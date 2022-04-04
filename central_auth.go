package steward

import "sync"

type signatureBase32 string
type argsString string
type centralAuth struct {
	schema         map[Node]map[argsString]signatureBase32
	nodePublicKeys nodePublicKeys
	configuration  *Configuration
}

func newCentralAuth(configuration *Configuration) *centralAuth {
	a := centralAuth{
		schema:         make(map[Node]map[argsString]signatureBase32),
		nodePublicKeys: *newNodePublicKeys(),
		configuration:  configuration,
	}

	return &a
}

type nodePublicKeys struct {
	mu     sync.Mutex
	keyMap map[Node]string
}

func newNodePublicKeys() *nodePublicKeys {
	n := nodePublicKeys{
		keyMap: make(map[Node]string),
	}

	return &n
}
