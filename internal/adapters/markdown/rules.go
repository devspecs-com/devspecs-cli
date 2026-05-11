package markdown

import (
	"path/filepath"
	"strings"

	"github.com/devspecs-com/devspecs-cli/internal/config"
)

// MatchSourceRules returns kind/subtype/tags from the first matching rule.
// Rules match paths relative to each configured markdown source directory (repo-relative).
func MatchSourceRules(repoRel string, sourcePaths []string, rules []config.SourceRule) (kind, subtype string, extraTags []string, matched bool) {
	repoRel = filepath.ToSlash(repoRel)
	for _, rule := range rules {
		for _, prefix := range sourcePaths {
			prefix = filepath.ToSlash(strings.TrimSpace(prefix))
			if prefix == "" {
				continue
			}
			suffix, ok := pathSuffixUnderSource(repoRel, prefix)
			if !ok {
				continue
			}
			if globMatch(rule.Match, suffix) {
				tags := append([]string(nil), rule.Tags...)
				return rule.Kind, rule.Subtype, tags, true
			}
		}
	}
	return "", "", nil, false
}

func pathSuffixUnderSource(repoRel, prefix string) (suffix string, ok bool) {
	switch {
	case prefix == ".":
		return repoRel, true
	case repoRel == prefix:
		return filepath.Base(repoRel), true
	case strings.HasPrefix(repoRel, prefix+"/"):
		return strings.TrimPrefix(repoRel, prefix+"/"), true
	default:
		return "", false
	}
}

func globMatch(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	ok, err := filepath.Match(pattern, path)
	return err == nil && ok
}
