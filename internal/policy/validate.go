package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A policy document arrives from a file the user may have edited, copied from a
// colleague, or received from somewhere less friendly. Every bound below exists
// so a hostile document costs a clear error rather than memory, a wedged UI, or
// a scan of somewhere it was never meant to reach.
const (
	maxDocumentBytes  = 256 * 1024
	maxRoots          = 32
	maxExclusions     = 256
	maxPreferredTools = 32
	maxWeights        = 32
	maxSuppressions   = 512
	maxOverlays       = 16
	maxNameLength     = 120
	maxTokenLength    = 64
	maxPatternLength  = 256
	maxReasonLength   = 512
	maxRelativeLength = 512
	maxRetentionScans = 1000
	maxWindowDays     = 3650
)

// ErrTooLarge reports a document that exceeded the size bound before parsing.
var ErrTooLarge = errors.New("policy document is too large")

// Parse decodes and validates an untrusted policy document. It rejects rather
// than repairs anything that carries meaning (unknown schema, absolute paths,
// escaping patterns) and normalizes only what is unambiguous (case, ordering,
// duplicate entries).
func Parse(raw []byte) (Document, error) {
	if len(raw) == 0 {
		return Document{}, errors.New("policy document is empty")
	}
	if len(raw) > maxDocumentBytes {
		return Document{}, fmt.Errorf("%w: %d bytes exceeds the %d byte limit", ErrTooLarge, len(raw), maxDocumentBytes)
	}
	if !utf8.Valid(raw) {
		return Document{}, errors.New("policy document is not valid UTF-8")
	}
	var document Document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// Unknown fields are an error: silently ignoring them would let a newer or
	// tampered document appear to apply settings this build never reads.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("parse policy document: %w", err)
	}
	if decoder.More() {
		return Document{}, errors.New("policy document has trailing content")
	}
	return Validate(document)
}

// Marshal renders a document as stable, indented JSON suitable for export.
func Marshal(document Document) ([]byte, error) {
	validated, err := Validate(document)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(validated, "", "  ")
}

// Validate normalizes and bounds-checks a document from any source, including
// one this process just built: the protocol layer accepts caller-supplied
// fields, so in-process construction is no more trusted than a file.
func Validate(document Document) (Document, error) {
	if document.SchemaVersion == 0 {
		document.SchemaVersion = SchemaVersion
	}
	if document.SchemaVersion != SchemaVersion {
		return Document{}, fmt.Errorf("unsupported policy schema version %d (this build understands %d)", document.SchemaVersion, SchemaVersion)
	}
	if err := validateToken("policy id", document.ID, false); err != nil {
		return Document{}, err
	}
	if document.ID == "" {
		return Document{}, errors.New("policy id is required")
	}
	name, err := validateText("policy name", document.Name, maxNameLength)
	if err != nil {
		return Document{}, err
	}
	document.Name = name

	if document.Roots, err = validateRoots(document.Roots); err != nil {
		return Document{}, err
	}
	if document.Exclusions, err = validateExclusions(document.Exclusions); err != nil {
		return Document{}, err
	}
	if document.PreferredTools, err = validatePreferredTools(document.PreferredTools); err != nil {
		return Document{}, err
	}
	if document.FitMode, err = validateFitMode(document.FitMode); err != nil {
		return Document{}, err
	}
	if document.FitWeights, err = validateWeights(document.FitWeights); err != nil {
		return Document{}, err
	}
	if document.RiskThreshold, err = validateRiskThreshold(document.RiskThreshold); err != nil {
		return Document{}, err
	}
	if document.ActiveWithinDays, err = validateDays("activeWithinDays", document.ActiveWithinDays, DefaultActiveWithinDays); err != nil {
		return Document{}, err
	}
	if document.InactiveAfterDays, err = validateDays("inactiveAfterDays", document.InactiveAfterDays, DefaultInactiveAfterDays); err != nil {
		return Document{}, err
	}
	if document.Retention.ScanHistoryCount == 0 {
		document.Retention.ScanHistoryCount = DefaultScanHistoryCount
	}
	if document.Retention.ScanHistoryCount < 1 || document.Retention.ScanHistoryCount > maxRetentionScans {
		return Document{}, fmt.Errorf("retention.scanHistoryCount must be between 1 and %d", maxRetentionScans)
	}
	if document.Suppressions, err = validateSuppressions(document.Suppressions); err != nil {
		return Document{}, err
	}
	if document.Overlays, err = validateOverlays(document.Overlays); err != nil {
		return Document{}, err
	}
	return document, nil
}

func validateRoots(roots []Root) ([]Root, error) {
	if len(roots) > maxRoots {
		return nil, fmt.Errorf("policy declares %d roots, more than the %d allowed", len(roots), maxRoots)
	}
	seen := make(map[Root]struct{}, len(roots))
	out := make([]Root, 0, len(roots))
	for _, root := range roots {
		if root.Alias != AliasHome {
			return nil, fmt.Errorf("unknown root alias %q; only %q is portable", root.Alias, AliasHome)
		}
		relative, err := validateRelative(root.Relative)
		if err != nil {
			return nil, err
		}
		normalized := Root{Alias: root.Alias, Relative: relative}
		if _, duplicate := seen[normalized]; duplicate {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Relative < out[j].Relative })
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// validateRelative rejects anything that would resolve outside the alias. A
// policy that can say "../.." is a policy that can point a scan at another
// user's home directory on the machine that imports it.
func validateRelative(relative string) (string, error) {
	if relative == "" {
		return "", nil
	}
	if len(relative) > maxRelativeLength {
		return "", fmt.Errorf("root path segment is longer than %d characters", maxRelativeLength)
	}
	if err := rejectControlCharacters("root path segment", relative); err != nil {
		return "", err
	}
	if strings.ContainsRune(relative, '\\') {
		return "", fmt.Errorf("root path segment %q must use / separators", relative)
	}
	if path.IsAbs(relative) || strings.HasPrefix(relative, "~") {
		return "", fmt.Errorf("root path segment %q must be relative to the alias", relative)
	}
	cleaned := path.Clean(relative)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("root path segment %q escapes its alias", relative)
	}
	return cleaned, nil
}

func validateExclusions(patterns []string) ([]string, error) {
	if len(patterns) > maxExclusions {
		return nil, fmt.Errorf("policy declares %d exclusions, more than the %d allowed", len(patterns), maxExclusions)
	}
	seen := make(map[string]struct{}, len(patterns))
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > maxPatternLength {
			return nil, fmt.Errorf("exclusion pattern is longer than %d characters", maxPatternLength)
		}
		if err := rejectControlCharacters("exclusion pattern", trimmed); err != nil {
			return nil, err
		}
		// An absolute pattern is both a privacy leak on export and meaningless
		// on the importing machine.
		if path.IsAbs(trimmed) || strings.HasPrefix(trimmed, "~") {
			return nil, fmt.Errorf("exclusion pattern %q must be relative, not an absolute path", trimmed)
		}
		// A pattern that walks upward describes something outside the scanned
		// tree, so it can only ever be a mistake or an attempt to reach further
		// than the policy is allowed to reach.
		if hasParentSegment(trimmed) {
			return nil, fmt.Errorf("exclusion pattern %q escapes the scanned tree", trimmed)
		}
		if _, err := path.Match(trimmed, "probe"); err != nil {
			return nil, fmt.Errorf("exclusion pattern %q is not a valid glob: %w", trimmed, err)
		}
		if _, duplicate := seen[trimmed]; duplicate {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func validatePreferredTools(preferred map[string]string) (map[string]string, error) {
	if len(preferred) == 0 {
		return nil, nil
	}
	if len(preferred) > maxPreferredTools {
		return nil, fmt.Errorf("policy declares %d preferred tools, more than the %d allowed", len(preferred), maxPreferredTools)
	}
	out := make(map[string]string, len(preferred))
	for ecosystem, tool := range preferred {
		key := strings.ToLower(strings.TrimSpace(ecosystem))
		value := strings.ToLower(strings.TrimSpace(tool))
		if err := validateToken("ecosystem", key, false); err != nil {
			return nil, err
		}
		if err := validateToken("preferred tool", value, false); err != nil {
			return nil, err
		}
		if key == "" || value == "" {
			return nil, errors.New("preferred tools need both an ecosystem and a tool")
		}
		out[key] = value
	}
	return out, nil
}

func validateFitMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		return FitModeBalanced, nil
	}
	if slices.Contains(FitModes(), normalized) {
		return normalized, nil
	}
	return "", fmt.Errorf("unknown fit mode %q; expected one of %s", mode, strings.Join(FitModes(), ", "))
}

// validateWeights bounds each weight to 0..1. Weights are not normalized here:
// the portfolio package owns what a weight means and renormalizes when it
// applies them, so this layer only refuses values that cannot be a weight.
func validateWeights(weights map[string]float64) (map[string]float64, error) {
	if len(weights) == 0 {
		return nil, nil
	}
	if len(weights) > maxWeights {
		return nil, fmt.Errorf("policy declares %d fit weights, more than the %d allowed", len(weights), maxWeights)
	}
	out := make(map[string]float64, len(weights))
	total := 0.0
	for factor, weight := range weights {
		key := strings.ToLower(strings.TrimSpace(factor))
		if err := validateToken("fit weight factor", key, false); err != nil {
			return nil, err
		}
		if key == "" {
			return nil, errors.New("fit weights need a factor name")
		}
		if math.IsNaN(weight) || math.IsInf(weight, 0) {
			return nil, fmt.Errorf("fit weight for %q is not a number", key)
		}
		if weight < 0 || weight > 1 {
			return nil, fmt.Errorf("fit weight for %q must be between 0 and 1", key)
		}
		total += weight
		out[key] = weight
	}
	if total <= 0 {
		return nil, errors.New("fit weights cannot all be zero")
	}
	return out, nil
}

func validateRiskThreshold(threshold string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(threshold))
	if normalized == "" {
		return RiskHigh, nil
	}
	if _, known := riskOrder[normalized]; !known {
		return "", fmt.Errorf("unknown risk threshold %q", threshold)
	}
	return normalized, nil
}

func validateDays(field string, value, fallback int) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 1 || value > maxWindowDays {
		return 0, fmt.Errorf("%s must be between 1 and %d days", field, maxWindowDays)
	}
	return value, nil
}

func validateSuppressions(items []Suppression) ([]Suppression, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) > maxSuppressions {
		return nil, fmt.Errorf("policy declares %d suppressions, more than the %d allowed", len(items), maxSuppressions)
	}
	out := make([]Suppression, 0, len(items))
	for _, item := range items {
		item.RecommendationID = strings.TrimSpace(item.RecommendationID)
		item.Family = strings.ToLower(strings.TrimSpace(item.Family))
		item.Ecosystem = strings.ToLower(strings.TrimSpace(item.Ecosystem))
		if err := validateToken("suppression recommendation id", item.RecommendationID, true); err != nil {
			return nil, err
		}
		if err := validateToken("suppression family", item.Family, false); err != nil {
			return nil, err
		}
		if err := validateToken("suppression ecosystem", item.Ecosystem, false); err != nil {
			return nil, err
		}
		// A suppression naming nothing would hide every recommendation, which is
		// indistinguishable from the advisor being broken.
		if item.RecommendationID == "" && item.Family == "" {
			return nil, errors.New("a suppression must name a recommendation id or a family")
		}
		reason, err := validateText("suppression reason", item.Reason, maxReasonLength)
		if err != nil {
			return nil, err
		}
		item.Reason = reason
		if item.CreatedAt != "" {
			if err := validateDay(item.CreatedAt); err != nil {
				return nil, err
			}
		}
		out = append(out, item)
	}
	sortSuppressions(out)
	return out, nil
}

func validateOverlays(overlays []Overlay) ([]Overlay, error) {
	if len(overlays) == 0 {
		return nil, nil
	}
	if len(overlays) > maxOverlays {
		return nil, fmt.Errorf("policy declares %d machine overlays, more than the %d allowed", len(overlays), maxOverlays)
	}
	out := make([]Overlay, 0, len(overlays))
	for _, overlay := range overlays {
		overlay.Match.Architecture = strings.ToLower(strings.TrimSpace(overlay.Match.Architecture))
		overlay.Match.DiskClass = strings.ToLower(strings.TrimSpace(overlay.Match.DiskClass))
		overlay.Match.Role = strings.ToLower(strings.TrimSpace(overlay.Match.Role))
		for label, value := range map[string]string{
			"overlay architecture": overlay.Match.Architecture,
			"overlay disk class":   overlay.Match.DiskClass,
			"overlay role":         overlay.Match.Role,
		} {
			if err := validateToken(label, value, false); err != nil {
				return nil, err
			}
		}
		if overlay.Match.specificity() == 0 {
			return nil, errors.New("a machine overlay must match at least one machine attribute; use the document's own settings for defaults")
		}
		var err error
		if overlay.PreferredTools, err = validatePreferredTools(overlay.PreferredTools); err != nil {
			return nil, err
		}
		if overlay.FitMode != "" {
			if overlay.FitMode, err = validateFitMode(overlay.FitMode); err != nil {
				return nil, err
			}
		}
		if overlay.FitWeights, err = validateWeights(overlay.FitWeights); err != nil {
			return nil, err
		}
		if overlay.RiskThreshold != "" {
			if overlay.RiskThreshold, err = validateRiskThreshold(overlay.RiskThreshold); err != nil {
				return nil, err
			}
		}
		if overlay.ActiveWithinDays != 0 {
			if overlay.ActiveWithinDays, err = validateDays("overlay activeWithinDays", overlay.ActiveWithinDays, DefaultActiveWithinDays); err != nil {
				return nil, err
			}
		}
		if overlay.InactiveAfterDays != 0 {
			if overlay.InactiveAfterDays, err = validateDays("overlay inactiveAfterDays", overlay.InactiveAfterDays, DefaultInactiveAfterDays); err != nil {
				return nil, err
			}
		}
		if overlay.Exclusions, err = validateExclusions(overlay.Exclusions); err != nil {
			return nil, err
		}
		out = append(out, overlay)
	}
	// Least specific first, so applying in order lets a precise overlay win.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Match.specificity() < out[j].Match.specificity()
	})
	return out, nil
}

// validateToken bounds identifier-like fields. Recommendation IDs are allowed a
// wider alphabet because the rule registry mints them, but they are still
// checked: an ID is echoed back to the UI and compared against live advice.
func validateToken(label, value string, allowColonAndSlash bool) error {
	if value == "" {
		return nil
	}
	if len(value) > maxTokenLength {
		return fmt.Errorf("%s is longer than %d characters", label, maxTokenLength)
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		case char == '_', char == '-', char == '.':
		case allowColonAndSlash && (char == ':' || char == '/'):
		default:
			return fmt.Errorf("%s %q contains an unsupported character %q", label, value, string(char))
		}
	}
	return nil
}

// validateText bounds free-text fields the user typed and we will show back.
func validateText(label, value string, limit int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if utf8.RuneCountInString(trimmed) > limit {
		return "", fmt.Errorf("%s is longer than %d characters", label, limit)
	}
	if err := rejectControlCharacters(label, trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

// rejectControlCharacters keeps terminal escapes and line breaks out of values
// that reach logs, menus, and exported files.
func rejectControlCharacters(label, value string) error {
	for _, char := range value {
		if unicode.IsControl(char) {
			return fmt.Errorf("%s contains a control character", label)
		}
	}
	return nil
}

// hasParentSegment reports whether any segment of a pattern is "..".
func hasParentSegment(pattern string) bool {
	return slices.Contains(strings.Split(pattern, "/"), "..")
}

// validateDay accepts the day-resolution stamp written by Today.
func validateDay(value string) error {
	if len(value) != len("2006-01-02") {
		return fmt.Errorf("suppression date %q must be YYYY-MM-DD", value)
	}
	for index, char := range value {
		switch index {
		case 4, 7:
			if char != '-' {
				return fmt.Errorf("suppression date %q must be YYYY-MM-DD", value)
			}
		default:
			if char < '0' || char > '9' {
				return fmt.Errorf("suppression date %q must be YYYY-MM-DD", value)
			}
		}
	}
	return nil
}
