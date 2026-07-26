package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/policy"
	"github.com/yudgnahk/devhearth/internal/recommend"
	"github.com/yudgnahk/devhearth/internal/store"
	"github.com/yudgnahk/devhearth/internal/trend"
)

// fakeMonitoring is an in-memory stand-in for the SQLite store. It keeps the
// same contract: a document is validated before it is stored, so a test cannot
// smuggle in a policy the real store would have refused.
type fakeMonitoring struct {
	documents map[string]policy.Document
	active    string
	feedback  []store.Feedback
	snapshots []trend.Snapshot
	saveErr   error
}

func newFakeMonitoring() *fakeMonitoring {
	return &fakeMonitoring{documents: map[string]policy.Document{}}
}

func (f *fakeMonitoring) EnsureActivePolicy(_ context.Context, defaultID string) (policy.Document, error) {
	if f.active != "" {
		return f.documents[f.active], nil
	}
	document := policy.DefaultDocument(defaultID)
	f.documents[defaultID] = document
	f.active = defaultID
	return document, nil
}

func (f *fakeMonitoring) PolicyByID(_ context.Context, id string) (policy.Document, error) {
	document, found := f.documents[id]
	if !found {
		return policy.Document{}, errors.New("policy not found")
	}
	return document, nil
}

func (f *fakeMonitoring) SavePolicy(_ context.Context, document policy.Document, activate bool) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	validated, err := policy.Validate(document)
	if err != nil {
		return err
	}
	f.documents[validated.ID] = validated
	if activate {
		f.active = validated.ID
	}
	return nil
}

func (f *fakeMonitoring) RecordFeedback(_ context.Context, feedback store.Feedback) (store.Feedback, error) {
	if feedback.Verdict != store.VerdictAccepted && feedback.Verdict != store.VerdictRejected &&
		feedback.Verdict != store.VerdictUnclear && feedback.Verdict != store.VerdictLater {
		return store.Feedback{}, errors.New("unknown verdict")
	}
	feedback.ID = "feedback-1"
	feedback.CreatedAt = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	f.feedback = append(f.feedback, feedback)
	return feedback, nil
}

func (f *fakeMonitoring) Snapshots(context.Context, int) ([]trend.Snapshot, error) {
	return f.snapshots, nil
}

func monitoringOptions(monitor Monitoring) ServerOptions {
	return ServerOptions{
		Monitoring: monitor,
		HomeDir:    "/Users/example",
		Machine:    policy.Machine{Architecture: "arm64", DiskClass: "small", Role: "laptop"},
		Now:        func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) },
	}
}

// decodeResult pulls the result object out of the last JSON-RPC line.
func decodeResult(t *testing.T, out string, target any) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
		t.Fatalf("decode envelope: %v (line %q)", err, lines[len(lines)-1])
	}
	if envelope.Error != nil {
		t.Fatalf("unexpected error response: %+v", envelope.Error)
	}
	if err := json.Unmarshal(envelope.Result, target); err != nil {
		t.Fatalf("decode result: %v", err)
	}
}

func TestPolicyGetCreatesADefaultAndDescribesTheMachine(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.get","params":{}}`)

	var result PolicyResult
	decodeResult(t, out, &result)
	if result.Effective.PolicyID != DefaultPolicyID {
		t.Fatalf("policyId = %q, want the default policy", result.Effective.PolicyID)
	}
	if result.Effective.FitMode != policy.FitModeBalanced {
		t.Fatalf("fitMode = %q, want balanced", result.Effective.FitMode)
	}
	if result.Machine.Architecture != "arm64" {
		t.Fatalf("machine = %+v, want the host-supplied class", result.Machine)
	}
	if len(result.Choices.FitModes) == 0 || len(result.Choices.Verdicts) == 0 {
		t.Fatalf("choices = %+v, want the engine to publish its vocabulary", result.Choices)
	}
	// The document travels as the canonical portable JSON.
	if _, err := policy.Parse(result.Document); err != nil {
		t.Fatalf("the document on the wire must be a valid policy file: %v", err)
	}
}

func TestPolicyMethodsReportWhenStorageIsUnavailable(t *testing.T) {
	for _, request := range []string{
		`{"jsonrpc":"2.0","id":"1","method":"policy.get","params":{}}`,
		`{"jsonrpc":"2.0","id":"1","method":"policy.export","params":{}}`,
		`{"jsonrpc":"2.0","id":"1","method":"policy.import","params":{"document":"{}"}}`,
		`{"jsonrpc":"2.0","id":"1","method":"trends.list","params":{}}`,
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.feedback","params":{"recommendationId":"r1","verdict":"accepted"}}`,
	} {
		out := serveRequests(t, ServerOptions{}, request)
		if !strings.Contains(out, `"error"`) {
			t.Fatalf("without storage the request should fail rather than appear to work: %s", out)
		}
	}
}

func TestPolicySetValidatesClientInput(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.set","params":{"document":{"schemaVersion":1,"id":"p1","roots":[{"alias":"home","relative":"../../etc"}]}}}`)

	if !strings.Contains(out, "escapes its alias") {
		t.Fatalf("an escaping root from a client must be refused: %s", out)
	}
	if len(monitor.documents) != 0 {
		t.Fatalf("a rejected policy must not be stored: %+v", monitor.documents)
	}
}

func TestPolicySetStoresAValidDocument(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.set","params":{"document":{"schemaVersion":1,"id":"p1","name":"Laptop","fitMode":"prefer_disk_savings","preferredTools":{"node":"pnpm"},"roots":[{"alias":"home","relative":"Projects"}]}}}`)

	var result PolicyResult
	decodeResult(t, out, &result)
	if result.Effective.FitMode != policy.FitModePreferDiskSavings {
		t.Fatalf("fitMode = %q", result.Effective.FitMode)
	}
	if result.Effective.PreferredTools["node"] != "pnpm" {
		t.Fatalf("preferredTools = %v", result.Effective.PreferredTools)
	}
	if len(result.Effective.Roots) != 1 || result.Effective.Roots[0].Display != "~/Projects" {
		t.Fatalf("roots = %+v, want the portable display form", result.Effective.Roots)
	}
	if !result.Effective.Roots[0].Resolvable {
		t.Fatal("a home-relative root should resolve on a machine with a home directory")
	}
	if monitor.active != "p1" {
		t.Fatalf("active policy = %q, want the stored one", monitor.active)
	}
}

// The absolute path a root resolves to on this machine must never cross the
// protocol boundary; only the portable form does.
func TestPolicyResponsesNeverCarryAbsolutePaths(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.set","params":{"document":{"schemaVersion":1,"id":"p1","roots":[{"alias":"home","relative":"Projects/secret-client"}]}}}`,
		`{"jsonrpc":"2.0","id":"2","method":"policy.export","params":{}}`)

	if strings.Contains(out, "/Users/example") {
		t.Fatalf("a policy response leaked the machine's home directory: %s", out)
	}
}

func TestPolicyExportReturnsTheBytesToWrite(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.export","params":{}}`)

	var result PolicyExportResult
	decodeResult(t, out, &result)
	if result.SchemaVersion != policy.SchemaVersion {
		t.Fatalf("schemaVersion = %d", result.SchemaVersion)
	}
	if !strings.HasSuffix(result.SuggestedName, ".json") {
		t.Fatalf("suggestedName = %q", result.SuggestedName)
	}
	if len(result.Notes) == 0 {
		t.Fatal("an export should state what it does and does not contain")
	}
	parsed, err := policy.Parse([]byte(result.Document))
	if err != nil {
		t.Fatalf("exported bytes must re-import: %v", err)
	}
	if parsed.ID != DefaultPolicyID {
		t.Fatalf("exported policy id = %q", parsed.ID)
	}
}

func TestPolicyImportRejectsAHostileFile(t *testing.T) {
	tests := []struct {
		name     string
		document string
		wantErr  string
	}{
		{name: "unknown schema", document: `{\"schemaVersion\":99,\"id\":\"p1\"}`, wantErr: "unsupported policy schema version"},
		{name: "unknown field", document: `{\"schemaVersion\":1,\"id\":\"p1\",\"executeOperations\":true}`, wantErr: "unknown field"},
		{name: "absolute exclusion", document: `{\"schemaVersion\":1,\"id\":\"p1\",\"exclusions\":[\"/Users/someone/private\"]}`, wantErr: "must be relative"},
		{name: "blanket suppression", document: `{\"schemaVersion\":1,\"id\":\"p1\",\"suppressions\":[{\"reason\":\"everything\"}]}`, wantErr: "must name a recommendation id or a family"},
		{name: "not json", document: `not a policy`, wantErr: "rejected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			monitor := newFakeMonitoring()
			out := serveRequests(t, monitoringOptions(monitor),
				`{"jsonrpc":"2.0","id":"1","method":"policy.import","params":{"document":"`+test.document+`"}}`)
			if !strings.Contains(out, test.wantErr) {
				t.Fatalf("import error = %s, want it to mention %q", out, test.wantErr)
			}
			if len(monitor.documents) != 0 {
				t.Fatalf("a rejected import must not be stored: %+v", monitor.documents)
			}
		})
	}
}

func TestPolicyImportStoresAndWarns(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.import","params":{"document":"{\"schemaVersion\":1,\"id\":\"shared\",\"name\":\"Team baseline\",\"fitMode\":\"prefer_workflow_stability\"}"}}`)

	var result PolicyResult
	decodeResult(t, out, &result)
	if result.Effective.PolicyID != "shared" || result.Effective.FitMode != policy.FitModePreferStability {
		t.Fatalf("imported policy = %+v", result.Effective)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("an import should say what did not travel with the file")
	}
	if monitor.active != "shared" {
		t.Fatalf("active = %q, want the imported policy activated by default", monitor.active)
	}
}

func TestSuppressAddsAndUndoesASuppression(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.suppress","params":{"family":"adopt_shared_store","ecosystem":"node","reason":"not this quarter"}}`)

	var result PolicyResult
	decodeResult(t, out, &result)
	if result.Effective.SuppressionCount != 1 {
		t.Fatalf("suppressionCount = %d, want 1", result.Effective.SuppressionCount)
	}
	stored := monitor.documents[monitor.active]
	if len(stored.Suppressions) != 1 || stored.Suppressions[0].CreatedAt != "2026-07-01" {
		t.Fatalf("stored suppressions = %+v, want a day-resolution stamp", stored.Suppressions)
	}

	undone := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.suppress","params":{"family":"adopt_shared_store","ecosystem":"node","undo":true}}`)
	var after PolicyResult
	decodeResult(t, undone, &after)
	if after.Effective.SuppressionCount != 0 {
		t.Fatalf("suppressionCount after undo = %d, want 0", after.Effective.SuppressionCount)
	}
}

// Hiding a recommendation has to take effect immediately. The partition is
// computed during analysis, so without re-applying the policy to held scans a
// suppression would silently do nothing until the next scan.
func TestSuppressingTakesEffectWithoutRescanning(t *testing.T) {
	monitor := newFakeMonitoring()
	server := NewServer(monitoringOptions(monitor))

	visible := recommend.Recommendation{
		ID: "rec-1", Family: "adopt_shared_store", Ecosystem: "node",
		Title: "Share node dependencies", Risk: assets.RiskMedium, Confidence: 0.7,
	}
	other := recommend.Recommendation{
		ID: "rec-2", Family: "consolidate_runtime", Ecosystem: "node",
		Title: "Consolidate runtimes", Risk: assets.RiskLow, Confidence: 0.6,
	}
	server.scans["scan_01"] = &activeScan{
		status: "complete",
		advice: advisor.Result{
			All:             []recommend.Recommendation{visible, other},
			Recommendations: []recommend.Recommendation{visible, other},
		},
	}

	before := recommendationsList("scan_01", server.scans["scan_01"], "", "")
	if len(before.Recommendations) != 2 {
		t.Fatalf("expected both recommendations before suppression, got %d", len(before.Recommendations))
	}

	result, failure := server.suppress(context.Background(), RecommendationsSuppressParams{
		RecommendationID: "rec-1",
		Reason:           "handled by hand",
	})
	if failure != nil {
		t.Fatalf("suppress: %+v", failure)
	}
	if policyResult, ok := result.(PolicyResult); !ok || policyResult.Effective.SuppressionCount != 1 {
		t.Fatalf("suppress result = %+v", result)
	}

	after := recommendationsList("scan_01", server.scans["scan_01"], "", "")
	if len(after.Recommendations) != 1 || after.Recommendations[0].ID != "rec-2" {
		t.Fatalf("inbox after suppression = %+v, want only rec-2", after.Recommendations)
	}
	if after.SuppressedCount != 1 {
		t.Fatalf("suppressedCount = %d, want 1", after.SuppressedCount)
	}

	all := recommendationsList("scan_01", server.scans["scan_01"], "", IncludeAll)
	hidden := 0
	for _, item := range all.Recommendations {
		if item.Suppressed {
			hidden++
			if item.Confidence != visible.Confidence || item.Risk != string(visible.Risk) {
				t.Fatalf("suppression altered the recommendation: %+v", item)
			}
		}
	}
	if hidden != 1 {
		t.Fatalf("include=all showed %d suppressed items, want 1", hidden)
	}

	// Undo restores it without a rescan too.
	if _, failure := server.suppress(context.Background(), RecommendationsSuppressParams{
		RecommendationID: "rec-1", Undo: true,
	}); failure != nil {
		t.Fatalf("undo suppress: %+v", failure)
	}
	restored := recommendationsList("scan_01", server.scans["scan_01"], "", "")
	if len(restored.Recommendations) != 2 {
		t.Fatalf("inbox after undo = %+v, want both back", restored.Recommendations)
	}
}

// Raising the risk threshold must also apply to advice already on screen.
func TestRiskThresholdAppliesToHeldScans(t *testing.T) {
	monitor := newFakeMonitoring()
	server := NewServer(monitoringOptions(monitor))
	high := recommend.Recommendation{ID: "rec-1", Family: "hibernate_inactive_project", Risk: assets.RiskHigh}
	low := recommend.Recommendation{ID: "rec-2", Family: "consolidate_runtime", Risk: assets.RiskLow}
	server.scans["scan_01"] = &activeScan{
		status: "complete",
		advice: advisor.Result{
			All:             []recommend.Recommendation{high, low},
			Recommendations: []recommend.Recommendation{high, low},
		},
	}

	_, failure := server.setPolicy(context.Background(), PolicySetParams{
		Document: []byte(`{"schemaVersion":1,"id":"p1","riskThreshold":"low"}`),
	})
	if failure != nil {
		t.Fatalf("setPolicy: %+v", failure)
	}

	listed := recommendationsList("scan_01", server.scans["scan_01"], "", "")
	if len(listed.Recommendations) != 1 || listed.Recommendations[0].ID != "rec-2" {
		t.Fatalf("inbox = %+v, want only the low-risk item", listed.Recommendations)
	}
	if listed.HiddenByRiskCount != 1 {
		t.Fatalf("hiddenByRiskCount = %d, want 1", listed.HiddenByRiskCount)
	}
	if listed.RiskThreshold != "low" {
		t.Fatalf("riskThreshold = %q, want the new policy's threshold", listed.RiskThreshold)
	}
}

// Hiding advice must not read as recovering storage. The report's savings
// bounds describe the machine, so they stay put when the inbox shrinks.
func TestReportSavingsDoNotShrinkWhenAdviceIsHidden(t *testing.T) {
	monitor := newFakeMonitoring()
	server := NewServer(monitoringOptions(monitor))
	first := recommend.Recommendation{
		ID: "rec-1", Family: "adopt_shared_store", Risk: assets.RiskMedium,
		Savings: recommend.Savings{LowBytes: 1_000, HighBytes: 4_000},
	}
	second := recommend.Recommendation{
		ID: "rec-2", Family: "consolidate_runtime", Risk: assets.RiskLow,
		Savings: recommend.Savings{LowBytes: 500, HighBytes: 1_500},
	}
	scan := &activeScan{
		status: "complete",
		advice: advisor.Result{
			All:             []recommend.Recommendation{first, second},
			Recommendations: []recommend.Recommendation{first, second},
		},
	}
	server.scans["scan_01"] = scan

	before := adviceSummary(scan)
	if before.SavingsLowBytes != 1_500 || before.SavingsHighBytes != 5_500 {
		t.Fatalf("baseline savings = %d–%d", before.SavingsLowBytes, before.SavingsHighBytes)
	}

	if _, failure := server.suppress(context.Background(), RecommendationsSuppressParams{
		RecommendationID: "rec-1", Reason: "handled by hand",
	}); failure != nil {
		t.Fatalf("suppress: %+v", failure)
	}

	after := adviceSummary(server.scans["scan_01"])
	if after.SavingsLowBytes != before.SavingsLowBytes || after.SavingsHighBytes != before.SavingsHighBytes {
		t.Fatalf("hiding advice changed the savings total: %d–%d became %d–%d",
			before.SavingsLowBytes, before.SavingsHighBytes, after.SavingsLowBytes, after.SavingsHighBytes)
	}
	if after.RecommendationCount != 1 || after.TotalRecommendationCount != 2 {
		t.Fatalf("counts = %d visible of %d total, want 1 of 2", after.RecommendationCount, after.TotalRecommendationCount)
	}
	if after.WithheldSavingsLowBytes != 1_000 || after.WithheldSavingsHighBytes != 4_000 {
		t.Fatalf("withheld savings = %d–%d, want the hidden recommendation's range",
			after.WithheldSavingsLowBytes, after.WithheldSavingsHighBytes)
	}
	if after.SuppressedCount != 1 {
		t.Fatalf("suppressedCount = %d", after.SuppressedCount)
	}
}

// policy.set must be no more trusted than a file from a stranger.
func TestPolicySetRejectsUnknownFieldsLikeImport(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"policy.set","params":{"document":{"schemaVersion":1,"id":"p1","executeOperations":true}}}`)

	if !strings.Contains(out, "unknown field") {
		t.Fatalf("policy.set should refuse unknown fields: %s", out)
	}
	if len(monitor.documents) != 0 {
		t.Fatalf("a rejected policy must not be stored: %+v", monitor.documents)
	}
}

func TestIncludeAllFlagsRiskHiddenAdvice(t *testing.T) {
	hidden := recommend.Recommendation{ID: "rec-1", Family: "hibernate_inactive_project", Risk: assets.RiskHigh}
	visible := recommend.Recommendation{ID: "rec-2", Family: "consolidate_runtime", Risk: assets.RiskLow}
	scan := &activeScan{
		status: "complete",
		advice: advisor.Result{
			All:             []recommend.Recommendation{hidden, visible},
			Recommendations: []recommend.Recommendation{visible},
			HiddenByRisk:    []recommend.Recommendation{hidden},
		},
	}

	listed := recommendationsList("scan_01", scan, "", IncludeAll)
	if len(listed.Recommendations) != 2 {
		t.Fatalf("include=all returned %d items, want both", len(listed.Recommendations))
	}
	var flagged int
	for _, item := range listed.Recommendations {
		if item.HiddenByRisk {
			flagged++
			if item.ID != "rec-1" {
				t.Fatalf("wrong item flagged as risk-hidden: %+v", item)
			}
		}
	}
	if flagged != 1 {
		t.Fatalf("%d items flagged as risk-hidden, want 1", flagged)
	}

	single, found := findRecommendation(scan, "rec-1")
	if !found || !single.HiddenByRisk {
		t.Fatalf("recommendations.get should report the risk-hidden state: %+v", single)
	}
}

func TestSuppressRequiresATarget(t *testing.T) {
	out := serveRequests(t, monitoringOptions(newFakeMonitoring()),
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.suppress","params":{"reason":"all of it"}}`)
	if !strings.Contains(out, "recommendationId or family is required") {
		t.Fatalf("a suppression with no target must be refused: %s", out)
	}
}

func TestFeedbackIsRecordedAndMarkedLocal(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.feedback","params":{"recommendationId":"rec-1","verdict":"rejected","note":"we need both versions"}}`)

	var result RecommendationsFeedbackResult
	decodeResult(t, out, &result)
	if result.Verdict != store.VerdictRejected || !result.LocalOnly {
		t.Fatalf("feedback result = %+v, want a local-only rejection", result)
	}
	if len(monitor.feedback) != 1 || monitor.feedback[0].Note != "we need both versions" {
		t.Fatalf("stored feedback = %+v", monitor.feedback)
	}
}

func TestFeedbackRejectsAnUnknownVerdict(t *testing.T) {
	monitor := newFakeMonitoring()
	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.feedback","params":{"recommendationId":"rec-1","verdict":"whatever"}}`)
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("an unknown verdict must be refused: %s", out)
	}
	if len(monitor.feedback) != 0 {
		t.Fatal("a rejected verdict must not be stored")
	}
}

func TestTrendsListAnalysesStoredHistory(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	monitor := newFakeMonitoring()
	monitor.snapshots = []trend.Snapshot{
		{
			ScanID: "s1", CapturedAt: base, ScopeKey: "scope", RootCount: 1, AllocatedBytes: 100 << 30,
			Measurements: []trend.Measurement{{Key: trend.TotalKey, Label: "All scanned storage", AllocatedBytes: 100 << 30}},
		},
		{
			ScanID: "s2", CapturedAt: base.Add(24 * time.Hour), ScopeKey: "scope", RootCount: 1, AllocatedBytes: 110 << 30,
			Measurements: []trend.Measurement{{Key: trend.TotalKey, Label: "All scanned storage", AllocatedBytes: 110 << 30}},
		},
	}

	out := serveRequests(t, monitoringOptions(monitor),
		`{"jsonrpc":"2.0","id":"1","method":"trends.list","params":{"limit":10}}`)

	var result TrendsListResult
	decodeResult(t, out, &result)
	if result.Trends.SnapshotCount != 2 {
		t.Fatalf("snapshotCount = %d", result.Trends.SnapshotCount)
	}
	if result.Trends.Total.DeltaBytes != 10<<30 {
		t.Fatalf("total delta = %d, want 10 GiB", result.Trends.Total.DeltaBytes)
	}
	if result.Trends.Total.Direction != trend.DirectionGrowing {
		t.Fatalf("direction = %q", result.Trends.Total.Direction)
	}
	if len(result.Trends.Notes) == 0 {
		t.Fatal("a trend report should state what a movement can and cannot mean")
	}
}

func TestTrendsListExplainsAnEmptyHistory(t *testing.T) {
	out := serveRequests(t, monitoringOptions(newFakeMonitoring()),
		`{"jsonrpc":"2.0","id":"1","method":"trends.list","params":{}}`)
	var result TrendsListResult
	decodeResult(t, out, &result)
	if result.Trends.SnapshotCount != 0 || len(result.Trends.Notes) == 0 {
		t.Fatalf("empty history should be explained: %+v", result.Trends)
	}
}

func TestScanStartCanResolvePolicyRoots(t *testing.T) {
	monitor := newFakeMonitoring()
	document := policy.DefaultDocument("p1")
	document.Roots = []policy.Root{{Alias: policy.AliasHome, Relative: "Projects"}}
	validated, err := policy.Validate(document)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	monitor.documents["p1"] = validated
	monitor.active = "p1"

	// The absolute paths a policy resolves to must stay inside the engine, so
	// resolution is asserted here rather than through a response body.
	resolved, failure := NewServer(monitoringOptions(monitor)).policyRoots(context.Background(), "")
	if failure != nil {
		t.Fatalf("policyRoots: %+v", failure)
	}
	if len(resolved) != 1 || resolved[0] != "/Users/example/Projects" {
		t.Fatalf("resolved roots = %v", resolved)
	}
}

func TestScanStartWithoutRootsOrPolicyRootsFails(t *testing.T) {
	out := serveRequests(t, monitoringOptions(newFakeMonitoring()),
		`{"jsonrpc":"2.0","id":"1","method":"scan.start","params":{"roots":[]}}`)
	if !strings.Contains(out, "at least one scan root is required") {
		t.Fatalf("a scan with no roots must be refused: %s", out)
	}
}

func TestPolicyRootsFailsWhenNoneAreDeclared(t *testing.T) {
	server := NewServer(monitoringOptions(newFakeMonitoring()))
	if _, failure := server.policyRoots(context.Background(), ""); failure == nil {
		t.Fatal("a policy with no roots cannot start a scan")
	}
}
