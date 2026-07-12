package detect

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// Progress reports detection work against a completed metadata inventory.
type Progress struct {
	Phase          string
	CandidatesSeen int64
	AssetsFound    int64
}

// RunOptions configures the detector pass over inventory entries.
type RunOptions struct {
	ProgressInterval int
	Progress         func(Progress)
}

// Run executes all registered detectors against a metadata inventory. It never
// follows symlinks (inventory already excludes them as walk targets) and never
// mutates the filesystem. Content-reading detectors may open small manifests.
func Run(ctx context.Context, registry *Registry, inventory scan.Result, options RunOptions) (assets.Graph, error) {
	if registry == nil {
		return assets.Graph{}, nil
	}
	if options.ProgressInterval <= 0 {
		options.ProgressInterval = 64
	}

	index := buildIndex(inventory)
	findings := map[string]Finding{}
	var links []Link
	var candidatesSeen int64

	detectors := registry.All()
	for _, entry := range inventory.Entries {
		if err := ctx.Err(); err != nil {
			return assets.Graph{}, err
		}
		if entry.IsSymlink {
			continue
		}
		candidate := Candidate{
			Path:   entry.Path,
			Name:   filepath.Base(entry.Path),
			Kind:   entry.Kind,
			IsDir:  entry.Kind == "directory",
			Parent: entry.ParentPath,
			Root:   rootFor(entry.Path, inventory.Roots),
		}
		if candidate.IsDir {
			candidate.Children = index.children[entry.Path]
		}
		candidatesSeen++
		for _, detector := range detectors {
			if !detector.Match(candidate) {
				continue
			}
			result, err := detector.Detect(ctx, candidate)
			if err != nil {
				// Detectors treat untrusted input conservatively: a single
				// failure must not abort the whole graph.
				continue
			}
			for _, finding := range result.Findings {
				if finding.Key == "" {
					finding.Key = string(finding.Kind) + ":" + finding.Path
				}
				if existing, ok := findings[finding.Key]; ok {
					findings[finding.Key] = mergeFindings(existing, finding)
				} else {
					findings[finding.Key] = finding
				}
			}
			links = append(links, result.Links...)
		}
		if options.Progress != nil && candidatesSeen%int64(options.ProgressInterval) == 0 {
			options.Progress(Progress{Phase: "detection", CandidatesSeen: candidatesSeen, AssetsFound: int64(len(findings))})
		}
	}

	graph := materialize(findings, links)
	if options.Progress != nil {
		options.Progress(Progress{Phase: "detection", CandidatesSeen: candidatesSeen, AssetsFound: int64(len(graph.Assets))})
	}
	return graph, nil
}

type pathIndex struct {
	children map[string][]string
}

func buildIndex(inventory scan.Result) pathIndex {
	children := make(map[string][]string)
	for _, entry := range inventory.Entries {
		if entry.ParentPath == "" {
			continue
		}
		children[entry.ParentPath] = append(children[entry.ParentPath], filepath.Base(entry.Path))
	}
	for parent, names := range children {
		sort.Strings(names)
		children[parent] = names
	}
	return pathIndex{children: children}
}

func rootFor(path string, roots []string) string {
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+string(filepath.Separator)) {
			return root
		}
	}
	return ""
}

func mergeFindings(a, b Finding) Finding {
	if a.DisplayName == "" {
		a.DisplayName = b.DisplayName
	}
	if a.Risk == "" {
		a.Risk = b.Risk
	}
	if a.Ecosystem == "" {
		a.Ecosystem = b.Ecosystem
	}
	if a.Class == "" {
		a.Class = b.Class
	}
	if a.Attributes == nil {
		a.Attributes = map[string]string{}
	}
	for key, value := range b.Attributes {
		if _, exists := a.Attributes[key]; !exists {
			a.Attributes[key] = value
		}
	}
	a.Evidence = append(a.Evidence, b.Evidence...)
	return a
}

func materialize(findings map[string]Finding, links []Link) assets.Graph {
	keys := make([]string, 0, len(findings))
	for key := range findings {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	keyToID := make(map[string]string, len(keys))
	keyToDetector := make(map[string]struct{ id string; version int }, len(keys))
	graph := assets.Graph{Assets: make([]assets.Asset, 0, len(keys))}
	for _, key := range keys {
		finding := findings[key]
		id := uuid.NewString()
		keyToID[key] = id
		detectorID := "detect.unknown"
		detectorVersion := 1
		attrs := map[string]string{}
		for k, v := range finding.Attributes {
			attrs[k] = v
		}
		if value, ok := attrs["detector_id"]; ok && value != "" {
			detectorID = value
			delete(attrs, "detector_id")
		}
		if value, ok := attrs["detector_version"]; ok {
			if parsed := atoi(value); parsed > 0 {
				detectorVersion = parsed
			}
			delete(attrs, "detector_version")
		}
		keyToDetector[key] = struct {
			id      string
			version int
		}{id: detectorID, version: detectorVersion}
		if len(attrs) == 0 {
			attrs = nil
		}
		asset := assets.Asset{
			ID:              id,
			Kind:            finding.Kind,
			DisplayName:     finding.DisplayName,
			Path:            finding.Path,
			Risk:            finding.Risk,
			Ecosystem:       finding.Ecosystem,
			Class:           finding.Class,
			DetectorID:      detectorID,
			DetectorVersion: detectorVersion,
			Attributes:      attrs,
			Evidence:        finding.Evidence,
		}
		if asset.Risk == "" {
			asset.Risk = assets.RiskInformational
		}
		graph.Assets = append(graph.Assets, asset)
	}

	seenLinks := map[string]int{}
	for _, link := range links {
		sourceID, okSource := keyToID[link.SourceKey]
		targetID, okTarget := keyToID[link.TargetKey]
		if !okSource || !okTarget {
			continue
		}
		confidence := link.Confidence
		if confidence <= 0 {
			confidence = link.Evidence.Confidence
		}
		if confidence <= 0 {
			confidence = 0.5
		}
		dedupeKey := link.Kind + "|" + sourceID + "|" + targetID
		if existing, ok := seenLinks[dedupeKey]; ok {
			if confidence > graph.Relationships[existing].Confidence {
				graph.Relationships[existing].Confidence = confidence
				graph.Relationships[existing].Evidence = link.Evidence
			}
			continue
		}
		meta := keyToDetector[link.SourceKey]
		seenLinks[dedupeKey] = len(graph.Relationships)
		graph.Relationships = append(graph.Relationships, assets.Relationship{
			ID:              uuid.NewString(),
			SourceID:        sourceID,
			TargetID:        targetID,
			Kind:            link.Kind,
			Confidence:      confidence,
			DetectorID:      meta.id,
			DetectorVersion: meta.version,
			Evidence:        link.Evidence,
		})
	}
	sort.Slice(graph.Relationships, func(i, j int) bool {
		if graph.Relationships[i].Kind == graph.Relationships[j].Kind {
			return graph.Relationships[i].SourceID < graph.Relationships[j].SourceID
		}
		return graph.Relationships[i].Kind < graph.Relationships[j].Kind
	})
	return graph
}

func atoi(value string) int {
	n := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// StampDetector records which detector produced a finding.
func StampDetector(finding Finding, detectorID string, version int) Finding {
	if finding.Attributes == nil {
		finding.Attributes = map[string]string{}
	}
	finding.Attributes["detector_id"] = detectorID
	if version > 0 {
		finding.Attributes["detector_version"] = itoa(version)
	}
	return finding
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := value < 0
	if neg {
		value = -value
	}
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
