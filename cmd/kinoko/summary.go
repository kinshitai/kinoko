package main

import (
	"fmt"
	"os"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// printExtractionSummary prints a human-readable extraction summary to stderr.
// If sourcePath is non-empty, a "Source:" line is included (used by convert).
func printExtractionSummary(result *model.ExtractionResult, sourcePath string, dryRun bool) {
	w := os.Stderr

	if sourcePath != "" {
		fmt.Fprintln(w, "─── Convert Summary ───")
		fmt.Fprintf(w, "  Source:   %s\n", sourcePath)
	} else {
		fmt.Fprintln(w, "─── Extraction Summary ───")
	}
	fmt.Fprintf(w, "  Status:   %s\n", result.Status)

	// Print Stage 2 rubric scores if available.
	if result.Stage2 != nil {
		s := result.Stage2.RubricScores
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Stage 2 Scores:")
		fmt.Fprintf(w, "    problem_specificity:    %d\n", s.ProblemSpecificity)
		fmt.Fprintf(w, "    solution_completeness:  %d\n", s.SolutionCompleteness)
		fmt.Fprintf(w, "    context_portability:    %d\n", s.ContextPortability)
		fmt.Fprintf(w, "    reasoning_transparency: %d\n", s.ReasoningTransparency)
		fmt.Fprintf(w, "    technical_accuracy:     %d\n", s.TechnicalAccuracy)
		fmt.Fprintf(w, "    verification_evidence:  %d\n", s.VerificationEvidence)
		fmt.Fprintf(w, "    innovation_level:       %d\n", s.InnovationLevel)
		fmt.Fprintf(w, "    composite:              %.2f\n", s.CompositeScore)
	}

	// Print Stage 3 verdict if available.
	if result.Stage3 != nil {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  Stage 3 Verdict: %s\n", result.Stage3.CriticVerdict)
		fmt.Fprintf(w, "    confidence: %.2f\n", result.Stage3.RefinedScores.CriticConfidence)
		fmt.Fprintf(w, "    reasoning: %s\n", result.Stage3.CriticReasoning)
	}

	commitLabel := "Committed"
	if sourcePath == "" {
		commitLabel = "Pushed"
	}

	switch result.Status {
	case model.StatusExtracted:
		if result.Skill != nil {
			fmt.Fprintln(w)
			fmt.Fprintf(w, "  Skill:    %s\n", result.Skill.Name)
			fmt.Fprintf(w, "  Version:  %d\n", result.Skill.Version)
			fmt.Fprintf(w, "  Quality:  %.2f\n", result.Skill.Quality.CompositeScore)
		}
		switch {
		case dryRun:
			fmt.Fprintf(w, "  %s: no (dry-run)\n", commitLabel)
		case result.CommitHash != "":
			fmt.Fprintf(w, "  %s: yes (%s)\n", commitLabel, result.CommitHash)
		default:
			fmt.Fprintf(w, "  %s: no\n", commitLabel)
		}
	case model.StatusRejected:
		switch {
		case result.Stage1 != nil && !result.Stage1.Passed:
			fmt.Fprintf(w, "  Rejected at: Stage 1 — %s\n", result.Stage1.Reason)
		case result.Stage2 != nil && !result.Stage2.Passed:
			fmt.Fprintf(w, "  Rejected at: Stage 2 — %s\n", result.Stage2.Reason)
		case result.Stage3 != nil && !result.Stage3.Passed:
			fmt.Fprintf(w, "  Rejected at: Stage 3 — %s\n", result.Stage3.CriticReasoning)
		}
	case model.StatusError:
		fmt.Fprintf(w, "  Error:    %s\n", result.Error)
	}

	fmt.Fprintf(w, "  Duration: %dms\n", result.DurationMs)
	if sourcePath != "" {
		fmt.Fprintln(w, "───────────────────────")
	} else {
		fmt.Fprintln(w, "──────────────────────────")
	}
}
