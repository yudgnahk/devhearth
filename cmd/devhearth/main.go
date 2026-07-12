package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/yudgnahk/devhearth/internal/protocol"
)

func main() {
	mockScan := flag.Bool("mock-scan", false, "serve deterministic mocked scan events")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	server := protocol.NewServer(protocol.ServerOptions{MockScan: *mockScan, Logger: logger})
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
