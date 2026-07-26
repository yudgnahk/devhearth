// Package policy owns the portable, redacted preference document that Phase 4
// carries between machines (SPECS §7.10).
//
// The central invariant is structural rather than procedural: a Document has no
// field that can hold an absolute path, a repository name, or any inventory
// fact. Scan roots are stored as an alias plus a relative segment, so a document
// is portable and safe to share by construction rather than because an export
// step remembered to strip something. Everything here is untrusted input on the
// way in; see validate.go.
package policy

import (
	"fmt"
	"maps"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SchemaVersion is the portable document version. It is independent of the
// SQLite schema and the JSON-RPC protocol version: a policy file outlives both.
const SchemaVersion = 1

// RootAlias names a portable anchor for a scan root. Only anchors that exist on
// every Mac are allowed, because an anchor the second machine cannot resolve
// would silently scan nothing.
type RootAlias string

const (
	// AliasHome is the user's home directory on the machine applying the policy.
	AliasHome RootAlias = "home"
)

// Root is a scan root expressed portably. Relative is a cleaned, slash-relative
// segment beneath Alias; an empty Relative means the alias directory itself.
type Root struct {
	Alias    RootAlias `json:"alias"`
	Relative string    `json:"relative,omitempty"`
}

// Resolve returns the absolute path for this machine. It never creates or
// inspects the path: resolution is string work, and the caller decides whether
// the result is scannable.
func (r Root) Resolve(home string) (string, error) {
	if r.Alias != AliasHome {
		return "", fmt.Errorf("unknown root alias %q", r.Alias)
	}
	if home == "" {
		return "", fmt.Errorf("home directory is unknown on this machine")
	}
	if r.Relative == "" {
		return filepath.Clean(home), nil
	}
	return filepath.Join(home, filepath.FromSlash(r.Relative)), nil
}

// Display renders a root for the UI without touching the local filesystem.
func (r Root) Display() string {
	if r.Relative == "" {
		return "~"
	}
	return "~/" + r.Relative
}

// NewRoot converts an absolute path into a portable root. It reports false when
// the path cannot be expressed portably (outside the home directory), because
// exporting such a path would leak the personal layout the policy format exists
// to keep private.
func NewRoot(path, home string) (Root, bool) {
	if path == "" || home == "" {
		return Root{}, false
	}
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return Root{Alias: AliasHome}, true
	}
	relative, err := filepath.Rel(cleanHome, cleanPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Root{}, false
	}
	return Root{Alias: AliasHome, Relative: filepath.ToSlash(relative)}, true
}

// Fit weighting modes. A mode is a named tradeoff, never a claim that one tool
// is best; the ranking still includes stay-put under every mode.
const (
	FitModeBalanced          = "balanced"
	FitModePreferDiskSavings = "prefer_disk_savings"
	FitModePreferStability   = "prefer_workflow_stability"
)

// FitModes lists the selectable modes in display order.
func FitModes() []string {
	return []string{FitModeBalanced, FitModePreferDiskSavings, FitModePreferStability}
}

// Risk classes a policy may use as a display threshold (SPECS §8).
const (
	RiskInformational = "informational"
	RiskLow           = "low"
	RiskMedium        = "medium"
	RiskHigh          = "high"
)

// riskOrder ranks risk classes so a threshold can be compared. "prohibited" is
// deliberately absent: it is never a threshold a user may opt into.
var riskOrder = map[string]int{
	RiskInformational: 0,
	RiskLow:           1,
	RiskMedium:        2,
	RiskHigh:          3,
}

// Suppression hides advice the user has already decided about. Every field is
// path-free and stable across machines, which is what makes suppressions
// exportable at all.
type Suppression struct {
	// RecommendationID suppresses one specific recommendation.
	RecommendationID string `json:"recommendationId,omitempty"`
	// Family suppresses a whole rule family, optionally narrowed by ecosystem.
	Family    string `json:"family,omitempty"`
	Ecosystem string `json:"ecosystem,omitempty"`
	// Reason is the user's own words. It is shown back to them and exported, so
	// validation caps its length and rejects control characters.
	Reason string `json:"reason,omitempty"`
	// CreatedAt is day-resolution on purpose: a precise timestamp is a usage
	// trace, and a shared policy should not carry one.
	CreatedAt string `json:"createdAt,omitempty"`
}

// MachineSelector matches an overlay against the machine applying the policy.
// An empty field matches anything; an entirely empty selector is the default
// overlay and is rejected in favour of the document's own base settings.
type MachineSelector struct {
	Architecture string `json:"architecture,omitempty"`
	DiskClass    string `json:"diskClass,omitempty"`
	Role         string `json:"role,omitempty"`
}

// Machine describes the machine applying a policy. It is supplied by the host,
// never inferred from the inventory.
type Machine struct {
	Architecture string `json:"architecture,omitempty"`
	DiskClass    string `json:"diskClass,omitempty"`
	Role         string `json:"role,omitempty"`
}

// Matches reports whether every named field of the selector matches.
func (s MachineSelector) Matches(machine Machine) bool {
	if s.Architecture != "" && !strings.EqualFold(s.Architecture, machine.Architecture) {
		return false
	}
	if s.DiskClass != "" && !strings.EqualFold(s.DiskClass, machine.DiskClass) {
		return false
	}
	if s.Role != "" && !strings.EqualFold(s.Role, machine.Role) {
		return false
	}
	return true
}

// specificity ranks overlays so a more precise selector wins over a broader one
// regardless of document order.
func (s MachineSelector) specificity() int {
	count := 0
	for _, value := range []string{s.Architecture, s.DiskClass, s.Role} {
		if value != "" {
			count++
		}
	}
	return count
}

// Overlay is a per-machine-class override (SPECS §7.10). It may change how
// advice is weighted and filtered; it may never change what is safe.
type Overlay struct {
	Match             MachineSelector    `json:"match"`
	PreferredTools    map[string]string  `json:"preferredTools,omitempty"`
	FitMode           string             `json:"fitMode,omitempty"`
	FitWeights        map[string]float64 `json:"fitWeights,omitempty"`
	RiskThreshold     string             `json:"riskThreshold,omitempty"`
	ActiveWithinDays  int                `json:"activeWithinDays,omitempty"`
	InactiveAfterDays int                `json:"inactiveAfterDays,omitempty"`
	Exclusions        []string           `json:"exclusions,omitempty"`
}

// Retention bounds how much local history the engine keeps. It is a preference,
// not a safety control: nothing here deletes anything outside the inventory
// database.
type Retention struct {
	ScanHistoryCount int `json:"scanHistoryCount,omitempty"`
}

// Document is the portable policy. Field-by-field it is the SPECS §7.10 list,
// and nothing beyond that list may be added without a privacy review: absolute
// paths, repository names, asset inventory, credentials, scan history, and
// inferred portfolio fingerprints stay out.
type Document struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`

	Roots      []Root   `json:"roots,omitempty"`
	Exclusions []string `json:"exclusions,omitempty"`

	// PreferredTools maps ecosystem to the tool the user chose. These are
	// explicit choices, never the detected inventory.
	PreferredTools map[string]string  `json:"preferredTools,omitempty"`
	FitMode        string             `json:"fitMode,omitempty"`
	FitWeights     map[string]float64 `json:"fitWeights,omitempty"`

	RiskThreshold     string    `json:"riskThreshold,omitempty"`
	ActiveWithinDays  int       `json:"activeWithinDays,omitempty"`
	InactiveAfterDays int       `json:"inactiveAfterDays,omitempty"`
	Retention         Retention `json:"retention,omitzero"`

	Suppressions []Suppression `json:"suppressions,omitempty"`
	Overlays     []Overlay     `json:"machineOverlays,omitempty"`
}

// DefaultDocument is the policy a machine starts from: no roots, no
// suppressions, balanced weighting, and every risk class visible.
func DefaultDocument(id string) Document {
	return Document{
		SchemaVersion:     SchemaVersion,
		ID:                id,
		FitMode:           FitModeBalanced,
		RiskThreshold:     RiskHigh,
		ActiveWithinDays:  DefaultActiveWithinDays,
		InactiveAfterDays: DefaultInactiveAfterDays,
		Retention:         Retention{ScanHistoryCount: DefaultScanHistoryCount},
	}
}

// Defaults for the tunable windows, expressed in days because that is the unit
// a user reasons about and the unit the document stores.
const (
	DefaultActiveWithinDays  = 90
	DefaultInactiveAfterDays = 180
	DefaultScanHistoryCount  = 30
)

// WithSuppression returns a copy of the document with the suppression added.
// An equivalent suppression already present is refreshed rather than duplicated.
func (d Document) WithSuppression(suppression Suppression) Document {
	next := d.clone()
	for i, existing := range next.Suppressions {
		if existing.RecommendationID == suppression.RecommendationID &&
			existing.Family == suppression.Family &&
			existing.Ecosystem == suppression.Ecosystem {
			next.Suppressions[i] = suppression
			return next
		}
	}
	next.Suppressions = append(next.Suppressions, suppression)
	sortSuppressions(next.Suppressions)
	return next
}

// WithoutSuppression returns a copy with matching suppressions removed.
func (d Document) WithoutSuppression(suppression Suppression) Document {
	next := d.clone()
	kept := make([]Suppression, 0, len(next.Suppressions))
	for _, existing := range next.Suppressions {
		if existing.RecommendationID == suppression.RecommendationID &&
			existing.Family == suppression.Family &&
			existing.Ecosystem == suppression.Ecosystem {
			continue
		}
		kept = append(kept, existing)
	}
	next.Suppressions = kept
	return next
}

// clone deep-copies the mutable members so callers never share slices or maps
// with a stored document.
func (d Document) clone() Document {
	next := d
	next.Roots = append([]Root(nil), d.Roots...)
	next.Exclusions = append([]string(nil), d.Exclusions...)
	next.Suppressions = append([]Suppression(nil), d.Suppressions...)
	next.PreferredTools = copyStringMap(d.PreferredTools)
	next.FitWeights = copyFloatMap(d.FitWeights)
	next.Overlays = make([]Overlay, 0, len(d.Overlays))
	for _, overlay := range d.Overlays {
		copied := overlay
		copied.PreferredTools = copyStringMap(overlay.PreferredTools)
		copied.FitWeights = copyFloatMap(overlay.FitWeights)
		copied.Exclusions = append([]string(nil), overlay.Exclusions...)
		next.Overlays = append(next.Overlays, copied)
	}
	if len(next.Overlays) == 0 {
		next.Overlays = nil
	}
	return next
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func copyFloatMap(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	maps.Copy(out, in)
	return out
}

// sortSuppressions keeps a stable on-disk order so two machines that made the
// same choices produce byte-identical exports.
func sortSuppressions(items []Suppression) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Family != items[j].Family {
			return items[i].Family < items[j].Family
		}
		if items[i].Ecosystem != items[j].Ecosystem {
			return items[i].Ecosystem < items[j].Ecosystem
		}
		return items[i].RecommendationID < items[j].RecommendationID
	})
}

// Today formats a day-resolution stamp for new suppressions.
func Today(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}
