// Package scan orchestrates artifact discovery: walks the repo, dispatches
// adapters, and upserts artifacts/revisions/todos/criteria into the store.
package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/adapters"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/markdown"
	"github.com/devspecs-com/devspecs-cli/internal/adapters/todoparse"
	"github.com/devspecs-com/devspecs-cli/internal/config"
	"github.com/devspecs-com/devspecs-cli/internal/format"
	"github.com/devspecs-com/devspecs-cli/internal/idgen"
	"github.com/devspecs-com/devspecs-cli/internal/ignore"
	"github.com/devspecs-com/devspecs-cli/internal/repo"
	docsections "github.com/devspecs-com/devspecs-cli/internal/sections"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/userident"
)

// Result type and tally helpers live in result.go and labels.go.

// Scanner runs adapters against a repo and persists results.
type Scanner struct {
	db       *store.DB
	ids      *idgen.Factory
	adapters []adapters.Adapter
}

var (
	fileFirstCommitDate  = repo.FileFirstCommitDate
	fileFirstCommitDates = repo.FileFirstCommitDates
)

type RunOptions struct {
	MaxCandidatesByAdapter    map[string]int
	FileWorkerCount           int
	UseTransaction            bool
	SkipAuthoredAtLookup      bool
	FreshIndex                bool
	IncludeGitEvidence        bool
	IncludeWorkstreamEvidence bool
	RichTypedIndex            bool
	RecentSourceContext       bool
	FirstPartySourceContext   bool
	SourceManifest            bool
	GitMaxCommits             int
	GitMaxFilesPerCommit      int
	IgnoreRules               bool
	PhaseTiming               bool
	Progress                  func(ProgressEvent)
	ProgressInterval          time.Duration
}

// New creates a Scanner with the given store and adapters.
func New(db *store.DB, ids *idgen.Factory, adpts []adapters.Adapter) *Scanner {
	return &Scanner{db: db, ids: ids, adapters: adpts}
}

// Run scans the repo at repoRoot, using config if available.
func (s *Scanner) Run(ctx context.Context, repoRoot string, cfg *config.RepoConfig) (*Result, error) {
	return s.RunWithOptions(ctx, repoRoot, cfg, RunOptions{})
}

// RunWithOptions scans the repo with optional eval/runtime controls.
func (s *Scanner) RunWithOptions(ctx context.Context, repoRoot string, cfg *config.RepoConfig, opts RunOptions) (*Result, error) {
	if cfg != nil {
		if err := config.ValidateRepoConfig(cfg); err != nil {
			return nil, fmt.Errorf("repo config: %w", err)
		}
	}
	adapterNames := make([]string, 0, len(s.adapters))
	for _, a := range s.adapters {
		adapterNames = append(adapterNames, a.Name())
	}
	result := newResult(adapterNames)
	now := time.Now().UTC().Format(time.RFC3339)
	progress := newProgressReporter(opts.Progress, opts.ProgressInterval)
	timing := newScanPhaseRecorder(opts.PhaseTiming)

	if !opts.IgnoreRules {
		matcher, _ := ignore.NewMatcher(repoRoot)
		ctx = ignore.WithContext(ctx, matcher)
	}

	phase := timing.start("ensure_repo", "")
	repoID, err := s.ensureRepo(repoRoot, now)
	phase.finish(nil, statusTimingDetails(err))
	if err != nil {
		return nil, fmt.Errorf("ensure repo: %w", err)
	}

	inTx := false
	if opts.UseTransaction {
		phase := timing.start("begin_transaction", "")
		if _, err := s.db.Exec("BEGIN IMMEDIATE"); err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("begin scan transaction: %w", err)
		}
		phase.finish(nil, nil)
		inTx = true
		defer func() {
			if inTx {
				_, _ = s.db.Exec("ROLLBACK")
			}
		}()
	}

	state := &scanRunState{}
	if opts.FreshIndex {
		phase := timing.start("fresh_index_prepare", "")
		state.shortIDs, err = s.seedExistingShortIDClaims()
		if err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("seed fresh index short ids: %w", err)
		}
		state.fresh, err = newFreshInserter(s.db)
		if err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("prepare fresh index inserts: %w", err)
		}
		phase.finish(map[string]int{"short_id_claims": len(state.shortIDs)}, nil)
		defer state.fresh.close()
	}

	progress.emit(ProgressEvent{
		Phase:                "scan",
		Event:                "start",
		TransactionEnabled:   opts.UseTransaction,
		SkipAuthoredAtLookup: opts.SkipAuthoredAtLookup,
	})
	phase = timing.start("shared_discovery", "")
	sharedCandidates, traversal, err := s.discoverSharedFileCandidates(ctx, repoRoot, cfg, opts, progress)
	phase.finish(candidateTimingCounts(sharedCandidates, traversal), statusTimingDetails(err))
	if err != nil {
		return nil, fmt.Errorf("shared file discovery: %w", err)
	}
	result.Traversal = traversal
	phase = timing.start("source_companion_admission", "")
	if diagnostics, companions := buildTestSourceCompanionCandidates(ctx, repoRoot, sharedCandidates["test_case"], sharedCandidates["source_context"]); diagnostics != nil {
		result.SourceCompanions = diagnostics
		if len(companions) > 0 {
			sharedCandidates["source_context"] = append(sharedCandidates["source_context"], companions...)
			sortCandidates(sharedCandidates["source_context"])
		}
	}
	phase.finish(candidateTimingCounts(sharedCandidates, traversal), nil)
	if opts.RecentSourceContext {
		phase := timing.start("recent_source_context", "")
		if companions := buildRecentGitSourceContextCandidates(ctx, repoRoot, sharedCandidates["source_context"], opts); len(companions) > 0 {
			sharedCandidates["source_context"] = append(sharedCandidates["source_context"], companions...)
			sortCandidates(sharedCandidates["source_context"])
		}
		phase.finish(candidateTimingCounts(sharedCandidates, traversal), nil)
	}
	if opts.FirstPartySourceContext {
		phase := timing.start("first_party_source_context", "")
		if candidates := buildFirstPartySourceContextCandidates(ctx, repoRoot, sharedCandidates["source_context"]); len(candidates) > 0 {
			sharedCandidates["source_context"] = append(sharedCandidates["source_context"], candidates...)
			sortCandidates(sharedCandidates["source_context"])
		}
		phase.finish(candidateTimingCounts(sharedCandidates, traversal), nil)
	}
	parsedByAdapter := map[string]int{}
	upsertedByAdapter := map[string]int{}
	for _, adapter := range s.adapters {
		adapterPhase := timing.start("adapter_parse_persist", adapter.Name())
		var candidates []adapters.Candidate
		if _, ok := adapter.(adapters.FileDiscoveryAdapter); ok {
			candidates = sharedCandidates[adapter.Name()]
		} else {
			var err error
			candidates, err = adapter.Discover(ctx, repoRoot, cfg)
			if err != nil {
				adapterPhase.finish(map[string]int{"candidates": len(candidates)}, map[string]string{"status": "discover_error"})
				continue
			}
		}
		progress.emit(ProgressEvent{
			Phase:             "parse_upsert",
			Event:             "adapter_start",
			CurrentAdapter:    adapter.Name(),
			CandidatesTotal:   len(candidates),
			CandidatesParsed:  cloneIntMap(parsedByAdapter),
			ArtifactsUpserted: cloneIntMap(upsertedByAdapter),
		})
		authoredAtPhase := timing.start("authored_at_prefetch", adapter.Name())
		prefetchedAuthoredAt := 0
		if benefitsFromAuthoredAtPrefetch(adapter.Name()) {
			prefetchedAuthoredAt = state.prefetchAuthoredAt(ctx, repoRoot, candidates, now, opts)
		}
		authoredAtPhase.finish(map[string]int{"paths": prefetchedAuthoredAt}, nil)

		if opts.FreshIndex {
			parsed, parsedCount, err := parseCandidatesForFreshIndex(ctx, repoRoot, adapter, candidates, opts, progress)
			if err != nil {
				adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedCount}, statusTimingDetails(err))
				return nil, err
			}
			parsedByAdapter[adapter.Name()] += parsedCount
			var writerDurationMS int64
			for i, parsedArtifact := range parsed {
				if !parsedArtifact.ok {
					continue
				}
				writerStarted := time.Now()
				if err := s.upsertArtifact(repoRoot, repoID, adapter.Name(), parsedArtifact.artifact, parsedArtifact.sources, parsedArtifact.parseResult, now, result, opts, state); err != nil {
					adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()]}, statusTimingDetails(err))
					return nil, fmt.Errorf("upsert artifact %q: %w", parsedArtifact.artifact.SourceIdentity, err)
				}
				writerDurationMS += time.Since(writerStarted).Milliseconds()
				upsertedByAdapter[adapter.Name()]++
				progress.maybe(ProgressEvent{
					Phase:               "persist",
					CurrentAdapter:      adapter.Name(),
					CandidatesTotal:     len(candidates),
					CandidatesProcessed: i + 1,
					CandidatesParsed:    cloneIntMap(parsedByAdapter),
					ArtifactsUpserted:   cloneIntMap(upsertedByAdapter),
					WriterDurationMS:    writerDurationMS,
					RowsWritten:         state.fresh.rowCounts(),
					ChunksFlushed:       state.fresh.chunkCounts(),
				})
			}
			progress.emit(ProgressEvent{
				Phase:               "persist",
				Event:               "adapter_done",
				CurrentAdapter:      adapter.Name(),
				CandidatesTotal:     len(candidates),
				CandidatesProcessed: len(candidates),
				CandidatesParsed:    cloneIntMap(parsedByAdapter),
				ArtifactsUpserted:   cloneIntMap(upsertedByAdapter),
				WriterDurationMS:    writerDurationMS,
				RowsWritten:         state.fresh.rowCounts(),
				ChunksFlushed:       state.fresh.chunkCounts(),
			})
			adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()]}, nil)
			continue
		}

		parsed, parsedCount, err := parseCandidatesForFreshIndex(ctx, repoRoot, adapter, candidates, opts, progress)
		if err != nil {
			adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedCount}, statusTimingDetails(err))
			return nil, err
		}
		parsedByAdapter[adapter.Name()] += parsedCount
		sourceIdentityCounts := parsedSourceIdentityCounts(parsed)
		existingArtifacts, err := s.existingArtifactsBySourceIdentity(sourceIdentityKeys(sourceIdentityCounts))
		if err != nil {
			adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()]}, statusTimingDetails(err))
			return nil, fmt.Errorf("lookup existing artifacts for %s: %w", adapter.Name(), err)
		}
		existingCount := 0
		batchNewCount := 0
		var writerDurationMS int64
		for i, parsedArtifact := range parsed {
			if !parsedArtifact.ok {
				continue
			}
			writerStarted := time.Now()
			art := parsedArtifact.artifact
			_, alreadyIndexed := existingArtifacts[art.SourceIdentity]
			if !alreadyIndexed && sourceIdentityCounts[art.SourceIdentity] == 1 {
				if err := s.insertBatchedNewArtifact(repoRoot, repoID, adapter.Name(), art, parsedArtifact.sources, parsedArtifact.parseResult, now, result, opts, state); err != nil {
					adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()], "existing": existingCount, "batch_new": batchNewCount}, statusTimingDetails(err))
					return nil, fmt.Errorf("batch insert artifact %q: %w", art.SourceIdentity, err)
				}
				batchNewCount++
			} else {
				if err := s.upsertArtifact(repoRoot, repoID, adapter.Name(), art, parsedArtifact.sources, parsedArtifact.parseResult, now, result, opts, state); err != nil {
					adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()], "existing": existingCount, "batch_new": batchNewCount}, statusTimingDetails(err))
					return nil, fmt.Errorf("upsert artifact %q: %w", art.SourceIdentity, err)
				}
				existingCount++
			}
			writerDurationMS += time.Since(writerStarted).Milliseconds()
			upsertedByAdapter[adapter.Name()]++
			var rowsWritten map[string]int
			var chunksFlushed map[string]int
			if state != nil && state.batchNew != nil {
				rowsWritten = state.batchNew.rowCounts()
				chunksFlushed = state.batchNew.chunkCounts()
			}
			progress.maybe(ProgressEvent{
				Phase:               "persist",
				CurrentAdapter:      adapter.Name(),
				CandidatesTotal:     len(candidates),
				CandidatesProcessed: i + 1,
				CandidatesParsed:    cloneIntMap(parsedByAdapter),
				ArtifactsUpserted:   cloneIntMap(upsertedByAdapter),
				WriterDurationMS:    writerDurationMS,
				RowsWritten:         rowsWritten,
				ChunksFlushed:       chunksFlushed,
			})
		}
		if state != nil && state.batchNew != nil {
			if err := state.batchNew.flushRows(); err != nil {
				adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()], "existing": existingCount, "batch_new": batchNewCount}, statusTimingDetails(err))
				return nil, fmt.Errorf("flush batch-new rows for %s: %w", adapter.Name(), err)
			}
		}
		progress.emit(ProgressEvent{
			Phase:               "persist",
			Event:               "adapter_done",
			CurrentAdapter:      adapter.Name(),
			CandidatesTotal:     len(candidates),
			CandidatesProcessed: len(candidates),
			CandidatesParsed:    cloneIntMap(parsedByAdapter),
			ArtifactsUpserted:   cloneIntMap(upsertedByAdapter),
			WriterDurationMS:    writerDurationMS,
		})
		adapterPhase.finish(map[string]int{"candidates": len(candidates), "parsed": parsedByAdapter[adapter.Name()], "upserted": upsertedByAdapter[adapter.Name()], "existing": existingCount, "batch_new": batchNewCount}, nil)
	}

	if state != nil && state.batchNew != nil {
		phase := timing.start("batch_new_writer_flush", "")
		flushStarted := time.Now()
		if err := state.batchNew.flushRows(); err != nil {
			phase.finish(state.batchNew.rowCounts(), statusTimingDetails(err))
			return nil, fmt.Errorf("flush batch-new rows: %w", err)
		}
		flushDurationMS := time.Since(flushStarted).Milliseconds()
		phase.finish(state.batchNew.rowCounts(), nil)
		progress.emit(ProgressEvent{
			Phase:           "batch_new_writer",
			Event:           "rows_flushed",
			FlushDurationMS: flushDurationMS,
			RowsWritten:     state.batchNew.rowCounts(),
			ChunksFlushed:   state.batchNew.chunkCounts(),
			DeferredFTSRows: state.batchNew.pendingFTSRows(),
		})
	}

	if state != nil && state.fresh != nil {
		phase := timing.start("fresh_index_writer_flush", "")
		flushStarted := time.Now()
		if err := state.fresh.flushRows(); err != nil {
			phase.finish(state.fresh.rowCounts(), statusTimingDetails(err))
			return nil, fmt.Errorf("flush fresh index rows: %w", err)
		}
		flushDurationMS := time.Since(flushStarted).Milliseconds()
		phase.finish(state.fresh.rowCounts(), nil)
		progress.emit(ProgressEvent{
			Phase:           "fresh_index_writer",
			Event:           "rows_flushed",
			FlushDurationMS: flushDurationMS,
			RowsWritten:     state.fresh.rowCounts(),
			ChunksFlushed:   state.fresh.chunkCounts(),
			DeferredFTSRows: state.fresh.pendingFTSRows(),
		})
	}

	phase = timing.start("openspec_links", "")
	if err := s.syncOpenSpecLinks(repoID, now); err != nil {
		phase.finish(nil, statusTimingDetails(err))
		return nil, fmt.Errorf("sync openspec links: %w", err)
	}
	phase.finish(nil, nil)
	progress.emit(ProgressEvent{
		Phase: "evidence_graph",
		Event: "start",
	})
	phase = timing.start("evidence_graph", "")
	if diagnostics, err := s.rebuildEvidenceGraph(repoID, now, evidenceGraphBuildOptions{RichTypedIndex: opts.RichTypedIndex, PhaseTiming: opts.PhaseTiming}); err != nil {
		phase.finish(nil, statusTimingDetails(err))
		return nil, fmt.Errorf("rebuild evidence graph: %w", err)
	} else {
		result.EvidenceGraph = diagnostics
		phase.finish(map[string]int{
			"concepts": diagnostics.ConceptsIndexed,
			"mentions": diagnostics.MentionsIndexed,
			"edges":    diagnostics.EdgesIndexed,
		}, nil)
		progress.emit(ProgressEvent{
			Phase: "evidence_graph",
			Event: "done",
			RowsWritten: map[string]int{
				"concepts": diagnostics.ConceptsIndexed,
				"mentions": diagnostics.MentionsIndexed,
				"edges":    diagnostics.EdgesIndexed,
			},
		})
	}
	if opts.IncludeGitEvidence || opts.IncludeWorkstreamEvidence {
		phase := timing.start("git_evidence", "")
		if gitDiagnostics, workstreamDiagnostics, err := s.rebuildGitEvidence(ctx, repoRoot, repoID, now, opts); err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("rebuild git evidence: %w", err)
		} else {
			result.GitEvidence = gitDiagnostics
			result.WorkstreamEvidence = workstreamDiagnostics
			counts := map[string]int{}
			if gitDiagnostics != nil {
				counts["commits"] = gitDiagnostics.CommitsStored
				counts["files"] = gitDiagnostics.FilesStored
				counts["edges"] = gitDiagnostics.EdgesIndexed
			}
			if workstreamDiagnostics != nil {
				counts["workstream_edges"] = workstreamDiagnostics.EdgesIndexed
			}
			phase.finish(counts, nil)
		}
	} else {
		phase := timing.start("git_facts_delete", "")
		if err := s.db.DeleteRepoGitFacts(repoID); err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("delete git facts: %w", err)
		}
		phase.finish(nil, nil)
	}
	if opts.SourceManifest {
		progress.emit(ProgressEvent{
			Phase: "source_manifest",
			Event: "start",
		})
		phase := timing.start("source_manifest", "")
		if diagnostics, err := s.rebuildSourceManifest(ctx, repoRoot, repoID, now, opts.PhaseTiming, opts.FileWorkerCount); err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("rebuild source manifest: %w", err)
		} else {
			result.SourceManifest = diagnostics
			phase.finish(map[string]int{
				"inventory_files": diagnostics.InventoryFiles,
				"indexed_files":   diagnostics.IndexedFiles,
				"symbols":         diagnostics.SymbolRows,
				"tests":           diagnostics.TestRows,
				"imports":         diagnostics.ImportRows,
				"fts_rows":        diagnostics.FTSRows,
			}, nil)
			progress.emit(ProgressEvent{
				Phase:        "source_manifest",
				Event:        "done",
				FilesTotal:   diagnostics.InventoryFiles,
				FilesScanned: diagnostics.IndexedFiles,
				RowsWritten: map[string]int{
					"files":   diagnostics.IndexedFiles,
					"symbols": diagnostics.SymbolRows,
					"tests":   diagnostics.TestRows,
					"imports": diagnostics.ImportRows,
					"fts":     diagnostics.FTSRows,
				},
			})
		}
	}
	phase = timing.start("openspec_metrics", "")
	if metrics, err := s.computeOpenSpecMetrics(repoRoot, repoID); err != nil {
		phase.finish(nil, statusTimingDetails(err))
		return nil, fmt.Errorf("compute openspec metrics: %w", err)
	} else {
		result.OpenSpec = metrics
		phase.finish(nil, nil)
	}

	phase = timing.start("record_scan_meta", "")
	s.recordScanMeta(repoID, repoRoot, now)
	phase.finish(nil, nil)
	phase = timing.start("sources_breakdown", "")
	result.finalizeSourcesBreakdown()
	phase.finish(map[string]int{"rows": len(result.SourcesBreakdown)}, nil)
	if state != nil && state.fresh != nil {
		progress.emit(ProgressEvent{
			Phase:           "fresh_index_fts",
			Event:           "start",
			DeferredFTSRows: state.fresh.pendingFTSRows(),
		})
		phase := timing.start("fresh_index_fts", "")
		ftsStarted := time.Now()
		if err := state.fresh.flushDeferredFTS(); err != nil {
			phase.finish(state.fresh.rowCounts(), statusTimingDetails(err))
			return nil, fmt.Errorf("flush deferred fresh index FTS: %w", err)
		}
		ftsDurationMS := time.Since(ftsStarted).Milliseconds()
		phase.finish(state.fresh.rowCounts(), nil)
		progress.emit(ProgressEvent{
			Phase:           "fresh_index_fts",
			Event:           "done",
			FTSDurationMS:   ftsDurationMS,
			RowsWritten:     state.fresh.rowCounts(),
			ChunksFlushed:   state.fresh.chunkCounts(),
			DeferredFTSRows: 0,
		})
	}
	if state != nil && state.batchNew != nil {
		progress.emit(ProgressEvent{
			Phase:           "batch_new_fts",
			Event:           "start",
			DeferredFTSRows: state.batchNew.pendingFTSRows(),
		})
		phase := timing.start("batch_new_fts", "")
		ftsStarted := time.Now()
		if err := state.batchNew.flushDeferredFTS(); err != nil {
			phase.finish(state.batchNew.rowCounts(), statusTimingDetails(err))
			return nil, fmt.Errorf("flush deferred batch-new FTS: %w", err)
		}
		ftsDurationMS := time.Since(ftsStarted).Milliseconds()
		phase.finish(state.batchNew.rowCounts(), nil)
		progress.emit(ProgressEvent{
			Phase:           "batch_new_fts",
			Event:           "done",
			FTSDurationMS:   ftsDurationMS,
			RowsWritten:     state.batchNew.rowCounts(),
			ChunksFlushed:   state.batchNew.chunkCounts(),
			DeferredFTSRows: 0,
		})
	}
	if inTx {
		phase := timing.start("commit_transaction", "")
		if _, err := s.db.Exec("COMMIT"); err != nil {
			phase.finish(nil, statusTimingDetails(err))
			return nil, fmt.Errorf("commit scan transaction: %w", err)
		}
		phase.finish(nil, nil)
		inTx = false
	}
	progress.emit(ProgressEvent{
		Phase:                "scan",
		Event:                "done",
		CandidatesParsed:     cloneIntMap(parsedByAdapter),
		ArtifactsUpserted:    cloneIntMap(upsertedByAdapter),
		TransactionEnabled:   opts.UseTransaction,
		SkipAuthoredAtLookup: opts.SkipAuthoredAtLookup,
	})
	result.PhaseTiming = timing.finish()
	return result, nil
}

func (s *Scanner) recordScanMeta(repoID, repoRoot, now string) {
	commit := repo.HeadCommit(repoRoot)
	user := userident.Detect(repoRoot)
	s.db.UpdateScanMeta(repoID, commit, user, now)
}

type fileInventoryEntry struct {
	primaryPath string
	relPath     string
	size        int64
}

type fileInventoryResult struct {
	files      []fileInventoryEntry
	diagnostic TraversalDiagnostics
}

type fileCandidateResult struct {
	index     int
	byAdapter map[string][]adapters.Candidate
}

type progressReporter struct {
	started  time.Time
	next     time.Time
	interval time.Duration
	emitFn   func(ProgressEvent)
}

func newProgressReporter(emit func(ProgressEvent), interval time.Duration) *progressReporter {
	if emit == nil {
		return &progressReporter{}
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	now := time.Now()
	return &progressReporter{
		started:  now,
		next:     now.Add(interval),
		interval: interval,
		emitFn:   emit,
	}
}

func (p *progressReporter) emit(event ProgressEvent) {
	if p == nil || p.emitFn == nil {
		return
	}
	event.ElapsedMS = time.Since(p.started).Milliseconds()
	p.emitFn(event)
	p.next = time.Now().Add(p.interval)
}

func (p *progressReporter) maybe(event ProgressEvent) {
	if p == nil || p.emitFn == nil || time.Now().Before(p.next) {
		return
	}
	p.emit(event)
}

type scanPhaseRecorder struct {
	enabled bool
	started time.Time
	rows    []PhaseTimingRow
}

type activeScanPhase struct {
	rec     *scanPhaseRecorder
	name    string
	adapter string
	started time.Time
}

func newScanPhaseRecorder(enabled bool) *scanPhaseRecorder {
	return &scanPhaseRecorder{
		enabled: enabled,
		started: time.Now(),
	}
}

func (r *scanPhaseRecorder) start(name, adapter string) *activeScanPhase {
	if r == nil || !r.enabled {
		return nil
	}
	return &activeScanPhase{
		rec:     r,
		name:    name,
		adapter: adapter,
		started: time.Now(),
	}
}

func (p *activeScanPhase) finish(counts map[string]int, details map[string]string) {
	if p == nil || p.rec == nil || !p.rec.enabled {
		return
	}
	p.rec.rows = append(p.rec.rows, PhaseTimingRow{
		Name:       p.name,
		Adapter:    p.adapter,
		DurationMS: time.Since(p.started).Milliseconds(),
		Counts:     cloneIntMap(counts),
		Details:    cloneStringMap(details),
	})
}

func (r *scanPhaseRecorder) finish() *PhaseTimingDiagnostics {
	if r == nil || !r.enabled {
		return nil
	}
	return &PhaseTimingDiagnostics{
		Enabled: true,
		TotalMS: time.Since(r.started).Milliseconds(),
		Phases:  append([]PhaseTimingRow(nil), r.rows...),
	}
}

func candidateTimingCounts(candidates map[string][]adapters.Candidate, traversal *TraversalDiagnostics) map[string]int {
	counts := map[string]int{}
	total := 0
	for name, rows := range candidates {
		counts[name] = len(rows)
		total += len(rows)
	}
	if total > 0 {
		counts["total_candidates"] = total
	}
	if traversal != nil {
		counts["inventory_files"] = traversal.InventoryFiles
		counts["skipped_dirs"] = traversal.SkippedDirs
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func statusTimingDetails(err error) map[string]string {
	if err == nil {
		return nil
	}
	return map[string]string{"status": "error"}
}

func (s *Scanner) discoverSharedFileCandidates(ctx context.Context, repoRoot string, cfg *config.RepoConfig, opts RunOptions, progress *progressReporter) (map[string][]adapters.Candidate, *TraversalDiagnostics, error) {
	fileAdapters := make([]adapters.FileDiscoveryAdapter, 0)
	for _, adapter := range s.adapters {
		if fileAdapter, ok := adapter.(adapters.FileDiscoveryAdapter); ok {
			fileAdapters = append(fileAdapters, fileAdapter)
		}
	}
	out := make(map[string][]adapters.Candidate, len(fileAdapters))
	if len(fileAdapters) == 0 {
		return out, nil, nil
	}
	inventoryResult, err := collectFileInventory(ctx, repoRoot)
	if err != nil {
		return nil, nil, err
	}
	inventory := inventoryResult.files
	traversal := inventoryResult.diagnostics()
	progress.emit(ProgressEvent{
		Phase:           "shared_discovery",
		Event:           "inventory_done",
		InventoryFiles:  len(inventory),
		FilesTotal:      len(inventory),
		SkippedDirs:     inventoryResult.diagnostic.SkippedDirs,
		SkippedByReason: cloneIntMap(inventoryResult.diagnostic.SkippedByReason),
		SharedDiscovery: true,
		CappedDiscovery: hasCandidateLimits(fileAdapters, opts),
	})
	if hasCandidateLimits(fileAdapters, opts) || opts.FileWorkerCount == 1 {
		candidates, err := discoverSharedFileCandidatesSequential(ctx, repoRoot, cfg, inventory, fileAdapters, opts, progress)
		return candidates, traversal, err
	}
	candidates, err := discoverSharedFileCandidatesParallel(ctx, repoRoot, cfg, inventory, fileAdapters, opts, progress)
	return candidates, traversal, err
}

func collectFileInventory(ctx context.Context, repoRoot string) (fileInventoryResult, error) {
	var result fileInventoryResult
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if m := ignore.FromContext(ctx); m != nil && m.ShouldSkip(rel, d.IsDir()) {
			if d.IsDir() {
				result.recordSkip(rel, "ignore_rules")
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if isSharedFileIgnoredDir(d.Name()) {
				result.recordSkip(rel, "generated_vendor_or_build")
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.IsDir() {
			return nil
		}
		result.files = append(result.files, fileInventoryEntry{
			primaryPath: path,
			relPath:     rel,
			size:        info.Size(),
		})
		return nil
	})
	sort.Slice(result.files, func(i, j int) bool { return result.files[i].relPath < result.files[j].relPath })
	return result, err
}

func discoverSharedFileCandidatesSequential(ctx context.Context, repoRoot string, cfg *config.RepoConfig, inventory []fileInventoryEntry, fileAdapters []adapters.FileDiscoveryAdapter, opts RunOptions, progress *progressReporter) (map[string][]adapters.Candidate, error) {
	out := make(map[string][]adapters.Candidate, len(fileAdapters))
	counts := map[string]int{}
	capped := hasCandidateLimits(fileAdapters, opts)
	for i, file := range inventory {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var active []adapters.FileDiscoveryAdapter
		for _, adapter := range fileAdapters {
			name := adapter.Name()
			limit := candidateLimit(name, opts)
			if limit > 0 && counts[name] >= limit {
				continue
			}
			if adapter.AcceptsFile(file.relPath, file.size, cfg) {
				active = append(active, adapter)
			}
		}
		if len(active) == 0 {
			continue
		}
		body, err := os.ReadFile(file.primaryPath)
		if err != nil {
			continue
		}
		fc := adapters.FileCandidate{
			RepoRoot:    repoRoot,
			PrimaryPath: file.primaryPath,
			RelPath:     file.relPath,
			Size:        file.size,
			Body:        body,
		}
		for _, adapter := range active {
			name := adapter.Name()
			candidates, err := adapter.DiscoverFile(ctx, fc, cfg)
			if err != nil {
				continue
			}
			if limit := candidateLimit(name, opts); limit > 0 && counts[name]+len(candidates) > limit {
				candidates = candidates[:limit-counts[name]]
			}
			out[name] = append(out[name], candidates...)
			counts[name] += len(candidates)
		}
		progress.maybe(ProgressEvent{
			Phase:                "shared_discovery",
			FilesTotal:           len(inventory),
			FilesScanned:         i + 1,
			CandidatesDiscovered: cloneIntMap(counts),
			SharedDiscovery:      true,
			CappedDiscovery:      capped,
		})
	}
	for name := range out {
		sortCandidates(out[name])
	}
	progress.emit(ProgressEvent{
		Phase:                "shared_discovery",
		Event:                "done",
		FilesTotal:           len(inventory),
		FilesScanned:         len(inventory),
		CandidatesDiscovered: cloneIntMap(counts),
		SharedDiscovery:      true,
		CappedDiscovery:      capped,
	})
	return out, nil
}

func discoverSharedFileCandidatesParallel(ctx context.Context, repoRoot string, cfg *config.RepoConfig, inventory []fileInventoryEntry, fileAdapters []adapters.FileDiscoveryAdapter, opts RunOptions, progress *progressReporter) (map[string][]adapters.Candidate, error) {
	workers := opts.FileWorkerCount
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	results := make(chan fileCandidateResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				file := inventory[index]
				result := fileCandidateResult{index: index, byAdapter: map[string][]adapters.Candidate{}}
				var active []adapters.FileDiscoveryAdapter
				for _, adapter := range fileAdapters {
					if adapter.AcceptsFile(file.relPath, file.size, cfg) {
						active = append(active, adapter)
					}
				}
				if len(active) > 0 {
					if body, err := os.ReadFile(file.primaryPath); err == nil {
						fc := adapters.FileCandidate{
							RepoRoot:    repoRoot,
							PrimaryPath: file.primaryPath,
							RelPath:     file.relPath,
							Size:        file.size,
							Body:        body,
						}
						for _, adapter := range active {
							if candidates, err := adapter.DiscoverFile(ctx, fc, cfg); err == nil && len(candidates) > 0 {
								result.byAdapter[adapter.Name()] = candidates
							}
						}
					}
				}
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range inventory {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	byIndex := make([]fileCandidateResult, 0, len(inventory))
	counts := map[string]int{}
	for result := range results {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		byIndex = append(byIndex, result)
		for name, candidates := range result.byAdapter {
			counts[name] += len(candidates)
		}
		progress.maybe(ProgressEvent{
			Phase:                "shared_discovery",
			FilesTotal:           len(inventory),
			FilesScanned:         len(byIndex),
			CandidatesDiscovered: cloneIntMap(counts),
			SharedDiscovery:      true,
			ParallelWorkers:      workers,
		})
	}
	sort.Slice(byIndex, func(i, j int) bool { return byIndex[i].index < byIndex[j].index })
	out := make(map[string][]adapters.Candidate, len(fileAdapters))
	for _, result := range byIndex {
		for _, adapter := range fileAdapters {
			out[adapter.Name()] = append(out[adapter.Name()], result.byAdapter[adapter.Name()]...)
		}
	}
	for name := range out {
		sortCandidates(out[name])
	}
	progress.emit(ProgressEvent{
		Phase:                "shared_discovery",
		Event:                "done",
		FilesTotal:           len(inventory),
		FilesScanned:         len(inventory),
		CandidatesDiscovered: cloneIntMap(counts),
		SharedDiscovery:      true,
		ParallelWorkers:      workers,
	})
	return out, nil
}

func sortCandidates(candidates []adapters.Candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].RelPath == candidates[j].RelPath {
			if candidates[i].UnitStartLine == candidates[j].UnitStartLine {
				return candidates[i].UnitName < candidates[j].UnitName
			}
			return candidates[i].UnitStartLine < candidates[j].UnitStartLine
		}
		return candidates[i].RelPath < candidates[j].RelPath
	})
}

type parsedFreshCandidate struct {
	index                int
	ok                   bool
	artifact             adapters.Artifact
	sources              []adapters.Source
	parseResult          todoparse.ParseResult
	parseDurationMS      int64
	classifierDurationMS int64
}

func parseCandidatesForFreshIndex(ctx context.Context, repoRoot string, adapter adapters.Adapter, candidates []adapters.Candidate, opts RunOptions, progress *progressReporter) ([]parsedFreshCandidate, int, error) {
	workers := opts.FileWorkerCount
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers < 1 {
		workers = 1
	}
	if workers == 1 || len(candidates) < 2 {
		out := make([]parsedFreshCandidate, len(candidates))
		parsed := 0
		var parseDurationMS int64
		var classifierDurationMS int64
		for i, c := range candidates {
			if err := ctx.Err(); err != nil {
				return nil, parsed, err
			}
			parseStarted := time.Now()
			art, sources, pr, err := adapter.Parse(ctx, c)
			parseMS := time.Since(parseStarted).Milliseconds()
			if err != nil {
				continue
			}
			classifierStarted := time.Now()
			art = attachClassifierMetadata(repoRoot, c, art)
			classifierMS := time.Since(classifierStarted).Milliseconds()
			parseDurationMS += parseMS
			classifierDurationMS += classifierMS
			parsed++
			out[i] = parsedFreshCandidate{
				index:                i,
				ok:                   true,
				artifact:             art,
				sources:              sources,
				parseResult:          pr,
				parseDurationMS:      parseMS,
				classifierDurationMS: classifierMS,
			}
			progress.maybe(ProgressEvent{
				Phase:                "extract",
				CurrentAdapter:       adapter.Name(),
				CandidatesTotal:      len(candidates),
				CandidatesProcessed:  i + 1,
				CandidatesParsed:     map[string]int{adapter.Name(): parsed},
				ParseDurationMS:      parseDurationMS,
				ClassifierDurationMS: classifierDurationMS,
				ParallelWorkers:      workers,
			})
		}
		progress.emit(ProgressEvent{
			Phase:                "extract",
			Event:                "adapter_done",
			CurrentAdapter:       adapter.Name(),
			CandidatesTotal:      len(candidates),
			CandidatesProcessed:  len(candidates),
			CandidatesParsed:     map[string]int{adapter.Name(): parsed},
			ParseDurationMS:      parseDurationMS,
			ClassifierDurationMS: classifierDurationMS,
			ParallelWorkers:      workers,
		})
		return out, parsed, nil
	}

	jobs := make(chan int)
	results := make(chan parsedFreshCandidate, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				c := candidates[index]
				parseStarted := time.Now()
				art, sources, pr, err := adapter.Parse(ctx, c)
				parseMS := time.Since(parseStarted).Milliseconds()
				if err == nil {
					classifierStarted := time.Now()
					art = attachClassifierMetadata(repoRoot, c, art)
					classifierMS := time.Since(classifierStarted).Milliseconds()
					select {
					case results <- parsedFreshCandidate{
						index:                index,
						ok:                   true,
						artifact:             art,
						sources:              sources,
						parseResult:          pr,
						parseDurationMS:      parseMS,
						classifierDurationMS: classifierMS,
					}:
					case <-ctx.Done():
						return
					}
					continue
				}
				select {
				case results <- parsedFreshCandidate{index: index}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range candidates {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]parsedFreshCandidate, len(candidates))
	processed := 0
	parsed := 0
	var parseDurationMS int64
	var classifierDurationMS int64
	for result := range results {
		if err := ctx.Err(); err != nil {
			return nil, parsed, err
		}
		processed++
		if result.ok {
			parsed++
			parseDurationMS += result.parseDurationMS
			classifierDurationMS += result.classifierDurationMS
			out[result.index] = result
		}
		progress.maybe(ProgressEvent{
			Phase:                "extract",
			CurrentAdapter:       adapter.Name(),
			CandidatesTotal:      len(candidates),
			CandidatesProcessed:  processed,
			CandidatesParsed:     map[string]int{adapter.Name(): parsed},
			ParseDurationMS:      parseDurationMS,
			ClassifierDurationMS: classifierDurationMS,
			ParallelWorkers:      workers,
		})
	}
	progress.emit(ProgressEvent{
		Phase:                "extract",
		Event:                "adapter_done",
		CurrentAdapter:       adapter.Name(),
		CandidatesTotal:      len(candidates),
		CandidatesProcessed:  processed,
		CandidatesParsed:     map[string]int{adapter.Name(): parsed},
		ParseDurationMS:      parseDurationMS,
		ClassifierDurationMS: classifierDurationMS,
		ParallelWorkers:      workers,
	})
	return out, parsed, nil
}

func hasCandidateLimits(fileAdapters []adapters.FileDiscoveryAdapter, opts RunOptions) bool {
	for _, adapter := range fileAdapters {
		if candidateLimit(adapter.Name(), opts) > 0 {
			return true
		}
	}
	return false
}

func candidateLimit(adapterName string, opts RunOptions) int {
	if opts.MaxCandidatesByAdapter == nil {
		return 0
	}
	return opts.MaxCandidatesByAdapter[adapterName]
}

func cloneIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func isSharedFileIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".cache", ".git", ".devspecs", ".next", ".pytest_cache", ".turbo", ".venv",
		"__pycache__", "build", "coverage", "dist", "generated", "node_modules",
		"out", "target", "tmp", "vendor":
		return true
	default:
		return false
	}
}

func (r *fileInventoryResult) recordSkip(rel, reason string) {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "skipped"
	}
	r.diagnostic.SkippedDirs++
	if r.diagnostic.SkippedByReason == nil {
		r.diagnostic.SkippedByReason = map[string]int{}
	}
	r.diagnostic.SkippedByReason[reason]++
	if len(r.diagnostic.TopSkippedDirs) < 8 {
		r.diagnostic.TopSkippedDirs = append(r.diagnostic.TopSkippedDirs, TraversalSkippedPath{
			Path:   rel,
			Reason: reason,
		})
	}
}

func (r fileInventoryResult) diagnostics() *TraversalDiagnostics {
	diag := r.diagnostic
	diag.InventoryFiles = len(r.files)
	if diag.InventoryFiles == 0 && diag.SkippedDirs == 0 {
		return nil
	}
	if len(diag.SkippedByReason) == 0 {
		diag.SkippedByReason = nil
	}
	if len(diag.TopSkippedDirs) == 0 {
		diag.TopSkippedDirs = nil
	}
	return &diag
}

func (s *Scanner) ensureRepo(rootPath, now string) (string, error) {
	var id string
	err := s.db.QueryRow("SELECT id FROM repos WHERE root_path = ?", rootPath).Scan(&id)
	if err == nil {
		s.db.Exec("UPDATE repos SET updated_at = ? WHERE id = ?", now, id)
		return id, nil
	}
	id = s.ids.NewWithPrefix("repo_")
	_, err = s.db.Exec(
		"INSERT INTO repos (id, root_path, created_at, updated_at) VALUES (?, ?, ?, ?)",
		id, rootPath, now, now,
	)
	return id, err
}

type scanRunState struct {
	fresh        *freshInserter
	batchNew     *freshInserter
	shortIDs     map[string]int
	authoredAt   map[string]string
	authoredAtMu sync.Mutex
}

func (s *Scanner) seedExistingShortIDClaims() (map[string]int, error) {
	claims := map[string]int{}
	rows, err := s.db.Query("SELECT COALESCE(short_id, '') FROM artifacts WHERE COALESCE(short_id, '') <> ''")
	if err != nil {
		return claims, err
	}
	defer rows.Close()
	for rows.Next() {
		var shortID string
		if err := rows.Scan(&shortID); err != nil {
			return claims, err
		}
		base, next := shortIDClaim(shortID)
		if base == "" {
			continue
		}
		if claims[base] < next {
			claims[base] = next
		}
	}
	return claims, rows.Err()
}

func shortIDClaim(shortID string) (string, int) {
	shortID = strings.TrimSpace(shortID)
	if shortID == "" {
		return "", 0
	}
	if len(shortID) >= 8 && isShortIDHexPrefix(shortID[:8]) {
		base := shortID[:8]
		suffix := shortID[8:]
		if suffix == "" {
			return base, 1
		}
		if allASCIIDigits(suffix) {
			n, err := strconv.Atoi(suffix)
			if err == nil {
				return base, n + 1
			}
		}
	}
	return shortID, 1
}

func isShortIDHexPrefix(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func allASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type existingArtifactRow struct {
	artifactID     string
	currentRevID   string
	sourceIdentity string
}

func parsedSourceIdentityCounts(parsed []parsedFreshCandidate) map[string]int {
	counts := map[string]int{}
	for _, item := range parsed {
		if !item.ok {
			continue
		}
		counts[item.artifact.SourceIdentity]++
	}
	return counts
}

func sourceIdentityKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for identity := range counts {
		keys = append(keys, identity)
	}
	sort.Strings(keys)
	return keys
}

const existingArtifactLookupChunkSize = 500

func (s *Scanner) existingArtifactsBySourceIdentity(identities []string) (map[string]existingArtifactRow, error) {
	out := map[string]existingArtifactRow{}
	if len(identities) == 0 {
		return out, nil
	}
	for start := 0; start < len(identities); start += existingArtifactLookupChunkSize {
		end := start + existingArtifactLookupChunkSize
		if end > len(identities) {
			end = len(identities)
		}
		chunk := identities[start:end]
		var b strings.Builder
		b.WriteString("SELECT s.source_identity, a.id, COALESCE(a.current_revision_id, '') FROM sources s JOIN artifacts a ON a.id = s.artifact_id WHERE s.source_identity IN (")
		args := make([]any, 0, len(chunk))
		for i, identity := range chunk {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('?')
			args = append(args, identity)
		}
		b.WriteString(") ORDER BY s.source_identity, s.rowid")
		rows, err := s.db.Query(b.String(), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var row existingArtifactRow
			if err := rows.Scan(&row.sourceIdentity, &row.artifactID, &row.currentRevID); err != nil {
				rows.Close()
				return nil, err
			}
			if _, exists := out[row.sourceIdentity]; !exists {
				out[row.sourceIdentity] = row
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

func (s *Scanner) upsertArtifact(repoRoot, repoID, adapterName string, art adapters.Artifact, sources []adapters.Source, pr todoparse.ParseResult, now string, result *Result, opts RunOptions, state *scanRunState) error {
	if opts.FreshIndex {
		return s.insertFreshArtifact(repoRoot, repoID, adapterName, art, sources, pr, now, result, opts, state)
	}
	// Check if artifact exists by source_identity
	var artifactID, currentRevID string
	err := s.db.QueryRow(
		"SELECT a.id, COALESCE(a.current_revision_id, '') FROM artifacts a JOIN sources s ON s.artifact_id = a.id WHERE s.source_identity = ?",
		art.SourceIdentity,
	).Scan(&artifactID, &currentRevID)

	contentHash := hashContent(art.Body)

	if err != nil {
		// New artifact
		artifactID = s.ids.New()
		revID := s.ids.NewWithPrefix("rev_")
		if err := s.insertArtifact(artifactID, repoRoot, repoID, art, sources, revID, now, opts.SkipAuthoredAtLookup, state); err != nil {
			return err
		}
		s.assignShortID(artifactID, art.SourceIdentity, state)
		if err := s.insertRevision(revID, artifactID, contentHash, art.Body, art.Extracted, now); err != nil {
			return err
		}
		for _, src := range sources {
			if err := s.insertSource(artifactID, repoID, src, now); err != nil {
				return err
			}
		}
		sections, err := s.indexSections(artifactID, revID, art, sources, now)
		if err != nil {
			return err
		}
		if err := s.replaceTodos(artifactID, revID, pr.Todos, now, sections); err != nil {
			return err
		}
		if err := s.replaceCriteria(artifactID, revID, pr.Criteria, now, sections); err != nil {
			return err
		}
		s.replaceTags(artifactID, art, now)
		s.indexFTS(artifactID, art)
		result.New++
		tallyIndexed(result, adapterName, sources, art)
		return nil
	}

	// Refresh last_observed_at only; updated_at moves on new revision or capture/status updates.
	s.db.Exec("UPDATE artifacts SET last_observed_at = ? WHERE id = ?", now, artifactID)

	// Ensure short_id is set (covers artifacts created before v0.1)
	s.assignShortID(artifactID, art.SourceIdentity, state)

	if err := s.syncSources(artifactID, repoID, sources, now); err != nil {
		return err
	}

	// Check if content changed
	var existingHash string
	if currentRevID != "" {
		s.db.QueryRow("SELECT content_hash FROM artifact_revisions WHERE id = ?", currentRevID).Scan(&existingHash)
	}

	if existingHash == contentHash {
		// Body hash unchanged: keep existing revision row (and extracted_json) until
		// file content changes — CLI logic-only enrichments won't rewrite revisions alone.
		if err := s.ensureSectionsIndexed(artifactID, currentRevID, art, sources, now); err != nil {
			return err
		}
		s.replaceTags(artifactID, art, now)
		result.Unchanged++
		tallyIndexed(result, adapterName, sources, art)
		return nil
	}

	// New revision
	revID := s.ids.NewWithPrefix("rev_")
	if err := s.insertRevision(revID, artifactID, contentHash, art.Body, art.Extracted, now); err != nil {
		return err
	}
	s.db.Exec("UPDATE artifacts SET current_revision_id = ?, title = ?, status = ?, kind = ?, subtype = ?, updated_at = ? WHERE id = ?",
		revID, art.Title, art.Status, art.Kind, art.Subtype, now, artifactID)
	sections, err := s.indexSections(artifactID, revID, art, sources, now)
	if err != nil {
		return err
	}
	if err := s.replaceTodos(artifactID, revID, pr.Todos, now, sections); err != nil {
		return err
	}
	if err := s.replaceCriteria(artifactID, revID, pr.Criteria, now, sections); err != nil {
		return err
	}
	s.replaceTags(artifactID, art, now)
	s.indexFTS(artifactID, art)
	result.Updated++
	tallyIndexed(result, adapterName, sources, art)
	return nil
}

func (s *Scanner) indexFTS(artifactID string, art adapters.Artifact) {
	sourcePath := ""
	if art.PrimaryPath != "" {
		sourcePath = art.PrimaryPath
	}
	s.db.IndexArtifactFTS(artifactID, art.Title, art.Body, sourcePath)
}

const freshInsertChunkSize = 75

type freshInserter struct {
	db        *store.DB
	chunkSize int

	artifacts []freshArtifactRow
	revisions []freshRevisionRow
	sources   []freshSourceRow
	todos     []freshTodoRow
	criteria  []freshCriterionRow
	tags      []freshTagRow
	fts       []freshFTSRow

	rows   map[string]int
	chunks map[string]int
}

type freshArtifactRow struct {
	id         string
	repoID     string
	shortID    string
	kind       string
	subtype    string
	title      string
	status     string
	revID      string
	now        string
	authoredAt string
}

type freshRevisionRow struct {
	id            string
	artifactID    string
	contentHash   string
	body          string
	extractedJSON any
	now           string
}

type freshSourceRow struct {
	id             string
	artifactID     string
	repoID         string
	sourceType     string
	path           string
	sourceIdentity string
	formatProfile  string
	layoutGroup    any
	now            string
}

type freshTodoRow struct {
	id         string
	artifactID string
	revID      string
	sectionID  string
	ordinal    int
	text       string
	done       int
	sourceFile string
	sourceLine int
	now        string
}

type freshCriterionRow struct {
	id           string
	artifactID   string
	revID        string
	sectionID    string
	ordinal      int
	text         string
	done         int
	sourceFile   string
	sourceLine   int
	criteriaKind string
	now          string
}

type freshTagRow struct {
	artifactID string
	tag        string
	source     string
	now        string
}

type freshFTSRow struct {
	artifactID string
	title      string
	body       string
	sourcePath string
}

func newFreshInserter(db *store.DB) (*freshInserter, error) {
	return &freshInserter{
		db:        db,
		chunkSize: freshInsertChunkSize,
		rows:      map[string]int{},
		chunks:    map[string]int{},
	}, nil
}

func (f *freshInserter) close() {}

func (s *Scanner) insertFreshArtifact(repoRoot, repoID, adapterName string, art adapters.Artifact, sources []adapters.Source, pr todoparse.ParseResult, now string, result *Result, opts RunOptions, state *scanRunState) error {
	if state == nil || state.fresh == nil {
		return fmt.Errorf("fresh index inserter is not initialized")
	}
	return s.insertBufferedNewArtifact(state.fresh, repoRoot, repoID, adapterName, art, sources, pr, now, result, opts, state)
}

func (s *Scanner) insertBatchedNewArtifact(repoRoot, repoID, adapterName string, art adapters.Artifact, sources []adapters.Source, pr todoparse.ParseResult, now string, result *Result, opts RunOptions, state *scanRunState) error {
	if state == nil {
		return fmt.Errorf("batch-new inserter state is not initialized")
	}
	if state.batchNew == nil {
		if state.shortIDs == nil {
			claims, err := s.seedExistingShortIDClaims()
			if err != nil {
				return fmt.Errorf("seed batch-new short ids: %w", err)
			}
			state.shortIDs = claims
		}
		inserter, err := newFreshInserter(s.db)
		if err != nil {
			return fmt.Errorf("prepare batch-new inserts: %w", err)
		}
		state.batchNew = inserter
	}
	return s.insertBufferedNewArtifact(state.batchNew, repoRoot, repoID, adapterName, art, sources, pr, now, result, opts, state)
}

func (s *Scanner) insertBufferedNewArtifact(inserter *freshInserter, repoRoot, repoID, adapterName string, art adapters.Artifact, sources []adapters.Source, pr todoparse.ParseResult, now string, result *Result, opts RunOptions, state *scanRunState) error {
	if inserter == nil {
		return fmt.Errorf("buffered artifact inserter is not initialized")
	}
	artifactID := s.ids.New()
	revID := s.ids.NewWithPrefix("rev_")
	contentHash := hashContent(art.Body)
	shortID := state.claimShortID(art.SourceIdentity)
	authoredAt := now
	if !opts.SkipAuthoredAtLookup {
		authoredAt = state.resolveAuthoredAt(repoRoot, art, sources, now)
	}
	if err := inserter.insertArtifact(artifactID, repoID, shortID, art, revID, now, authoredAt); err != nil {
		return err
	}
	if err := inserter.insertRevision(revID, artifactID, contentHash, art.Body, art.Extracted, now); err != nil {
		return err
	}
	for _, src := range sources {
		if err := inserter.insertSource(s.ids.NewWithPrefix("src_"), artifactID, repoID, src, now); err != nil {
			return err
		}
	}
	var sections []docsections.Section
	if isMarkdownSectionSource(art, sources) {
		if err := inserter.flushRows(); err != nil {
			return err
		}
		var err error
		sections, err = s.indexSections(artifactID, revID, art, sources, now)
		if err != nil {
			return err
		}
	}
	for _, todo := range pr.Todos {
		if err := inserter.insertTodo(s.ids.NewWithPrefix("todo_"), artifactID, revID, todo, now, sections); err != nil {
			return err
		}
	}
	for _, criterion := range pr.Criteria {
		if err := inserter.insertCriterion(s.ids.NewWithPrefix("crit_"), artifactID, revID, criterion, now, sections); err != nil {
			return err
		}
	}
	if err := inserter.insertTags(artifactID, art, sources, now); err != nil {
		return err
	}
	if err := inserter.deferFTS(artifactID, art); err != nil {
		return err
	}
	if err := inserter.flushRowsIfFull(); err != nil {
		return err
	}
	result.New++
	tallyIndexed(result, adapterName, sources, art)
	return nil
}

func (s *scanRunState) claimShortID(sourceIdentity string) string {
	base := idgen.ShortID(sourceIdentity)
	if base == "" {
		base = "artifact"
	}
	if s.shortIDs == nil {
		s.shortIDs = map[string]int{}
	}
	count := s.shortIDs[base]
	s.shortIDs[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s%d", base, count)
}

const maxAuthoredAtLookupWorkers = 8
const minBulkAuthoredAtPaths = 16

func benefitsFromAuthoredAtPrefetch(adapterName string) bool {
	switch adapterName {
	case "test_case", "code_comment":
		return true
	default:
		return false
	}
}

func (s *scanRunState) prefetchAuthoredAt(ctx context.Context, repoRoot string, candidates []adapters.Candidate, now string, opts RunOptions) int {
	if s == nil || opts.SkipAuthoredAtLookup || len(candidates) == 0 {
		return 0
	}
	rels := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		rel := authoredAtCandidateRelPath(repoRoot, candidate)
		if rel == "" || seen[rel] || s.cachedAuthoredAt(rel) {
			continue
		}
		seen[rel] = true
		rels = append(rels, rel)
	}
	if len(rels) == 0 {
		return 0
	}
	sort.Strings(rels)
	if len(rels) >= minBulkAuthoredAtPaths {
		for rel, authoredAt := range fileFirstCommitDates(repoRoot, rels) {
			if authoredAt == "" {
				continue
			}
			s.storeAuthoredAt(rel, authoredAt)
		}
		rels = filterUncachedAuthoredAtRels(s, rels)
		if len(rels) == 0 {
			return len(seen)
		}
	}
	workers := opts.FileWorkerCount
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > maxAuthoredAtLookupWorkers {
		workers = maxAuthoredAtLookupWorkers
	}
	if workers > len(rels) {
		workers = len(rels)
	}
	if workers < 1 {
		workers = 1
	}
	if workers == 1 || len(rels) == 1 {
		for _, rel := range rels {
			if ctx.Err() != nil {
				break
			}
			s.storeAuthoredAt(rel, firstCommitDateOrNow(repoRoot, rel, now))
		}
		return len(rels)
	}

	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range jobs {
				if ctx.Err() != nil {
					continue
				}
				s.storeAuthoredAt(rel, firstCommitDateOrNow(repoRoot, rel, now))
			}
		}()
	}
	for _, rel := range rels {
		if ctx.Err() != nil {
			break
		}
		jobs <- rel
	}
	close(jobs)
	wg.Wait()
	return len(seen)
}

func filterUncachedAuthoredAtRels(state *scanRunState, rels []string) []string {
	out := rels[:0]
	for _, rel := range rels {
		if state.cachedAuthoredAt(rel) {
			continue
		}
		out = append(out, rel)
	}
	return out
}

func authoredAtCandidateRelPath(repoRoot string, candidate adapters.Candidate) string {
	if candidate.PrimaryPath != "" {
		if rel, err := filepath.Rel(repoRoot, candidate.PrimaryPath); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(candidate.RelPath)
}

func (s *scanRunState) cachedAuthoredAt(rel string) bool {
	s.authoredAtMu.Lock()
	defer s.authoredAtMu.Unlock()
	_, ok := s.authoredAt[rel]
	return ok
}

func (s *scanRunState) storeAuthoredAt(rel, authoredAt string) {
	s.authoredAtMu.Lock()
	defer s.authoredAtMu.Unlock()
	if s.authoredAt == nil {
		s.authoredAt = map[string]string{}
	}
	s.authoredAt[rel] = authoredAt
}

func (f *freshInserter) insertArtifact(id, repoID, shortID string, art adapters.Artifact, revID, now, authoredAt string) error {
	f.artifacts = append(f.artifacts, freshArtifactRow{
		id:         id,
		repoID:     repoID,
		shortID:    shortID,
		kind:       art.Kind,
		subtype:    art.Subtype,
		title:      art.Title,
		status:     art.Status,
		revID:      revID,
		now:        now,
		authoredAt: authoredAt,
	})
	return nil
}

func (f *freshInserter) insertRevision(id, artifactID, contentHash, body string, extracted map[string]any, now string) error {
	var extractedArg any
	if len(extracted) > 0 {
		b, err := json.Marshal(extracted)
		if err != nil {
			return fmt.Errorf("marshal extracted: %w", err)
		}
		extractedArg = string(b)
	}
	f.revisions = append(f.revisions, freshRevisionRow{
		id:            id,
		artifactID:    artifactID,
		contentHash:   contentHash,
		body:          body,
		extractedJSON: extractedArg,
		now:           now,
	})
	return nil
}

func (f *freshInserter) insertSource(id, artifactID, repoID string, src adapters.Source, now string) error {
	fp := src.FormatProfile
	if fp == "" {
		fp = format.ProfileGeneric
	}
	var layoutArg any
	if src.LayoutGroup != "" {
		layoutArg = src.LayoutGroup
	}
	f.sources = append(f.sources, freshSourceRow{
		id:             id,
		artifactID:     artifactID,
		repoID:         repoID,
		sourceType:     src.SourceType,
		path:           src.Path,
		sourceIdentity: src.SourceIdentity,
		formatProfile:  fp,
		layoutGroup:    layoutArg,
		now:            now,
	})
	return nil
}

func (f *freshInserter) insertTodo(id, artifactID, revID string, todo todoparse.Todo, now string, sections []docsections.Section) error {
	done := 0
	if todo.Done {
		done = 1
	}
	sectionID := docsections.EnclosingSectionID(sections, todo.SourceLine)
	f.todos = append(f.todos, freshTodoRow{
		id:         id,
		artifactID: artifactID,
		revID:      revID,
		sectionID:  sectionID,
		ordinal:    todo.Ordinal,
		text:       todo.Text,
		done:       done,
		sourceFile: todo.SourceFile,
		sourceLine: todo.SourceLine,
		now:        now,
	})
	return nil
}

func (f *freshInserter) insertCriterion(id, artifactID, revID string, criterion todoparse.Criterion, now string, sections []docsections.Section) error {
	done := 0
	if criterion.Done {
		done = 1
	}
	sectionID := docsections.EnclosingSectionID(sections, criterion.SourceLine)
	f.criteria = append(f.criteria, freshCriterionRow{
		id:           id,
		artifactID:   artifactID,
		revID:        revID,
		sectionID:    sectionID,
		ordinal:      criterion.Ordinal,
		text:         criterion.Text,
		done:         done,
		sourceFile:   criterion.SourceFile,
		sourceLine:   criterion.SourceLine,
		criteriaKind: criterion.CriteriaKind,
		now:          now,
	})
	return nil
}

func (f *freshInserter) insertTags(artifactID string, art adapters.Artifact, sources []adapters.Source, now string) error {
	for _, tag := range art.Tags {
		f.tags = append(f.tags, freshTagRow{artifactID: artifactID, tag: tag, source: "frontmatter", now: now})
	}
	if len(art.Tags) == 0 {
		if dirTag := markdown.InferDirectoryTag(primarySectionSourcePath(art, sources)); dirTag != "" {
			f.tags = append(f.tags, freshTagRow{artifactID: artifactID, tag: dirTag, source: "inferred", now: now})
		}
	}
	return nil
}

func (f *freshInserter) deferFTS(artifactID string, art adapters.Artifact) error {
	sourcePath := ""
	if art.PrimaryPath != "" {
		sourcePath = art.PrimaryPath
	}
	f.fts = append(f.fts, freshFTSRow{artifactID: artifactID, title: art.Title, body: art.Body, sourcePath: sourcePath})
	return nil
}

func (f *freshInserter) flushRowsIfFull() error {
	if f == nil {
		return nil
	}
	if len(f.artifacts) >= f.chunkSize || len(f.revisions) >= f.chunkSize || len(f.sources) >= f.chunkSize ||
		len(f.todos) >= f.chunkSize || len(f.criteria) >= f.chunkSize || len(f.tags) >= f.chunkSize {
		return f.flushRows()
	}
	return nil
}

func (f *freshInserter) flushRows() error {
	if f == nil {
		return nil
	}
	if err := f.flushArtifacts(); err != nil {
		return err
	}
	if err := f.flushRevisions(); err != nil {
		return err
	}
	if err := f.flushSources(); err != nil {
		return err
	}
	if err := f.flushTodos(); err != nil {
		return err
	}
	if err := f.flushCriteria(); err != nil {
		return err
	}
	if err := f.flushTags(); err != nil {
		return err
	}
	return nil
}

func (f *freshInserter) flushDeferredFTS() error {
	if f == nil || len(f.fts) == 0 {
		return nil
	}
	rows := f.fts
	if err := f.execBatches("artifacts_fts",
		"INSERT INTO artifacts_fts (artifact_id, title, body, source_path) VALUES ",
		4,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.artifactID, row.title, row.body, row.sourcePath}
		},
	); err != nil {
		return err
	}
	f.fts = nil
	return nil
}

func (f *freshInserter) flushArtifacts() error {
	if len(f.artifacts) == 0 {
		return nil
	}
	rows := f.artifacts
	if err := f.execBatches("artifacts",
		"INSERT INTO artifacts (id, repo_id, short_id, kind, subtype, title, status, current_revision_id, created_at, updated_at, last_observed_at, authored_at) VALUES ",
		12,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.id, row.repoID, row.shortID, row.kind, row.subtype, row.title, row.status, row.revID, row.now, row.now, row.now, row.authoredAt}
		},
	); err != nil {
		return err
	}
	f.artifacts = nil
	return nil
}

func (f *freshInserter) flushRevisions() error {
	if len(f.revisions) == 0 {
		return nil
	}
	rows := f.revisions
	if err := f.execBatches("artifact_revisions",
		"INSERT INTO artifact_revisions (id, artifact_id, content_hash, body, extracted_json, observed_at) VALUES ",
		6,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.id, row.artifactID, row.contentHash, row.body, row.extractedJSON, row.now}
		},
	); err != nil {
		return err
	}
	f.revisions = nil
	return nil
}

func (f *freshInserter) flushSources() error {
	if len(f.sources) == 0 {
		return nil
	}
	rows := f.sources
	if err := f.execBatches("sources",
		"INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES ",
		10,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.id, row.artifactID, row.repoID, row.sourceType, row.path, row.sourceIdentity, row.formatProfile, row.layoutGroup, row.now, row.now}
		},
	); err != nil {
		return err
	}
	f.sources = nil
	return nil
}

func (f *freshInserter) flushTodos() error {
	if len(f.todos) == 0 {
		return nil
	}
	rows := f.todos
	if err := f.execBatches("artifact_todos",
		"INSERT INTO artifact_todos (id, artifact_id, revision_id, section_id, ordinal, text, done, source_file, source_line, created_at) VALUES ",
		10,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.id, row.artifactID, row.revID, row.sectionID, row.ordinal, row.text, row.done, row.sourceFile, row.sourceLine, row.now}
		},
	); err != nil {
		return err
	}
	f.todos = nil
	return nil
}

func (f *freshInserter) flushCriteria() error {
	if len(f.criteria) == 0 {
		return nil
	}
	rows := f.criteria
	if err := f.execBatches("artifact_criteria",
		"INSERT INTO artifact_criteria (id, artifact_id, revision_id, section_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES ",
		11,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.id, row.artifactID, row.revID, row.sectionID, row.ordinal, row.text, row.done, row.sourceFile, row.sourceLine, row.criteriaKind, row.now}
		},
	); err != nil {
		return err
	}
	f.criteria = nil
	return nil
}

func (f *freshInserter) flushTags() error {
	if len(f.tags) == 0 {
		return nil
	}
	rows := f.tags
	if err := f.execBatches("artifact_tags",
		"INSERT INTO artifact_tags (artifact_id, tag, source, created_at) VALUES ",
		4,
		len(rows),
		func(i int) []any {
			row := rows[i]
			return []any{row.artifactID, row.tag, row.source, row.now}
		},
	); err != nil {
		return err
	}
	f.tags = nil
	return nil
}

func (f *freshInserter) execBatches(table, prefix string, valuesPerRow, rowCount int, argsForRow func(int) []any) error {
	if rowCount == 0 {
		return nil
	}
	chunkSize := f.chunkSize
	if chunkSize <= 0 {
		chunkSize = freshInsertChunkSize
	}
	for start := 0; start < rowCount; start += chunkSize {
		end := start + chunkSize
		if end > rowCount {
			end = rowCount
		}
		var b strings.Builder
		b.WriteString(prefix)
		args := make([]any, 0, (end-start)*valuesPerRow)
		for i := start; i < end; i++ {
			if i > start {
				b.WriteString(", ")
			}
			b.WriteByte('(')
			for col := 0; col < valuesPerRow; col++ {
				if col > 0 {
					b.WriteString(", ")
				}
				b.WriteByte('?')
			}
			b.WriteByte(')')
			args = append(args, argsForRow(i)...)
		}
		if _, err := f.db.Exec(b.String(), args...); err != nil {
			return fmt.Errorf("insert %s rows %d-%d: %w", table, start, end, err)
		}
		f.rows[table] += end - start
		f.chunks[table]++
	}
	return nil
}

func (f *freshInserter) rowCounts() map[string]int {
	if f == nil {
		return nil
	}
	return cloneIntMap(f.rows)
}

func (f *freshInserter) chunkCounts() map[string]int {
	if f == nil {
		return nil
	}
	return cloneIntMap(f.chunks)
}

func (f *freshInserter) pendingFTSRows() int {
	if f == nil {
		return 0
	}
	return len(f.fts)
}

func (s *Scanner) ensureSectionsIndexed(artifactID, revID string, art adapters.Artifact, sources []adapters.Source, now string) error {
	if revID == "" || !isMarkdownSectionSource(art, sources) {
		return nil
	}
	existing, err := s.db.GetSectionsForArtifact(artifactID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	_, err = s.indexSections(artifactID, revID, art, sources, now)
	return err
}

func (s *Scanner) indexSections(artifactID, revID string, art adapters.Artifact, sources []adapters.Source, now string) ([]docsections.Section, error) {
	if revID == "" || !isMarkdownSectionSource(art, sources) {
		return nil, s.db.ReplaceArtifactSections(artifactID, revID, nil, now)
	}
	sourcePath := primarySectionSourcePath(art, sources)
	indexed := docsections.AssignStableIDs(docsections.ExtractMarkdown(art.Body), artifactID, revID, sourcePath)
	for i := range indexed {
		indexed[i].Kind = inferIndexedSectionKind(indexed[i])
		indexed[i].Metadata = map[string]string{
			"artifact_kind":    art.Kind,
			"artifact_subtype": art.Subtype,
			"artifact_status":  art.Status,
		}
		if role, ok := art.Extracted["openspec_role"]; ok && strings.TrimSpace(fmt.Sprint(role)) != "" {
			indexed[i].Metadata["openspec_role"] = fmt.Sprint(role)
		}
		if scope, ok := art.Extracted["artifact_scope"]; ok && strings.TrimSpace(fmt.Sprint(scope)) != "" {
			indexed[i].Metadata["artifact_scope"] = fmt.Sprint(scope)
		}
	}
	if err := s.db.ReplaceArtifactSections(artifactID, revID, indexed, now); err != nil {
		return nil, err
	}
	return indexed, nil
}

func primarySectionSourcePath(art adapters.Artifact, sources []adapters.Source) string {
	for _, src := range sources {
		if strings.TrimSpace(src.Path) != "" {
			return filepath.ToSlash(src.Path)
		}
	}
	if art.PrimaryPath != "" {
		return filepath.ToSlash(art.PrimaryPath)
	}
	return ""
}

func isMarkdownSectionSource(art adapters.Artifact, sources []adapters.Source) bool {
	path := primarySectionSourcePath(art, sources)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".mdx":
		return strings.TrimSpace(art.Body) != ""
	default:
		return false
	}
}

func inferIndexedSectionKind(section docsections.Section) string {
	heading := strings.ToLower(section.HeadingPath)
	switch {
	case strings.Contains(heading, "decision") || strings.Contains(heading, "rationale") || strings.Contains(heading, "consequences"):
		return "decision"
	case strings.Contains(heading, "requirement") || strings.Contains(heading, "acceptance") || strings.Contains(heading, "criteria"):
		return "requirement"
	case strings.Contains(heading, "task") || strings.Contains(heading, "todo") || strings.Contains(heading, "next step") || len(section.Tasks) > 0:
		return "task"
	case strings.Contains(heading, "design") || strings.Contains(heading, "architecture"):
		return "design"
	case strings.Contains(heading, "risk") || strings.Contains(heading, "open question"):
		return "risk"
	default:
		return ""
	}
}

func (s *Scanner) insertArtifact(id, repoRoot, repoID string, art adapters.Artifact, sources []adapters.Source, revID, now string, skipAuthoredAtLookup bool, state *scanRunState) error {
	authoredAt := now
	if !skipAuthoredAtLookup {
		authoredAt = state.resolveAuthoredAt(repoRoot, art, sources, now)
	}
	_, err := s.db.Exec(
		`INSERT INTO artifacts (id, repo_id, kind, subtype, title, status, current_revision_id, created_at, updated_at, last_observed_at, authored_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, repoID, art.Kind, art.Subtype, art.Title, art.Status, revID, now, now, now, authoredAt,
	)
	return err
}

func (s *scanRunState) resolveAuthoredAt(repoRoot string, art adapters.Artifact, sources []adapters.Source, now string) string {
	rel := authoredAtRelPath(repoRoot, art, sources)
	if rel == "" {
		return now
	}
	if s != nil {
		s.authoredAtMu.Lock()
		if authoredAt, ok := s.authoredAt[rel]; ok {
			s.authoredAtMu.Unlock()
			return authoredAt
		}
		s.authoredAtMu.Unlock()
	}
	authoredAt := firstCommitDateOrNow(repoRoot, rel, now)
	if s != nil {
		s.storeAuthoredAt(rel, authoredAt)
	}
	return authoredAt
}

func firstCommitDateOrNow(repoRoot, rel, now string) string {
	if d := fileFirstCommitDate(repoRoot, rel); d != "" {
		return d
	}
	return now
}

func authoredAtRelPath(repoRoot string, art adapters.Artifact, sources []adapters.Source) string {
	var rel string
	if art.PrimaryPath != "" {
		if rel2, err := filepath.Rel(repoRoot, art.PrimaryPath); err == nil {
			rel = filepath.ToSlash(rel2)
		}
	}
	if rel == "" && len(sources) > 0 && sources[0].Path != "" {
		rel = filepath.ToSlash(sources[0].Path)
	}
	return rel
}

func (s *Scanner) insertRevision(id, artifactID, contentHash, body string, extracted map[string]any, now string) error {
	var extractedArg any
	if len(extracted) > 0 {
		b, err := json.Marshal(extracted)
		if err != nil {
			return fmt.Errorf("marshal extracted: %w", err)
		}
		extractedArg = string(b)
	}
	_, err := s.db.Exec(
		"INSERT INTO artifact_revisions (id, artifact_id, content_hash, body, extracted_json, observed_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, artifactID, contentHash, body, extractedArg, now,
	)
	return err
}

func (s *Scanner) insertSource(artifactID, repoID string, src adapters.Source, now string) error {
	id := s.ids.NewWithPrefix("src_")
	fp := src.FormatProfile
	if fp == "" {
		fp = format.ProfileGeneric
	}
	var layoutArg any
	if src.LayoutGroup != "" {
		layoutArg = src.LayoutGroup
	}
	_, err := s.db.Exec(
		"INSERT INTO sources (id, artifact_id, repo_id, source_type, path, source_identity, format_profile, layout_group, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, artifactID, repoID, src.SourceType, src.Path, src.SourceIdentity, fp, layoutArg, now, now,
	)
	return err
}

func (s *Scanner) syncSources(artifactID, repoID string, sources []adapters.Source, now string) error {
	if _, err := s.db.Exec("DELETE FROM sources WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, src := range sources {
		if err := s.insertSource(artifactID, repoID, src, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) replaceTodos(artifactID, revID string, todos []todoparse.Todo, now string, indexedSections []docsections.Section) error {
	// Delete existing todos for this artifact
	if _, err := s.db.Exec("DELETE FROM artifact_todos WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, todo := range todos {
		id := s.ids.NewWithPrefix("todo_")
		done := 0
		if todo.Done {
			done = 1
		}
		sectionID := docsections.EnclosingSectionID(indexedSections, todo.SourceLine)
		if _, err := s.db.Exec(
			"INSERT INTO artifact_todos (id, artifact_id, revision_id, section_id, ordinal, text, done, source_file, source_line, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			id, artifactID, revID, sectionID, todo.Ordinal, todo.Text, done, todo.SourceFile, todo.SourceLine, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) replaceCriteria(artifactID, revID string, criteria []todoparse.Criterion, now string, indexedSections []docsections.Section) error {
	if _, err := s.db.Exec("DELETE FROM artifact_criteria WHERE artifact_id = ?", artifactID); err != nil {
		return err
	}
	for _, c := range criteria {
		id := s.ids.NewWithPrefix("crit_")
		done := 0
		if c.Done {
			done = 1
		}
		sectionID := docsections.EnclosingSectionID(indexedSections, c.SourceLine)
		if _, err := s.db.Exec(
			"INSERT INTO artifact_criteria (id, artifact_id, revision_id, section_id, ordinal, text, done, source_file, source_line, criteria_kind, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			id, artifactID, revID, sectionID, c.Ordinal, c.Text, done, c.SourceFile, c.SourceLine, c.CriteriaKind, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scanner) assignShortID(artifactID, sourceIdentity string, state *scanRunState) {
	if err := s.db.AssignArtifactShortID(artifactID, idgen.ShortID(sourceIdentity)); err != nil {
		return
	}
	if state == nil || state.shortIDs == nil {
		return
	}
	var shortID string
	if err := s.db.QueryRow("SELECT COALESCE(short_id, '') FROM artifacts WHERE id = ?", artifactID).Scan(&shortID); err != nil {
		return
	}
	base, next := shortIDClaim(shortID)
	if base != "" && state.shortIDs[base] < next {
		state.shortIDs[base] = next
	}
}

func (s *Scanner) replaceTags(artifactID string, art adapters.Artifact, now string) {
	s.db.DeleteAutoTags(artifactID)

	for _, tag := range art.Tags {
		s.db.InsertTag(artifactID, tag, "frontmatter", now)
	}

	// Infer directory tag if no frontmatter tags
	if len(art.Tags) == 0 {
		relPath := ""
		if art.PrimaryPath != "" {
			// Find the relative path from sources
			rows, _ := s.db.Query("SELECT path FROM sources WHERE artifact_id = ? LIMIT 1", artifactID)
			if rows != nil {
				if rows.Next() {
					rows.Scan(&relPath)
				}
				rows.Close()
			}
		}
		if relPath != "" {
			if dirTag := markdown.InferDirectoryTag(relPath); dirTag != "" {
				s.db.InsertTag(artifactID, dirTag, "inferred", now)
			}
		}
	}
}

func hashContent(body string) string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	normalized = strings.Join(lines, "\n")
	h := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(h[:])
}
