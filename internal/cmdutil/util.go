package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// ── Styles (shared across all cmdutil) ────────────────────────────────────────

var (
	StyleBold   = lipgloss.NewStyle().Bold(true)
	StyleGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	StyleYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	StyleRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	StyleDim    = lipgloss.NewStyle().Faint(true)
	StyleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(0, 1)
)

// ── Generic dir loader ────────────────────────────────────────────────────────

// LoadDir reads all *.yaml files from dir and unmarshals each into T.
// Files that fail to parse are skipped with a warning — a single bad file
// does not abort the whole load.
func LoadDir[T any](dir string) ([]T, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", dir, err)
	}
	var results []T
	for _, path := range entries {
		clean := filepath.Clean(path)
		data, err := os.ReadFile(clean) //nolint:gosec
		if err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not read %s: %v\n"), path, err)
			continue
		}
		// First unmarshal into raw map
		var raw map[string]any
		if err := yaml.Unmarshal(data, &raw); err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not parse %s: %v\n"), path, err)
			continue
		}
		// Then decode via mapstructure to respect mapstructure tags
		var v T
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &v,
			TagName:          "mapstructure",
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			),
		})
		if err != nil {
			continue
		}
		if err := decoder.Decode(raw); err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not decode %s: %v\n"), path, err)
			continue
		}
		results = append(results, v)
	}
	return results, nil
}

// WriteYAML writes v to path as YAML with 2-space indentation.
// Creates or truncates the file. Permissions: 0600.
func WriteYAML(path string, v any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not close %s: %v\n"), path, err)
		}
	}()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	return enc.Encode(v)
}

// ReadYAML reads path and unmarshals into v.
func ReadYAML(path string, v any) error {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, v)
}

// AppendEnvVar appends KEY=value to the .env file at path,
// skipping if the key already exists.
func AppendEnvVar(envPath, vaultPath, value string) error {
	// telegram/bot_token → TELEGRAM_BOT_TOKEN
	envKey := strings.ToUpper(strings.ReplaceAll(vaultPath, "/", "_"))

	data, err := os.ReadFile(envPath) //nolint:gosec
	if err != nil {
		return err
	}
	if strings.Contains(string(data), envKey+"=") {
		return nil // already set
	}
	f, err := os.OpenFile(envPath, os.O_APPEND|os.O_WRONLY, 0600) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not close %s: %v\n"), envPath, err)
		}
	}()
	_, err = fmt.Fprintf(f, "%s=%s\n", envKey, value)
	return err
}

// ViewResource prints a resource config. If field is set, prints just that
// field's value (useful for scripting). If raw is set, dumps the YAML file.
func ViewResource(path string, v any, field string, raw bool) error {
	if raw {
		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	}
	if field != "" {
		// Marshal to map, look up field
		data, err := yaml.Marshal(v)
		if err != nil {
			return err
		}
		var m map[string]any
		if err := yaml.Unmarshal(data, &m); err != nil {
			return err
		}
		if val, ok := m[field]; ok {
			fmt.Println(val)
			return nil
		}
		return fmt.Errorf("field %q not found", field)
	}
	// Default: pretty-print as YAML
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Print(StyleBorder.Render(string(data)))
	return nil
}

// ── Table helpers ─────────────────────────────────────────────────────────────

func YesNo(b bool) string {
	if b {
		return StyleGreen.Render("✓")
	}
	return StyleDim.Render("✗")
}

func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func EnabledLabel(b bool) string {
	if b {
		return StyleGreen.Render("enabled")
	}
	return StyleDim.Render("disabled")
}
