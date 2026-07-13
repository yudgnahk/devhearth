package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/scan"
)

func TestHelloNegotiatesVersions(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"1","method":"engine.hello","params":{"protocolVersions":[1],"schemaVersions":[1]}}` + "\n"
	var output bytes.Buffer
	if err := NewServer(ServerOptions{}).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result, ok := response.Result.(map[string]any)
	if !ok || result["readOnly"] != true || result["protocolVersion"] != float64(1) {
		t.Fatalf("unexpected result: %#v", response.Result)
	}
}

func TestBlankLinesAreIgnored(t *testing.T) {
	input := "\n\n" + `{"jsonrpc":"2.0","id":"1","method":"engine.hello","params":{"protocolVersions":[1],"schemaVersions":[1]}}` + "\n"
	var output bytes.Buffer
	if err := NewServer(ServerOptions{}).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "parse error") {
		t.Fatalf("blank lines should not produce parse errors: %s", output.String())
	}
	if !strings.Contains(output.String(), `"readOnly":true`) {
		t.Fatalf("expected hello result, got %s", output.String())
	}
}

func TestHelloRejectsIncompatibleVersion(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"1","method":"engine.hello","params":{"protocolVersions":[99],"schemaVersions":[1]}}` + "\n"
	var output bytes.Buffer
	if err := NewServer(ServerOptions{}).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"code":-32001`) {
		t.Fatalf("expected incompatibility error, got %s", output.String())
	}
}

func TestRequestRequiresID(t *testing.T) {
	input := `{"jsonrpc":"2.0","method":"engine.hello","params":{"protocolVersions":[1],"schemaVersions":[1]}}` + "\n"
	var output bytes.Buffer
	if err := NewServer(ServerOptions{}).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"code":-32600`) {
		t.Fatalf("expected invalid request error, got %s", output.String())
	}
}

func TestMockScanStreamsProgress(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"2","method":"scan.start","params":{"roots":["/synthetic"]}}` + "\n"
	var output bytes.Buffer
	if err := NewServer(ServerOptions{MockScan: true}).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 5 || !strings.Contains(lines[len(lines)-1], `"complete":true`) {
		t.Fatalf("unexpected stream: %s", output.String())
	}
}

func TestReportRedactsSelectedPaths(t *testing.T) {
	active := &activeScan{status: "complete", result: scan.Result{Roots: []string{"/Users/example/Projects"}, Inaccessible: []scan.InaccessiblePath{{Path: "/Users/example/Projects/private", Reason: "permission denied"}}}}
	report := report("scan_01", active)
	if report.Roots[0] != "<selected-root-1>" {
		t.Fatalf("root = %q", report.Roots[0])
	}
	if report.Inaccessible[0].Path != "<selected-root-1>/private" {
		t.Fatalf("inaccessible path = %q", report.Inaccessible[0].Path)
	}
}

func TestAssetsListRedactsPathsAndIncludesEvidence(t *testing.T) {
	active := &activeScan{
		status: "complete",
		result: scan.Result{Roots: []string{"/Users/example/Projects"}},
		graph: assets.Graph{Assets: []assets.Asset{{
			ID: "a1", Kind: assets.KindProject, DisplayName: "app",
			Path: "/Users/example/Projects/app", Risk: assets.RiskInformational,
			Ecosystem: "node", DetectorID: "detect.node", DetectorVersion: 1,
			Evidence: []assets.Evidence{{Kind: "path_signature", Value: "/Users/example/Projects/app/package.json", Confidence: 0.9}},
		}}},
	}
	listed := assetsList("scan_01", active)
	if listed.Assets[0].Path != "<selected-root-1>/app" {
		t.Fatalf("path = %q", listed.Assets[0].Path)
	}
	if listed.Assets[0].Evidence[0].Value != "<selected-root-1>/app/package.json" {
		t.Fatalf("evidence = %q", listed.Assets[0].Evidence[0].Value)
	}
}

func TestAssetsListRedactsAttributePaths(t *testing.T) {
	active := &activeScan{
		status: "complete",
		result: scan.Result{Roots: []string{"/Users/example/Projects"}},
		graph: assets.Graph{Assets: []assets.Asset{{
			ID: "wt1", Kind: assets.KindGitWorktree, DisplayName: "feature",
			Path: "/Users/example/Projects/feature", Risk: assets.RiskInformational,
			DetectorID: "detect.git", DetectorVersion: 1,
			Attributes: map[string]string{
				"git_kind": "worktree",
				"gitdir":   "/Users/example/Projects/main/.git/worktrees/feature",
				"tool":     "git",
			},
		}}},
	}
	listed := assetsList("scan_01", active)
	attrs := listed.Assets[0].Attributes
	if attrs["gitdir"] != "<selected-root-1>/main/.git/worktrees/feature" {
		t.Fatalf("gitdir attribute = %q", attrs["gitdir"])
	}
	if attrs["tool"] != "git" {
		t.Fatalf("non-path attribute should pass through: %#v", attrs)
	}
	if strings.Contains(attrs["gitdir"], "/Users/") {
		t.Fatalf("absolute path leaked in attributes: %#v", attrs)
	}
}

func FuzzServerInput(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"unknown"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if bytes.Contains(data, []byte{'\n'}) || len(data) > 64*1024 {
			t.Skip()
		}
		var output bytes.Buffer
		_ = NewServer(ServerOptions{}).Serve(context.Background(), bytes.NewReader(append(data, '\n')), &output)
	})
}
