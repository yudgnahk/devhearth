package detect

import "context"

type CostClass string

const (
	CostMetadata CostClass = "metadata"
	CostContent  CostClass = "content"
	CostExternal CostClass = "external"
)

type Descriptor struct {
	ID                   string
	Version              int
	Triggers             []string
	Cost                 CostClass
	RequiresContentReads bool
	Produces             []string
	RiskImplications     []string
}

type Candidate struct {
	Path string
}

type Evidence struct {
	Kind  string
	Value string
}

type Result struct {
	Assets        []any
	Relationships []any
	Evidence      []Evidence
}

// Detector classifies untrusted, read-only scan input. Implementations must not
// mutate the filesystem or directly execute operations.
type Detector interface {
	Descriptor() Descriptor
	Detect(context.Context, Candidate) (Result, error)
}
