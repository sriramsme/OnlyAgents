package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

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
func NewToolDef(skill SkillName, name, description string, params map[string]any) ToolDef {
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
		Skill:       skill,
		Name:        name,
		Description: description,
		Parameters:  params,
	}
}

func ObjectProp(description string, props map[string]Property, required ...string) Property {
	return Property{
		Type:        "object",
		Description: description,
		Properties:  props,
		Required:    required,
	}
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

func UnmarshalParams[T any](params []byte) (T, error) {
	var input T
	if err := json.Unmarshal(params, &input); err != nil {
		return input, fmt.Errorf("unmarshal params: %w", err)
	}
	return input, nil
}

// ====================
// Schema Builder
// ====================

// SchemaFromStruct builds a JSON Schema map[string]any from a Go struct.
// Supports the following struct tags:
//
//	json:"field_name,omitempty"  — controls field name and whether it's required
//	desc:"..."                   — sets the JSON Schema description
//	enum:"a,b,c"                 — sets allowed values for string fields
func SchemaFromStruct(v any) map[string]any {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	root := buildObjectSchema(t)

	// normalize slices to avoid nil -> null
	normalizeProperty(&root)

	required := root.Required
	if required == nil {
		required = []string{}
	}

	return map[string]any{
		"type":       "object",
		"properties": root.Properties,
		"required":   required,
	}
}

func buildObjectSchema(t reflect.Type) Property {
	props := make(map[string]Property)
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		name, omitempty := parseJSONTag(jsonTag)
		if name == "" {
			name = field.Name
		}

		prop := buildProperty(field.Type, field)
		props[name] = prop

		if !omitempty {
			required = append(required, name)
		}
	}

	return Property{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

func buildProperty(t reflect.Type, field reflect.StructField) Property {
	// Dereference pointers — optional fields can be *string, *int, etc.
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	desc := field.Tag.Get("desc")
	enumTag := field.Tag.Get("enum")

	switch t.Kind() {
	case reflect.String:
		prop := Property{Type: "string", Description: desc}
		if enumTag != "" {
			prop.Enum = strings.Split(enumTag, ",")
		}
		return prop

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Property{Type: "integer", Description: desc}

	case reflect.Float32, reflect.Float64:
		return Property{Type: "number", Description: desc}

	case reflect.Bool:
		return Property{Type: "boolean", Description: desc}

	case reflect.Slice, reflect.Array:
		itemProp := buildProperty(t.Elem(), reflect.StructField{})
		return Property{
			Type:        "array",
			Description: desc,
			Items:       &itemProp,
		}

	case reflect.Struct:
		nested := buildObjectSchema(t)
		nested.Description = desc
		return nested

	case reflect.Map:
		// map[string]any and similar — render as a generic object
		return Property{Type: "object", Description: desc}

	case reflect.Interface:
		// any fields — render as a generic object
		return Property{Type: "object", Description: desc}

	default:
		// Safe fallback
		return Property{Type: "string", Description: desc}
	}
}

func parseJSONTag(tag string) (name string, omitempty bool) {
	parts := strings.Split(tag, ",")
	name = parts[0]
	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}
	return
}

func normalizeProperty(p *Property) {
	if p.Required == nil {
		p.Required = []string{}
	}
	if p.Items != nil {
		normalizeProperty(p.Items)
	}
	for _, child := range p.Properties {
		normalizeProperty(&child)
	}
}

// ====================
// Schema Patch Helpers
// ====================

// InjectEnumOnArrayField constrains the items of a top-level string array
// field in a schema to the provided enum values.
//
// Example: InjectEnumOnArrayField(schema, "capabilities", core.AllCapabilityStrings())
func InjectEnumOnArrayField(schema map[string]any, field string, values []string) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	prop, ok := props[field].(map[string]any)
	if !ok {
		return
	}
	prop["items"] = map[string]any{
		"type": "string",
		"enum": values,
	}
	props[field] = prop
}

// InjectEnumOnNestedArrayField constrains an array field inside an
// array-of-objects field in a schema to the provided enum values.
//
// Example: InjectEnumOnNestedArrayField(schema, "tasks", "required_capabilities", core.AllCapabilityStrings())
func InjectEnumOnNestedArrayField(schema map[string]any, arrayField, nestedField string, values []string) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	arrayProp, ok := props[arrayField].(map[string]any)
	if !ok {
		return
	}
	items, ok := arrayProp["items"].(map[string]any)
	if !ok {
		return
	}
	nestedProps, ok := items["properties"].(map[string]any)
	if !ok {
		return
	}
	nestedProp, ok := nestedProps[nestedField].(map[string]any)
	if !ok {
		return
	}
	nestedProp["items"] = map[string]any{
		"type": "string",
		"enum": values,
	}
	nestedProps[nestedField] = nestedProp
}
