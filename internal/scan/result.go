package scan

import (
	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/openspecmetrics"
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

// RootSelectionWarning explains when a command appears to be running from a
// broad workspace root rather than a focused project root.
type RootSelectionWarning struct {
	Kind              string   `json:"kind"`
	Path              string   `json:"path"`
	Message           string   `json:"message"`
	CandidateRoots    []string `json:"candidate_roots,omitempty"`
	SuggestedCommands []string `json:"suggested_commands,omitempty"`
}

// TraversalDiagnostics explains the bounded filesystem walk that feeds scan
// discovery. It is intentionally coarse so JSON output can explain skipped
// heavy directories without becoming a file listing.
type TraversalDiagnostics struct {
	InventoryFiles  int                    `json:"inventory_files"`
	SkippedDirs     int                    `json:"skipped_dirs,omitempty"`
	SkippedByReason map[string]int         `json:"skipped_by_reason,omitempty"`
	TopSkippedDirs  []TraversalSkippedPath `json:"top_skipped_dirs,omitempty"`
}

// TraversalSkippedPath is one representative skipped directory.
type TraversalSkippedPath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// SourceCompanionAdmissionDiagnostics summarizes bounded source files admitted
// because indexed tests pointed at likely implementation companions.
type SourceCompanionAdmissionDiagnostics struct {
	Enabled                  bool                              `json:"enabled"`
	TestFiles                int                               `json:"test_files,omitempty"`
	ExistingSourceCandidates int                               `json:"existing_source_candidates,omitempty"`
	CandidatesConsidered     int                               `json:"candidates_considered,omitempty"`
	Admitted                 int                               `json:"admitted,omitempty"`
	AlreadyPresent           int                               `json:"already_present,omitempty"`
	SkippedByCap             int                               `json:"skipped_by_cap,omitempty"`
	RejectedByReason         map[string]int                    `json:"rejected_by_reason,omitempty"`
	TopAdmitted              []SourceCompanionAdmissionExample `json:"top_admitted,omitempty"`
	TopRejected              []SourceCompanionRejectionExample `json:"top_rejected,omitempty"`
}

// SourceCompanionAdmissionExample is a compact receipt for one admitted source companion.
type SourceCompanionAdmissionExample struct {
	Path       string   `json:"path"`
	Signals    []string `json:"signals,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	TestPaths  []string `json:"test_paths,omitempty"`
}

// SourceCompanionRejectionExample is a compact receipt for one rejected source companion.
type SourceCompanionRejectionExample struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Signal string `json:"signal,omitempty"`
}

// SourceManifestDiagnostics summarizes the hidden compact first-party source/test manifest.
type SourceManifestDiagnostics struct {
	Enabled         bool           `json:"enabled"`
	InventoryFiles  int            `json:"inventory_files,omitempty"`
	SourceLikeFiles int            `json:"source_like_files,omitempty"`
	TestLikeFiles   int            `json:"test_like_files,omitempty"`
	IndexedFiles    int            `json:"indexed_files,omitempty"`
	IndexedTests    int            `json:"indexed_tests,omitempty"`
	SymbolRows      int            `json:"symbol_rows,omitempty"`
	TestRows        int            `json:"test_rows,omitempty"`
	ImportRows      int            `json:"import_rows,omitempty"`
	FTSRows         int            `json:"fts_rows,omitempty"`
	IgnoredByReason map[string]int `json:"ignored_by_reason,omitempty"`
	RowsByRoot      map[string]int `json:"rows_by_root,omitempty"`
	RowsByLanguage  map[string]int `json:"rows_by_language,omitempty"`
	RowsByRole      map[string]int `json:"rows_by_role,omitempty"`
}

// ProgressEvent is a coarse scan heartbeat for long eval/index runs.
type ProgressEvent struct {
	Phase                string         `json:"phase"`
	Event                string         `json:"event,omitempty"`
	ElapsedMS            int64          `json:"elapsed_ms,omitempty"`
	FilesTotal           int            `json:"files_total,omitempty"`
	FilesScanned         int            `json:"files_scanned,omitempty"`
	CurrentAdapter       string         `json:"current_adapter,omitempty"`
	CandidatesTotal      int            `json:"candidates_total,omitempty"`
	CandidatesProcessed  int            `json:"candidates_processed,omitempty"`
	CandidatesDiscovered map[string]int `json:"candidates_discovered,omitempty"`
	CandidatesParsed     map[string]int `json:"candidates_parsed,omitempty"`
	ArtifactsUpserted    map[string]int `json:"artifacts_upserted,omitempty"`
	ParseDurationMS      int64          `json:"parse_duration_ms,omitempty"`
	ClassifierDurationMS int64          `json:"classifier_duration_ms,omitempty"`
	WriterDurationMS     int64          `json:"writer_duration_ms,omitempty"`
	FlushDurationMS      int64          `json:"flush_duration_ms,omitempty"`
	FTSDurationMS        int64          `json:"fts_duration_ms,omitempty"`
	RowsWritten          map[string]int `json:"rows_written,omitempty"`
	ChunksFlushed        map[string]int `json:"chunks_flushed,omitempty"`
	DeferredFTSRows      int            `json:"deferred_fts_rows,omitempty"`
	InventoryFiles       int            `json:"inventory_files,omitempty"`
	SkippedDirs          int            `json:"skipped_dirs,omitempty"`
	SkippedByReason      map[string]int `json:"skipped_by_reason,omitempty"`
	SharedDiscovery      bool           `json:"shared_discovery,omitempty"`
	ParallelWorkers      int            `json:"parallel_workers,omitempty"`
	CappedDiscovery      bool           `json:"capped_discovery,omitempty"`
	TransactionEnabled   bool           `json:"transaction_enabled,omitempty"`
	SkipAuthoredAtLookup bool           `json:"skip_authored_at_lookup,omitempty"`
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
	Found              map[string]int                       `json:"Found"`
	SourcesBreakdown   []SourceBreakdownRow                 `json:"sources_breakdown"`
	New                int                                  `json:"New"`
	Updated            int                                  `json:"Updated"`
	Unchanged          int                                  `json:"Unchanged"`
	Hints              []ScanHint                           `json:"hints,omitempty"`
	RootWarning        *RootSelectionWarning                `json:"root_warning,omitempty"`
	OpenSpec           *openspecmetrics.Metrics             `json:"openspec,omitempty"`
	EvidenceGraph      *EvidenceGraphDiagnostics            `json:"evidence_graph,omitempty"`
	GitEvidence        *GitEvidenceDiagnostics              `json:"git_evidence,omitempty"`
	WorkstreamEvidence *WorkstreamEvidenceDiagnostics       `json:"workstream_evidence,omitempty"`
	SourceCompanions   *SourceCompanionAdmissionDiagnostics `json:"source_companion_admission,omitempty"`
	SourceManifest     *SourceManifestDiagnostics           `json:"source_manifest,omitempty"`
	Traversal          *TraversalDiagnostics                `json:"traversal,omitempty"`

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
	order := []string{"markdown", "openspec", "adr", "source_context"}
	if _, enabled := r.Found["test_case"]; enabled || r.sourcesAgg["test_case"] != nil {
		order = append(order, "test_case")
	}
	if _, enabled := r.Found["code_comment"]; enabled || r.sourcesAgg["code_comment"] != nil {
		order = append(order, "code_comment")
	}
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
