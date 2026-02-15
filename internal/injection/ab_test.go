package injection

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/mycelium-dev/mycelium/internal/model"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

type recordingEventWriter struct {
	events []storage.InjectionEventRecord
}

func (r *recordingEventWriter) WriteInjectionEvent(_ context.Context, ev storage.InjectionEventRecord) error {
	r.events = append(r.events, ev)
	return nil
}

type stubInjector struct {
	resp *model.InjectionResponse
	err  error
	calls atomic.Int32
}

func (s *stubInjector) Inject(_ context.Context, _ model.InjectionRequest) (*model.InjectionResponse, error) {
	s.calls.Add(1)
	return s.resp, s.err
}

func TestABAssignmentDistribution(t *testing.T) {
	inner := &stubInjector{
		resp: &model.InjectionResponse{
			Skills: []model.InjectedSkill{
				{SkillID: "sk1", CompositeScore: 0.9, RankPosition: 1},
			},
		},
	}
	writer := &recordingEventWriter{}
	config := ABConfig{Enabled: true, ControlRatio: 0.3, MinSampleSize: 100}

	callIdx := 0
	// Deterministic sequence: 0.0, 0.1, 0.2, ..., 0.9 repeated
	ab := NewABInjector(inner, writer, config, slog.Default())
	ab.SetRandFunc(func() float64 {
		v := float64(callIdx%10) / 10.0
		callIdx++
		return v
	})

	controlCount := 0
	treatmentCount := 0
	n := 100

	for i := 0; i < n; i++ {
		req := model.InjectionRequest{
			SessionID: "sess-" + string(rune('a'+i%26)),
			Prompt:    "test",
		}
		resp, err := ab.Inject(context.Background(), req)
		if err != nil {
			t.Fatalf("inject %d: %v", i, err)
		}
		// With our deterministic func: values 0.0, 0.1, 0.2 are < 0.3 → control
		// Values 0.3..0.9 → treatment
		if resp.Skills == nil {
			controlCount++
		} else {
			treatmentCount++
		}
	}

	// 3 out of every 10 should be control
	expectedControl := 30
	if controlCount != expectedControl {
		t.Errorf("control count: got %d, want %d", controlCount, expectedControl)
	}
	if treatmentCount != n-expectedControl {
		t.Errorf("treatment count: got %d, want %d", treatmentCount, n-expectedControl)
	}
}

func TestABDisabled(t *testing.T) {
	inner := &stubInjector{
		resp: &model.InjectionResponse{
			Skills: []model.InjectedSkill{{SkillID: "sk1"}},
		},
	}
	writer := &recordingEventWriter{}
	config := ABConfig{Enabled: false}

	ab := NewABInjector(inner, writer, config, nil)
	resp, err := ab.Inject(context.Background(), model.InjectionRequest{Prompt: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(resp.Skills))
	}
	// No AB events should be written (disabled mode delegates directly).
	if len(writer.events) != 0 {
		t.Errorf("expected 0 ab events, got %d", len(writer.events))
	}
}

func TestABControlGroupNoDelivery(t *testing.T) {
	inner := &stubInjector{
		resp: &model.InjectionResponse{
			Skills: []model.InjectedSkill{
				{SkillID: "sk1", CompositeScore: 0.8, RankPosition: 1},
				{SkillID: "sk2", CompositeScore: 0.5, RankPosition: 2},
			},
			Classification: model.PromptClassification{Intent: "BUILD"},
		},
	}
	writer := &recordingEventWriter{}
	config := ABConfig{Enabled: true, ControlRatio: 0.5, MinSampleSize: 10}

	ab := NewABInjector(inner, writer, config, nil)
	// Force control.
	ab.SetRandFunc(func() float64 { return 0.1 })

	resp, err := ab.Inject(context.Background(), model.InjectionRequest{
		SessionID: "sess-ctrl",
		Prompt:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Control: no skills delivered.
	if resp.Skills != nil {
		t.Errorf("expected nil skills for control, got %d", len(resp.Skills))
	}
	// Classification still returned.
	if resp.Classification.Intent != "BUILD" {
		t.Errorf("expected classification preserved")
	}
	// Events logged with delivered=false.
	if len(writer.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(writer.events))
	}
	for _, ev := range writer.events {
		if ev.ABGroup != "control" {
			t.Errorf("expected control group, got %s", ev.ABGroup)
		}
		if ev.Delivered {
			t.Error("expected delivered=false for control")
		}
	}
}

func TestABTreatmentGroupDelivers(t *testing.T) {
	inner := &stubInjector{
		resp: &model.InjectionResponse{
			Skills: []model.InjectedSkill{{SkillID: "sk1", RankPosition: 1}},
		},
	}
	writer := &recordingEventWriter{}
	config := ABConfig{Enabled: true, ControlRatio: 0.5, MinSampleSize: 10}

	ab := NewABInjector(inner, writer, config, nil)
	ab.SetRandFunc(func() float64 { return 0.9 }) // > 0.5 → treatment

	resp, err := ab.Inject(context.Background(), model.InjectionRequest{
		SessionID: "sess-treat",
		Prompt:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(resp.Skills))
	}
	if len(writer.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(writer.events))
	}
	if writer.events[0].ABGroup != "treatment" {
		t.Errorf("expected treatment, got %s", writer.events[0].ABGroup)
	}
	if !writer.events[0].Delivered {
		t.Error("expected delivered=true for treatment")
	}
}
