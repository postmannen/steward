package steward

type tuiMessage struct {
	ToNode             *Node     `json:"toNode,omitempty" yaml:"toNode,omitempty"`
	ToNodes            *[]Node   `json:"toNodes,omitempty" yaml:"toNodes,omitempty"`
	Method             *Method   `json:"method,omitempty" yaml:"method,omitempty"`
	MethodArgs         *[]string `json:"methodArgs,omitempty" yaml:"methodArgs,omitempty"`
	ReplyMethod        *Method   `json:"replyMethod,omitempty" yaml:"replyMethod,omitempty"`
	ReplyMethodArgs    *[]string `json:"replyMethodArgs,omitempty" yaml:"replyMethodArgs,omitempty"`
	ACKTimeout         *int      `json:"ACKTimeout,omitempty" yaml:"ACKTimeout,omitempty"`
	Retries            *int      `json:"retries,omitempty" yaml:"retries,omitempty"`
	ReplyACKTimeout    *int      `json:"replyACKTimeout,omitempty" yaml:"replyACKTimeout,omitempty"`
	ReplyRetries       *int      `json:"replyRetries,omitempty" yaml:"replyRetries,omitempty"`
	MethodTimeout      *int      `json:"methodTimeout,omitempty" yaml:"methodTimeout,omitempty"`
	ReplyMethodTimeout *int      `json:"replyMethodTimeout,omitempty" yaml:"replyMethodTimeout,omitempty"`
	Directory          *string   `json:"directory,omitempty" yaml:"directory,omitempty"`
	FileName           *string   `json:"fileName,omitempty" yaml:"fileName,omitempty"`
}
