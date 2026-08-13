package sourcecontext

import (
	pathpkg "path"
	"strings"
	"unicode"
)

// ShellImportResolver resolves parser-extracted shell source references against
// a repository manifest without evaluating variables or shell expressions.
type ShellImportResolver struct {
	files     map[string]string
	suffixes  map[string]string
	ambiguous map[string]bool
}

// NewShellImportResolver builds a deterministic resolver for repository paths.
func NewShellImportResolver(paths []string) ShellImportResolver {
	resolver := ShellImportResolver{
		files:     map[string]string{},
		suffixes:  map[string]string{},
		ambiguous: map[string]bool{},
	}
	for _, rawPath := range paths {
		filePath := normalizeShellImportPath(rawPath)
		if filePath == "" {
			continue
		}
		resolver.files[strings.ToLower(filePath)] = filePath
		parts := strings.Split(filePath, "/")
		for i := range parts {
			suffix := strings.ToLower(strings.Join(parts[i:], "/"))
			if suffix == "" || resolver.ambiguous[suffix] {
				continue
			}
			if existing, ok := resolver.suffixes[suffix]; ok && !strings.EqualFold(existing, filePath) {
				delete(resolver.suffixes, suffix)
				resolver.ambiguous[suffix] = true
				continue
			}
			resolver.suffixes[suffix] = filePath
		}
	}
	return resolver
}

// Resolve returns one unambiguous repository path for a shell source reference.
func (r ShellImportResolver) Resolve(fromPath, importRef string) string {
	fromPath = normalizeShellImportPath(fromPath)
	importRef = strings.Trim(strings.TrimSpace(importRef), "\"'")
	if fromPath == "" || importRef == "" || strings.ContainsAny(importRef, "\r\n\t ") {
		return ""
	}

	staticRef, dynamic := shellImportStaticSuffix(importRef)
	if staticRef == "" {
		return ""
	}
	fromDir := pathpkg.Dir(fromPath)
	var candidates []string
	add := func(candidate string) {
		candidate = normalizeShellImportPath(candidate)
		if candidate != "" {
			candidates = appendUniqueShellImportString(candidates, candidate)
		}
	}
	if strings.HasPrefix(importRef, ".") {
		add(pathpkg.Join(fromDir, staticRef))
	} else if dynamic {
		if strings.Contains(staticRef, "/") {
			add(staticRef)
		}
		add(pathpkg.Join(fromDir, staticRef))
		add(staticRef)
	} else {
		add(staticRef)
		add(pathpkg.Join(fromDir, staticRef))
	}

	for _, candidate := range appendShellImportExtensions(candidates) {
		if resolved := r.files[strings.ToLower(candidate)]; resolved != "" {
			return resolved
		}
	}
	for _, candidate := range appendShellImportExtensions(candidates) {
		key := strings.ToLower(strings.TrimPrefix(candidate, "./"))
		if !r.ambiguous[key] {
			if resolved := r.suffixes[key]; resolved != "" {
				return resolved
			}
		}
	}
	return ""
}

func shellImportStaticSuffix(importRef string) (string, bool) {
	value := strings.Trim(strings.TrimSpace(importRef), "\"'")
	if !strings.Contains(value, "$") {
		return value, false
	}
	lastEnd := 0
	for offset := 0; offset < len(value); {
		relative := strings.IndexByte(value[offset:], '$')
		if relative < 0 {
			break
		}
		start := offset + relative
		end := shellImportVariableEnd(value, start)
		if end == start {
			return "", true
		}
		lastEnd = end
		offset = end
	}
	return strings.TrimLeft(value[lastEnd:], "/"), true
}

func shellImportVariableEnd(value string, start int) int {
	if start < 0 || start >= len(value) || value[start] != '$' || start+1 >= len(value) {
		return start
	}
	if value[start+1] == '{' {
		closeIndex := strings.IndexByte(value[start+2:], '}')
		if closeIndex < 1 {
			return start
		}
		return start + 2 + closeIndex + 1
	}
	end := start + 1
	for end < len(value) {
		r := rune(value[end])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		end++
	}
	if end == start+1 {
		return start
	}
	return end
}

func appendShellImportExtensions(values []string) []string {
	out := append([]string(nil), values...)
	for _, value := range values {
		if pathpkg.Ext(value) != "" {
			continue
		}
		out = appendUniqueShellImportString(out, value+".sh")
		out = appendUniqueShellImportString(out, value+".bash")
		out = appendUniqueShellImportString(out, value+".zsh")
		out = appendUniqueShellImportString(out, value+".bats")
	}
	return out
}

func normalizeShellImportPath(value string) string {
	value = strings.Trim(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")), "/")
	if value == "" {
		return ""
	}
	value = pathpkg.Clean(value)
	if value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return ""
	}
	return value
}

func appendUniqueShellImportString(values []string, value string) []string {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}
