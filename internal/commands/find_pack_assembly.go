package commands

import (
	"context"
	"fmt"

	"github.com/devspecs-com/devspecs-cli/internal/retrieval"
	"github.com/devspecs-com/devspecs-cli/internal/store"
	"github.com/devspecs-com/devspecs-cli/internal/telemetry"
)

type findPackAssemblyOptions struct {
	PackCompanions       string
	SourceTestReceipts   string
	GitReceipts          bool
	BoundaryPrimary      bool
	PackPresentationMode string
	PackScoutMode        string
}

type findPackAssemblyResult struct {
	Matches        []retrieval.Candidate
	Reasons        map[string][]string
	RolePack       retrieval.RoleGroupedPack
	ReceiptPack    retrieval.RoleGroupedPack
	RelatedTests   *FindRelatedTestContext
	GitTrust       *FindGitTrustContext
	CompanionAdded int
}

func retrieveFindMatches(retriever retrieval.WeightedFilesRetrieverV0, candidates []retrieval.Candidate, query string) []retrieval.Candidate {
	matches := retriever.Retrieve(candidates, query)
	if len(matches) == 0 {
		matches = retrieval.QueryBaseline(candidates, query)
	}
	return matches
}

func buildFindPackAssemblyFromMatches(ctx context.Context, db *store.DB, fp store.FilterParams, query string, matches, candidates []retrieval.Candidate, opts findPackAssemblyOptions) (findPackAssemblyResult, error) {
	opts.PackCompanions = normalizeFindPackCompanionMode(opts.PackCompanions)
	if opts.PackCompanions == "" {
		return findPackAssemblyResult{}, fmt.Errorf("unknown pack companion mode")
	}
	opts.SourceTestReceipts = normalizeFindSourceTestReceiptsMode(opts.SourceTestReceipts)
	if opts.SourceTestReceipts == "" {
		return findPackAssemblyResult{}, fmt.Errorf("unknown source-test receipt mode")
	}
	opts.PackPresentationMode = normalizeFindPackPresentationMode(opts.PackPresentationMode)
	if opts.PackPresentationMode == "" {
		return findPackAssemblyResult{}, fmt.Errorf("unknown pack presentation mode")
	}
	if opts.BoundaryPrimary && opts.PackPresentationMode != findPackPresentationModeOff {
		return findPackAssemblyResult{}, fmt.Errorf("boundary primary cannot be combined with pack presentation mode %s", opts.PackPresentationMode)
	}

	initialMatchCount := len(matches)
	matches = addFindPackCompanionCandidates(ctx, fp.RepoRoot, query, matches, candidates, opts.PackCompanions)
	reasons := reasonsByPath(retrieval.ExplainCandidates(matches, query))
	rolePack := retrieval.BuildRoleGroupedPack(matches, reasons, query)
	receiptPack := rolePack

	var relatedTests *FindRelatedTestContext
	if opts.SourceTestReceipts != findSourceTestReceiptsModeOff {
		var err error
		relatedTests, err = buildFindSourceTestReceipts(db, fp, query, receiptPack, opts.SourceTestReceipts)
		if err != nil {
			return findPackAssemblyResult{}, fmt.Errorf("find source test receipts: %w", err)
		}
	}

	var gitTrust *FindGitTrustContext
	if opts.GitReceipts && fp.RepoRoot != "" {
		gitTrust = buildFindGitTrustContext(ctx, fp.RepoRoot, query, receiptPack)
	}

	if opts.BoundaryPrimary {
		rolePack = retrieval.ApplyBoundaryPrimaryPackForQuery(rolePack, query)
	} else {
		rolePack = applyFindPackPresentationMode(rolePack, query, opts.PackPresentationMode)
	}
	if normalizeFindPackScoutMode(opts.PackScoutMode) == findPackScoutModeBetaV0 {
		rolePack = retrieval.ApplyScoutSourceTestRescueForQuery(rolePack, query)
		rolePack = retrieval.ApplyScoutSourcePrimaryPreservationForQuery(rolePack, query)
		rolePack = addFindPackScoutBodyEvidence(fp.RepoRoot, query, rolePack)
		rolePack = retrieval.ApplyDemotionOnlyNegativeEvidence(rolePack, query)
		rolePack = retrieval.ApplyScoutUncertaintyForQuery(rolePack, query)
	}
	rolePack = annotateFindPackScoutMode(rolePack, opts.PackScoutMode)

	return findPackAssemblyResult{
		Matches:        matches,
		Reasons:        reasons,
		RolePack:       rolePack,
		ReceiptPack:    receiptPack,
		RelatedTests:   relatedTests,
		GitTrust:       gitTrust,
		CompanionAdded: len(matches) - initialMatchCount,
	}, nil
}

func annotateFindPackScoutMode(rolePack retrieval.RoleGroupedPack, mode string) retrieval.RoleGroupedPack {
	mode = normalizeFindPackScoutMode(mode)
	if mode == "" || mode == findPackScoutModeOff {
		return rolePack
	}
	if rolePack.Metadata == nil {
		rolePack.Metadata = map[string]string{}
	}
	rolePack.Metadata["pack_scout_mode"] = mode
	rolePack.Metadata["pack_scout_contract"] = "o07_2_preserve_primary"
	return rolePack
}

func applyFindPackPresentationMode(rolePack retrieval.RoleGroupedPack, query, mode string) retrieval.RoleGroupedPack {
	switch normalizeFindPackPresentationMode(mode) {
	case findPackPresentationModeFamilyPrimaryV0:
		return retrieval.ApplyFamilyPrimaryPackForQuery(rolePack, query)
	case findPackPresentationModeFamilyPrimaryV1:
		return retrieval.ApplyFamilyPrimaryPackV1ForQuery(rolePack, query)
	case findPackPresentationModeFamilyPrimaryV2:
		return retrieval.ApplyFamilyPrimaryPackV2ForQuery(rolePack, query)
	default:
		return rolePack
	}
}

func recordFindPackPresentationProps(props map[string]any, rolePack retrieval.RoleGroupedPack) {
	if rolePack.Metadata == nil {
		return
	}
	props["family_primary_count_bucket"] = telemetry.CountBucket(metadataInt(rolePack.Metadata, "family_primary_count"))
	props["family_related_count_bucket"] = telemetry.CountBucket(metadataInt(rolePack.Metadata, "family_related_count"))
	if count := metadataInt(rolePack.Metadata, "negative_evidence_count"); count > 0 {
		props["pack_negative_evidence_count_bucket"] = telemetry.CountBucket(count)
	}
	if count := metadataInt(rolePack.Metadata, "pack_scout_source_rescue_count"); count > 0 {
		props["pack_scout_source_rescue_count_bucket"] = telemetry.CountBucket(count)
	}
	if count := metadataInt(rolePack.Metadata, "pack_scout_body_evidence_count"); count > 0 {
		props["pack_scout_body_evidence_count_bucket"] = telemetry.CountBucket(count)
	}
	if bytesRead := metadataInt(rolePack.Metadata, "pack_scout_body_evidence_bytes"); bytesRead > 0 {
		props["pack_scout_body_evidence_bytes_bucket"] = telemetry.CountBucket(bytesRead)
	}
	if rolePack.Metadata["pack_scout_uncertainty"] == "true" {
		props["pack_scout_uncertainty"] = true
	}
}
