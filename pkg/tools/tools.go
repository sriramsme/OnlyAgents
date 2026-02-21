package tools

import (
	"encoding/json"
	"fmt"
)

// ToolDef defines a tool/function that can be called by the LLM.
// This is the schema that gets sent to the LLM in function calling.
type ToolDef struct {
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

// ====================
// Helper Functions
// ====================

// ParseArguments parses the JSON arguments string into a map
func ParseArguments(args string) (map[string]any, error) {
	if args == "" {
		return make(map[string]any), nil
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(args), &result); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	return result, nil
}

// MustParseArguments is like ParseArguments but panics on error
func MustParseArguments(args string) map[string]any {
	result, err := ParseArguments(args)
	if err != nil {
		panic(err)
	}
	return result
}

// FormatResult serializes a tool result to JSON string
func FormatResult(result any) string {
	if result == nil {
		return "{}"
	}

	// If already a string, return as-is
	if s, ok := result.(string); ok {
		return s
	}

	// Otherwise, marshal to JSON
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to serialize result: %s"}`, err)
	}
	return string(data)
}

// ====================
// Builder Helpers
// ====================

// NewToolDef creates a new tool definition with basic validation
func NewToolDef(name, description string, params map[string]any) ToolDef {
	// Ensure params has required structure
	if params == nil {
		params = make(map[string]any)
	}
	if _, ok := params["type"]; !ok {
		params["type"] = "object"
	}
	if _, ok := params["properties"]; !ok {
		params["properties"] = make(map[string]any)
	}

	return ToolDef{
		Name:        name,
		Description: description,
		Parameters:  params,
	}
}

// Property is a helper for building tool parameters
type Property struct {
	Type        string    `json:"type"`
	Description string    `json:"description,omitempty"`
	Enum        []string  `json:"enum,omitempty"`
	Items       *Property `json:"items,omitempty"` // For arrays
	Default     any       `json:"default,omitempty"`
}

// StringProp creates a string property
func StringProp(description string) Property {
	return Property{Type: "string", Description: description}
}

// IntProp creates an integer property
func IntProp(description string) Property {
	return Property{Type: "integer", Description: description}
}

// BoolProp creates a boolean property
func BoolProp(description string) Property {
	return Property{Type: "boolean", Description: description}
}

// ArrayProp creates an array property
func ArrayProp(description string, items Property) Property {
	return Property{Type: "array", Description: description, Items: &items}
}

// EnumProp creates an enum string property
func EnumProp(description string, values []string) Property {
	return Property{Type: "string", Description: description, Enum: values}
}

// BuildParams is a helper for building tool parameters
func BuildParams(properties map[string]Property, required []string) map[string]any {
	props := make(map[string]any)
	for name, prop := range properties {
		props[name] = prop
	}

	params := map[string]any{
		"type":       "object",
		"properties": props,
	}

	if len(required) > 0 {
		params["required"] = required
	}

	return params
}

// ====================
// Validation
// ====================

// ValidateToolDef checks if a tool definition is valid
func ValidateToolDef(tool ToolDef) error {
	if tool.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if tool.Description == "" {
		return fmt.Errorf("tool description is required")
	}
	if tool.Parameters == nil {
		return fmt.Errorf("tool parameters are required")
	}

	// Check parameters structure
	if typ, ok := tool.Parameters["type"].(string); !ok || typ != "object" {
		return fmt.Errorf("parameters must have type: object")
	}
	if _, ok := tool.Parameters["properties"]; !ok {
		return fmt.Errorf("parameters must have properties field")
	}

	return nil
}

// ValidateToolCall checks if a tool call is valid
func ValidateToolCall(call ToolCall) error {
	if call.ID == "" {
		return fmt.Errorf("tool call ID is required")
	}
	if call.Function.Name == "" {
		return fmt.Errorf("function name is required")
	}
	// Arguments can be empty for tools with no parameters
	return nil
}
