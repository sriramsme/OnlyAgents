package cmdutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
)

// ── Form theme ────────────────────────────────────────────────────────────────

// Theme returns a consistent huh theme across all setup forms.
func Theme() *huh.Theme {
	return huh.ThemeCatppuccin()
}

// ── Common field constructors ─────────────────────────────────────────────────

// InputField returns a styled huh.Input with a title and optional placeholder.
func InputField(title, placeholder string, value *string) *huh.Input {
	f := huh.NewInput().
		Title(title).
		Value(value)
	if placeholder != "" {
		f = f.Placeholder(placeholder)
	}
	return f
}

// RequiredInput returns an InputField that rejects empty values.
func RequiredInput(title, placeholder string, value *string) *huh.Input {
	return InputField(title, placeholder, value).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("required")
			}
			return nil
		})
}

// SecretInput returns an Input that masks the value (for API keys, passwords).
func SecretInput(title string, value *string) *huh.Input {
	return huh.NewInput().
		Title(title).
		EchoMode(huh.EchoModePassword).
		Value(value).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("required")
			}
			return nil
		})
}

// SelectField returns a huh.Select with options built from a string slice.
func SelectField[T comparable](title string, options []huh.Option[T], value *T) *huh.Select[T] {
	return huh.NewSelect[T]().
		Title(title).
		Options(options...).
		Value(value)
}

// ConfirmField returns a huh.Confirm with a title and default value.
func ConfirmField(title string, value *bool) *huh.Confirm {
	return huh.NewConfirm().
		Title(title).
		Value(value)
}

// ── Form runner ───────────────────────────────────────────────────────────────

// RunForm runs a huh form and returns any error, unwrapping ErrUserAborted
// into a clean message.
func RunForm(groups ...*huh.Group) error {
	return huh.NewForm(groups...).
		WithTheme(Theme()).
		Run()
}

// ── Output helpers ────────────────────────────────────────────────────────────

// Info prints a dim informational line.
func Info(format string, args ...any) {
	fmt.Fprintln(os.Stdout, styleDim.Render("  "+fmt.Sprintf(format, args...)))
}

// Hint prints a boxed hint block — use before a form to give the user context.
func Hint(lines ...string) {
	body := strings.Join(lines, "\n")
	fmt.Fprintln(os.Stdout, styleBorder.Render(styleDim.Render(body)))
	fmt.Fprintln(os.Stdout)
}

// Success prints a green success line.
func Success(format string, args ...any) {
	fmt.Fprintln(os.Stdout, styleGreen.Render("  ✓ "+fmt.Sprintf(format, args...)))
}

// Warn prints a yellow warning line.
func Warn(format string, args ...any) {
	fmt.Fprintln(os.Stdout, styleYellow.Render("  ! "+fmt.Sprintf(format, args...)))
}

// Section prints a bold section title — for use inside a step's Run().
func Section(title string) {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, styleBold.Render("  "+title))
}
