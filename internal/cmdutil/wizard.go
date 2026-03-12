package cmdutil

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleDim    = lipgloss.NewStyle().Faint(true)
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(0, 1)
)

// ── SetupContext ──────────────────────────────────────────────────────────────

// SetupContext is the shared state bag passed through every setup step.
// Steps read from it (to check preconditions) and write to it (to pass
// collected data to subsequent steps).
type SetupContext struct {
	// Populated by BootstrapStep
	Paths *Paths

	// Populated by UserIdentityStep
	UserName          string
	UserPreferredName string
	UserRole          string
	UserTimezone      string

	// Populated by VaultStep
	EnvFilePath string
	EnvVars     map[string]string // accumulated key=value pairs for .env

	// Populated by LLMStep — provider choice per agent slot
	// key = agent id, value = LLMChoice
	LLMChoices map[string]LLMChoice

	// Populated by ChannelStep
	ChannelChoice string // "oachannel" | "telegram" | ...

	// Populated by AuthStep
	AuthPassword string

	// Writer for all output — defaults to os.Stdout
	Out io.Writer
}

// LLMChoice holds what the user picked for a single agent's LLM config.
type LLMChoice struct {
	Provider    string
	Model       string
	APIKeyVault string // vault path, e.g. "anthropic/api_key"
	EnvVarName  string // e.g. "ANTHROPIC_API_KEY"
}

// Paths mirrors bootstrap.Paths — defined here to avoid circular imports.
// The bootstrap step populates this from the real bootstrap.Init() result.
type Paths struct {
	Home       string
	Agents     string
	Channels   string
	Connectors string
	Skills     string
	Councils   string
	DBPath     string
	UserPath   string
	VaultPath  string
	EnvPath    string
}

func NewSetupContext() *SetupContext {
	return &SetupContext{
		EnvVars:    make(map[string]string),
		LLMChoices: make(map[string]LLMChoice),
		Out:        os.Stdout,
	}
}

// ── SetupStep ─────────────────────────────────────────────────────────────────

// SetupStep is the interface every setup step implements.
type SetupStep interface {
	// Name is the short display name shown in the step header.
	Name() string
	// Description is one line explaining what this step does.
	Description() string
	// IsDone returns true if this step is already complete — used to offer
	// skipping on re-runs.
	IsDone(ctx *SetupContext) bool
	// Run executes the step, reading from and writing to ctx.
	Run(ctx *SetupContext) error
}

// ── SetupRunner ───────────────────────────────────────────────────────────────

// SetupRunner walks a list of SetupSteps in order, handling skip logic and
// rendering the step UI.
type SetupRunner struct {
	steps []SetupStep
	ctx   *SetupContext
}

func NewSetupRunner(steps []SetupStep, ctx *SetupContext) *SetupRunner {
	return &SetupRunner{steps: steps, ctx: ctx}
}

func (r *SetupRunner) Run() error {
	out := r.ctx.Out

	printBanner(out)

	total := len(r.steps)
	for i, step := range r.steps {
		printStepHeader(out, i+1, total, step.Name(), step.Description())

		if step.IsDone(r.ctx) {
			skip := true
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("%s is already configured. Skip?", step.Name())).
						Value(&skip),
				),
			).Run()
			if err != nil {
				return fmt.Errorf("step %d (%s): %w", i+1, step.Name(), err)
			}
			if skip {
				fmt.Fprintln(out, styleGreen.Render("  ✓ skipped"))
				fmt.Fprintln(out)
				continue
			}
		}

		if err := step.Run(r.ctx); err != nil {
			return fmt.Errorf("step %d (%s): %w", i+1, step.Name(), err)
		}

		fmt.Fprintln(out, styleGreen.Render("  ✓ done"))
		fmt.Fprintln(out)
	}

	printSummary(out, r.ctx)
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func printBanner(w io.Writer) {
	banner := styleBorder.Render(
		styleHeader.Render("OnlyAgents Setup") + "\n" +
			styleDim.Render("Let's get your agent system up and running."),
	)
	fmt.Fprintln(w)
	fmt.Fprintln(w, banner)
	fmt.Fprintln(w)
}

func printStepHeader(w io.Writer, current, total int, name, desc string) {
	progress := styleDim.Render(fmt.Sprintf("[%d/%d]", current, total))
	title := styleBold.Render(name)
	fmt.Fprintf(w, "%s %s\n", progress, title)
	fmt.Fprintln(w, styleDim.Render("    "+desc))
	fmt.Fprintln(w)
}

func printSummary(w io.Writer, ctx *SetupContext) {
	lines := []string{
		styleHeader.Render("Setup complete. Here's what was configured:"),
		"",
	}

	if ctx.UserName != "" {
		lines = append(lines, fmt.Sprintf("  User        %s (%s)", ctx.UserName, ctx.UserPreferredName))
	}
	if ctx.EnvFilePath != "" {
		lines = append(lines, fmt.Sprintf("  Vault       %s", ctx.EnvFilePath))
	}
	if len(ctx.LLMChoices) > 0 {
		for id, c := range ctx.LLMChoices {
			lines = append(lines, fmt.Sprintf("  LLM %-10s %s / %s", id, c.Provider, c.Model))
		}
	}
	if ctx.ChannelChoice != "" {
		lines = append(lines, fmt.Sprintf("  Channel     %s", ctx.ChannelChoice))
	}
	if ctx.AuthPassword != "" {
		lines = append(lines, ("  Auth        password set"))
	}

	lines = append(lines, "")
	lines = append(lines, styleYellow.Render("  Run: onlyagents server start"))

	fmt.Fprintln(w, styleBorder.Render(strings.Join(lines, "\n")))
	fmt.Fprintln(w)
}
