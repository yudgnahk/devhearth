package detect

import (
	"context"
	"testing"
)

type stubDetector struct{ descriptor Descriptor }

func (s stubDetector) Descriptor() Descriptor { return s.descriptor }
func (stubDetector) Match(Candidate) bool     { return false }
func (stubDetector) Detect(context.Context, Candidate) (Result, error) {
	return Result{}, nil
}

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

func TestRegistryPreservesRegistrationOrder(t *testing.T) {
	r := NewRegistry()
	for _, id := range []string{"a", "b", "c"} {
		if err := r.Register(stubDetector{Descriptor{ID: id, Version: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	all := r.All()
	if len(all) != 3 || all[0].Descriptor().ID != "a" || all[2].Descriptor().ID != "c" {
		t.Fatalf("order = %#v", all)
	}
}
