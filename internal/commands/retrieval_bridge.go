package commands

import (
	"math"

	"github.com/devspecs-com/devspecs-cli/internal/indexquery"
	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
)

const commandTokenCounterName = indexquery.TokenCounterName

func loadRetrievalCandidates(db *store.DB, fp store.FilterParams) ([]retrieval.Candidate, error) {
	return indexquery.LoadCandidates(db, fp)
}

func loadRetrievalCandidatesForQuery(db *store.DB, fp store.FilterParams, query string) ([]retrieval.Candidate, error) {
	return indexquery.LoadCandidatesForQuery(db, fp, query)
}

func artifactCandidate(art store.ArtifactRow, sources []store.SourceRow, todos []store.TodoRow, body, extractedJSON string) retrieval.Candidate {
	return indexquery.ArtifactCandidate(art, sources, todos, body, extractedJSON)
}

func artifactCandidateWithLinks(art store.ArtifactRow, sources []store.SourceRow, links []store.LinkRow, todos []store.TodoRow, body, extractedJSON string) retrieval.Candidate {
	return indexquery.ArtifactCandidateWithLinks(art, sources, links, todos, nil, body, extractedJSON)
}

func approximateTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return int(math.Ceil(float64(len(text)) / 4.0))
}

func capCandidates(candidates []retrieval.Candidate, limit int) []retrieval.Candidate {
	if limit <= 0 || len(candidates) <= limit {
		return candidates
	}
	return candidates[:limit]
}

func shortCandidateID(c retrieval.Candidate) string {
	if c.Metadata != nil && c.Metadata["short_id"] != "" {
		return c.Metadata["short_id"]
	}
	if len(c.ID) > 8 {
		return c.ID[:8]
	}
	return c.ID
}
