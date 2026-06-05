package commands

import (
	"time"

	"github.com/devspecs-com/devspecs-cli/internal/indexquery"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

type findSourceManifestReport struct {
	Mode           string
	SelectedCount  int
	FallbackReason string
	ElapsedMS      int64
}

func loadFindSourceManifestCandidates(db *store.DB, fp store.FilterParams, query string, mode indexquery.SourceManifestCandidateMode) ([]retrieval.Candidate, findSourceManifestReport, error) {
	start := time.Now()
	opts := indexquery.DefaultSourceManifestCandidateOptions()
	opts.Mode = mode
	candidates, report, err := indexquery.LoadSourceManifestCandidatesForQuery(db, fp, query, opts)
	return candidates, findSourceManifestReport{
		Mode:           report.Mode,
		SelectedCount:  report.SelectedCount,
		FallbackReason: report.FallbackReason,
		ElapsedMS:      time.Since(start).Milliseconds(),
	}, err
}
