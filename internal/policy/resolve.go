package policy

import (
	"maps"
	"path"
	"strings"
	"time"
)

// Effective is a document resolved for one machine: base settings with matching
// overlays applied. It is what the analysis stages actually read, so the rest of
// the engine never has to know overlays exist.
type Effective struct {
	PolicyID string
	Name     string

	Roots      []Root
	Exclusions []string

	PreferredTools map[string]string
	FitMode        string
	FitWeights     map[string]float64

	RiskThreshold string
	ActiveWithin  time.Duration
	InactiveAfter time.Duration

	Suppressions     []Suppression
	ScanHistoryCount int

	// AppliedOverlays records which machine classes contributed, so the UI can
	// explain why this machine sees different weighting than the exported file.
	AppliedOverlays []MachineSelector
}

// Resolve applies every overlay whose selector matches, least specific first.
// The document is not mutated and the result shares no maps or slices with it.
func Resolve(document Document, machine Machine) Effective {
	base := document.clone()
	effective := Effective{
		PolicyID:         base.ID,
		Name:             base.Name,
		Roots:            base.Roots,
		Exclusions:       base.Exclusions,
		PreferredTools:   base.PreferredTools,
		FitMode:          orDefault(base.FitMode, FitModeBalanced),
		FitWeights:       base.FitWeights,
		RiskThreshold:    orDefault(base.RiskThreshold, RiskHigh),
		ActiveWithin:     days(base.ActiveWithinDays, DefaultActiveWithinDays),
		InactiveAfter:    days(base.InactiveAfterDays, DefaultInactiveAfterDays),
		Suppressions:     base.Suppressions,
		ScanHistoryCount: orDefaultInt(base.Retention.ScanHistoryCount, DefaultScanHistoryCount),
	}

	for _, overlay := range base.Overlays {
		if !overlay.Match.Matches(machine) {
			continue
		}
		effective.AppliedOverlays = append(effective.AppliedOverlays, overlay.Match)
		if len(overlay.PreferredTools) > 0 {
			merged := copyStringMap(effective.PreferredTools)
			if merged == nil {
				merged = make(map[string]string, len(overlay.PreferredTools))
			}
			maps.Copy(merged, overlay.PreferredTools)
			effective.PreferredTools = merged
		}
		if overlay.FitMode != "" {
			effective.FitMode = overlay.FitMode
			// A mode and explicit weights are two ways to say the same thing.
			// Choosing a mode in an overlay replaces the inherited weights so the
			// machine does not end up weighted by a mode it never selected.
			effective.FitWeights = nil
		}
		if len(overlay.FitWeights) > 0 {
			merged := copyFloatMap(effective.FitWeights)
			if merged == nil {
				merged = make(map[string]float64, len(overlay.FitWeights))
			}
			maps.Copy(merged, overlay.FitWeights)
			effective.FitWeights = merged
		}
		if overlay.RiskThreshold != "" {
			effective.RiskThreshold = overlay.RiskThreshold
		}
		if overlay.ActiveWithinDays > 0 {
			effective.ActiveWithin = days(overlay.ActiveWithinDays, DefaultActiveWithinDays)
		}
		if overlay.InactiveAfterDays > 0 {
			effective.InactiveAfter = days(overlay.InactiveAfterDays, DefaultInactiveAfterDays)
		}
		if len(overlay.Exclusions) > 0 {
			// Exclusions accumulate: an overlay narrows what a machine looks at,
			// it never re-includes something the base policy excluded.
			effective.Exclusions = mergeExclusions(effective.Exclusions, overlay.Exclusions)
		}
	}
	return effective
}

// Suppresses reports whether the user has already dismissed this advice. A
// suppression matches when every field it names matches; a suppression naming
// only a family hides that whole family.
func (e Effective) Suppresses(recommendationID, family, ecosystem string) bool {
	for _, item := range e.Suppressions {
		if item.RecommendationID != "" && item.RecommendationID != recommendationID {
			continue
		}
		if item.Family != "" && !strings.EqualFold(item.Family, family) {
			continue
		}
		if item.Ecosystem != "" && !strings.EqualFold(item.Ecosystem, ecosystem) {
			continue
		}
		return true
	}
	return false
}

// AllowsRisk reports whether a risk class is at or below the display threshold.
// An unknown class is allowed through: hiding advice because its risk label was
// unrecognized would be the wrong failure direction.
func (e Effective) AllowsRisk(risk string) bool {
	threshold, known := riskOrder[strings.ToLower(e.RiskThreshold)]
	if !known {
		return true
	}
	level, known := riskOrder[strings.ToLower(risk)]
	if !known {
		return true
	}
	return level <= threshold
}

// ExcludesPath reports whether a path is excluded, matching each pattern
// against the full relative path and against any single path segment. Patterns
// are relative by validation, so this compares only the portion of the path
// below the scan root.
func (e Effective) ExcludesPath(relative string) bool {
	if len(e.Exclusions) == 0 || relative == "" {
		return false
	}
	cleaned := path.Clean(strings.TrimPrefix(relative, "/"))
	segments := strings.Split(cleaned, "/")
	for _, pattern := range e.Exclusions {
		if matched, err := path.Match(pattern, cleaned); err == nil && matched {
			return true
		}
		for _, segment := range segments {
			if matched, err := path.Match(pattern, segment); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// ResolveRoots turns portable roots into absolute paths for this machine and
// reports the ones that could not be resolved, so the UI can say which roots a
// shared policy could not reproduce here.
func (e Effective) ResolveRoots(home string) (resolved []string, unresolved []Root) {
	for _, root := range e.Roots {
		absolute, err := root.Resolve(home)
		if err != nil {
			unresolved = append(unresolved, root)
			continue
		}
		resolved = append(resolved, absolute)
	}
	return resolved, unresolved
}

func mergeExclusions(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, group := range [][]string{base, extra} {
		for _, pattern := range group {
			if _, duplicate := seen[pattern]; duplicate {
				continue
			}
			seen[pattern] = struct{}{}
			out = append(out, pattern)
		}
	}
	return out
}

func days(value, fallback int) time.Duration {
	if value <= 0 {
		value = fallback
	}
	return time.Duration(value) * 24 * time.Hour
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func orDefaultInt(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
