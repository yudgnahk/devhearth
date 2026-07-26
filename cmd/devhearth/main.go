package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/builtin"
	"github.com/yudgnahk/devhearth/internal/policy"
	"github.com/yudgnahk/devhearth/internal/protocol"
	"github.com/yudgnahk/devhearth/internal/scan"
	"github.com/yudgnahk/devhearth/internal/store"
	"github.com/yudgnahk/devhearth/internal/trend"
)

func main() {
	mockScan := flag.Bool("mock-scan", false, "serve deterministic mocked scan events")
	databasePath := flag.String("database", defaultDatabasePath(), "SQLite inventory database path")
	machineRole := flag.String("machine-role", "", "machine role for policy overlays (for example laptop or workstation)")
	diskClass := flag.String("machine-disk-class", "", "machine disk class for policy overlays (for example small or large)")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	var inventory *store.Store
	if !*mockScan {
		var err error
		inventory, err = store.Open(*databasePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open inventory database:", err)
			os.Exit(1)
		}
		defer inventory.Close()
	}

	var registry *detect.Registry
	if !*mockScan {
		var err error
		registry, err = builtin.NewRegistry()
		if err != nil {
			fmt.Fprintln(os.Stderr, "register detectors:", err)
			os.Exit(1)
		}
	}

	server := protocol.NewServer(protocol.ServerOptions{
		MockScan: *mockScan,
		Logger:   logger,
		// Machine class comes from the host, never from the scanned inventory:
		// an overlay must not depend on what a user happens to store.
		Machine: policy.Machine{
			Architecture: runtime.GOARCH,
			DiskClass:    *diskClass,
			Role:         *machineRole,
		},
		HomeDir:    homeDirectory(),
		Monitoring: monitoring(inventory),
		Detect: func(ctx context.Context, result scan.Result, progress func(detect.Progress)) (assets.Graph, error) {
			if registry == nil {
				return assets.Graph{}, nil
			}
			return detect.Run(ctx, registry, result, detect.RunOptions{Progress: progress})
		},
		Advise: func(ctx context.Context, _ scan.Result, graph assets.Graph, dirIndex map[string][]scan.DirectoryNode, active policy.Effective) (advisor.Result, error) {
			if err := ctx.Err(); err != nil {
				return advisor.Result{}, err
			}
			// Analysis is pure and in-memory. The policy supplies weighting and
			// filtering; it can hide advice but never alters a savings figure, a
			// confidence, or a risk class.
			return advisor.Analyze(graph, dirIndex, advisor.Options{
				Preferred:       active.PreferredTools,
				FitMode:         active.FitMode,
				WeightOverrides: active.FitWeights,
				ActiveWithin:    active.ActiveWithin,
				InactiveAfter:   active.InactiveAfter,
				Filter:          active,
			}), nil
		},
		OnComplete: func(ctx context.Context, result scan.Result, advice advisor.Result, status string, dirIndex map[string][]scan.DirectoryNode, progress func(written, total int64)) error {
			if inventory == nil {
				return nil
			}
			options := store.SaveOptions{DirectoryIndex: dirIndex, Progress: progress}
			// Only a complete scan becomes trend history. A cancelled or failed
			// scan measured part of the tree, and comparing it against a full one
			// would report storage that never disappeared.
			if status == "complete" {
				// History records the full rule output, not the filtered inbox.
				// Snapshotting only visible advice would make hiding a
				// recommendation look like recovering storage on the trend line.
				snapshot := trend.Capture("", time.Now().UTC(), result.Roots, trend.Totals{
					EntriesVisited: result.EntriesVisited,
					LogicalBytes:   result.LogicalBytes,
					AllocatedBytes: result.AllocatedBytes,
				}, advice.Graph, advice.All)
				options.Snapshot = &snapshot
			}
			if _, err := inventory.SaveWithOptions(ctx, result, advice, status, options); err != nil {
				return err
			}
			if err := pruneHistory(ctx, inventory); err != nil {
				// The scan is already durable; stale history is not worth
				// reporting the scan as failed.
				logger.Warn("prune trend history", "error", err)
			}
			return nil
		},
	})
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// monitoring adapts the SQLite store to the protocol's Phase 4 interface. A nil
// store (mock mode) yields a nil interface so the policy, feedback, and trend
// methods report that they are unavailable instead of appearing to work.
func monitoring(inventory *store.Store) protocol.Monitoring {
	if inventory == nil {
		return nil
	}
	return monitoringStore{store: inventory}
}

type monitoringStore struct{ store *store.Store }

func (m monitoringStore) EnsureActivePolicy(ctx context.Context, defaultID string) (policy.Document, error) {
	return m.store.EnsureActivePolicy(ctx, defaultID)
}

func (m monitoringStore) PolicyByID(ctx context.Context, id string) (policy.Document, error) {
	return m.store.PolicyByID(ctx, id)
}

func (m monitoringStore) SavePolicy(ctx context.Context, document policy.Document, activate bool) error {
	return m.store.SavePolicy(ctx, document, activate)
}

func (m monitoringStore) RecordFeedback(ctx context.Context, feedback store.Feedback) (store.Feedback, error) {
	return m.store.RecordFeedback(ctx, feedback)
}

func (m monitoringStore) Snapshots(ctx context.Context, limit int) ([]trend.Snapshot, error) {
	return m.store.ListSnapshots(ctx, limit)
}

// pruneHistory trims trend history to the policy's retention preference.
func pruneHistory(ctx context.Context, inventory *store.Store) error {
	document, err := inventory.ActivePolicy(ctx)
	if err != nil {
		// No policy stored yet means the default retention applies.
		return inventory.PruneSnapshots(ctx, policy.DefaultScanHistoryCount)
	}
	return inventory.PruneSnapshots(ctx, document.Retention.ScanHistoryCount)
}

func homeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func defaultDatabasePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "devhearth-inventory.sqlite")
	}
	return filepath.Join(dir, "DevHearth", "inventory.sqlite")
}
