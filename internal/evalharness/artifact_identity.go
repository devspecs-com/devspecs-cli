package evalharness

import (
	"path/filepath"
	"strings"
)

func normalizeEvalArtifactPath(path string) string {
	return strings.ToLower(strings.TrimSpace(filepath.ToSlash(path)))
}

func evalArtifactBasePath(path string) string {
	path = normalizeEvalArtifactPath(path)
	idx := strings.LastIndex(path, "#l")
	if idx < 0 {
		return path
	}
	if !validEvalLineRefSuffix(path[idx+2:]) {
		return path
	}
	return path[:idx]
}

func evalArtifactHasLineRef(path string) bool {
	path = normalizeEvalArtifactPath(path)
	idx := strings.LastIndex(path, "#l")
	return idx >= 0 && validEvalLineRefSuffix(path[idx+2:])
}

func evalArtifactIdentityMatch(a, b string) bool {
	aNorm := normalizeEvalArtifactPath(a)
	bNorm := normalizeEvalArtifactPath(b)
	if aNorm == "" || bNorm == "" {
		return false
	}
	if aNorm == bNorm {
		return true
	}
	aBase := evalArtifactBasePath(aNorm)
	bBase := evalArtifactBasePath(bNorm)
	if aBase == "" || aBase != bBase {
		return false
	}
	return !evalArtifactHasLineRef(aNorm) || !evalArtifactHasLineRef(bNorm)
}

func evalArtifactPathInSetByIdentity(path string, set map[string]bool) bool {
	if set[filepath.ToSlash(path)] {
		return true
	}
	for candidate := range set {
		if evalArtifactIdentityMatch(path, candidate) {
			return true
		}
	}
	return false
}

func evalMarkdownLikePath(path string) bool {
	base := evalArtifactBasePath(path)
	switch strings.ToLower(filepath.Ext(base)) {
	case ".md", ".mdx":
		return true
	default:
		return false
	}
}

func validEvalLineRefSuffix(suffix string) bool {
	if suffix == "" {
		return false
	}
	if allDigits(suffix) {
		return true
	}
	parts := strings.Split(suffix, "-l")
	if len(parts) != 2 {
		return false
	}
	return allDigits(parts[0]) && allDigits(parts[1])
}
