package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

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
	server := protocol.NewServer(protocol.ServerOptions{MockScan: *mockScan, Logger: logger, OnComplete: func(ctx context.Context, result scan.Result, status string) error {
		if inventory == nil {
			return nil
		}
		_, err := inventory.Save(ctx, result, status)
		return err
	}})
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
