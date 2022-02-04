package steward

import "sync"

type signature string

// allowedSignatures is the structure for reading and writing from
// the signatures map. It holds a mutex to use when interacting with
// the map.
type signatures struct {
	// allowed is a map for holding all the allowed signatures.
	allowed map[signature]struct{}
	mu      sync.Mutex
}

func newSignatures() *signatures {
	s := signatures{
		allowed: make(map[signature]struct{}),
	}

	return &s
}
