package tools

// ToolDef defines a tool/function that can be called by the LLM.
// This is the schema that gets sent to the LLM in function calling.
type ToolDef struct {
	// Skill is the name of the skill that defines this tool
	Skill string `json:"-"` // internal only, never sent to LLM

	// Name of the tool (e.g., "email_send", "calendar_create_event")
	Name string `json:"name"`

	// Description of what the tool does (helps LLM decide when to use it)
	Description string `json:"description"`

	// Parameters is JSON Schema defining the tool's input
	// Must include "type": "object" and "properties"
	Parameters map[string]any `json:"parameters"`

	// Optional: Whether this tool requires user confirmation before execution
	RequiresConfirmation bool `json:"requires_confirmation,omitempty"`
}

// ToolCall represents a tool call request from the LLM.
// The LLM returns this when it wants to execute a tool.
type ToolCall struct {
	// ID is a unique identifier for this tool call (from LLM response)
	// Must be echoed back in the tool result
	ID string `json:"id"`

	// Type is always "function" for now (OpenAI/Anthropic convention)
	Type string `json:"type"`

	// Function contains the actual tool call details
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function being called
type FunctionCall struct {
	// Name of the tool to execute (matches ToolDef.Name)
	Name string `json:"name"`

	// Arguments is a JSON string containing the tool parameters
	// Must be parsed into map[string]any for execution
	Arguments string `json:"arguments"`
}

// ToolResult represents the result of a tool execution.
// This gets sent back to the LLM after a tool is executed.
type ToolResult struct {
	// ToolCallID echoes the ID from the ToolCall
	ToolCallID string `json:"tool_use_id"` // Anthropic uses "tool_use_id"

	// Content is the result of the tool execution
	// Can be a string, map, or any JSON-serializable value
	Content any `json:"content"`

	// IsError indicates if the tool execution failed
	IsError bool `json:"is_error,omitempty"`
}

// Property is a helper for building tool parameters
type Property struct {
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	Enum        []string            `json:"enum,omitempty"`
	Items       *Property           `json:"items,omitempty"` // For arrays
	Properties  map[string]Property `json:"properties,omitempty"`
	Required    []string            `json:"required,omitempty"`
	Default     any                 `json:"default,omitempty"`
}
