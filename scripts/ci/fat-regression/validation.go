package main

import "errors"

// Check structure before trusting aggregate counters. Errors are public-safe:
// never include repository identities, paths, or raw command diagnostics.
func validateResultSets(activation activationResult, baseline, candidate scanResult) error {
	if activation.Schema != "devspecs.activation_matrix.v1" ||
		activation.Profile != "fat" || activation.CloneMode != "full" || activation.IndexState != "cold" {
		return errors.New("activation result schema or execution mode is invalid")
	}
	total := activation.RepoSet.Total
	if total < 1 || activation.Summary.Total != total*2 || len(activation.Cases) != total*2 {
		return errors.New("activation result is empty or incomplete")
	}
	repos := make(map[string]map[string]bool)
	passed, failed, missing := 0, 0, 0
	for _, item := range activation.Cases {
		if item.RepoID == "" || (item.Command != "recent" && item.Command != "map") {
			return errors.New("activation result contains an invalid case")
		}
		if repos[item.RepoID] == nil {
			repos[item.RepoID] = make(map[string]bool)
		}
		if repos[item.RepoID][item.Command] {
			return errors.New("activation result contains duplicate cases")
		}
		repos[item.RepoID][item.Command] = true
		switch item.Status {
		case "passed":
			passed++
		case "failed":
			failed++
		case "missing":
			missing++
		default:
			return errors.New("activation result contains an invalid status")
		}
	}
	if len(repos) != total || passed != activation.Summary.Passed || failed != activation.Summary.Failed || missing != activation.Summary.Missing {
		return errors.New("activation summary does not match its cases")
	}
	for _, commands := range repos {
		if !commands["recent"] || !commands["map"] {
			return errors.New("activation result is missing a repository command")
		}
	}
	if err := validateScanResult(baseline, repos); err != nil {
		return err
	}
	return validateScanResult(candidate, repos)
}

func validateScanResult(result scanResult, repos map[string]map[string]bool) error {
	if result.Schema != "devspecs.activation_scan_benchmark.v1" ||
		result.Profile != "fat" || result.CloneMode != "full" || result.IndexState != "cold" {
		return errors.New("scan result schema or execution mode is invalid")
	}
	total := len(repos)
	if result.RepoSet.Total != total || result.Summary.Total != total || len(result.Cases) != total {
		return errors.New("scan result is empty or incomplete")
	}
	seen := make(map[string]bool)
	passed, failed := 0, 0
	for _, item := range result.Cases {
		if repos[item.RepoID] == nil || item.Command != "scan" || seen[item.RepoID] {
			return errors.New("scan result contains mismatched or duplicate cases")
		}
		seen[item.RepoID] = true
		switch item.Status {
		case "passed":
			passed++
		case "failed":
			failed++
		default:
			return errors.New("scan result contains an invalid status")
		}
	}
	if passed != result.Summary.Passed || failed != result.Summary.Failed ||
		result.Summary.RegressionFailures < 0 || result.Summary.RegressionFailures > failed {
		return errors.New("scan summary does not match its cases")
	}
	return nil
}
