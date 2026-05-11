package scan

import (
	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/format"
)

// SourceBreakdownRow is one element of `ds scan --json` field "sources_breakdown".
type SourceBreakdownRow struct {
	SourceType string         `json:"source_type"`
	Label      string         `json:"label"`
	Count      int            `json:"count"`
	Formats    map[string]int `json:"formats"`
}

// ScanHint is one recovery hint when a scan finds zero artifacts (`hints` in ds scan --json).
type ScanHint struct {
	Path           string `json:"path"`
	SourceType     string `json:"source_type,omitempty"`
	SuggestCommand string `json:"suggest_command,omitempty"`
}

type sourceAgg struct {
	count   int
	formats map[string]int
}

// Result holds scan summary counts and per-source breakdown for CLI output.
//
// JSON shape (see ds scan --json):
//   - "Found": map of adapter/source pipeline name → count of successfully indexed artifacts
//   - "sources_breakdown": array of { source_type, label, count, formats }
//   - "New", "Updated", "Unchanged": revision outcomes
//   - "hints": optional; only when all adapters indexed zero artifacts AND at least one hint
//     candidate exists. Empty candidate list omits the key (encoding/json omitempty on []ScanHint).
type Result struct {
	Found            map[string]int       `json:"Found"`
	SourcesBreakdown []SourceBreakdownRow `json:"sources_breakdown"`
	New              int                  `json:"New"`
	Updated          int                  `json:"Updated"`
	Unchanged        int                  `json:"Unchanged"`
	Hints            []ScanHint           `json:"hints,omitempty"`

	sourcesAgg map[string]*sourceAgg `json:"-"`
}

func newResult(adapters []string) *Result {
	r := &Result{
		Found:      make(map[string]int),
		sourcesAgg: make(map[string]*sourceAgg),
	}
	for _, name := range adapters {
		r.Found[name] = 0
	}
	return r
}

func (r *Result) finalizeSourcesBreakdown() {
	// Fixed pipeline list for phase-2 UX and stable JSON. New adapters still
	// increment Found[adapterName] but need an explicit row here + labels to appear in sources_breakdown.
	order := []string{"markdown", "openspec", "adr"}
	out := make([]SourceBreakdownRow, 0, len(order))
	for _, st := range order {
		row := SourceBreakdownRow{
			SourceType: st,
			Label:      SourceTypeDisplayLabel(st),
			Count:      0,
			Formats:    map[string]int{},
		}
		if agg := r.sourcesAgg[st]; agg != nil {
			row.Count = agg.count
			for k, v := range agg.formats {
				row.Formats[k] = v
			}
		}
		out = append(out, row)
	}
	r.SourcesBreakdown = out
}

func tallyIndexed(r *Result, adapterName string, sources []adapters.Source, art adapters.Artifact) {
	r.Found[adapterName]++

	// v0: each adapter returns one primary Source; breakdown uses that row.
	// If multiple sources diverge in format_profile, only sources[0] drives this tally.
	st := adapterName
	if len(sources) > 0 {
		st = sources[0].SourceType
	}

	prof := format.ProfileGeneric
	if len(sources) > 0 && sources[0].FormatProfile != "" {
		prof = sources[0].FormatProfile
	} else if art.FormatProfile != "" {
		prof = art.FormatProfile
	}

	agg, ok := r.sourcesAgg[st]
	if !ok {
		agg = &sourceAgg{formats: make(map[string]int)}
		r.sourcesAgg[st] = agg
	}
	agg.count++
	agg.formats[prof]++
}
