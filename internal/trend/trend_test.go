package trend

import (
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/recommend"
)

var base = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

const gib = int64(1) << 30

func snapshotAt(scanID string, at time.Time, scope string, series map[string]int64) Snapshot {
	snapshot := Snapshot{ScanID: scanID, CapturedAt: at, ScopeKey: scope, RootCount: 1}
	for key, bytes := range series {
		snapshot.Measurements = append(snapshot.Measurements, Measurement{
			Key: key, Label: key, AllocatedBytes: bytes,
		})
		if key == TotalKey {
			snapshot.AllocatedBytes = bytes
		}
	}
	return snapshot
}

func TestScopeKeyIgnoresRootOrderAndSeparatesSelections(t *testing.T) {
	first := ScopeKeyFor([]string{"/a/one", "/a/two"})
	reordered := ScopeKeyFor([]string{"/a/two", "/a/one"})
	if first != reordered {
		t.Fatalf("scope key changed with root order: %q vs %q", first, reordered)
	}
	if first == ScopeKeyFor([]string{"/a/one"}) {
		t.Fatal("a narrower selection must produce a different scope")
	}
	if ScopeKeyFor(nil) != "empty" {
		t.Fatalf("no roots should be an explicit key, got %q", ScopeKeyFor(nil))
	}
}

// A scope key is a fingerprint of local paths and must not carry them.
func TestScopeKeyDoesNotEmbedPaths(t *testing.T) {
	key := ScopeKeyFor([]string{"/Users/example/Projects/secret-client"})
	for _, fragment := range []string{"Users", "example", "secret-client", "/"} {
		if contains(key, fragment) {
			t.Fatalf("scope key %q leaked %q", key, fragment)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestCaptureUsesExclusiveAttributedBytesOnly(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		{
			ID: "p1", Kind: assets.KindProject, Ecosystem: "node", Class: assets.ClassProject,
			Size: assets.Size{Attributed: true, AllocatedBytes: 10 * gib, ExclusiveAllocatedBytes: 2 * gib, LogicalBytes: 2 * gib},
		},
		{
			ID: "i1", Kind: assets.KindProjectLocalInstall, Ecosystem: "node", Class: assets.ClassProjectLocalInstall,
			Size: assets.Size{Attributed: true, AllocatedBytes: 8 * gib, ExclusiveAllocatedBytes: 8 * gib, LogicalBytes: 8 * gib},
		},
		{
			// Unattributed: contributes a count but must contribute no bytes.
			ID: "u1", Kind: assets.KindProject, Ecosystem: "rust", Class: assets.ClassProject,
			Size: assets.Size{AllocatedBytes: 99 * gib},
		},
	}}
	recommendations := []recommend.Recommendation{
		{ID: "r1", Savings: recommend.Savings{LowBytes: gib, HighBytes: 2 * gib}},
	}

	snapshot := Capture("scan_1", base, []string{"/r"}, Totals{
		EntriesVisited: 400, LogicalBytes: 10 * gib, AllocatedBytes: 12 * gib,
	}, graph, recommendations)

	node := measurement(t, snapshot, SeriesEcosystem+":node")
	if node.AllocatedBytes != 10*gib {
		t.Fatalf("node allocated = %d, want exclusive bytes summed (10 GiB)", node.AllocatedBytes)
	}
	rust := measurement(t, snapshot, SeriesEcosystem+":rust")
	if rust.AllocatedBytes != 0 {
		t.Fatalf("unattributed asset contributed %d bytes", rust.AllocatedBytes)
	}
	if rust.ItemCount != 1 {
		t.Fatalf("unattributed asset should still be counted, got %d", rust.ItemCount)
	}
	total := measurement(t, snapshot, TotalKey)
	if total.AllocatedBytes != 12*gib {
		t.Fatalf("total = %d, want the scan total", total.AllocatedBytes)
	}
	if snapshot.RecommendationCount != 1 || snapshot.SavingsLowBytes != gib || snapshot.SavingsHighBytes != 2*gib {
		t.Fatalf("advice roll-up = %+v", snapshot)
	}
}

func TestCaptureMarksUncertainSeries(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{{
		ID: "s1", Kind: assets.KindDependencyStore, Ecosystem: "node", Class: assets.ClassDependencyStore,
		Size: assets.Size{Attributed: true, AllocatedBytes: gib, ExclusiveAllocatedBytes: gib, Shared: true},
	}}}
	snapshot := Capture("scan_1", base, []string{"/r"}, Totals{}, graph, nil)
	if !measurement(t, snapshot, SeriesEcosystem+":node").Uncertain {
		t.Fatal("a shared store must mark its series uncertain")
	}
}

func measurement(t *testing.T, snapshot Snapshot, key string) Measurement {
	t.Helper()
	for _, item := range snapshot.Measurements {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("no measurement %q in %+v", key, snapshot.Measurements)
	return Measurement{}
}

func TestAnalyzeWithoutHistory(t *testing.T) {
	report := Analyze(nil, Options{Now: base})
	if report.SnapshotCount != 0 || len(report.Notes) == 0 {
		t.Fatalf("empty history should be explained, got %+v", report)
	}

	single := Analyze([]Snapshot{snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 10 * gib})}, Options{Now: base})
	if single.SnapshotCount != 1 {
		t.Fatalf("snapshotCount = %d, want 1", single.SnapshotCount)
	}
	if len(single.Series) != 0 {
		t.Fatalf("a single snapshot cannot produce series deltas: %+v", single.Series)
	}
	if len(single.Notes) == 0 {
		t.Fatal("a single snapshot should say why there is no trend")
	}
}

// Comparing scans of different roots would invent growth that never happened.
func TestAnalyzeOnlyComparesMatchingScopes(t *testing.T) {
	snapshots := []Snapshot{
		snapshotAt("wide", base, "scope-home", map[string]int64{TotalKey: 500 * gib}),
		snapshotAt("narrow1", base.Add(24*time.Hour), "scope-projects", map[string]int64{TotalKey: 10 * gib}),
		snapshotAt("narrow2", base.Add(48*time.Hour), "scope-projects", map[string]int64{TotalKey: 11 * gib}),
	}
	report := Analyze(snapshots, Options{Now: base.Add(48 * time.Hour)})

	if report.ComparedScope != "scope-projects" {
		t.Fatalf("comparedScope = %q, want the newest scope", report.ComparedScope)
	}
	if report.SnapshotCount != 2 || report.SkippedSnapshots != 1 {
		t.Fatalf("counts = %d compared / %d skipped", report.SnapshotCount, report.SkippedSnapshots)
	}
	if report.Total.DeltaBytes != gib {
		t.Fatalf("total delta = %d, want 1 GiB rather than a cross-scope difference", report.Total.DeltaBytes)
	}
	for _, regression := range report.Regressions {
		if regression.DeltaBytes < 0 {
			t.Fatalf("cross-scope comparison leaked into regressions: %+v", regression)
		}
	}
}

func TestAnalyzeDirectionAndRate(t *testing.T) {
	key := SeriesEcosystem + ":node"
	snapshots := []Snapshot{
		snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 100 * gib, key: 10 * gib}),
		snapshotAt("s2", base.Add(48*time.Hour), "scope", map[string]int64{TotalKey: 104 * gib, key: 14 * gib}),
	}
	report := Analyze(snapshots, Options{Now: base.Add(48 * time.Hour)})

	node := series(t, report, key)
	if node.Direction != DirectionGrowing {
		t.Fatalf("direction = %q, want growing", node.Direction)
	}
	if node.DeltaBytes != 4*gib {
		t.Fatalf("delta = %d, want 4 GiB", node.DeltaBytes)
	}
	if node.PercentChange != 40 {
		t.Fatalf("percentChange = %v, want 40", node.PercentChange)
	}
	if node.BytesPerDay != float64(2*gib) {
		t.Fatalf("bytesPerDay = %v, want 2 GiB/day", node.BytesPerDay)
	}
}

func TestAnalyzeTreatsSmallMovesAsStable(t *testing.T) {
	snapshots := []Snapshot{
		snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 100 * gib}),
		snapshotAt("s2", base.Add(24*time.Hour), "scope", map[string]int64{TotalKey: 100*gib + (100 << 10)}),
	}
	report := Analyze(snapshots, Options{Now: base.Add(24 * time.Hour)})
	if report.Total.Direction != DirectionStable {
		t.Fatalf("direction = %q, want stable for a 100 KiB move", report.Total.Direction)
	}
	if len(report.Regressions) != 0 {
		t.Fatalf("ordinary churn produced regressions: %+v", report.Regressions)
	}
}

func TestSuddenGrowthNeedsBothAbsoluteAndRelativeSize(t *testing.T) {
	key := SeriesClass + ":ai"
	tests := []struct {
		name       string
		from, to   int64
		wantSignal bool
	}{
		{name: "large and proportionally large", from: 2 * gib, to: 6 * gib, wantSignal: true},
		{name: "large but proportionally small", from: 400 * gib, to: 402 * gib, wantSignal: false},
		{name: "proportionally large but small", from: 100 << 20, to: 400 << 20, wantSignal: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshots := []Snapshot{
				snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 500 * gib, key: test.from}),
				snapshotAt("s2", base.Add(24*time.Hour), "scope", map[string]int64{TotalKey: 500 * gib, key: test.to}),
			}
			report := Analyze(snapshots, Options{Now: base.Add(24 * time.Hour)})
			got := hasRegression(report, RegressionSuddenGrowth, key)
			if got != test.wantSignal {
				t.Fatalf("sudden growth signalled = %v, want %v (regressions: %+v)", got, test.wantSignal, report.Regressions)
			}
		})
	}
}

func TestSustainedGrowthNeedsConsecutiveIncreases(t *testing.T) {
	key := SeriesEcosystem + ":python"
	rising := []Snapshot{
		snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 100 * gib, key: 10 * gib}),
		snapshotAt("s2", base.Add(24*time.Hour), "scope", map[string]int64{TotalKey: 100 * gib, key: 11 * gib}),
		snapshotAt("s3", base.Add(48*time.Hour), "scope", map[string]int64{TotalKey: 100 * gib, key: 12 * gib}),
		snapshotAt("s4", base.Add(72*time.Hour), "scope", map[string]int64{TotalKey: 100 * gib, key: 13 * gib}),
	}
	report := Analyze(rising, Options{Now: base.Add(72 * time.Hour)})
	if !hasRegression(report, RegressionSustainedGrowth, key) {
		t.Fatalf("three consecutive increases should be a sustained-growth notice: %+v", report.Regressions)
	}

	// One dip in the window breaks the pattern.
	dipping := append([]Snapshot(nil), rising...)
	dipping[2] = snapshotAt("s3", base.Add(48*time.Hour), "scope", map[string]int64{TotalKey: 100 * gib, key: 9 * gib})
	dipped := Analyze(dipping, Options{Now: base.Add(72 * time.Hour)})
	if hasRegression(dipped, RegressionSustainedGrowth, key) {
		t.Fatalf("a dip must break the sustained-growth run: %+v", dipped.Regressions)
	}
}

func TestAdviceGrowthUsesTheConservativeBound(t *testing.T) {
	first := snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 100 * gib})
	first.SavingsLowBytes, first.SavingsHighBytes, first.RecommendationCount = gib, 40*gib, 2
	second := snapshotAt("s2", base.Add(24*time.Hour), "scope", map[string]int64{TotalKey: 100 * gib})
	// The upper bound explodes while the conservative bound barely moves: the
	// signal must follow the bound that is not double-counted.
	second.SavingsLowBytes, second.SavingsHighBytes, second.RecommendationCount = gib+(1<<20), 400*gib, 3

	report := Analyze([]Snapshot{first, second}, Options{Now: base.Add(24 * time.Hour)})
	for _, regression := range report.Regressions {
		if regression.Kind == RegressionAdviceGrowth {
			t.Fatalf("advice growth fired on the optimistic bound: %+v", regression)
		}
	}

	second.SavingsLowBytes = 5 * gib
	grown := Analyze([]Snapshot{first, second}, Options{Now: base.Add(24 * time.Hour)})
	if !hasRegression(grown, RegressionAdviceGrowth, "") {
		t.Fatalf("a 4 GiB rise in the conservative bound should be reported: %+v", grown.Regressions)
	}
}

func TestAnalyzeDropsSeriesBelowTheFloor(t *testing.T) {
	small := SeriesClass + ":other"
	snapshots := []Snapshot{
		snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 100 * gib, small: 1 << 20}),
		snapshotAt("s2", base.Add(24*time.Hour), "scope", map[string]int64{TotalKey: 100 * gib, small: 2 << 20}),
	}
	report := Analyze(snapshots, Options{Now: base.Add(24 * time.Hour)})
	for _, item := range report.Series {
		if item.Key == small {
			t.Fatalf("a 2 MiB series should be below the reporting floor: %+v", item)
		}
	}
}

func TestAnalyzeIsDeterministic(t *testing.T) {
	snapshots := []Snapshot{
		snapshotAt("s2", base.Add(24*time.Hour), "scope", map[string]int64{TotalKey: 110 * gib, SeriesEcosystem + ":node": 20 * gib, SeriesEcosystem + ":python": 20 * gib}),
		snapshotAt("s1", base, "scope", map[string]int64{TotalKey: 100 * gib, SeriesEcosystem + ":node": 10 * gib, SeriesEcosystem + ":python": 15 * gib}),
	}
	first := Analyze(snapshots, Options{Now: base.Add(24 * time.Hour)})
	second := Analyze(snapshots, Options{Now: base.Add(24 * time.Hour)})
	if len(first.Series) != len(second.Series) {
		t.Fatalf("series count differed between runs: %d vs %d", len(first.Series), len(second.Series))
	}
	for index := range first.Series {
		if first.Series[index].Key != second.Series[index].Key {
			t.Fatalf("series order differed at %d: %q vs %q", index, first.Series[index].Key, second.Series[index].Key)
		}
	}
	// Ordered by delta: node grew 10 GiB, python 5 GiB.
	if first.Series[0].Key != SeriesEcosystem+":node" {
		t.Fatalf("series[0] = %q, want the largest grower first", first.Series[0].Key)
	}
	// Unsorted input must not change the outcome.
	if first.Total.DeltaBytes != 10*gib {
		t.Fatalf("total delta = %d, want snapshots ordered by capture time", first.Total.DeltaBytes)
	}
}

func series(t *testing.T, report Report, key string) Series {
	t.Helper()
	if key == TotalKey {
		return report.Total
	}
	for _, item := range report.Series {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("no series %q in %+v", key, report.Series)
	return Series{}
}

func hasRegression(report Report, kind, seriesKey string) bool {
	for _, regression := range report.Regressions {
		if regression.Kind != kind {
			continue
		}
		if seriesKey == "" || regression.SeriesKey == seriesKey {
			return true
		}
	}
	return false
}
