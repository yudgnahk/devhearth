package detect

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
)

// ErrSymlinkRefused is returned when a content open would follow a symlink.
var ErrSymlinkRefused = errors.New("refusing to follow symlink")

// HasChild reports whether a candidate directory lists name among children.
func HasChild(candidate Candidate, name string) bool {
	for _, child := range candidate.Children {
		if child == name {
			return true
		}
	}
	return false
}

// HasParentChild reports whether Parent lists name among its inventory children.
func HasParentChild(candidate Candidate, name string) bool {
	for _, child := range candidate.ParentChildren {
		if child == name {
			return true
		}
	}
	return false
}

// HasAnyParentChild reports whether Parent lists any of the given names.
func HasAnyParentChild(candidate Candidate, names ...string) bool {
	if len(names) == 0 || len(candidate.ParentChildren) == 0 {
		return false
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	for _, child := range candidate.ParentChildren {
		if _, ok := wanted[child]; ok {
			return true
		}
	}
	return false
}

// HasChildSuffix reports whether any child name ends with suffix.
func HasChildSuffix(candidate Candidate, suffix string) bool {
	for _, child := range candidate.Children {
		if strings.HasSuffix(child, suffix) {
			return true
		}
	}
	return false
}

// ChildNames returns the set of immediate children for membership checks.
func ChildNames(candidate Candidate) map[string]struct{} {
	set := make(map[string]struct{}, len(candidate.Children))
	for _, child := range candidate.Children {
		set[child] = struct{}{}
	}
	return set
}

// ProjectKey builds a stable finding key for a project rooted at path.
func ProjectKey(ecosystem, path string) string {
	return "project:" + ecosystem + ":" + path
}

// AssetKey builds a stable finding key for an asset kind and path.
func AssetKey(kind assets.Kind, path string) string {
	return string(kind) + ":" + path
}

// DisplayNameFromPath returns a short label for UI lists.
func DisplayNameFromPath(path string) string {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return path
	}
	return base
}

// PathHasComponent reports whether path contains a path component equal to name
// (case-insensitive). It does not match arbitrary substrings inside a component.
func PathHasComponent(path, name string) bool {
	if path == "" || name == "" {
		return false
	}
	target := strings.ToLower(name)
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.EqualFold(part, target) {
			return true
		}
	}
	return false
}

// ReadFileLimited reads at most maxBytes from path. It refuses to follow
// symlinks (O_NOFOLLOW on Darwin/Linux; Lstat guard elsewhere). Callers should
// only open paths already classified as regular files when possible.
func ReadFileLimited(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}
	file, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	buf := make([]byte, maxBytes+1)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	if int64(n) > maxBytes {
		return buf[:maxBytes], nil
	}
	return buf[:n], nil
}

// Evidence conf helper.
func Evidence(kind, value string, confidence float64) assets.Evidence {
	if confidence <= 0 {
		confidence = 0.5
	}
	if confidence > 1 {
		confidence = 1
	}
	return assets.Evidence{Kind: kind, Value: value, Confidence: confidence}
}
