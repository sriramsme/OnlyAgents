// pkg/skills/runner/connector.go
package runner

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sriramsme/OnlyAgents/pkg/tools"
)

type ConnectorEntry[T any] struct {
	Proto   any
	Factory func(cfg any) (T, error)
}

type ConnectorRegistry[T any] struct {
	entries map[string]ConnectorEntry[T]
	def     string // default connector name
}

func NewConnectorRegistry[T any](defaultConnector string) *ConnectorRegistry[T] {
	return &ConnectorRegistry[T]{
		entries: make(map[string]ConnectorEntry[T]),
		def:     defaultConnector,
	}
}

func (r *ConnectorRegistry[T]) Register(name string, proto any, factory func(any) (T, error)) {
	r.entries[name] = ConnectorEntry[T]{Proto: proto, Factory: factory}
}

func (r *ConnectorRegistry[T]) Build(name string, root *cobra.Command) (T, error) {
	var zero T
	entry, ok := r.entries[name]
	if !ok {
		return zero, fmt.Errorf("unknown connector %q — rebuild with appropriate build tag", name)
	}
	cfg, err := buildConnectorConfig(name, entry.Proto, root)
	if err != nil {
		return zero, err
	}
	return entry.Factory(cfg)
}

// RegisterFlags adds --connector flag and all connector-specific prefixed flags.
func (r *ConnectorRegistry[T]) RegisterFlags(root *cobra.Command) {
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	root.PersistentFlags().String("connector", r.def,
		fmt.Sprintf("connector: %s", strings.Join(names, ", ")))

	for connName, entry := range r.entries {
		if entry.Proto == nil {
			continue
		}
		t := reflect.TypeOf(entry.Proto)
		for i := range t.NumField() {
			f := t.Field(i)
			meta := tools.ParseFieldMeta(f)
			if meta.JSONName == "" || meta.JSONName == "-" {
				continue
			}
			flagName := connName + "-" + meta.JSONName
			desc := meta.SchemaDesc
			if meta.CLIHelp != "" {
				desc += " (" + meta.CLIHelp + ")"
			}
			root.PersistentFlags().String(flagName, "", desc)
		}
	}
}

func buildConnectorConfig(connName string, proto any, root *cobra.Command) (any, error) {
	if proto == nil {
		return nil, nil
	}
	t := reflect.TypeOf(proto)
	raw := make(map[string]string, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		name := jsonFieldName(f)
		if name == "" || name == "-" {
			continue
		}
		val, err := root.PersistentFlags().GetString(connName + "-" + name)
		if err != nil {
			return nil, err
		}
		if val != "" {
			raw[name] = val
		}
	}
	return MarshalConfig(proto, raw)
}

// MarshalConfig builds a typed config from raw string flag values.
func MarshalConfig(proto any, raw map[string]string) (any, error) {
	b, err := marshalPayload(proto, raw)
	if err != nil {
		return nil, err
	}
	ptr := reflect.New(reflect.TypeOf(proto))
	if err := json.Unmarshal(b, ptr.Interface()); err != nil {
		return nil, err
	}
	return ptr.Elem().Interface(), nil
}
