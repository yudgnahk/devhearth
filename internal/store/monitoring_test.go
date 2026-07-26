package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/policy"
	"github.com/yudgnahk/devhearth/internal/scan"
	"github.com/yudgnahk/devhearth/internal/trend"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "inventory.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestActivePolicyReportsAbsenceRatherThanInventingOne(t *testing.T) {
	database := testStore(t)
	if _, err := database.ActivePolicy(context.Background()); !errors.Is(err, ErrNoPolicy) {
		t.Fatalf("ActivePolicy error = %v, want ErrNoPolicy", err)
	}
}

func TestEnsureActivePolicyCreatesAndReusesOne(t *testing.T) {
	database := testStore(t)
	ctx := context.Background()

	created, err := database.EnsureActivePolicy(ctx, "machine-policy")
	if err != nil {
		t.Fatalf("EnsureActivePolicy: %v", err)
	}
	if created.ID != "machine-policy" || created.FitMode != policy.FitModeBalanced {
		t.Fatalf("created policy = %+v", created)
	}
	again, err := database.EnsureActivePolicy(ctx, "different-id")
	if err != nil {
		t.Fatalf("EnsureActivePolicy second call: %v", err)
	}
	if again.ID != created.ID {
		t.Fatalf("a second call created a new policy: %q", again.ID)
	}
	policies, err := database.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(policies) != 1 || !policies[0].Active {
		t.Fatalf("policies = %+v, want exactly one active policy", policies)
	}
}

func TestSavePolicyRoundTrips(t *testing.T) {
	database := testStore(t)
	ctx := context.Background()

	document := policy.DefaultDocument("p1")
	document.Name = "Laptop"
	document.PreferredTools = map[string]string{"node": "pnpm"}
	document.Roots = []policy.Root{{Alias: policy.AliasHome, Relative: "Projects"}}
	document = document.WithSuppression(policy.Suppression{Family: "adopt_shared_store", Reason: "not yet"})

	if err := database.SavePolicy(ctx, document, true); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}
	loaded, err := database.ActivePolicy(ctx)
	if err != nil {
		t.Fatalf("ActivePolicy: %v", err)
	}
	if loaded.Name != "Laptop" || loaded.PreferredTools["node"] != "pnpm" {
		t.Fatalf("loaded policy = %+v", loaded)
	}
	if len(loaded.Suppressions) != 1 || loaded.Suppressions[0].Family != "adopt_shared_store" {
		t.Fatalf("suppressions = %+v", loaded.Suppressions)
	}
	byID, err := database.PolicyByID(ctx, "p1")
	if err != nil || byID.ID != "p1" {
		t.Fatalf("PolicyByID = %+v, %v", byID, err)
	}
}

func TestSavePolicyRejectsInvalidDocuments(t *testing.T) {
	database := testStore(t)
	invalid := policy.DefaultDocument("p1")
	invalid.Roots = []policy.Root{{Alias: policy.AliasHome, Relative: "../../etc"}}

	if err := database.SavePolicy(context.Background(), invalid, true); err == nil {
		t.Fatal("expected an escaping root to be refused before storage")
	}
	if _, err := database.ActivePolicy(context.Background()); !errors.Is(err, ErrNoPolicy) {
		t.Fatal("a rejected policy must not have been written")
	}
}

// Only one policy may be active, and activating another must not leave a window
// where two are.
func TestActivatingAPolicyDeactivatesThePrevious(t *testing.T) {
	database := testStore(t)
	ctx := context.Background()

	if err := database.SavePolicy(ctx, policy.DefaultDocument("first"), true); err != nil {
		t.Fatalf("SavePolicy first: %v", err)
	}
	if err := database.SavePolicy(ctx, policy.DefaultDocument("second"), true); err != nil {
		t.Fatalf("SavePolicy second: %v", err)
	}
	active, err := database.ActivePolicy(ctx)
	if err != nil {
		t.Fatalf("ActivePolicy: %v", err)
	}
	if active.ID != "second" {
		t.Fatalf("active policy = %q, want the most recently activated", active.ID)
	}
	policies, err := database.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	activeCount := 0
	for _, summary := range policies {
		if summary.Active {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("%d policies are active, want exactly 1", activeCount)
	}
}

func TestRecordFeedbackValidatesVerdicts(t *testing.T) {
	database := testStore(t)
	ctx := context.Background()

	if _, err := database.RecordFeedback(ctx, Feedback{RecommendationID: "rec-1", Verdict: "maybe"}); err == nil {
		t.Fatal("expected an unknown verdict to be rejected")
	}
	if _, err := database.RecordFeedback(ctx, Feedback{Verdict: VerdictAccepted}); err == nil {
		t.Fatal("expected feedback without a recommendation id to be rejected")
	}
	stored, err := database.RecordFeedback(ctx, Feedback{
		RecommendationID: "rec-1", Family: "adopt_shared_store", Ecosystem: "node",
		Verdict: VerdictAccepted, Note: "did this",
	})
	if err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if stored.ID == "" || stored.CreatedAt.IsZero() {
		t.Fatalf("stored feedback = %+v, want an id and timestamp", stored)
	}
}

func TestLatestFeedbackReturnsTheNewestVerdictPerRecommendation(t *testing.T) {
	database := testStore(t)
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	for _, entry := range []Feedback{
		{RecommendationID: "rec-1", Verdict: VerdictLater, CreatedAt: base},
		{RecommendationID: "rec-1", Verdict: VerdictRejected, CreatedAt: base.Add(time.Hour)},
		{RecommendationID: "rec-2", Verdict: VerdictAccepted, CreatedAt: base.Add(2 * time.Hour)},
	} {
		if _, err := database.RecordFeedback(ctx, entry); err != nil {
			t.Fatalf("RecordFeedback: %v", err)
		}
	}

	all, err := database.LatestFeedback(ctx, "")
	if err != nil {
		t.Fatalf("LatestFeedback: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("latest feedback = %+v, want one row per recommendation", all)
	}
	one, err := database.LatestFeedback(ctx, "rec-1")
	if err != nil {
		t.Fatalf("LatestFeedback(rec-1): %v", err)
	}
	if len(one) != 1 || one[0].Verdict != VerdictRejected {
		t.Fatalf("rec-1 latest = %+v, want the rejection", one)
	}
}

// The note is user text shown back to them; an unbounded paste must not become
// an unbounded row.
func TestRecordFeedbackCapsTheNote(t *testing.T) {
	database := testStore(t)
	stored, err := database.RecordFeedback(context.Background(), Feedback{
		RecommendationID: "rec-1", Verdict: VerdictUnclear, Note: strings.Repeat("x", 5000),
	})
	if err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if len(stored.Note) != 1000 {
		t.Fatalf("note length = %d, want it capped at 1000", len(stored.Note))
	}
}

func saveScanWithSnapshot(t *testing.T, database *Store, at time.Time, scope string, allocated int64) string {
	t.Helper()
	result := scan.Result{
		Roots: []string{"/fixtures"}, StartedAt: at, CompletedAt: at,
		AllocatedBytes: allocated,
		Entries: []scan.Entry{
			{Path: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: at},
		},
	}
	snapshot := trend.Snapshot{
		CapturedAt: at, ScopeKey: scope, RootCount: 1, AllocatedBytes: allocated,
		Measurements: []trend.Measurement{
			{Key: trend.TotalKey, Label: "All scanned storage", AllocatedBytes: allocated},
			{Key: trend.SeriesEcosystem + ":node", Label: "node", AllocatedBytes: allocated / 2, ItemCount: 3},
		},
	}
	id, err := database.SaveWithOptions(context.Background(), result, advisor.Result{}, "complete", SaveOptions{Snapshot: &snapshot})
	if err != nil {
		t.Fatalf("SaveWithOptions: %v", err)
	}
	return id
}

func TestSnapshotsPersistWithTheScanAndReadBackInOrder(t *testing.T) {
	database := testStore(t)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	first := saveScanWithSnapshot(t, database, base, "scope-a", 10<<30)
	second := saveScanWithSnapshot(t, database, base.Add(24*time.Hour), "scope-a", 12<<30)

	snapshots, err := database.ListSnapshots(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snapshots))
	}
	if snapshots[0].ScanID != first || snapshots[1].ScanID != second {
		t.Fatalf("snapshots are not chronological: %q then %q", snapshots[0].ScanID, snapshots[1].ScanID)
	}
	if len(snapshots[0].Measurements) != 2 {
		t.Fatalf("measurements = %+v, want both series", snapshots[0].Measurements)
	}

	report := trend.Analyze(snapshots, trend.Options{Now: base.Add(24 * time.Hour)})
	if report.Total.DeltaBytes != 2<<30 {
		t.Fatalf("total delta = %d, want 2 GiB from the persisted history", report.Total.DeltaBytes)
	}
}

// The stored snapshot must describe the scan it was committed with, not the id
// the caller happened to set.
func TestSnapshotAdoptsTheDurableScanID(t *testing.T) {
	database := testStore(t)
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	result := scan.Result{Roots: []string{"/fixtures"}, StartedAt: at, CompletedAt: at}
	snapshot := trend.Snapshot{ScanID: "not-the-real-id", CapturedAt: at, ScopeKey: "scope-a"}

	scanID, err := database.SaveWithOptions(context.Background(), result, advisor.Result{}, "complete", SaveOptions{Snapshot: &snapshot})
	if err != nil {
		t.Fatalf("SaveWithOptions: %v", err)
	}
	snapshots, err := database.ListSnapshots(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].ScanID != scanID {
		t.Fatalf("snapshot scan id = %+v, want %q", snapshots, scanID)
	}
	if snapshot.ScanID != "not-the-real-id" {
		t.Fatal("SaveWithOptions mutated the caller's snapshot")
	}
}

func TestPruneSnapshotsKeepsTheNewest(t *testing.T) {
	database := testStore(t)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	var ids []string
	for day := range 5 {
		ids = append(ids, saveScanWithSnapshot(t, database, base.Add(time.Duration(day)*24*time.Hour), "scope-a", int64(day+1)<<30))
	}

	if err := database.PruneSnapshots(context.Background(), 2); err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}
	snapshots, err := database.ListSnapshots(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("kept %d snapshots, want 2", len(snapshots))
	}
	if snapshots[1].ScanID != ids[len(ids)-1] {
		t.Fatalf("newest snapshot was pruned: %+v", snapshots)
	}
	for _, snapshot := range snapshots {
		if len(snapshot.Measurements) == 0 {
			t.Fatalf("pruning removed series for a retained snapshot: %+v", snapshot)
		}
	}
}

func TestSaveWithoutASnapshotRecordsNoHistory(t *testing.T) {
	database := testStore(t)
	at := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	result := scan.Result{Roots: []string{"/fixtures"}, StartedAt: at, CompletedAt: at}
	if _, err := database.SaveWithOptions(context.Background(), result, advisor.Result{}, "complete", SaveOptions{}); err != nil {
		t.Fatalf("SaveWithOptions: %v", err)
	}
	snapshots, err := database.ListSnapshots(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("snapshots = %+v, want none", snapshots)
	}
}
