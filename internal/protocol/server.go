package protocol

import (
	"bufio"
	"bytes"
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
	"time"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/policy"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// rfc3339 is the timestamp format every protocol response uses.
const rfc3339 = time.RFC3339

type ServerOptions struct {
	MockScan  bool
	Logger    *slog.Logger
	Inventory func(context.Context, []string, func(scan.Progress)) (scan.Result, error)
	Detect    func(context.Context, scan.Result, func(detect.Progress)) (assets.Graph, error)
	// Advise runs read-only Phase 3 analysis over the detected graph: size and
	// activity attribution, portfolio fit, and recommendation rules. It receives
	// the inventory rollup so attribution does not rebuild it, plus the policy
	// resolved for this machine so weighting and suppression are decided in one
	// place. When nil, the detector graph is used unchanged and no advice is
	// produced.
	Advise func(ctx context.Context, result scan.Result, graph assets.Graph, dirIndex map[string][]scan.DirectoryNode, active policy.Effective) (advisor.Result, error)
	// OnComplete persists a finished scan. progress may be nil; when non-nil it
	// reports durable rows written so far (written/total) during persistence.
	// dirIndex is the precomputed inventory rollup for this result (may be empty).
	// advice carries the attributed graph, so persistence must use advice.Graph.
	OnComplete func(ctx context.Context, result scan.Result, advice advisor.Result, status string, dirIndex map[string][]scan.DirectoryNode, progress func(written, total int64)) error

	// Monitoring is the durable Phase 4 state: portable policy, local feedback,
	// and trend history. Nil leaves those methods reporting "not available"
	// rather than silently discarding a user's preferences.
	Monitoring Monitoring
	// Machine describes this machine so policy overlays can match it. It is
	// supplied by the host and never inferred from the inventory.
	Machine policy.Machine
	// HomeDir resolves portable policy roots. The resolved absolute paths stay
	// inside the engine.
	HomeDir string
	// Now anchors policy timestamps; zero uses the current UTC time.
	Now func() time.Time
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
	// advice holds the attributed graph plus fit and recommendation output. Its
	// Graph field is the authoritative asset view for every query.
	advice    advisor.Result
	dirIndex  map[string][]scan.DirectoryNode // parent path -> direct children
	pathIndex map[string]scan.DirectoryNode   // path -> node (for parent lookup)
	// policy is the effective policy this scan was analysed under, kept so
	// later queries describe the scan as it was produced rather than as the
	// policy reads now.
	policy policy.Effective
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

// now anchors policy and feedback timestamps so tests can pin them.
func (s *Server) now() time.Time {
	if s.options.Now == nil {
		return time.Now().UTC()
	}
	return s.options.Now().UTC()
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

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			// Ignore blank lines from clients or terminals; they are not requests.
			continue
		}

		var request Request
		if err := json.Unmarshal(line, &request); err != nil {
			s.options.Logger.Warn("protocol parse error", "error", err, "line", truncateForLog(line, 120))
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

func truncateForLog(line []byte, max int) string {
	if max <= 0 || len(line) <= max {
		return string(line)
	}
	return string(line[:max]) + "…"
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
		if err := decodeParams(request.Params, &params); err != nil {
			return s.write(encoder, failure(request.ID, -32602, "invalid params"))
		}
		roots := params.Roots
		if len(roots) == 0 && params.UsePolicyRoots {
			resolved, policyErr := s.policyRoots(ctx, params.PolicyID)
			if policyErr != nil {
				return s.write(encoder, failure(request.ID, policyErr.Code, policyErr.Message))
			}
			roots = resolved
		}
		if len(roots) == 0 {
			return s.write(encoder, failure(request.ID, -32602, "at least one scan root is required"))
		}
		// The policy is resolved once, before the scan starts, so a policy edit
		// mid-scan cannot change the weighting halfway through the analysis.
		active := s.resolvePolicy(ctx, params.PolicyID)
		scanID := fmt.Sprintf("scan_%06d", s.nextScanID.Add(1))
		scanContext, cancel := context.WithCancel(ctx)
		s.scansMu.Lock()
		s.scans[scanID] = &activeScan{cancel: cancel, status: "running", policy: active}
		s.scansMu.Unlock()
		if err := s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: ScanStarted{ScanID: scanID}}); err != nil {
			cancel()
			return err
		}
		s.workers.Add(1)
		go func() { defer s.workers.Done(); s.runScan(scanContext, encoder, scanID, roots, active) }()
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
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan report is not available",
			func(current *activeScan) (any, *Error) {
				return report(params.ScanID, current), nil
			})
	case "assets.list":
		var params AssetsListParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan assets are not available",
			func(current *activeScan) (any, *Error) {
				return assetsList(params.ScanID, current), nil
			})
	case "assets.get":
		var params AssetsGetParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" || params.AssetID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId and assetId are required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan assets are not available",
			func(current *activeScan) (any, *Error) {
				for _, asset := range current.advice.Graph.Assets {
					if asset.ID == params.AssetID {
						return AssetsGetResult{
							ScanID: params.ScanID,
							Asset:  summarizeAsset(asset, current.result.Roots),
						}, nil
					}
				}
				return nil, &Error{Code: -32005, Message: "asset not found"}
			})
	case "portfolio.list":
		var params PortfolioListParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan portfolio is not available",
			func(current *activeScan) (any, *Error) {
				return portfolioList(params.ScanID, current), nil
			})
	case "inventory.children":
		var params InventoryChildrenParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan inventory is not available",
			func(current *activeScan) (any, *Error) {
				result, ok := inventoryChildren(params.ScanID, params.PathKey, current)
				if !ok {
					return nil, &Error{Code: -32006, Message: "inventory path not found in scan"}
				}
				return result, nil
			})
	case "fit.list":
		var params FitListParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan fit analysis is not available",
			func(current *activeScan) (any, *Error) {
				return fitList(params.ScanID, current), nil
			})
	case "fit.get":
		var params FitGetParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" || params.Ecosystem == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId and ecosystem are required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan fit analysis is not available",
			func(current *activeScan) (any, *Error) {
				assessment, found := findAssessment(current, params.Ecosystem)
				if !found {
					return nil, &Error{Code: -32007, Message: "no fit assessment for that ecosystem"}
				}
				return FitGetResult{ScanID: params.ScanID, Fit: assessment}, nil
			})
	case "recommendations.list":
		var params RecommendationsListParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId is required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan recommendations are not available",
			func(current *activeScan) (any, *Error) {
				return recommendationsList(params.ScanID, current, params.Family, params.Include), nil
			})
	case "recommendations.get":
		var params RecommendationsGetParams
		if err := decodeParams(request.Params, &params); err != nil || params.ScanID == "" || params.RecommendationID == "" {
			return s.write(encoder, failure(request.ID, -32602, "scanId and recommendationId are required"))
		}
		return s.withCompletedScan(encoder, request.ID, params.ScanID, "completed scan recommendations are not available",
			func(current *activeScan) (any, *Error) {
				recommendation, found := findRecommendation(current, params.RecommendationID)
				if !found {
					return nil, &Error{Code: -32008, Message: "recommendation not found"}
				}
				return RecommendationsGetResult{ScanID: params.ScanID, Recommendation: recommendation}, nil
			})
	case "policy.get":
		var params PolicyGetParams
		// Policy reads take no required parameter, so an absent params object is
		// valid here where every scan-scoped method rejects it.
		_ = decodeParams(request.Params, &params)
		document, policyErr := s.activePolicy(ctx, params.PolicyID)
		if policyErr != nil {
			return s.write(encoder, failure(request.ID, policyErr.Code, policyErr.Message))
		}
		result, err := policyResult(document, s.options.Machine, s.options.HomeDir)
		if err != nil {
			s.options.Logger.Error("render policy", "error", err)
			return s.write(encoder, failure(request.ID, -32012, "the stored policy could not be rendered"))
		}
		return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: result})
	case "policy.set":
		var params PolicySetParams
		if err := decodeParams(request.Params, &params); err != nil || len(params.Document) == 0 {
			return s.write(encoder, failure(request.ID, -32602, "a policy document is required"))
		}
		result, failed := s.setPolicy(ctx, params)
		return s.respond(encoder, request.ID, result, failed)
	case "policy.export":
		var params PolicyExportParams
		_ = decodeParams(request.Params, &params)
		document, policyErr := s.activePolicy(ctx, params.PolicyID)
		if policyErr != nil {
			return s.write(encoder, failure(request.ID, policyErr.Code, policyErr.Message))
		}
		encoded, err := policy.Marshal(document)
		if err != nil {
			s.options.Logger.Error("export policy", "error", err)
			return s.write(encoder, failure(request.ID, -32012, "the stored policy could not be exported"))
		}
		return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: request.ID, Result: PolicyExportResult{
			Document:      string(encoded),
			SchemaVersion: document.SchemaVersion,
			SuggestedName: "devhearth-policy-" + document.ID + ".json",
			Notes: []string{
				"this file carries preferences only: no absolute paths, no inventory, no scan history, and no recommendation feedback",
				"scan roots travel as portable aliases and resolve against the home directory of the machine that imports them",
				// Honest rather than reassuring: a root the user stored as
				// ~/Projects/acme-migration does carry that folder name, and
				// somebody sharing a policy should know before they send it.
				"a stored root keeps its folder names below the alias, so review the roots before sharing this file",
			},
		}})
	case "policy.import":
		var params PolicyImportParams
		if err := decodeParams(request.Params, &params); err != nil || params.Document == "" {
			return s.write(encoder, failure(request.ID, -32602, "a policy document is required"))
		}
		result, failed := s.importPolicy(ctx, params)
		return s.respond(encoder, request.ID, result, failed)
	case "recommendations.suppress":
		var params RecommendationsSuppressParams
		if err := decodeParams(request.Params, &params); err != nil {
			return s.write(encoder, failure(request.ID, -32602, "invalid params"))
		}
		if params.RecommendationID == "" && params.Family == "" {
			return s.write(encoder, failure(request.ID, -32602, "recommendationId or family is required"))
		}
		result, failed := s.suppress(ctx, params)
		return s.respond(encoder, request.ID, result, failed)
	case "recommendations.feedback":
		var params RecommendationsFeedbackParams
		if err := decodeParams(request.Params, &params); err != nil || params.RecommendationID == "" || params.Verdict == "" {
			return s.write(encoder, failure(request.ID, -32602, "recommendationId and verdict are required"))
		}
		result, failed := s.recordFeedback(ctx, params)
		return s.respond(encoder, request.ID, result, failed)
	case "trends.list":
		var params TrendsListParams
		_ = decodeParams(request.Params, &params)
		result, failed := s.trends(ctx, params)
		return s.respond(encoder, request.ID, result, failed)
	default:
		return s.write(encoder, failure(request.ID, -32601, "method not found"))
	}
}

// respond writes a handler's result or its error, so the Phase 4 handlers can
// return a plain (any, *Error) pair instead of repeating the envelope.
func (s *Server) respond(encoder *json.Encoder, requestID json.RawMessage, result any, failed *Error) error {
	if failed != nil {
		return s.write(encoder, failure(requestID, failed.Code, failed.Message))
	}
	return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: requestID, Result: result})
}

// withCompletedScan resolves a scan that has finished and projects a result
// while holding the scan lock, so a concurrent scan cannot swap state midway.
func (s *Server) withCompletedScan(
	encoder *json.Encoder,
	requestID json.RawMessage,
	scanID string,
	unavailable string,
	project func(*activeScan) (any, *Error),
) error {
	s.scansMu.Lock()
	current, found := s.scans[scanID]
	if !found || current.status == "running" || current.status == "cancelling" {
		s.scansMu.Unlock()
		return s.write(encoder, failure(requestID, -32004, unavailable))
	}
	result, projectErr := project(current)
	s.scansMu.Unlock()
	if projectErr != nil {
		return s.write(encoder, failure(requestID, projectErr.Code, projectErr.Message))
	}
	return s.write(encoder, Response{JSONRPC: JSONRPCVersion, ID: requestID, Result: result})
}

func (s *Server) runScan(ctx context.Context, encoder *json.Encoder, scanID string, roots []string, active policy.Effective) {
	progress := func(value scan.Progress) {
		_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{ScanID: scanID, Phase: value.Phase, EntriesVisited: value.EntriesVisited, AllocatedBytes: value.AllocatedBytes}})
	}
	var result scan.Result
	var graph assets.Graph
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
	} else if s.options.Detect != nil {
		detectProgress := func(value detect.Progress) {
			_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{
				ScanID: scanID, Phase: value.Phase, EntriesVisited: result.EntriesVisited,
				AllocatedBytes: result.AllocatedBytes, AssetsFound: value.AssetsFound,
			}})
		}
		graph, err = s.options.Detect(ctx, result, detectProgress)
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
		} else if err != nil {
			s.options.Logger.Error("detect assets", "error", err)
			status = "failed"
		}
	}
	// Build the drill-down index once and reuse it for advice, persistence, and
	// live queries.
	dirIndex := scan.BuildDirectoryIndex(result)
	pathIndex := make(map[string]scan.DirectoryNode, len(result.Entries))
	for _, nodes := range dirIndex {
		for _, node := range nodes {
			pathIndex[node.Path] = node
		}
	}

	// Analysis runs before persistence so the durable inventory records the
	// attributed graph, the fit assessments, and the recommendations together.
	advice := advisor.Result{Graph: graph}
	if status == "complete" && s.options.Advise != nil {
		_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{
			ScanID: scanID, Phase: "advice", EntriesVisited: result.EntriesVisited,
			AllocatedBytes: result.AllocatedBytes, AssetsFound: int64(len(graph.Assets)),
		}})
		analysed, adviseErr := s.options.Advise(ctx, result, graph, dirIndex, active)
		switch {
		case errors.Is(adviseErr, context.Canceled):
			status = "cancelled"
		case adviseErr != nil:
			s.options.Logger.Error("analyze scan", "error", adviseErr)
			status = "failed"
		default:
			advice = analysed
		}
	}

	if s.options.OnComplete != nil {
		assetsFound := int64(len(advice.Graph.Assets))
		_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{
			ScanID: scanID, Phase: "persist", EntriesVisited: result.EntriesVisited,
			AllocatedBytes: result.AllocatedBytes, AssetsFound: assetsFound,
		}})
		persistProgress := func(written, total int64) {
			_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{
				ScanID: scanID, Phase: "persist", EntriesVisited: result.EntriesVisited,
				AllocatedBytes: result.AllocatedBytes, AssetsFound: assetsFound,
				RowsWritten: written, RowsTotal: total,
			}})
		}
		if persistErr := s.options.OnComplete(context.Background(), result, advice, status, dirIndex, persistProgress); persistErr != nil {
			s.options.Logger.Error("persist scan", "error", persistErr)
			status = "failed"
		}
	}

	s.scansMu.Lock()
	current := s.scans[scanID]
	current.status = status
	current.result = result
	current.advice = advice
	current.dirIndex = dirIndex
	current.pathIndex = pathIndex
	current.policy = active
	s.scansMu.Unlock()
	// Complete marks a terminal event; clients must not wait forever after a
	// cancellation or failure. The phase preserves the outcome.
	_ = s.write(encoder, Notification{JSONRPC: JSONRPCVersion, Method: "scan.progress", Params: ScanProgress{
		ScanID: scanID, Phase: status, EntriesVisited: result.EntriesVisited,
		AllocatedBytes: result.AllocatedBytes, AssetsFound: int64(len(advice.Graph.Assets)),
		RecommendationsFound: int64(len(advice.Recommendations)), Complete: true,
	}})
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
	byKind := map[string]int{}
	for _, asset := range active.advice.Graph.Assets {
		byKind[string(asset.Kind)]++
	}
	return ScanReport{
		ScanID: id, Status: active.status, Roots: roots,
		EntriesVisited: active.result.EntriesVisited, LogicalBytes: active.result.LogicalBytes,
		AllocatedBytes: active.result.AllocatedBytes, Inaccessible: inaccessible,
		AssetCount: len(active.advice.Graph.Assets), AssetsByKind: byKind,
		Portfolio: toProtocolPortfolio(assets.SummarizePortfolio(active.advice.Graph)),
		Advice:    adviceSummary(active),
	}
}

func assetsList(id string, active *activeScan) AssetsListResult {
	summaries := make([]AssetSummary, 0, len(active.advice.Graph.Assets))
	for _, asset := range active.advice.Graph.Assets {
		summaries = append(summaries, summarizeAsset(asset, active.result.Roots))
	}
	rels := make([]RelationshipSummary, 0, len(active.advice.Graph.Relationships))
	for _, rel := range active.advice.Graph.Relationships {
		rels = append(rels, RelationshipSummary{
			ID: rel.ID, SourceID: rel.SourceID, TargetID: rel.TargetID,
			Kind: rel.Kind, Confidence: rel.Confidence, DetectorID: rel.DetectorID,
		})
	}
	return AssetsListResult{ScanID: id, Assets: summaries, Relationships: rels}
}

func portfolioList(id string, active *activeScan) PortfolioListResult {
	return PortfolioListResult{ScanID: id, Portfolio: toProtocolPortfolio(assets.SummarizePortfolio(active.advice.Graph))}
}

func inventoryChildren(id, pathKey string, active *activeScan) (InventoryChildrenResult, bool) {
	if active.dirIndex == nil {
		active.dirIndex = scan.BuildDirectoryIndex(active.result)
	}
	parentKey := pathKey
	if parentKey != "" {
		if _, ok := active.pathIndex[parentKey]; !ok {
			// Allow listing when path exists only as a parent of known children.
			if _, hasChildren := active.dirIndex[parentKey]; !hasChildren {
				return InventoryChildrenResult{}, false
			}
		}
	}
	nodes := scan.ChildrenOf(active.dirIndex, parentKey)
	children := make([]DirectoryChild, 0, len(nodes))
	for _, node := range nodes {
		children = append(children, DirectoryChild{
			Name:                node.Name,
			Path:                redactPath(node.Path, active.result.Roots),
			PathKey:             node.Path,
			Kind:                node.Kind,
			LogicalBytes:        node.LogicalBytes,
			AllocatedBytes:      node.AllocatedBytes,
			TotalLogicalBytes:   node.TotalLogicalBytes,
			TotalAllocatedBytes: node.TotalAllocatedBytes,
			DirectChildCount:    node.DirectChildCount,
			IsSymlink:           node.IsSymlink,
		})
	}
	displayPath := ""
	parentOfParent := ""
	if parentKey == "" {
		displayPath = ""
	} else {
		displayPath = redactPath(parentKey, active.result.Roots)
		if node, ok := active.pathIndex[parentKey]; ok {
			parentOfParent = node.ParentPath
		}
	}
	return InventoryChildrenResult{
		ScanID:    id,
		PathKey:   parentKey,
		Path:      displayPath,
		ParentKey: parentOfParent,
		Children:  children,
	}, true
}

func summarizeAsset(asset assets.Asset, roots []string) AssetSummary {
	evidence := make([]EvidenceSummary, 0, len(asset.Evidence))
	for _, item := range asset.Evidence {
		evidence = append(evidence, EvidenceSummary{
			Kind: item.Kind, Value: redactEvidenceValue(item.Value, roots), Confidence: item.Confidence,
		})
	}
	lastActivity := ""
	if !asset.LastActivityAt.IsZero() {
		lastActivity = asset.LastActivityAt.UTC().Format(time.RFC3339)
	}
	return AssetSummary{
		ID: asset.ID, Kind: string(asset.Kind), DisplayName: asset.DisplayName,
		Path: redactPath(asset.Path, roots), Risk: string(asset.Risk),
		Ecosystem: asset.Ecosystem, Class: string(asset.Class),
		DetectorID: asset.DetectorID, DetectorVersion: asset.DetectorVersion,
		Attributes: redactAttributes(asset.Attributes, roots), Evidence: evidence,
		Size: AssetSize{
			Attributed:              asset.Size.Attributed,
			LogicalBytes:            asset.Size.LogicalBytes,
			AllocatedBytes:          asset.Size.AllocatedBytes,
			ExclusiveAllocatedBytes: asset.Size.ExclusiveAllocatedBytes,
			Shared:                  asset.Size.Shared,
			Uncertain:               asset.Size.Uncertain,
		},
		LastActivityAt: lastActivity,
	}
}

// redactAttributes redacts any attribute value that looks like a filesystem path
// (e.g. gitdir absolute paths from worktree detection).
func redactAttributes(attrs map[string]string, roots []string) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for key, value := range attrs {
		out[key] = redactEvidenceValue(value, roots)
	}
	return out
}

func toProtocolPortfolio(items []assets.PortfolioSummary) []PortfolioSummary {
	result := make([]PortfolioSummary, 0, len(items))
	for _, item := range items {
		result = append(result, PortfolioSummary{
			Ecosystem: item.Ecosystem, ProjectCount: item.ProjectCount,
			PackageManagers: item.PackageManagers, VersionManagers: item.VersionManagers,
			ProjectLocalInstallCount: item.ProjectLocalInstallCount,
			SharedStoreCount:         item.SharedStoreCount, DownloadCacheCount: item.DownloadCacheCount,
			BuildOutputCount: item.BuildOutputCount, DominantPackageTool: item.DominantPackageTool,
			ProjectLocalInstallBytes: item.ProjectLocalInstallBytes,
			SharedStoreBytes:         item.SharedStoreBytes,
			DownloadCacheBytes:       item.DownloadCacheBytes,
			BuildOutputBytes:         item.BuildOutputBytes,
			SizesUncertain:           item.SizesUncertain,
		})
	}
	return result
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
	if path == "" {
		return ""
	}
	return "<outside-selected-roots>"
}

func redactEvidenceValue(value string, roots []string) string {
	if value == "" {
		return value
	}
	// Evidence may carry absolute paths (signatures) or non-path facts (tool names).
	if strings.Contains(value, string(filepath.Separator)) || filepath.IsAbs(value) {
		return redactPath(value, roots)
	}
	return value
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
		{ScanID: scanID, Phase: "detection", EntriesVisited: 256, AllocatedBytes: 16_777_216, AssetsFound: 12},
		{ScanID: scanID, Phase: "complete", EntriesVisited: 256, AllocatedBytes: 16_777_216, AssetsFound: 12, Complete: true},
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
