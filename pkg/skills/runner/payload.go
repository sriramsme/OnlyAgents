// pkg/skills/runner/payload.go
package runner

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// buildPayload reads cobra flags and converts them to a JSON payload
// using autoTransform to handle complex types.
func buildPayload(cmd *cobra.Command, proto any, args []string) ([]byte, error) {
	t := reflect.TypeOf(proto)
	raw := make(map[string]string, t.NumField())

	// positional arg → primary field
	if posField, ok := cmd.Annotations["pos_field"]; ok && len(args) > 0 {
		val, err := cmd.Flags().GetString(posField)
		if err != nil {
			return nil, err
		}
		if val == "" {
			raw[posField] = args[0]
		}
	}

	for i := range t.NumField() {
		f := t.Field(i)
		name := jsonFieldName(f)
		if name == "" || name == "-" {
			continue
		}
		val, err := cmd.Flags().GetString(name)
		if err != nil || val == "" {
			continue
		}
		raw[name] = val
	}

	return marshalPayload(proto, raw)
}

// marshalPayload transforms raw string flag values into properly typed JSON.
func marshalPayload(proto any, raw map[string]string) ([]byte, error) {
	rt := reflect.TypeOf(proto)
	data := make(map[string]any, rt.NumField())

	for i := range rt.NumField() {
		field := rt.Field(i)
		name := jsonFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		val, ok := raw[name]
		if !ok || val == "" {
			continue
		}
		parsed, err := autoTransform(field.Type, val)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", name, err)
		}
		data[name] = parsed
	}

	return json.Marshal(data)
}

// nolint:gocyclo
func autoTransform(fieldType reflect.Type, raw string) (any, error) {
	switch fieldType.Kind() {
	case reflect.Slice:
		elem := fieldType.Elem()
		switch elem.Kind() {
		case reflect.Struct:
			if strings.HasPrefix(strings.TrimSpace(raw), "[") {
				var items []map[string]any
				if err := json.Unmarshal([]byte(raw), &items); err != nil {
					return nil, fmt.Errorf("expected JSON array: %w", err)
				}
				for _, item := range items {
					for j := range elem.NumField() {
						f := elem.Field(j)
						name := jsonFieldName(f)
						val, ok := item[name]
						if !ok {
							continue
						}
						strVal, isStr := val.(string)
						if !isStr {
							continue
						}
						transformed, err := autoTransform(f.Type, strVal)
						if err != nil {
							return nil, fmt.Errorf("%s: %w", name, err)
						}
						item[name] = transformed
					}
				}
				b, err := json.Marshal(items)
				if err != nil {
					return nil, err
				}
				ptr := reflect.New(fieldType)
				if err := json.Unmarshal(b, ptr.Interface()); err != nil {
					return nil, err
				}
				return ptr.Elem().Interface(), nil
			}
			// shorthand: bare string → set Title field
			slice := reflect.MakeSlice(fieldType, 0, 1)
			item := reflect.New(elem).Elem()
			if f := item.FieldByName("Title"); f.IsValid() && f.CanSet() {
				f.SetString(raw)
			}
			return reflect.Append(slice, item).Interface(), nil

		case reflect.String:
			parts := strings.Split(raw, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return parts, nil
		}

	case reflect.Bool:
		return strconv.ParseBool(raw)

	case reflect.Int, reflect.Int64:
		return strconv.ParseInt(raw, 10, 64)

	case reflect.Float64:
		return strconv.ParseFloat(raw, 64)
	}

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed, nil
	}
	return raw, nil
}
