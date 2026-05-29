package scan

// SourceTypeDisplayLabel returns human-facing copy for `ds scan` summaries.
// Internal pipeline IDs remain markdown / openspec / adr in config and the DB;
// this layer is display-only.
func SourceTypeDisplayLabel(sourceType string) string {
	switch sourceType {
	case "markdown":
		return "Planning docs"
	case "openspec":
		return "OpenSpec"
	case "adr":
		return "ADRs"
	case "source_context":
		return "Source context"
	case "test_case":
		return "Test cases"
	case "code_comment":
		return "Code comments"
	default:
		return sourceType
	}
}
