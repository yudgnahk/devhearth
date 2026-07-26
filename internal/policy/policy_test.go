package policy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewRootKeepsOnlyPortablePaths(t *testing.T) {
	const home = "/Users/example"
	tests := []struct {
		name     string
		path     string
		wantOK   bool
		wantRoot Root
	}{
		{name: "home itself", path: home, wantOK: true, wantRoot: Root{Alias: AliasHome}},
		{name: "below home", path: home + "/Projects/work", wantOK: true, wantRoot: Root{Alias: AliasHome, Relative: "Projects/work"}},
		{name: "trailing slash", path: home + "/Projects/", wantOK: true, wantRoot: Root{Alias: AliasHome, Relative: "Projects"}},
		{name: "outside home", path: "/Volumes/External/Code", wantOK: false},
		{name: "another user", path: "/Users/someone-else/Projects", wantOK: false},
		{name: "empty", path: "", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, ok := NewRoot(test.path, home)
			if ok != test.wantOK {
				t.Fatalf("NewRoot(%q) ok = %v, want %v", test.path, ok, test.wantOK)
			}
			if ok && root != test.wantRoot {
				t.Fatalf("NewRoot(%q) = %+v, want %+v", test.path, root, test.wantRoot)
			}
		})
	}
}

func TestRootResolve(t *testing.T) {
	root := Root{Alias: AliasHome, Relative: "Projects/work"}
	got, err := root.Resolve("/Users/example")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := "/Users/example/Projects/work"; got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
	if _, err := (Root{Alias: "dropbox"}).Resolve("/Users/example"); err == nil {
		t.Fatal("expected an unknown alias to fail")
	}
	if _, err := root.Resolve(""); err == nil {
		t.Fatal("expected an unknown home directory to fail")
	}
}

// An exported policy must not be able to carry the machine's layout. This is a
// structural claim about the type, so it is asserted on the serialized bytes
// rather than on any single field.
func TestDocumentNeverSerializesAnAbsolutePath(t *testing.T) {
	const home = "/Users/example"
	root, ok := NewRoot(home+"/Projects/secret-client", home)
	if !ok {
		t.Fatal("expected a home-relative root to be portable")
	}
	document := DefaultDocument("policy-1")
	document.Roots = []Root{root}
	document.Exclusions = []string{"node_modules", "*.iso"}
	document.PreferredTools = map[string]string{"node": "pnpm"}
	document.Suppressions = []Suppression{{Family: "hibernate_inactive_project", Reason: "keeping these warm", CreatedAt: Today(time.Unix(0, 0))}}

	encoded, err := Marshal(document)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, home) || strings.Contains(text, "/Users/") {
		t.Fatalf("policy export leaked an absolute path:\n%s", text)
	}
	if !strings.Contains(text, "Projects/secret-client") {
		t.Fatalf("expected the relative segment to survive export:\n%s", text)
	}
}

func TestParseRejectsUntrustedDocuments(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "unknown schema version",
			raw:     `{"schemaVersion":99,"id":"p1"}`,
			wantErr: "unsupported policy schema version",
		},
		{
			name:    "unknown field",
			raw:     `{"schemaVersion":1,"id":"p1","executeOperations":true}`,
			wantErr: "unknown field",
		},
		{
			name:    "missing id",
			raw:     `{"schemaVersion":1}`,
			wantErr: "policy id is required",
		},
		{
			name:    "absolute root",
			raw:     `{"schemaVersion":1,"id":"p1","roots":[{"alias":"home","relative":"/etc"}]}`,
			wantErr: "must be relative",
		},
		{
			name:    "escaping root",
			raw:     `{"schemaVersion":1,"id":"p1","roots":[{"alias":"home","relative":"../../etc"}]}`,
			wantErr: "escapes its alias",
		},
		{
			name:    "unknown alias",
			raw:     `{"schemaVersion":1,"id":"p1","roots":[{"alias":"root","relative":"etc"}]}`,
			wantErr: "unknown root alias",
		},
		{
			name:    "absolute exclusion",
			raw:     `{"schemaVersion":1,"id":"p1","exclusions":["/Users/example/private"]}`,
			wantErr: "must be relative",
		},
		{
			name:    "escaping exclusion",
			raw:     `{"schemaVersion":1,"id":"p1","exclusions":["../../secrets"]}`,
			wantErr: "escapes the scanned tree",
		},
		{
			name:    "escaping exclusion mid-pattern",
			raw:     `{"schemaVersion":1,"id":"p1","exclusions":["Work/../../secrets"]}`,
			wantErr: "escapes the scanned tree",
		},
		{
			name:    "bad glob",
			raw:     `{"schemaVersion":1,"id":"p1","exclusions":["["]}`,
			wantErr: "not a valid glob",
		},
		{
			name:    "weight out of range",
			raw:     `{"schemaVersion":1,"id":"p1","fitWeights":{"in_use_share":4}}`,
			wantErr: "must be between 0 and 1",
		},
		{
			name:    "weights all zero",
			raw:     `{"schemaVersion":1,"id":"p1","fitWeights":{"in_use_share":0}}`,
			wantErr: "cannot all be zero",
		},
		{
			name:    "unknown fit mode",
			raw:     `{"schemaVersion":1,"id":"p1","fitMode":"always_migrate"}`,
			wantErr: "unknown fit mode",
		},
		{
			name:    "unknown risk threshold",
			raw:     `{"schemaVersion":1,"id":"p1","riskThreshold":"prohibited"}`,
			wantErr: "unknown risk threshold",
		},
		{
			name:    "blanket suppression",
			raw:     `{"schemaVersion":1,"id":"p1","suppressions":[{"reason":"all of it"}]}`,
			wantErr: "must name a recommendation id or a family",
		},
		{
			name:    "overlay matching everything",
			raw:     `{"schemaVersion":1,"id":"p1","machineOverlays":[{"match":{},"fitMode":"balanced"}]}`,
			wantErr: "must match at least one machine attribute",
		},
		{
			name:    "control character in name",
			raw:     `{"schemaVersion":1,"id":"p1","name":"work\u001b[2Jpolicy"}`,
			wantErr: "control character",
		},
		{
			name:    "trailing content",
			raw:     `{"schemaVersion":1,"id":"p1"}{"schemaVersion":1,"id":"p2"}`,
			wantErr: "trailing content",
		},
		{
			name:    "empty",
			raw:     ``,
			wantErr: "empty",
		},
		{
			name:    "retention out of range",
			raw:     `{"schemaVersion":1,"id":"p1","retention":{"scanHistoryCount":100000}}`,
			wantErr: "scanHistoryCount must be between",
		},
		{
			name:    "window out of range",
			raw:     `{"schemaVersion":1,"id":"p1","activeWithinDays":-5}`,
			wantErr: "must be between 1 and",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.raw))
			if err == nil {
				t.Fatalf("expected %q to be rejected", test.raw)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func TestParseRejectsOversizedDocument(t *testing.T) {
	padding := strings.Repeat("a", maxDocumentBytes)
	raw := `{"schemaVersion":1,"id":"p1","name":"` + padding + `"}`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected an oversized document to be rejected")
	}
}

func TestParseNormalizes(t *testing.T) {
	raw := `{
	  "schemaVersion": 1,
	  "id": "p1",
	  "roots": [{"alias":"home","relative":"Work/./api"},{"alias":"home","relative":"Work/api"}],
	  "exclusions": ["node_modules", "node_modules", " .cache "],
	  "preferredTools": {"Node":"PNPM"},
	  "suppressions": [{"family":"HIBERNATE_INACTIVE_PROJECT","ecosystem":"Python"}]
	}`
	document, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(document.Roots) != 1 || document.Roots[0].Relative != "Work/api" {
		t.Fatalf("roots = %+v, want one cleaned, de-duplicated root", document.Roots)
	}
	if len(document.Exclusions) != 2 {
		t.Fatalf("exclusions = %v, want duplicates removed", document.Exclusions)
	}
	if document.PreferredTools["node"] != "pnpm" {
		t.Fatalf("preferredTools = %v, want lower-cased keys and values", document.PreferredTools)
	}
	if document.Suppressions[0].Family != "hibernate_inactive_project" || document.Suppressions[0].Ecosystem != "python" {
		t.Fatalf("suppressions = %+v, want lower-cased fields", document.Suppressions)
	}
	if document.FitMode != FitModeBalanced || document.RiskThreshold != RiskHigh {
		t.Fatalf("defaults not applied: mode=%q risk=%q", document.FitMode, document.RiskThreshold)
	}
	if document.ActiveWithinDays != DefaultActiveWithinDays || document.Retention.ScanHistoryCount != DefaultScanHistoryCount {
		t.Fatalf("window defaults not applied: %+v", document)
	}
}

// A policy exported from one machine and re-imported must be identical, or a
// second Mac cannot be trusted to behave like the first.
func TestRoundTripIsStable(t *testing.T) {
	document := DefaultDocument("p1")
	document.Name = "Polyglot laptop"
	document.Roots = []Root{{Alias: AliasHome, Relative: "Projects"}}
	document.Exclusions = []string{"*.iso", "node_modules"}
	document.PreferredTools = map[string]string{"node": "pnpm", "python": "uv"}
	document.FitMode = FitModePreferStability
	document.Suppressions = []Suppression{
		{Family: "remove_obsolete_worktree", Reason: "handled manually", CreatedAt: "2026-01-02"},
		{RecommendationID: "rec-1", Family: "adopt_shared_store"},
	}
	document.Overlays = []Overlay{{
		Match:         MachineSelector{Architecture: "arm64", DiskClass: "small"},
		FitMode:       FitModePreferDiskSavings,
		RiskThreshold: RiskMedium,
	}}

	first, err := Marshal(document)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	parsed, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	second, err := Marshal(parsed)
	if err != nil {
		t.Fatalf("Marshal round two: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("round trip changed the document:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestResolveAppliesMatchingOverlays(t *testing.T) {
	document := DefaultDocument("p1")
	document.PreferredTools = map[string]string{"node": "npm", "python": "uv"}
	document.Exclusions = []string{"node_modules"}
	document.Overlays = []Overlay{
		{
			Match:          MachineSelector{Architecture: "arm64"},
			PreferredTools: map[string]string{"node": "pnpm"},
			Exclusions:     []string{"*.iso"},
		},
		{
			Match:             MachineSelector{Architecture: "arm64", DiskClass: "small"},
			FitMode:           FitModePreferDiskSavings,
			RiskThreshold:     RiskLow,
			InactiveAfterDays: 30,
		},
		{
			Match:   MachineSelector{Role: "ci"},
			FitMode: FitModePreferStability,
		},
	}
	document, err := Validate(document)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	effective := Resolve(document, Machine{Architecture: "arm64", DiskClass: "small", Role: "laptop"})
	if effective.PreferredTools["node"] != "pnpm" {
		t.Fatalf("overlay preferred tool not applied: %v", effective.PreferredTools)
	}
	if effective.PreferredTools["python"] != "uv" {
		t.Fatalf("base preferred tool lost: %v", effective.PreferredTools)
	}
	if effective.FitMode != FitModePreferDiskSavings {
		t.Fatalf("fit mode = %q, want the more specific overlay to win", effective.FitMode)
	}
	if effective.RiskThreshold != RiskLow {
		t.Fatalf("risk threshold = %q, want low", effective.RiskThreshold)
	}
	if effective.InactiveAfter != 30*24*time.Hour {
		t.Fatalf("inactiveAfter = %v, want 30 days", effective.InactiveAfter)
	}
	if len(effective.Exclusions) != 2 {
		t.Fatalf("exclusions = %v, want base plus overlay", effective.Exclusions)
	}
	if len(effective.AppliedOverlays) != 2 {
		t.Fatalf("appliedOverlays = %+v, want the two matching overlays", effective.AppliedOverlays)
	}

	// A machine that matches nothing keeps the base settings.
	untouched := Resolve(document, Machine{Architecture: "amd64", Role: "workstation"})
	if untouched.PreferredTools["node"] != "npm" || untouched.FitMode != FitModeBalanced {
		t.Fatalf("non-matching machine got overlay settings: %+v", untouched)
	}
	if len(untouched.AppliedOverlays) != 0 {
		t.Fatalf("appliedOverlays = %+v, want none", untouched.AppliedOverlays)
	}
}

// Resolve must not hand callers a view into the stored document; advice runs
// concurrently with policy edits.
func TestResolveDoesNotShareStateWithDocument(t *testing.T) {
	document := DefaultDocument("p1")
	document.PreferredTools = map[string]string{"node": "npm"}
	document.Exclusions = []string{"node_modules"}

	effective := Resolve(document, Machine{})
	effective.PreferredTools["node"] = "bun"
	effective.Exclusions[0] = "changed"

	if document.PreferredTools["node"] != "npm" {
		t.Fatal("Resolve shared the preferred-tools map with the document")
	}
	if document.Exclusions[0] != "node_modules" {
		t.Fatal("Resolve shared the exclusions slice with the document")
	}
}

func TestSuppressesMatching(t *testing.T) {
	effective := Resolve(mustValidate(t, func() Document {
		document := DefaultDocument("p1")
		document.Suppressions = []Suppression{
			{RecommendationID: "rec-42"},
			{Family: "adopt_shared_store", Ecosystem: "python"},
			{Family: "remove_obsolete_worktree"},
		}
		return document
	}()), Machine{})

	tests := []struct {
		name      string
		id        string
		family    string
		ecosystem string
		want      bool
	}{
		{name: "exact id", id: "rec-42", family: "anything", want: true},
		{name: "other id", id: "rec-43", family: "anything", want: false},
		{name: "family and ecosystem", id: "rec-9", family: "adopt_shared_store", ecosystem: "python", want: true},
		{name: "family with other ecosystem", id: "rec-9", family: "adopt_shared_store", ecosystem: "node", want: false},
		{name: "whole family", id: "rec-7", family: "remove_obsolete_worktree", ecosystem: "node", want: true},
		{name: "unrelated", id: "rec-8", family: "consolidate_runtime", ecosystem: "node", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := effective.Suppresses(test.id, test.family, test.ecosystem); got != test.want {
				t.Fatalf("Suppresses(%q,%q,%q) = %v, want %v", test.id, test.family, test.ecosystem, got, test.want)
			}
		})
	}
}

func TestAllowsRisk(t *testing.T) {
	effective := Effective{RiskThreshold: RiskMedium}
	tests := map[string]bool{
		RiskInformational: true,
		RiskLow:           true,
		RiskMedium:        true,
		RiskHigh:          false,
		"prohibited":      true, // unknown label: never hidden by a threshold
	}
	for risk, want := range tests {
		if got := effective.AllowsRisk(risk); got != want {
			t.Fatalf("AllowsRisk(%q) = %v, want %v", risk, got, want)
		}
	}
	if !(Effective{}).AllowsRisk(RiskHigh) {
		t.Fatal("an unset threshold must not hide advice")
	}
}

func TestExcludesPath(t *testing.T) {
	effective := Effective{Exclusions: []string{"node_modules", "*.iso", "Work/scratch"}}
	tests := map[string]bool{
		"Work/api/node_modules": true,
		"Work/scratch":          true,
		"downloads/ubuntu.iso":  true,
		"Work/api/src":          false,
		"":                      false,
	}
	for candidate, want := range tests {
		if got := effective.ExcludesPath(candidate); got != want {
			t.Fatalf("ExcludesPath(%q) = %v, want %v", candidate, got, want)
		}
	}
}

func TestResolveRootsReportsUnresolvable(t *testing.T) {
	effective := Effective{Roots: []Root{
		{Alias: AliasHome, Relative: "Projects"},
		{Alias: "dropbox", Relative: "Shared"},
	}}
	resolved, unresolved := effective.ResolveRoots("/Users/example")
	if len(resolved) != 1 || resolved[0] != "/Users/example/Projects" {
		t.Fatalf("resolved = %v", resolved)
	}
	if len(unresolved) != 1 || unresolved[0].Alias != "dropbox" {
		t.Fatalf("unresolved = %+v", unresolved)
	}
}

func TestWithSuppressionIsImmutableAndDeduplicated(t *testing.T) {
	base := DefaultDocument("p1")
	first := base.WithSuppression(Suppression{Family: "adopt_shared_store", Reason: "later"})
	if len(base.Suppressions) != 0 {
		t.Fatal("WithSuppression mutated the receiver")
	}
	second := first.WithSuppression(Suppression{Family: "adopt_shared_store", Reason: "changed my mind"})
	if len(second.Suppressions) != 1 {
		t.Fatalf("suppressions = %+v, want the equivalent entry replaced", second.Suppressions)
	}
	if second.Suppressions[0].Reason != "changed my mind" {
		t.Fatalf("reason = %q, want the refreshed value", second.Suppressions[0].Reason)
	}
	if first.Suppressions[0].Reason != "later" {
		t.Fatal("WithSuppression mutated the earlier document")
	}
	removed := second.WithoutSuppression(Suppression{Family: "adopt_shared_store"})
	if len(removed.Suppressions) != 0 {
		t.Fatalf("suppressions = %+v, want the entry removed", removed.Suppressions)
	}
}

func TestDefaultDocumentValidates(t *testing.T) {
	document, err := Validate(DefaultDocument("p1"))
	if err != nil {
		t.Fatalf("the default document must be valid: %v", err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := Parse(encoded); err != nil {
		t.Fatalf("the default document must survive a round trip: %v", err)
	}
}

func mustValidate(t *testing.T, document Document) Document {
	t.Helper()
	validated, err := Validate(document)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return validated
}
