// Package attribute assigns filesystem size and activity evidence to assets.
// It works from the inventory rollup only: it never touches the filesystem and
// never mutates its inputs.
package attribute

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// gitDirName is skipped when measuring source activity: Git rewrites it on
// every read-only command, so it says nothing about source work.
const gitDirName = ".git"

// Apply returns a copy of graph whose assets carry attributed sizes and last
// source activity. Assets whose paths are absent from the inventory (or whose
// kind does not own storage) are returned with an unattributed size.
func Apply(graph assets.Graph, index map[string][]scan.DirectoryNode) assets.Graph {
	nodes := flatten(index)
	owners, generated := ownerPaths(graph, nodes)
	nested := nestedBytes(owners, nodes)
	activity := activityIndex(index, nodes, generated)

	out := assets.Graph{
		Assets:        make([]assets.Asset, 0, len(graph.Assets)),
		Relationships: graph.Relationships,
	}
	for _, asset := range graph.Assets {
		out.Assets = append(out.Assets, attributed(asset, nodes, owners, nested, activity))
	}
	return out
}

func attributed(
	asset assets.Asset,
	nodes map[string]scan.DirectoryNode,
	owners map[string]struct{},
	nested map[string]int64,
	activity map[string]activityTimes,
) assets.Asset {
	if !asset.Kind.OwnsStorage() {
		return asset
	}
	if _, owns := owners[asset.Path]; !owns {
		return asset
	}
	node, ok := nodes[asset.Path]
	if !ok {
		return asset
	}
	// Defensive clamp: hard-link accounting can only reduce totals, never invert
	// them, so a negative here means the rollup and asset paths disagree.
	exclusive := max(node.TotalAllocatedBytes-nested[asset.Path], 0)
	asset.Size = assets.Size{
		Attributed:              true,
		LogicalBytes:            node.TotalLogicalBytes,
		AllocatedBytes:          node.TotalAllocatedBytes,
		ExclusiveAllocatedBytes: exclusive,
		Shared:                  asset.Kind.IsShared(),
		Uncertain:               node.HardLinkAliasCount > 0,
	}
	times := activity[asset.Path]
	if asset.Kind.IsGenerated() {
		asset.LastActivityAt = times.subtree
	} else {
		asset.LastActivityAt = times.source
	}
	return asset
}

func flatten(index map[string][]scan.DirectoryNode) map[string]scan.DirectoryNode {
	nodes := make(map[string]scan.DirectoryNode, len(index))
	for _, children := range index {
		for _, node := range children {
			nodes[node.Path] = node
		}
	}
	return nodes
}

// ownerPaths returns the asset paths that own storage and, of those, the ones
// holding generated material. Several assets may share a path; the sets are
// keyed by path so the exclusive math counts each path once.
func ownerPaths(graph assets.Graph, nodes map[string]scan.DirectoryNode) (owners, generated map[string]struct{}) {
	owners = make(map[string]struct{}, len(graph.Assets))
	generated = make(map[string]struct{}, len(graph.Assets))
	for _, asset := range graph.Assets {
		if !asset.Kind.OwnsStorage() || asset.Path == "" {
			continue
		}
		if _, ok := nodes[asset.Path]; !ok {
			continue
		}
		owners[asset.Path] = struct{}{}
		if asset.Kind.IsGenerated() {
			generated[asset.Path] = struct{}{}
		}
	}
	return owners, generated
}

// nestedBytes sums, for each owning path, the subtree totals of the owning
// paths directly beneath it. Only the nearest owning ancestor is charged, so a
// project containing a build output inside a nested install is not charged twice.
func nestedBytes(owners map[string]struct{}, nodes map[string]scan.DirectoryNode) map[string]int64 {
	nested := make(map[string]int64, len(owners))
	for path := range owners {
		ancestor, found := nearestOwner(path, owners, nodes)
		if !found {
			continue
		}
		nested[ancestor] += nodes[path].TotalAllocatedBytes
	}
	return nested
}

// nearestOwner walks up from path (exclusive) to the closest owning asset path.
func nearestOwner(path string, owners map[string]struct{}, nodes map[string]scan.DirectoryNode) (string, bool) {
	for current := parentOf(path, nodes); current != ""; current = parentOf(current, nodes) {
		if _, ok := owners[current]; ok {
			return current, true
		}
	}
	return "", false
}

func parentOf(path string, nodes map[string]scan.DirectoryNode) string {
	if node, ok := nodes[path]; ok {
		return node.ParentPath
	}
	parent := filepath.Dir(path)
	if parent == path {
		return ""
	}
	return parent
}

// activityTimes holds the two activity readings a rollup can support: the
// whole subtree, and the subtree with generated and Git material removed.
type activityTimes struct {
	subtree time.Time
	source  time.Time
}

// activityIndex rolls modification times upward. Deepest paths are processed
// first so a parent always sees finished child readings.
func activityIndex(
	index map[string][]scan.DirectoryNode,
	nodes map[string]scan.DirectoryNode,
	generated map[string]struct{},
) map[string]activityTimes {
	times := make(map[string]activityTimes, len(nodes))
	for _, path := range pathsDeepestFirst(nodes) {
		node := nodes[path]
		current := activityTimes{subtree: node.ModifiedAt, source: node.ModifiedAt}
		for _, child := range index[path] {
			childTimes := times[child.Path]
			current.subtree = later(current.subtree, childTimes.subtree)
			if skipForSource(child, generated) {
				continue
			}
			current.source = later(current.source, childTimes.source)
		}
		times[path] = current
	}
	return times
}

// skipForSource excludes regenerable and Git-internal subtrees so an install or
// a `git status` does not read as source activity.
func skipForSource(node scan.DirectoryNode, generated map[string]struct{}) bool {
	if node.Name == gitDirName {
		return true
	}
	_, isGenerated := generated[node.Path]
	return isGenerated
}

func pathsDeepestFirst(nodes map[string]scan.DirectoryNode) []string {
	paths := make([]string, 0, len(nodes))
	for path := range nodes {
		paths = append(paths, path)
	}
	separator := string(filepath.Separator)
	sort.Slice(paths, func(i, j int) bool {
		di, dj := strings.Count(paths[i], separator), strings.Count(paths[j], separator)
		if di != dj {
			return di > dj
		}
		return paths[i] > paths[j]
	})
	return paths
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
