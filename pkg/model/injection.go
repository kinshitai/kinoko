package model

// InjectionRequest is the input to the Injector.
type InjectionRequest struct {
	Prompt     string
	LibraryIDs []string
	MaxSkills  int
	SessionID  string
}

// InjectionResponse is the output of the Injector.
type InjectionResponse struct {
	Skills         []InjectedSkill
	Classification PromptClassification
}

// InjectedSkill wraps a scored skill with the rank position used in injection.
type InjectedSkill struct {
	SkillID        string
	PatternOverlap float64
	CosineSim      float64
	HistoricalRate float64
	CompositeScore float64
	RankPosition   int
}

// PromptClassification describes how a prompt was parsed for injection.
type PromptClassification struct {
	Intent   string
	Domain   string
	Patterns []string
}
