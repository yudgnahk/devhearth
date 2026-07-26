// Package trend turns a history of scan roll-ups into growth series and
// regression signals (Phase 4 monitoring).
//
// A snapshot is deliberately small and path-free: it records how many bytes an
// ecosystem or storage class holds, never which directories held them. That
// keeps the history cheap to retain and keeps a trend view from becoming a
// second copy of the inventory.
package trend

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/recommend"
)

// Series key prefixes. The prefix decides how a measurement is grouped and
// labelled, so the UI does not have to parse free-form keys.
const (
	SeriesEcosystem = "ecosystem"
	SeriesClass     = "class"
	SeriesTotal     = "total"
)

// TotalKey is the whole-scan allocated-bytes series.
const TotalKey = SeriesTotal + ":allocated"

// Measurement is one named quantity inside a snapshot.
type Measurement struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	AllocatedBytes int64  `json:"allocatedBytes"`
	LogicalBytes   int64  `json:"logicalBytes"`
	ItemCount      int    `json:"itemCount"`
	// Uncertain marks a measurement whose bytes are a lower bound (hard links,
	// shared stores). A trend built from lower bounds is still a trend, but it
	// must not be presented as an exact figure.
	Uncertain bool `json:"uncertain,omitempty"`
}

// Totals are the scan-wide figures a snapshot records.
type Totals struct {
	EntriesVisited int64
	LogicalBytes   int64
	AllocatedBytes int64
}

// Snapshot is one scan reduced to comparable numbers.
type Snapshot struct {
	ScanID     string    `json:"scanId"`
	CapturedAt time.Time `json:"capturedAt"`

	// ScopeKey identifies which roots produced this snapshot. Comparing a scan
	// of one folder against a scan of the whole home directory would report
	// enormous fake growth, so analysis only compares snapshots that share a
	// scope. It is derived from local paths and therefore stays local: it is
	// never part of a policy export or a report.
	ScopeKey  string `json:"scopeKey"`
	RootCount int    `json:"rootCount"`

	EntriesVisited int64 `json:"entriesVisited"`
	LogicalBytes   int64 `json:"logicalBytes"`
	AllocatedBytes int64 `json:"allocatedBytes"`

	RecommendationCount int   `json:"recommendationCount"`
	SavingsLowBytes     int64 `json:"savingsLowBytes"`
	SavingsHighBytes    int64 `json:"savingsHighBytes"`

	Measurements []Measurement `json:"measurements,omitempty"`
}

// ScopeKeyFor fingerprints a set of scan roots so snapshots from the same
// selection can be compared. The digest is one-way and never leaves the local
// database; it exists to answer "is this the same scope?" without storing the
// paths a second time.
func ScopeKeyFor(roots []string) string {
	if len(roots) == 0 {
		return "empty"
	}
	normalized := append([]string(nil), roots...)
	sort.Strings(normalized)
	digest := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return hex.EncodeToString(digest[:8])
}

// Capture reduces one analysed scan to a snapshot. It reads only attributed
// sizes: an unattributed asset contributes its item count but no bytes, so a
// series never inherits a number nobody measured.
//
// Callers must pass the full rule output rather than the policy-filtered inbox.
// A snapshot describes the machine; recording only visible advice would make
// hiding a recommendation show up on the trend line as storage recovered.
func Capture(
	scanID string,
	capturedAt time.Time,
	roots []string,
	totals Totals,
	graph assets.Graph,
	recommendations []recommend.Recommendation,
) Snapshot {
	snapshot := Snapshot{
		ScanID:         scanID,
		CapturedAt:     capturedAt.UTC(),
		ScopeKey:       ScopeKeyFor(roots),
		RootCount:      len(roots),
		EntriesVisited: totals.EntriesVisited,
		LogicalBytes:   totals.LogicalBytes,
		AllocatedBytes: totals.AllocatedBytes,
	}
	for _, recommendation := range recommendations {
		snapshot.RecommendationCount++
		snapshot.SavingsLowBytes += recommendation.Savings.LowBytes
		snapshot.SavingsHighBytes += recommendation.Savings.HighBytes
	}

	accumulator := map[string]*Measurement{}
	add := func(key, label string, asset assets.Asset) {
		current, found := accumulator[key]
		if !found {
			current = &Measurement{Key: key, Label: label}
			accumulator[key] = current
		}
		current.ItemCount++
		if !asset.Size.Attributed {
			return
		}
		// Exclusive bytes are used so a project and its own node_modules are not
		// both counted into the same series.
		current.AllocatedBytes += asset.Size.ExclusiveAllocatedBytes
		current.LogicalBytes += asset.Size.LogicalBytes
		if asset.Size.Uncertain || asset.Size.Shared {
			current.Uncertain = true
		}
	}

	for _, asset := range graph.Assets {
		if asset.Ecosystem != "" {
			add(SeriesEcosystem+":"+asset.Ecosystem, asset.Ecosystem, asset)
		}
		if asset.Class != "" {
			add(SeriesClass+":"+string(asset.Class), classLabel(asset.Class), asset)
		}
	}

	snapshot.Measurements = make([]Measurement, 0, len(accumulator)+1)
	snapshot.Measurements = append(snapshot.Measurements, Measurement{
		Key:            TotalKey,
		Label:          "All scanned storage",
		AllocatedBytes: totals.AllocatedBytes,
		LogicalBytes:   totals.LogicalBytes,
		ItemCount:      len(graph.Assets),
	})
	for _, measurement := range accumulator {
		snapshot.Measurements = append(snapshot.Measurements, *measurement)
	}
	sort.Slice(snapshot.Measurements, func(i, j int) bool {
		return snapshot.Measurements[i].Key < snapshot.Measurements[j].Key
	})
	return snapshot
}

func classLabel(class assets.Class) string {
	return strings.ReplaceAll(string(class), "_", " ")
}

// Describe renders a snapshot for logs and tests without leaking paths.
func (s Snapshot) Describe() string {
	return fmt.Sprintf("scan %s at %s: %d entries, %d allocated bytes, %d measurements",
		s.ScanID, s.CapturedAt.Format(time.RFC3339), s.EntriesVisited, s.AllocatedBytes, len(s.Measurements))
}
