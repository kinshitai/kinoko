package extraction

import (
	"context"
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestStage3Critic(t *testing.T) {
	tests := []struct {
		name        string
		llmResponse string
		llmErr      error
		stage2      *model.Stage2Result
		content     []byte
		wantPassed  *bool
		wantVerdict string
		wantErr     bool
		checkResult func(t *testing.T, r *model.Stage3Result)
	}{
		{
			name: "extract verdict with high scores", llmResponse: extractVerdictJSON(),
			stage2: passingStage2(), content: []byte("meaningful session content"),
			wantPassed: boolPtr(true), wantVerdict: "extract",
		},
		{
			name: "reject verdict with low scores", llmResponse: rejectVerdictJSON(),
			stage2: passingStage2(), content: []byte("trivial session"),
			wantPassed: boolPtr(false), wantVerdict: "reject",
		},
		{
			name: "extract with flags", llmResponse: extractVerdictWithFlags(true, true, false),
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(true),
			checkResult: func(t *testing.T, r *model.Stage3Result) {
				if !r.ReusablePattern {
					t.Error("expected ReusablePattern=true")
				}
				if !r.ExplicitReasoning {
					t.Error("expected ExplicitReasoning=true")
				}
				if r.ContradictsBestPractices {
					t.Error("expected ContradictsBestPractices=false")
				}
			},
		},
		{
			name: "response wrapped in json block", llmResponse: "```json\n" + extractVerdictJSON() + "\n```",
			stage2: passingStage2(), content: []byte("session"),
			wantPassed: boolPtr(true), wantVerdict: "extract",
		},
		{
			name: "response with preamble text", llmResponse: "Here is my analysis:\n\n" + extractVerdictJSON(),
			stage2: passingStage2(), content: []byte("session"),
			wantPassed: boolPtr(true), wantVerdict: "extract",
		},
		{
			name: "malformed JSON treated as rejection", llmResponse: "I think this is good {broken",
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
			checkResult: func(t *testing.T, r *model.Stage3Result) {
				if r.CriticVerdict != "reject" {
					t.Errorf("expected reject, got %s", r.CriticVerdict)
				}
			},
		},
		{
			name: "empty LLM response", llmResponse: "",
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
		},
		{
			name: "valid JSON missing required fields", llmResponse: `{"verdict": "extract"}`,
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
		},
		{
			name:        "verdict=extract but all scores are 1",
			llmResponse: contradictoryVerdictJSON("extract", 1, 1, 1, 1, 1, 1, 1),
			stage2:      passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
			checkResult: func(t *testing.T, r *model.Stage3Result) {
				if r.Passed {
					t.Error("should not pass with all-1 scores")
				}
			},
		},
		{
			name:        "verdict=reject but all scores are 5 overrides to extract",
			llmResponse: contradictoryVerdictJSON("reject", 5, 5, 5, 5, 5, 5, 5),
			stage2:      passingStage2(), content: []byte("session"),
			wantPassed: boolPtr(true), wantVerdict: "extract",
		},
		{
			name: "empty reasoning string", llmResponse: verdictWithEmptyReasoning(),
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(true),
		},
		{
			name: "score out of range 47", llmResponse: verdictWithInvalidScore(47),
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
		},
		{
			name: "score zero", llmResponse: verdictWithInvalidScore(0),
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
		},
		{
			name: "score negative", llmResponse: verdictWithInvalidScore(-1),
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
		},
		{
			name: "confidence > 1.0 clamped", llmResponse: verdictWithConfidence(1.5),
			stage2: passingStage2(), content: []byte("session"),
			checkResult: func(t *testing.T, r *model.Stage3Result) {
				if r.RefinedScores.CriticConfidence > 1.0 {
					t.Error("confidence must be clamped to [0, 1]")
				}
			},
		},
		{
			name: "confidence negative clamped", llmResponse: verdictWithConfidence(-0.5),
			stage2: passingStage2(), content: []byte("session"),
			checkResult: func(t *testing.T, r *model.Stage3Result) {
				if r.RefinedScores.CriticConfidence < 0 {
					t.Error("confidence must be clamped to [0, 1]")
				}
			},
		},
		{
			name: "LLM returns error", llmErr: &llm.LLMError{StatusCode: 503, Message: "service unavailable"},
			stage2: passingStage2(), content: []byte("session"), wantErr: true,
		},
		{
			name: "nil stage2 input", llmResponse: extractVerdictJSON(),
			stage2: nil, content: []byte("session"), wantErr: true,
		},
		{
			name: "nil content", llmResponse: extractVerdictJSON(),
			stage2: passingStage2(), content: nil, wantErr: true,
		},
		{
			name: "empty content", llmResponse: extractVerdictJSON(),
			stage2: passingStage2(), content: []byte(""), wantErr: true,
		},
		{
			name: "verdict EXTRACT normalized to lowercase", llmResponse: verdictWithString("EXTRACT"),
			stage2: passingStage2(), content: []byte("session"),
			wantPassed: boolPtr(true), wantVerdict: "extract",
		},
		{
			name: "verdict maybe treated as rejection", llmResponse: verdictWithString("maybe"),
			stage2: passingStage2(), content: []byte("session"), wantPassed: boolPtr(false),
		},
		{
			name: "stage2.Passed=false", llmResponse: extractVerdictJSON(),
			stage2: &model.Stage2Result{Passed: false}, content: []byte("session"), wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var l llm.LLMClient
			if tt.llmErr != nil {
				l = s3errLLM(tt.llmErr)
			} else {
				l = s3okLLM(tt.llmResponse)
			}
			critic := newTestCritic(l)
			result, err := critic.Evaluate(context.Background(), s3testSession(), tt.content, tt.stage2)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantPassed != nil && result.Passed != *tt.wantPassed {
				t.Errorf("passed: got %v, want %v", result.Passed, *tt.wantPassed)
			}
			if tt.wantVerdict != "" && result.CriticVerdict != tt.wantVerdict {
				t.Errorf("verdict: got %q, want %q", result.CriticVerdict, tt.wantVerdict)
			}
			if result.CriticVerdict == "reject" && result.Passed {
				t.Error("Passed=true but verdict=reject")
			}
			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestStage3Critic_SkillMDParsed(t *testing.T) {
	critic := newTestCritic(s3okLLM(extractVerdictJSON()))
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillMD == "" {
		t.Error("expected non-empty SkillMD for extract verdict")
	}
	if !strings.Contains(result.SkillMD, "fix-db-timeout") {
		t.Error("SkillMD should contain the skill name")
	}
}

func TestStage3Critic_SkillMDEmptyOnReject(t *testing.T) {
	critic := newTestCritic(s3okLLM(rejectVerdictJSON()))
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillMD != "" {
		t.Error("SkillMD should be empty for reject verdict")
	}
}

func TestStage3Critic_SkillMDEmptyWhenNotProvided(t *testing.T) {
	critic := newTestCritic(s3okLLM(extractVerdictJSONNoSkillMD()))
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
	if err != nil {
		t.Fatal(err)
	}
	if result.SkillMD != "" {
		t.Error("SkillMD should be empty when not in LLM response")
	}
}

func TestStage3Critic_PassedVerdictConsistency(t *testing.T) {
	for _, tt := range []struct {
		name     string
		response string
		wantPass bool
	}{
		{"extract", extractVerdictJSON(), true},
		{"reject", rejectVerdictJSON(), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			critic := newTestCritic(s3okLLM(tt.response))
			result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed != tt.wantPass {
				t.Errorf("Passed=%v but verdict=%s", result.Passed, result.CriticVerdict)
			}
		})
	}
}

func TestStage3Critic_CompositeScoreRecomputed(t *testing.T) {
	critic := newTestCritic(s3okLLM(extractVerdictJSON()))
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
	if err != nil {
		t.Fatal(err)
	}
	expected := compositeScore(result.RefinedScores)
	if result.RefinedScores.CompositeScore != expected {
		t.Errorf("composite: got %f, want %f", result.RefinedScores.CompositeScore, expected)
	}
}

func TestStage3Critic_Consistency(t *testing.T) {
	critic := newTestCritic(s3okLLM(extractVerdictJSON()))
	var verdicts []string
	for i := 0; i < 10; i++ {
		result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("consistent"), passingStage2())
		if err != nil {
			t.Fatal(err)
		}
		verdicts = append(verdicts, result.CriticVerdict)
	}
	for i := 1; i < len(verdicts); i++ {
		if verdicts[i] != verdicts[0] {
			t.Errorf("inconsistent verdict on call %d: got %s, expected %s", i, verdicts[i], verdicts[0])
		}
	}
}

func TestStage3Critic_ContradictionEdgeCases(t *testing.T) {
	for _, tt := range []struct {
		name        string
		response    string
		wantVerdict string
		wantPassed  bool
	}{
		{"extract with all scores=1 → reject", contradictoryVerdictJSON("extract", 1, 1, 1, 1, 1, 1, 1), "reject", false},
		{"extract with nearly-all scores=1 one=2 → reject", contradictoryVerdictJSON("extract", 1, 1, 1, 1, 1, 1, 2), "reject", false},
		{"reject with all scores=5 → extract", contradictoryVerdictJSON("reject", 5, 5, 5, 5, 5, 5, 5), "extract", true},
		{"reject with all scores=4 → extract", contradictoryVerdictJSON("reject", 4, 4, 4, 4, 4, 4, 4), "extract", true},
		{"reject with mixed high scores one=3 → no override", contradictoryVerdictJSON("reject", 5, 5, 5, 5, 5, 5, 3), "reject", false},
		{"extract with scores=2 average above 1.5 → no override", contradictoryVerdictJSON("extract", 2, 2, 2, 2, 2, 2, 2), "extract", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			critic := newTestCritic(s3okLLM(tt.response))
			result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
			if err != nil {
				t.Fatal(err)
			}
			if result.CriticVerdict != tt.wantVerdict {
				t.Errorf("verdict = %q, want %q", result.CriticVerdict, tt.wantVerdict)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("passed = %v, want %v", result.Passed, tt.wantPassed)
			}
		})
	}
}
