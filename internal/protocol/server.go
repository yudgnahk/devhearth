package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yudgnahk/devhearth/internal/scan"
)

type ServerOptions struct {
	MockScan   bool
	Logger     *slog.Logger
	Inventory  func(context.Context, []string, func(scan.Progress)) (scan.Result, error)
	OnComplete func(context.Context, scan.Result, string) error
}

type Server struct {
	options    ServerOptions
	mu         sync.Mutex
	scansMu    sync.Mutex
	scans      map[string]*activeScan
	nextScanID atomic.Uint64
	workers    sync.WaitGroup
}

type activeScan struct {
	cancel context.CancelFunc
	status string
	result scan.Result
}

func NewServer(options ServerOptions) *Server {
	if options.Logger == nil {
		options.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if options.Inventory == nil {
		options.Inventory = defaultInventory
	}
	return &Server{options: options, scans: make(map[string]*activeScan)}
}

func (s *Server) Serve(ctx context.Context, input io.Reader, output io.Writer) error {
	defer s.workers.Wait()
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
		if err := s.handle(ctx, encoder, request); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, encoder *json.Encoder, request Request) error {
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
		var params ScanStartParams
		if err := decodeParams(request.Params, &params); err != nil || len(params.Roots) == 0 {
			return s.write(encoder, failure(request.ID, -32602, "at least one scan root is required"))
		}
		scanID := fmt.Sprintf("scan_%06d", s.nextScanID.Add(1))
		scanContext, cancel := context.WithCancel(ctx)
		s.scansMu.Lock()
		s.scans[scanID] = &activeScan{cancel: cancel, status: "running"}
		s.scansMu.Unlock()
		if err := s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: ScanStarted{ScanID: scanID}}); err != nil {
			cancel()
			return err
		}
		s.workers.Add(1)
		go func() { defer s.workers.Done(); s.runScan(scanContext, encoder, scanID, params.Roots) }()
		return nil
	case "scan.cancel":
		var params ScanCancelParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		s.scansMu.Lock()
		current, found := s.scans[params.ScanID]
		if found && current.status == "running" {
			current.cancel()
			current.status = "cancelling"
		}
		s.scansMu.Unlock()
		if !found {
			return s.write(encoder, failure(request.ID, -32003, "scan not found"))
		}
		return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: ScanStatus{ScanID: params.ScanID, Status: "cancelling"}})
	case "scan.status":
		var params ScanStatusParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		s.scansMu.Lock()
		current, found := s.scans[params.ScanID]
		status := ""
		if found {
			status = current.status
		}
		s.scansMu.Unlock()
		if !found {
			return s.write(encoder, failure(request.ID, -32003, "scan not found"))
		}
		return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: ScanStatus{ScanID: params.ScanID, Status: status}})
	case "report.export":
		var params ReportExportParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		s.scansMu.Lock()
		current, found := s.scans[params.ScanID]
		if !found || current.status == "running" || current.status == "cancelling" {
			s.scansMu.Unlock()
			return s.write(encoder, failure(request.ID, -32004, "completed scan report is not available"))
		}
		result := report(params.ScanID, current)
		s.scansMu.Unlock()
		return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: result})
	default:
		return s.write(encoder, failure(request.ID, -32601, "method not found"))
	}
}

func (s *Server) runScan(ctx context.Context, encoder *json.Encoder, scanID string, roots []string) {
	progress := func(value scan.Progress) {
		_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{ScanID: scanID, Phase: value.Phase, EntriesVisited: value.EntriesVisited, AllocatedBytes: value.AllocatedBytes}})
	}
	var result scan.Result
	var err error
	if s.options.MockScan {
		for _, event := range mockProgress(scanID) {
			_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: event})
		}
		s.scansMu.Lock()
		s.scans[scanID].status = "complete"
		s.scansMu.Unlock()
		return
	}
	progress(scan.Progress{Phase: "scope"})
	result, err = s.options.Inventory(ctx, roots, progress)
	status := "complete"
	if errors.Is(err, context.Canceled) {
		status = "cancelled"
	} else if err != nil {
		status = "failed"
	}
	if s.options.OnComplete != nil {
		if persistErr := s.options.OnComplete(context.Background(), result, status); persistErr != nil {
			s.options.Logger.Error("persist scan", "error", persistErr)
			status = "failed"
		}
	}
	s.scansMu.Lock()
	current := s.scans[scanID]
	current.status = status
	current.result = result
	s.scansMu.Unlock()
	// Complete marks a terminal event; clients must not wait forever after a
	// cancellation or failure. The phase preserves the outcome.
	complete := true
	_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{ScanID: scanID, Phase: status, EntriesVisited: result.EntriesVisited, AllocatedBytes: result.AllocatedBytes, Complete: complete}})
}

func defaultInventory(ctx context.Context, roots []string, progress func(scan.Progress)) (scan.Result, error) {
	return scan.Inventory(ctx, roots, scan.Options{Progress: progress})
}

func report(id string, active *activeScan) ScanReport {
	inaccessible := make([]InaccessiblePath, len(active.result.Inaccessible))
	roots := make([]string, len(active.result.Roots))
	for i := range active.result.Roots {
		roots[i] = fmt.Sprintf("<selected-root-%d>", i+1)
	}
	for i, value := range active.result.Inaccessible {
		inaccessible[i] = InaccessiblePath{Path: redactPath(value.Path, active.result.Roots), Reason: value.Reason}
	}
	return ScanReport{ScanID: id, Status: active.status, Roots: roots, EntriesVisited: active.result.EntriesVisited, LogicalBytes: active.result.LogicalBytes, AllocatedBytes: active.result.AllocatedBytes, Inaccessible: inaccessible}
}

func redactPath(path string, roots []string) string {
	for index, root := range roots {
		if path == root {
			return fmt.Sprintf("<selected-root-%d>", index+1)
		}
		if relative, err := filepath.Rel(root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Sprintf("<selected-root-%d>/%s", index+1, relative)
		}
	}
	return "<outside-selected-roots>"
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
