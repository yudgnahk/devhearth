package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
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
