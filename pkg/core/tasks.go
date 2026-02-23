package core

// ExecutiveAnalysis represents the executive agent's task analysis
type ExecutiveAnalysis struct {
	// Task breakdown
	Tasks []Task `json:"tasks"`

	// Capabilities needed across all tasks
	RequiredCapabilities []Capability `json:"required_capabilities"`

	// Suggested agent routing
	RoutingDecision RoutingDecision `json:"routing_decision"`
}

// Task represents a decomposed unit of work
type Task struct {
	ID                string       `json:"id"`           // Unique task ID
	Description       string       `json:"description"`  // What needs to be done
	Capabilities      []Capability `json:"capabilities"` // Required capabilities
	DependsOn         []string     `json:"depends_on"`   // Task IDs this depends on
	Priority          int          `json:"priority"`     // Higher = more important
	EstimatedDuration string       `json:"estimated_duration,omitempty"`
}

// RoutingDecision tells kernel how to route
type RoutingDecision struct {
	Strategy     RoutingStrategy `json:"strategy"`      // specialized, general, parallel
	AgentID      string          `json:"agent_id"`      // Specific agent (if specialized)
	TaskSequence []TaskRouting   `json:"task_sequence"` // For complex workflows
}

type RoutingStrategy string

const (
	RoutingSpecialized RoutingStrategy = "specialized" // Use specialized agent
	RoutingGeneral     RoutingStrategy = "general"     // Use general agent with dynamic tools
	RoutingParallel    RoutingStrategy = "parallel"    // Execute multiple tasks in parallel
	RoutingSequential  RoutingStrategy = "sequential"  // Execute tasks one by one
)

// TaskRouting maps a task to an agent
type TaskRouting struct {
	TaskID  string   `json:"task_id"`
	AgentID string   `json:"agent_id"`
	Tools   []string `json:"tools,omitempty"` // Optional: specific tools to enable
}
