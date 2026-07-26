package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/portfolio"
	"github.com/yudgnahk/devhearth/internal/recommend"
	"github.com/yudgnahk/devhearth/internal/scan"
)

const testRoot = "/Users/example/Projects"

func adviceScan() *activeScan {
	return &activeScan{
		status: "complete",
		result: scan.Result{Roots: []string{testRoot}},
		advice: advisor.Result{
			Graph: assets.Graph{Assets: []assets.Asset{{
				ID: "install-1", Kind: assets.KindProjectLocalInstall, DisplayName: "node_modules",
				Path: testRoot + "/app/node_modules", Risk: assets.RiskLow, Ecosystem: "node",
				DetectorID: "detect.node", DetectorVersion: 1,
				Size: assets.Size{Attributed: true, AllocatedBytes: 4096, ExclusiveAllocatedBytes: 4096, Uncertain: true},
			}}},
			Assessments: []portfolio.Assessment{{
				Ecosystem: "node", Depth: portfolio.DepthDeep, ProjectCount: 2, Baseline: "npm",
				RecommendedTool: "pnpm", ProjectLocalInstallBytes: 8192,
				Notes: []string{"no content hashing has run"},
				Options: []portfolio.Option{
					{
						Tool: "pnpm", Rank: 1, Score: 0.7, Confidence: 0.7, Installed: true,
						ImmediateSavingsLowBytes: 1024, ImmediateSavingsHighBytes: 2048,
						Factors:         []portfolio.Factor{{Kind: portfolio.FactorInUseShare, Score: 0.5, Weight: 0.3, Detail: "1 of 2 projects"}},
						DominantFactors: []portfolio.FactorKind{portfolio.FactorInUseShare},
					},
					{Tool: "npm", StayPut: true, Rank: 2, Score: 0.6, Confidence: 0.8, ProjectsUsing: 2},
				},
			}},
			Recommendations: []recommend.Recommendation{{
				ID: "rec_000000000001", Family: recommend.FamilyAdoptSharedStore,
				Title: "Share node dependencies through the pnpm store", Ecosystem: "node",
				Explanation: "two installs could share one store", Risk: assets.RiskMedium,
				Confidence: 0.7, Priority: 1000,
				Savings:          recommend.Savings{LowBytes: 1024, HighBytes: 2048, Uncertain: true},
				Blockers:         []string{"native addons unverified"},
				AffectedAssetIDs: []string{"install-1"},
				Evidence: []assets.Evidence{
					{Kind: "path_signature", Value: testRoot + "/app/node_modules", Confidence: 0.95},
					{Kind: "matching_name_and_size", Value: "2 copies at 4.0 KiB each", Confidence: 0.6},
				},
				Alternatives: []recommend.Alternative{{Label: "npm", StayPut: true, Rank: 2, Score: 0.6}},
				RuleID:       "rule.adopt_shared_store", RuleVersion: 1,
			}},
		},
	}
}

func TestRecommendationsListRedactsEvidencePaths(t *testing.T) {
	listed := recommendationsList("scan_01", adviceScan(), "")

	if len(listed.Recommendations) != 1 {
		t.Fatalf("want one recommendation, got %d", len(listed.Recommendations))
	}
	recommendation := listed.Recommendations[0]
	if recommendation.Evidence[0].Value != "<selected-root-1>/app/node_modules" {
		t.Fatalf("evidence path not redacted: %q", recommendation.Evidence[0].Value)
	}
	if recommendation.Evidence[1].Value != "2 copies at 4.0 KiB each" {
		t.Fatalf("non-path evidence should pass through: %q", recommendation.Evidence[1].Value)
	}
	if len(recommendation.AffectedAssets) != 1 || recommendation.AffectedAssets[0].Path != "<selected-root-1>/app/node_modules" {
		t.Fatalf("affected asset path not redacted: %#v", recommendation.AffectedAssets)
	}
	encoded, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/Users/") {
		t.Fatalf("absolute path leaked in recommendations: %s", encoded)
	}
}

func TestRecommendationsListCarriesSafetyFields(t *testing.T) {
	recommendation := recommendationsList("scan_01", adviceScan(), "").Recommendations[0]

	if recommendation.Risk != "medium" || recommendation.Confidence == 0 {
		t.Fatalf("risk and confidence must travel: %#v", recommendation)
	}
	if recommendation.Savings.HighBytes != 2048 || !recommendation.Savings.Uncertain {
		t.Fatalf("savings range must travel as a range: %#v", recommendation.Savings)
	}
	if len(recommendation.Blockers) != 1 {
		t.Fatalf("blockers must travel: %#v", recommendation.Blockers)
	}
	if len(recommendation.Alternatives) != 1 || !recommendation.Alternatives[0].StayPut {
		t.Fatalf("the stay-put alternative must travel: %#v", recommendation.Alternatives)
	}
	if recommendation.RuleID == "" || recommendation.RuleVersion == 0 {
		t.Fatalf("rule identity must travel: %#v", recommendation)
	}
	if !recommendation.AdviceOnly {
		t.Fatal("this protocol version cannot execute advice and must say so")
	}
}

func TestRecommendationsListFiltersByFamily(t *testing.T) {
	active := adviceScan()

	if got := recommendationsList("scan_01", active, "adopt_shared_store"); len(got.Recommendations) != 1 {
		t.Fatalf("matching family should return the recommendation: %#v", got)
	}
	if got := recommendationsList("scan_01", active, "hibernate_inactive_project"); len(got.Recommendations) != 0 {
		t.Fatalf("non-matching family should return nothing: %#v", got)
	}
}

func TestFitListProjectsFactorsAndStayPut(t *testing.T) {
	listed := fitList("scan_01", adviceScan())

	if len(listed.Fit) != 1 {
		t.Fatalf("want one assessment, got %d", len(listed.Fit))
	}
	assessment := listed.Fit[0]
	if assessment.Depth != "deep" || assessment.RecommendedTool != "pnpm" {
		t.Fatalf("assessment header wrong: %#v", assessment)
	}
	if len(assessment.Options) != 2 {
		t.Fatalf("both options must travel: %#v", assessment.Options)
	}
	if len(assessment.Options[0].Factors) != 1 || assessment.Options[0].Factors[0].Kind != "in_use_share" {
		t.Fatalf("factor evidence must travel: %#v", assessment.Options[0].Factors)
	}
	if !assessment.Options[1].StayPut {
		t.Fatal("the stay-put option must be identifiable")
	}
	if len(assessment.Notes) == 0 {
		t.Fatal("scope limits must travel with the assessment")
	}
}

func TestFitGetSelectsOneEcosystem(t *testing.T) {
	active := adviceScan()

	if _, found := findAssessment(active, "node"); !found {
		t.Fatal("node assessment should be found")
	}
	if _, found := findAssessment(active, "rust"); found {
		t.Fatal("an ecosystem without an assessment must not be invented")
	}
}

func TestAdviceSummaryRollsUpRangesSeparately(t *testing.T) {
	summary := adviceSummary(adviceScan())

	if summary == nil {
		t.Fatal("advice summary missing")
	}
	if summary.RecommendationCount != 1 || summary.BlockedCount != 1 {
		t.Fatalf("counts wrong: %#v", summary)
	}
	if summary.SavingsLowBytes != 1024 || summary.SavingsHighBytes != 2048 {
		t.Fatalf("savings bounds must stay separate: %#v", summary)
	}
	if !summary.SavingsUncertain {
		t.Fatal("uncertainty must roll up")
	}
	if len(summary.Fit) != 1 || summary.Fit[0].Ecosystem != "node" {
		t.Fatalf("fit headlines wrong: %#v", summary.Fit)
	}
	if summary.ByFamily["adopt_shared_store"] != 1 || summary.ByRisk["medium"] != 1 {
		t.Fatalf("family and risk breakdown wrong: %#v", summary)
	}
}

func TestAdviceSummaryAbsentWithoutAnalysis(t *testing.T) {
	active := &activeScan{status: "complete", result: scan.Result{Roots: []string{testRoot}}}

	if summary := adviceSummary(active); summary != nil {
		t.Fatalf("no analysis should mean no advice section: %#v", summary)
	}
}

func TestAssetSummaryCarriesAttributedSize(t *testing.T) {
	summary := summarizeAsset(adviceScan().advice.Graph.Assets[0], []string{testRoot})

	if !summary.Size.Attributed || summary.Size.ExclusiveAllocatedBytes != 4096 {
		t.Fatalf("attributed size must travel: %#v", summary.Size)
	}
	if !summary.Size.Uncertain {
		t.Fatal("size uncertainty must travel")
	}
}

// serveRequests drives the server over a scripted stdin and returns its output.
func serveRequests(t *testing.T, options ServerOptions, requests ...string) string {
	t.Helper()
	var output bytes.Buffer
	input := strings.Join(requests, "\n") + "\n"
	if err := NewServer(options).Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestAdviceMethodsRejectUnknownScan(t *testing.T) {
	cases := []struct {
		name    string
		request string
	}{
		{name: "fit.list", request: `{"jsonrpc":"2.0","id":"1","method":"fit.list","params":{"scanId":"missing"}}`},
		{name: "fit.get", request: `{"jsonrpc":"2.0","id":"1","method":"fit.get","params":{"scanId":"missing","ecosystem":"node"}}`},
		{name: "recommendations.list", request: `{"jsonrpc":"2.0","id":"1","method":"recommendations.list","params":{"scanId":"missing"}}`},
		{name: "recommendations.get", request: `{"jsonrpc":"2.0","id":"1","method":"recommendations.get","params":{"scanId":"missing","recommendationId":"rec_1"}}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := serveRequests(t, ServerOptions{}, testCase.request)
			if !strings.Contains(out, `"code":-32004`) {
				t.Fatalf("expected unavailable error, got %s", out)
			}
		})
	}
}

func TestAdviceMethodsValidateParams(t *testing.T) {
	cases := []string{
		`{"jsonrpc":"2.0","id":"1","method":"fit.get","params":{"scanId":"scan_000001"}}`,
		`{"jsonrpc":"2.0","id":"1","method":"recommendations.get","params":{"scanId":"scan_000001"}}`,
		`{"jsonrpc":"2.0","id":"1","method":"fit.list","params":{}}`,
	}
	for _, request := range cases {
		out := serveRequests(t, ServerOptions{}, request)
		if !strings.Contains(out, `"code":-32602`) {
			t.Fatalf("expected invalid params for %s, got %s", request, out)
		}
	}
}

func TestRunScanProducesAdviceAndPersistsAttributedGraph(t *testing.T) {
	inventory := scan.Result{
		Roots: []string{testRoot},
		Entries: []scan.Entry{
			{Path: testRoot, Kind: "directory"},
			{Path: testRoot + "/app", ParentPath: testRoot, Kind: "directory"},
			{Path: testRoot + "/app/node_modules", ParentPath: testRoot + "/app", Kind: "directory", AllocatedBytes: 8192},
		},
	}
	graph := assets.Graph{Assets: []assets.Asset{{
		ID: "install-1", Kind: assets.KindProjectLocalInstall, DisplayName: "node_modules",
		Path: testRoot + "/app/node_modules", Ecosystem: "node", Risk: assets.RiskLow,
	}}}

	var persisted advisor.Result
	var persistedStatus string
	options := ServerOptions{
		Inventory: func(context.Context, []string, func(scan.Progress)) (scan.Result, error) {
			return inventory, nil
		},
		Detect: func(context.Context, scan.Result, func(detect.Progress)) (assets.Graph, error) {
			return graph, nil
		},
		Advise: func(_ context.Context, _ scan.Result, detected assets.Graph, index map[string][]scan.DirectoryNode) (advisor.Result, error) {
			return advisor.Analyze(detected, index, advisor.Options{}), nil
		},
		OnComplete: func(_ context.Context, _ scan.Result, advice advisor.Result, status string, _ map[string][]scan.DirectoryNode, _ func(written, total int64)) error {
			persisted = advice
			persistedStatus = status
			return nil
		},
	}

	out := serveRequests(t, options,
		`{"jsonrpc":"2.0","id":"1","method":"scan.start","params":{"roots":["`+testRoot+`"]}}`)

	if !strings.Contains(out, `"phase":"advice"`) {
		t.Fatalf("advice phase should be reported: %s", out)
	}
	if persistedStatus != "complete" {
		t.Fatalf("status = %q", persistedStatus)
	}
	if len(persisted.Graph.Assets) != 1 || !persisted.Graph.Assets[0].Size.Attributed {
		t.Fatalf("persistence must receive the attributed graph: %#v", persisted.Graph.Assets)
	}
	if persisted.Graph.Assets[0].Size.AllocatedBytes != 8192 {
		t.Fatalf("attributed bytes = %d, want 8192", persisted.Graph.Assets[0].Size.AllocatedBytes)
	}
}

func TestRunScanFailsWhenAnalysisFails(t *testing.T) {
	options := ServerOptions{
		Inventory: func(context.Context, []string, func(scan.Progress)) (scan.Result, error) {
			return scan.Result{Roots: []string{testRoot}, Entries: []scan.Entry{{Path: testRoot, Kind: "directory"}}}, nil
		},
		Detect: func(context.Context, scan.Result, func(detect.Progress)) (assets.Graph, error) {
			return assets.Graph{}, nil
		},
		Advise: func(context.Context, scan.Result, assets.Graph, map[string][]scan.DirectoryNode) (advisor.Result, error) {
			return advisor.Result{}, errAnalysis
		},
	}

	out := serveRequests(t, options,
		`{"jsonrpc":"2.0","id":"1","method":"scan.start","params":{"roots":["`+testRoot+`"]}}`)

	if !strings.Contains(out, `"phase":"failed"`) {
		t.Fatalf("a failed analysis must fail the scan: %s", out)
	}
}

// errAnalysis stands in for any analysis failure.
var errAnalysis = errors.New("analysis failed")

func TestWithCompletedScanMapsStatusAndProjectionErrors(t *testing.T) {
	// Every completed-scan handler shares this helper, so its status gate and
	// error mapping are tested once here rather than per method.
	server := NewServer(ServerOptions{})
	server.scans["done"] = adviceScan()
	server.scans["busy"] = &activeScan{status: "running"}
	server.scans["stopping"] = &activeScan{status: "cancelling"}

	cases := []struct {
		name      string
		scanID    string
		project   func(*activeScan) (any, *Error)
		wantInOut string
	}{
		{
			name:   "completed scan projects a result",
			scanID: "done",
			project: func(*activeScan) (any, *Error) {
				return ScanStatus{ScanID: "done", Status: "complete"}, nil
			},
			wantInOut: `"status":"complete"`,
		},
		{
			name:      "unknown scan is unavailable",
			scanID:    "missing",
			project:   func(*activeScan) (any, *Error) { return ScanStatus{}, nil },
			wantInOut: `"code":-32004`,
		},
		{
			name:      "running scan is unavailable",
			scanID:    "busy",
			project:   func(*activeScan) (any, *Error) { return ScanStatus{}, nil },
			wantInOut: `"code":-32004`,
		},
		{
			name:      "cancelling scan is unavailable",
			scanID:    "stopping",
			project:   func(*activeScan) (any, *Error) { return ScanStatus{}, nil },
			wantInOut: `"code":-32004`,
		},
		{
			name:   "projection error reaches the client with its own code",
			scanID: "done",
			project: func(*activeScan) (any, *Error) {
				return nil, &Error{Code: -32005, Message: "asset not found"}
			},
			wantInOut: `"code":-32005`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			encoder := json.NewEncoder(&output)
			err := server.withCompletedScan(encoder, json.RawMessage(`"1"`), testCase.scanID, "not available", testCase.project)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), testCase.wantInOut) {
				t.Fatalf("output %s does not contain %s", output.String(), testCase.wantInOut)
			}
		})
	}
}

func TestAssetsGetReportsMissingAssetThroughTheSharedHelper(t *testing.T) {
	server := NewServer(ServerOptions{})
	server.scans["done"] = adviceScan()

	var output bytes.Buffer
	request := `{"jsonrpc":"2.0","id":"1","method":"assets.get","params":{"scanId":"done","assetId":"nope"}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"code":-32005`) {
		t.Fatalf("expected asset-not-found error, got %s", output.String())
	}

	output.Reset()
	found := `{"jsonrpc":"2.0","id":"2","method":"assets.get","params":{"scanId":"done","assetId":"install-1"}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(found), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"displayName":"node_modules"`) {
		t.Fatalf("expected the asset summary, got %s", output.String())
	}
	if strings.Contains(output.String(), "/Users/") {
		t.Fatalf("absolute path leaked: %s", output.String())
	}
}

func TestFindRecommendationResolvesOnlyItsOwnAssets(t *testing.T) {
	active := adviceScan()
	// A second asset the recommendation does not name must not be resolved.
	active.advice.Graph.Assets = append(active.advice.Graph.Assets, assets.Asset{
		ID: "unrelated", Kind: assets.KindProject, DisplayName: "other",
		Path: testRoot + "/other", Risk: assets.RiskInformational,
	})

	summary, found := findRecommendation(active, "rec_000000000001")
	if !found {
		t.Fatal("recommendation should be found by id")
	}
	if len(summary.AffectedAssets) != 1 || summary.AffectedAssets[0].ID != "install-1" {
		t.Fatalf("only named assets belong in the summary: %#v", summary.AffectedAssets)
	}
	if _, found := findRecommendation(active, "rec_missing"); found {
		t.Fatal("an unknown id must not match")
	}
}
