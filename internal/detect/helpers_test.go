package detect_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yudgnahk/devhearth/internal/detect"
)

func TestPathHasComponent(t *testing.T) {
	if !detect.PathHasComponent("/models/LoRa/weights", "lora") {
		t.Fatal("expected case-insensitive component match")
	}
	if detect.PathHasComponent("/models/floral/weights", "lora") {
		t.Fatal("substring inside component must not match")
	}
	if detect.PathHasComponent("/models/loras/weights", "lora") {
		t.Fatal("plural component must not match exact name")
	}
}

func TestReadFileLimitedRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := detect.ReadFileLimited(link, 1024)
	if err == nil {
		t.Fatal("expected symlink open to fail")
	}
	if err != detect.ErrSymlinkRefused {
		// Platforms may surface ELOOP wrapped differently; still require failure.
		t.Logf("symlink refusal error: %v", err)
	}
	data, err := detect.ReadFileLimited(target, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret" {
		t.Fatalf("data = %q", data)
	}
}
