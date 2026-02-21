// Package extraction implements the 3-stage skill extraction pipeline.
// Stage 1 filters sessions by metadata heuristics, Stage 2 scores novelty
// and rubric quality via embeddings and LLM, and Stage 3 applies an LLM
// critic for final extract/reject verdicts. Extracted skills are persisted
// as SKILL.md files with structured front matter.
package extraction
