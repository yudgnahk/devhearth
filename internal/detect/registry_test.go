package detect

import (
	"context"
	"testing"
)

type stubDetector struct{ descriptor Descriptor }

func (s stubDetector) Descriptor() Descriptor                          { return s.descriptor }
func (stubDetector) Detect(context.Context, Candidate) (Result, error) { return Result{}, nil }

func TestRegistryRejectsDuplicateIDs(t *testing.T) {
	r := NewRegistry()
	d := stubDetector{Descriptor{ID: "fixture.git", Version: 1}}
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(d); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}
