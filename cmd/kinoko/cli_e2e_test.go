package main

// NOTE: tests mutate package globals; do not use t.Parallel()

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/serverclient"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// ── mock API server ──

// newMockAPI spins up an httptest server that handles /api/v1/embed and
// /api/v1/discover with canned responses.
func newMockAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// /api/v1/embed — returns a fake 384-dim vector
	mux.HandleFunc("POST /api/v1/embed", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		// Generate deterministic but input-dependent vectors via FNV hash
		h := fnv.New32a()
		h.Write([]byte(req.Text))
		seed := h.Sum32()
		vec := make([]float32, 384)
		for i := range vec {
			// Mix seed with index for per-dimension variation
			bits := seed ^ uint32(i*2654435761)
			vec[i] = float32(bits%1000) / 1000.0
		}
		// Normalize to unit vector
		var norm float64
		for _, v := range vec {
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for i := range vec {
				vec[i] = float32(float64(vec[i]) / norm)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"vector": vec,
			"model":  "mock",
			"dims":   384,
		})
	})

	// /api/v1/discover — unified endpoint for all discovery needs
	mux.HandleFunc("POST /api/v1/discover", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt     string    `json:"prompt,omitempty"`
			Embedding  []float64 `json:"embedding,omitempty"`
			Patterns   []string  `json:"patterns,omitempty"`
			LibraryIDs []string  `json:"library_ids,omitempty"`
			MinQuality float64   `json:"min_quality,omitempty"`
			TopK       int       `json:"top_k,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}

		// Return mock skill results with different scores based on request type
		score := 0.95
		if req.TopK > 5 {
			// For novelty checking (high TopK), return lower similarity score
			score = 0.1
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"skills": []map[string]any{
				{
					"repo":        "local/test-skill",
					"name":        "test-skill",
					"description": "# Test Skill\nSome content.",
					"score":       score,
					"clone_url":   "https://example.com/test-skill.git",
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// writeTmpConfig creates a minimal kinoko config YAML pointing at a temp SQLite DB.
func writeTmpConfig(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, "test.db")
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf(`server:
  host: 127.0.0.1
  port: 23233
  data_dir: %s
storage:
  driver: sqlite
  dsn: %s
libraries:
  - id: local
    name: local
    path: %s/libs/local
`, dir, dbPath, dir)
	os.MkdirAll(filepath.Join(dir, "libs", "local"), 0755)
	os.WriteFile(cfgPath, []byte(cfg), 0644)
	return cfgPath
}

// writeValidSkillMD writes a well-formed SKILL.md with front matter.
func writeValidSkillMD(t *testing.T, dir, name string) string {
	t.Helper()
	content := fmt.Sprintf(`---
name: %s
description: A test skill for E2E testing
version: 1
category: tactical
tags:
  - go
  - testing
---
# %s

This is a test skill for E2E testing.
`, name, strings.ReplaceAll(name, "-", " "))
	p := filepath.Join(dir, "SKILL.md")
	os.WriteFile(p, []byte(content), 0644)
	return p
}

// ── ingest --force with real infrastructure ──

func TestIngestForce_RealSQLite(t *testing.T) {
	dir := t.TempDir()
	srv := newMockAPI(t)
	cfgPath := writeTmpConfig(t, dir)
	skillFile := writeValidSkillMD(t, dir, "real-test-skill")

	// Set globals that runIngest reads
	oldForce, oldCfg, oldAPI := ingestForce, ingestConfigPath, ingestAPIURL
	oldDryRun, oldLib := ingestDryRun, ingestLibrary
	ingestForce = true
	ingestConfigPath = cfgPath
	ingestAPIURL = srv.URL
	ingestDryRun = true // skip git push
	ingestLibrary = "local"
	t.Cleanup(func() {
		ingestForce = oldForce
		ingestConfigPath = oldCfg
		ingestAPIURL = oldAPI
		ingestDryRun = oldDryRun
		ingestLibrary = oldLib
	})

	err := runIngest(ingestCmd, []string{skillFile})
	if err != nil {
		t.Fatalf("runIngest failed: %v", err)
	}
}

func TestIngestForce_RejectsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	srv := newMockAPI(t)
	cfgPath := writeTmpConfig(t, dir)
	f := filepath.Join(dir, "empty.md")
	os.WriteFile(f, []byte{}, 0644)

	oldForce, oldCfg, oldAPI := ingestForce, ingestConfigPath, ingestAPIURL
	ingestForce = true
	ingestConfigPath = cfgPath
	ingestAPIURL = srv.URL
	t.Cleanup(func() {
		ingestForce = oldForce
		ingestConfigPath = oldCfg
		ingestAPIURL = oldAPI
	})

	err := runIngest(ingestCmd, []string{f})
	if err == nil || !strings.Contains(err.Error(), "file is empty") {
		t.Fatalf("expected 'file is empty', got %v", err)
	}
}

func TestIngestForce_RejectsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	srv := newMockAPI(t)
	cfgPath := writeTmpConfig(t, dir)
	f := filepath.Join(dir, "binary.md")
	os.WriteFile(f, []byte{0xff, 0xfe, 0x00, 0x01, 'h', 'e', 'l', 'l', 'o'}, 0644)

	oldForce, oldCfg, oldAPI := ingestForce, ingestConfigPath, ingestAPIURL
	ingestForce = true
	ingestConfigPath = cfgPath
	ingestAPIURL = srv.URL
	t.Cleanup(func() {
		ingestForce = oldForce
		ingestConfigPath = oldCfg
		ingestAPIURL = oldAPI
	})

	err := runIngest(ingestCmd, []string{f})
	if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Fatalf("expected UTF-8 error, got %v", err)
	}
}

func TestIngestForce_RejectsLargeFile(t *testing.T) {
	dir := t.TempDir()
	srv := newMockAPI(t)
	cfgPath := writeTmpConfig(t, dir)
	f := filepath.Join(dir, "big.md")
	data := make([]byte, (1<<20)+1)
	for i := range data {
		data[i] = 'A'
	}
	os.WriteFile(f, data, 0644)

	oldForce, oldCfg, oldAPI := ingestForce, ingestConfigPath, ingestAPIURL
	ingestForce = true
	ingestConfigPath = cfgPath
	ingestAPIURL = srv.URL
	t.Cleanup(func() {
		ingestForce = oldForce
		ingestConfigPath = oldCfg
		ingestAPIURL = oldAPI
	})

	err := runIngest(ingestCmd, []string{f})
	if err == nil || !strings.Contains(err.Error(), "file too large") {
		t.Fatalf("expected 'file too large', got %v", err)
	}
}

func TestIngestForce_RejectsNoFrontMatter(t *testing.T) {
	dir := t.TempDir()
	srv := newMockAPI(t)
	cfgPath := writeTmpConfig(t, dir)
	f := filepath.Join(dir, "nofm.md")
	os.WriteFile(f, []byte("# Just a heading\nNo front matter here.\n"), 0644)

	oldForce, oldCfg, oldAPI := ingestForce, ingestConfigPath, ingestAPIURL
	ingestForce = true
	ingestConfigPath = cfgPath
	ingestAPIURL = srv.URL
	t.Cleanup(func() {
		ingestForce = oldForce
		ingestConfigPath = oldCfg
		ingestAPIURL = oldAPI
	})

	err := runIngest(ingestCmd, []string{f})
	if err == nil || !strings.Contains(err.Error(), "front matter") {
		t.Fatalf("expected front matter error, got %v", err)
	}
}

// ── match with real injection client ──

func TestMatchCmd_RequiresQueryOrFile(t *testing.T) {
	// Reset matchFile to ensure clean state
	oldFile := matchFile
	matchFile = ""
	t.Cleanup(func() { matchFile = oldFile })

	err := runMatch(matchCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "provide query text") {
		t.Fatalf("expected 'provide query' error, got %v", err)
	}
}

func TestMatchCmd_EmptyQueryError(t *testing.T) {
	err := runMatch(matchCmd, []string{""})
	if err == nil || !strings.Contains(err.Error(), "query text is empty") {
		t.Fatalf("expected 'empty' error, got %v", err)
	}
}

func TestMatchCmd_EmptyFileError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.txt")
	os.WriteFile(f, []byte(""), 0644)

	oldFile := matchFile
	matchFile = f
	t.Cleanup(func() { matchFile = oldFile })

	err := runMatch(matchCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "query text is empty") {
		t.Fatalf("expected 'empty' error, got %v", err)
	}
}

func TestMatchCmd_RealAPICall(t *testing.T) {
	srv := newMockAPI(t)

	oldAPI, oldTimeout := matchAPIURL, matchTimeout
	matchAPIURL = srv.URL
	matchTimeout = 5 * 1e9 // 5s
	t.Cleanup(func() {
		matchAPIURL = oldAPI
		matchTimeout = oldTimeout
	})

	// Capture stdout
	out := captureStdout(func() {
		err := runMatch(matchCmd, []string{"database timeout in Go"})
		if err != nil {
			t.Fatalf("runMatch failed: %v", err)
		}
	})

	if !strings.Contains(out, "test-skill") {
		t.Fatalf("expected 'test-skill' in output, got:\n%s", out)
	}
}

func TestMatchCmd_FileReadsFromDisk(t *testing.T) {
	srv := newMockAPI(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "query.txt")
	os.WriteFile(f, []byte("test query from file"), 0644)

	oldFile, oldAPI, oldTimeout := matchFile, matchAPIURL, matchTimeout
	matchFile = f
	matchAPIURL = srv.URL
	matchTimeout = 5 * 1e9
	t.Cleanup(func() {
		matchFile = oldFile
		matchAPIURL = oldAPI
		matchTimeout = oldTimeout
	})

	out := captureStdout(func() {
		err := runMatch(matchCmd, nil)
		if err != nil {
			t.Fatalf("runMatch failed: %v", err)
		}
	})

	if !strings.Contains(out, "test-skill") {
		t.Fatalf("expected match results in output, got:\n%s", out)
	}
}

// ── Full flow: ingest → SQLite → match ──

func TestFullFlow_IngestThenMatch(t *testing.T) {
	dir := t.TempDir()
	srv := newMockAPI(t)
	cfgPath := writeTmpConfig(t, dir)
	skillFile := writeValidSkillMD(t, dir, "flow-test-skill")

	// Ingest with --force --dry-run: validates and prints summary without committing.
	oldForce, oldCfg, oldAPI := ingestForce, ingestConfigPath, ingestAPIURL
	oldDryRun, oldLib := ingestDryRun, ingestLibrary
	ingestForce = true
	ingestConfigPath = cfgPath
	ingestAPIURL = srv.URL
	ingestDryRun = true
	ingestLibrary = "local"
	t.Cleanup(func() {
		ingestForce = oldForce
		ingestConfigPath = oldCfg
		ingestAPIURL = oldAPI
		ingestDryRun = oldDryRun
		ingestLibrary = oldLib
	})

	// Capture dry-run output and verify summary fields.
	ingestOut := captureStdout(func() {
		err := runIngest(ingestCmd, []string{skillFile})
		if err != nil {
			t.Fatalf("ingest failed: %v", err)
		}
	})

	// Verify dry-run summary contains expected metadata.
	for _, want := range []string{
		"flow-test-skill",
		"Verdict:  force",
		"Category: tactical",
		"Library:  local",
		"Version:  1",
		"Committed: no (dry-run)",
	} {
		if !strings.Contains(ingestOut, want) {
			t.Errorf("ingest output missing %q, got:\n%s", want, ingestOut)
		}
	}

	// Match against the mock server (independent of ingest — uses API).
	oldMatchAPI, oldMatchTimeout := matchAPIURL, matchTimeout
	matchAPIURL = srv.URL
	matchTimeout = 5 * 1e9
	t.Cleanup(func() {
		matchAPIURL = oldMatchAPI
		matchTimeout = oldMatchTimeout
	})

	out := captureStdout(func() {
		err := runMatch(matchCmd, []string{"flow test query"})
		if err != nil {
			t.Fatalf("match failed: %v", err)
		}
	})

	if !strings.Contains(out, "test-skill") {
		t.Fatalf("expected match result, got:\n%s", out)
	}
}

// ── HTTPEmbedder tests ──

func TestHTTPEmbedder_Dimensions(t *testing.T) {
	e := serverclient.NewHTTPEmbedder(serverclient.New("http://unused"), 384)
	if d := e.Dimensions(); d != 384 {
		t.Fatalf("expected 384, got %d", d)
	}
}

// ── printExtractSummary tests ──

func TestPrintExtractSummary_Extracted(t *testing.T) {
	result := &model.ExtractionResult{
		Status: model.StatusExtracted,
		Skill: &model.SkillRecord{
			Name:    "test-skill",
			Version: 1,
			Quality: model.QualityScores{CompositeScore: 0.85},
		},
		DurationMs: 123,
		CommitHash: "abc123",
	}

	out := captureStdout(func() {
		printExtractSummary(result, false)
	})

	mustContain(t, out, "extracted")
	mustContain(t, out, "test-skill")
	mustContain(t, out, "abc123")
	mustContain(t, out, "123ms")
}

func TestPrintExtractSummary_ExtractedDryRun(t *testing.T) {
	result := &model.ExtractionResult{
		Status: model.StatusExtracted,
		Skill: &model.SkillRecord{
			Name:    "test-skill",
			Version: 1,
			Quality: model.QualityScores{CompositeScore: 0.85},
		},
		DurationMs: 50,
	}

	out := captureStdout(func() {
		printExtractSummary(result, true)
	})

	mustContain(t, out, "dry-run")
}

func TestPrintExtractSummary_RejectedStage1(t *testing.T) {
	result := &model.ExtractionResult{
		Status:     model.StatusRejected,
		Stage1:     &model.Stage1Result{Passed: false, Reason: "too short"},
		DurationMs: 10,
	}

	out := captureStdout(func() {
		printExtractSummary(result, false)
	})

	mustContain(t, out, "Stage 1")
	mustContain(t, out, "too short")
}

func TestPrintExtractSummary_RejectedStage2(t *testing.T) {
	result := &model.ExtractionResult{
		Status:     model.StatusRejected,
		Stage1:     &model.Stage1Result{Passed: true},
		Stage2:     &model.Stage2Result{Passed: false, Reason: "low novelty"},
		DurationMs: 20,
	}

	out := captureStdout(func() {
		printExtractSummary(result, false)
	})

	mustContain(t, out, "Stage 2")
	mustContain(t, out, "low novelty")
}

func TestPrintExtractSummary_RejectedStage3(t *testing.T) {
	result := &model.ExtractionResult{
		Status:     model.StatusRejected,
		Stage1:     &model.Stage1Result{Passed: true},
		Stage2:     &model.Stage2Result{Passed: true},
		Stage3:     &model.Stage3Result{Passed: false, CriticReasoning: "not reusable"},
		DurationMs: 30,
	}

	out := captureStdout(func() {
		printExtractSummary(result, false)
	})

	mustContain(t, out, "Stage 3")
	mustContain(t, out, "not reusable")
}

func TestPrintExtractSummary_Error(t *testing.T) {
	result := &model.ExtractionResult{
		Status:     model.StatusError,
		Error:      "something broke",
		DurationMs: 5,
	}

	out := captureStdout(func() {
		printExtractSummary(result, false)
	})

	mustContain(t, out, "error")
	mustContain(t, out, "something broke")
}

// ── sanitizeSkillName tests ──

func TestSanitizeSkillName(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"My_Cool-Skill", "my_cool-skill", false},
		{"hello world", "helloworld", false},
		{"../../../etc/passwd", "etcpasswd", false},
		{"!!!", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := sanitizeSkillName(tc.input)
			if tc.err && err == nil {
				t.Fatal("expected error")
			}
			if !tc.err && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── helpers ──

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output missing %q in:\n%s", needle, haystack)
	}
}
