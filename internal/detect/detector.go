package detect

import (
	"context"

	"github.com/yudgnahk/devhearth/internal/assets"
)

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

// Candidate is untrusted scan input. Path is the only required field for
// metadata detectors; implementations must not mutate the filesystem.
type Candidate struct {
	Path     string
	Name     string
	Kind     string // directory, file, symlink, other
	IsDir    bool
	Parent   string
	Root     string
	Children []string // immediate child names when available
}

// Finding is a detector observation before stable asset IDs are assigned.
type Finding struct {
	Key         string // stable within a scan, typically kind+path
	Kind        assets.Kind
	DisplayName string
	Path        string
	Risk        assets.Risk
	Ecosystem   string
	Class       assets.Class
	Attributes  map[string]string
	Evidence    []assets.Evidence
}

// Link is a path-keyed relationship before asset IDs are assigned.
type Link struct {
	SourceKey  string
	TargetKey  string
	Kind       string
	Confidence float64
	Evidence   assets.Evidence
}

type Result struct {
	Findings []Finding
	Links    []Link
}

// Detector classifies untrusted, read-only scan input. Implementations must not
// mutate the filesystem or directly execute operations.
type Detector interface {
	Descriptor() Descriptor
	// Match reports whether this detector should inspect the candidate. The
	// runner uses Triggers as a fast path when Match is not needed; detectors
	// may still implement Match for multi-name heuristics.
	Match(Candidate) bool
	Detect(context.Context, Candidate) (Result, error)
}
