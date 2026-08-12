package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"strings"
)

const defaultCoverageFloor = "80.0"

type coverageSummary struct {
	CoveredStatements int64
	TotalStatements   int64
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check-coverage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profilePath := flags.String("profile", "coverage.out", "Go coverage profile to evaluate")
	floor := flags.String("floor", defaultCoverageFloor, "minimum aggregate statement coverage percentage")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected positional arguments")
		return 2
	}

	profile, err := os.Open(*profilePath)
	if err != nil {
		fmt.Fprintf(stderr, "open coverage profile: %v\n", err)
		return 1
	}
	defer profile.Close()

	summary, err := evaluateCoverage(profile, *floor)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "total coverage %.3f%% (%d/%d statements; floor %s%%)\n",
		summary.Percent(), summary.CoveredStatements, summary.TotalStatements, *floor)
	return 0
}

func evaluateCoverage(profile io.Reader, floor string) (coverageSummary, error) {
	summary, err := readCoverageProfile(profile)
	if err != nil {
		return coverageSummary{}, err
	}
	floorRatio, ok := new(big.Rat).SetString(strings.TrimSpace(floor))
	if !ok || floorRatio.Sign() < 0 || floorRatio.Cmp(big.NewRat(100, 1)) > 0 {
		return coverageSummary{}, fmt.Errorf("invalid coverage floor %q; expected a percentage from 0 through 100", floor)
	}

	coveredPercent := new(big.Rat).SetFrac(
		new(big.Int).Mul(big.NewInt(summary.CoveredStatements), big.NewInt(100)),
		big.NewInt(summary.TotalStatements),
	)
	if coveredPercent.Cmp(floorRatio) < 0 {
		return summary, fmt.Errorf("total coverage %.3f%% (%d/%d statements) is below %s%%",
			summary.Percent(), summary.CoveredStatements, summary.TotalStatements, floor)
	}
	return summary, nil
}

func readCoverageProfile(profile io.Reader) (coverageSummary, error) {
	scanner := bufio.NewScanner(profile)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return coverageSummary{}, fmt.Errorf("read coverage profile: %w", err)
		}
		return coverageSummary{}, fmt.Errorf("coverage profile is empty")
	}
	mode := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "mode:"))
	if !strings.HasPrefix(scanner.Text(), "mode:") || (mode != "set" && mode != "count" && mode != "atomic") {
		return coverageSummary{}, fmt.Errorf("unsupported coverage profile header %q", scanner.Text())
	}

	var summary coverageSummary
	lineNumber := 1
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return coverageSummary{}, fmt.Errorf("parse coverage profile line %d: expected location, statement count, and execution count", lineNumber)
		}
		statements, err := strconv.ParseInt(fields[len(fields)-2], 10, 64)
		if err != nil || statements < 0 {
			return coverageSummary{}, fmt.Errorf("parse coverage profile line %d: invalid statement count %q", lineNumber, fields[len(fields)-2])
		}
		executions, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
		if err != nil || executions < 0 {
			return coverageSummary{}, fmt.Errorf("parse coverage profile line %d: invalid execution count %q", lineNumber, fields[len(fields)-1])
		}
		summary.TotalStatements += statements
		if executions > 0 {
			summary.CoveredStatements += statements
		}
	}
	if err := scanner.Err(); err != nil {
		return coverageSummary{}, fmt.Errorf("read coverage profile: %w", err)
	}
	if summary.TotalStatements == 0 {
		return coverageSummary{}, fmt.Errorf("coverage profile contains no statements")
	}
	return summary, nil
}

func (s coverageSummary) Percent() float64 {
	if s.TotalStatements == 0 {
		return 0
	}
	return float64(s.CoveredStatements) * 100 / float64(s.TotalStatements)
}
