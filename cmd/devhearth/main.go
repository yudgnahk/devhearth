package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/builtin"
	"github.com/yudgnahk/devhearth/internal/protocol"
	"github.com/yudgnahk/devhearth/internal/scan"
	"github.com/yudgnahk/devhearth/internal/store"
)

func main() {
	mockScan := flag.Bool("mock-scan", false, "serve deterministic mocked scan events")
	databasePath := flag.String("database", defaultDatabasePath(), "SQLite inventory database path")
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
		Detect: func(ctx context.Context, result scan.Result, progress func(detect.Progress)) (assets.Graph, error) {
			if registry == nil {
				return assets.Graph{}, nil
			}
			return detect.Run(ctx, registry, result, detect.RunOptions{Progress: progress})
		},
		Advise: func(ctx context.Context, _ scan.Result, graph assets.Graph, dirIndex map[string][]scan.DirectoryNode) (advisor.Result, error) {
			if err := ctx.Err(); err != nil {
				return advisor.Result{}, err
			}
			// Analysis is pure and in-memory; policy-driven weights arrive with
			// the Phase 4 portable policy format.
			return advisor.Analyze(graph, dirIndex, advisor.Options{}), nil
		},
		OnComplete: func(ctx context.Context, result scan.Result, advice advisor.Result, status string, dirIndex map[string][]scan.DirectoryNode, progress func(written, total int64)) error {
			if inventory == nil {
				return nil
			}
			_, err := inventory.SaveWithOptions(ctx, result, advice, status, store.SaveOptions{
				DirectoryIndex: dirIndex,
				Progress:       progress,
			})
			return err
		},
	})
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultDatabasePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "devhearth-inventory.sqlite")
	}
	return filepath.Join(dir, "DevHearth", "inventory.sqlite")
}
