package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
)

type ServerOptions struct {
	MockScan bool
	Logger   *slog.Logger
}

type Server struct {
	options ServerOptions
	mu      sync.Mutex
}

func NewServer(options ServerOptions) *Server {
	if options.Logger == nil {
		options.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{options: options}
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	const maxMessageBytes = 1024 * 1024
	scanner.Buffer(make([]byte, 4096), maxMessageBytes)
	encoder := json.NewEncoder(output)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			if writeErr := s.write(encoder, Response{JSONRPC: JSONRPCVersion, Error: &Error{Code: -32700, Message: "parse error"}}); writeErr != nil {
				return writeErr
			}
			continue
		}
		if err := s.handle(encoder, request); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handle(encoder *json.Encoder, request Request) error {
	if request.JSONRPC != JSONRPCVersion || request.Method == "" || len(request.ID) == 0 || string(request.ID) == "null" {
		return s.write(encoder, failure(request.ID, -32600, "invalid request"))
	}

	switch request.Method {
	case "engine.hello":
		var params HelloParams
		if err := decodeParams(request.Params, &params); err != nil {
			return s.write(encoder, failure(request.ID, -32602, "invalid params"))
		}
		if !slices.Contains(params.ProtocolVersions, ProtocolVersion) || !slices.Contains(params.SchemaVersions, SchemaVersion) {
			return s.write(encoder, failure(request.ID, -32001, "no compatible protocol or schema version"))
		}
		return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: HelloResult{
			Engine: "devhearth", ProtocolVersion: ProtocolVersion, SchemaVersion: SchemaVersion, ReadOnly: true,
		}})
	case "scan.start":
		if !s.options.MockScan {
			return s.write(encoder, failure(request.ID, -32002, "filesystem scanning is not available in Phase 0"))
		}
		var params ScanStartParams
		if err := decodeParams(request.Params, &params); err != nil || len(params.Roots) == 0 {
			return s.write(encoder, failure(request.ID, -32602, "at least one scan root is required"))
		}
		const scanID = "mock_scan_01"
		if err := s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: ScanStarted{ScanID: scanID}}); err != nil {
			return err
		}
		for _, progress := range mockProgress(scanID) {
			if err := s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: progress}); err != nil {
				return err
			}
		}
		return nil
	default:
		return s.write(encoder, failure(request.ID, -32601, "method not found"))
	}
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return errors.New("params are required")
	}
	return json.Unmarshal(raw, target)
}

func failure(id json.RawMessage, code int, message string) Response {
	return Response{JSONRPC: JSONRPCVersion, ID: id, Error: &Error{Code: code, Message: message}}
}

func mockProgress(scanID string) []ScanProgress {
	return []ScanProgress{
		{ScanID: scanID, Phase: "scope", EntriesVisited: 0, AllocatedBytes: 0},
		{ScanID: scanID, Phase: "metadata", EntriesVisited: 128, AllocatedBytes: 8_388_608},
		{ScanID: scanID, Phase: "detection", EntriesVisited: 256, AllocatedBytes: 16_777_216},
		{ScanID: scanID, Phase: "complete", EntriesVisited: 256, AllocatedBytes: 16_777_216, Complete: true},
	}
}

func (s *Server) write(encoder *json.Encoder, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write protocol message: %w", err)
	}
	return nil
}
