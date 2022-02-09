package steward

type signatureBase32 string
type argsString string

type centralAuth struct {
	schema map[Node]map[argsString]signatureBase32
}

func newCentralAuth() *centralAuth {
	a := centralAuth{
		schema: make(map[Node]map[argsString]signatureBase32),
	}

	return &a
}
