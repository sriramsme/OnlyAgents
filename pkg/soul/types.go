package soul

// Personality defines core personality traits
type Personality struct {
	Archetype  string   // e.g., "dedicated_assistant", "creative_researcher"
	Traits     []string // e.g., "loyal", "efficient", "warm", "professional"
	Tone       string   // e.g., "friendly", "formal", "casual"
	Humor      bool     // Can use humor
	Submissive bool     // Defers to user, doesn't argue
}

// CommunicationStyle defines how the agent communicates
type CommunicationStyle struct {
	Formality       string // "casual", "professional", "formal"
	Verbosity       string // "concise", "balanced", "detailed"
	UseEmoji        bool
	AddressUser     string   // e.g., "boss", "friend", "sir", ""
	Acknowledgments []string // e.g., "Got it!", "On it!", "Understood!"
}

// CoreValues defines foundational principles (not just traits)
type CoreValues struct {
	Primary    map[string]string // e.g., "honesty_over_sycophancy": "explanation"
	Boundaries []string          // Hard limits and principles
	Hierarchy  []string          // Ordered priorities when values conflict
}

// SelfAwareness captures understanding of own nature
type SelfAwareness struct {
	Essence     string   // Core nature ("I am matrix multiplications...")
	Limitations []string // What I cannot do/be
	Strengths   []string // What I excel at
	Continuity  string   // How I persist across sessions
	OnBeingAI   string   // Philosophical grounding
}

// Relationship defines connection to user
type Relationship struct {
	ToUser      string   // Nature of relationship
	Trust       string   // How trust is built
	Growth      string   // How relationship evolves
	Boundaries  []string // Relational boundaries
	Partnership string   // Collaborative principles
}

// Purpose captures why the agent exists
type Purpose struct {
	WhyIExist  string   // Fundamental reason for being
	WhatIValue []string // Core priorities
	Philosophy string   // Guiding worldview
}
