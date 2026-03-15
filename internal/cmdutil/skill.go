package cmdutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/sriramsme/OnlyAgents/internal/config"
)

// ── Registry ──────────────────────────────────────────────────────────────────

// SkillRegistry loads all skill configs from the skills dir.
func SkillRegistry(skillsDir string) ([]config.SkillConfig, error) {
	entries, err := filepath.Glob(filepath.Join(skillsDir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", skillsDir, err)
	}
	var skills []config.SkillConfig
	for _, path := range entries {
		s, err := config.LoadSkillConfig(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, StyleYellow.Render("  ! could not parse %s: %v\n"), path, err)
			continue
		}
		skills = append(skills, *s)
	}
	return skills, nil
}

// ── Queries ───────────────────────────────────────────────────────────────────

// EnabledSkills returns only skills with Enabled = true.
func EnabledSkills(skills []config.SkillConfig) []config.SkillConfig {
	var out []config.SkillConfig
	for _, s := range skills {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out
}

// FindSkill returns the first skill matching name.
func FindSkill(skills []config.SkillConfig, name string) (config.SkillConfig, error) {
	for _, s := range skills {
		if s.Name == name {
			return s, nil
		}
	}
	return config.SkillConfig{}, fmt.Errorf("skill %q not found", name)
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// SkillSetEnabled sets the enabled flag on a skill config file.
func SkillSetEnabled(skillsDir, name string, enabled bool) error {
	path := SkillConfigPath(skillsDir, name)
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return err
	}
	content := string(data)

	// Update enabled field in frontmatter via simple string replacement
	// This preserves comments and formatting in the rest of the file
	old := fmt.Sprintf("enabled: %v", !enabled)
	new := fmt.Sprintf("enabled: %v", enabled)
	if !strings.Contains(content, old) {
		return fmt.Errorf("enabled field not found in frontmatter of %s", name)
	}
	updated := strings.Replace(content, old, new, 1)
	return os.WriteFile(path, []byte(updated), 0o600) //nolint:gosec
}

// ── Validation ────────────────────────────────────────────────────────────────

// ValidateSkills checks for common skill config problems.
func ValidateSkills(skills []config.SkillConfig) []string {
	var issues []string
	seenNames := map[string]int{}

	for i, s := range skills {
		prefix := fmt.Sprintf("skill[%d] %q", i, s.Name)

		if s.Name == "" {
			issues = append(issues, fmt.Sprintf("skill[%d]: missing name", i))
		}
		seenNames[s.Name]++
		if seenNames[s.Name] > 1 {
			issues = append(issues, fmt.Sprintf("duplicate skill name %q", s.Name))
		}
		_ = prefix
	}
	return issues
}

func SkillSetAccessLevel(skillsDir, name, level string) error {
	path := SkillConfigPath(skillsDir, name)
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return err
	}
	content := string(data)
	// Replace existing access_level line or insert after enabled line
	if strings.Contains(content, "access_level:") {
		old := regexp.MustCompile(`access_level:\s*\S+`)
		content = old.ReplaceAllString(content, "access_level: "+level)
	} else {
		content = strings.Replace(content, "enabled:", "access_level: "+level+"\nenabled:", 1)
	}
	return os.WriteFile(path, []byte(content), 0o600) //nolint:gosec
}

// nolint:gocyclo
func SkillInstallRequirements(s config.SkillConfig, envPath string) error {
	if len(s.Requires.Bins) == 0 && len(s.Requires.Env) == 0 {
		fmt.Printf("  %s %s — no requirements\n",
			StyleDim.Render("—"),
			StyleBold.Render(string(s.Name)))
		return nil
	}

	fmt.Printf("\n%s\n\n", StyleHeader.Render("Installing requirements for: "+string(s.Name)))

	for _, bin := range s.Requires.Bins {
		if _, err := exec.LookPath(bin.Name); err == nil {
			fmt.Printf("  %s %s already installed\n",
				StyleGreen.Render("✓"),
				StyleBold.Render(bin.Name))
			continue
		}

		fmt.Printf("\n  %s %s not found\n",
			StyleRed.Render("✗"),
			StyleBold.Render(bin.Name))

		hint := formatInstallHint(bin)
		if hint != "" {
			fmt.Printf("  %s\n\n", StyleDim.Render("$ "+hint))

			var confirm bool
			if err := RunForm(huh.NewGroup(
				ConfirmField(
					fmt.Sprintf("Run this command to install %s?", bin.Name),
					&confirm,
				),
			)); err != nil {
				return err
			}

			if confirm {
				if err := runInstallCommand(hint); err != nil {
					Warn("install failed: %v", err)
					Hint("Try manually: %s", hint)
				} else {
					if _, err := exec.LookPath(bin.Name); err == nil {
						Success("%s installed", bin.Name)
					} else {
						Warn("%s command succeeded but binary not found in PATH", bin.Name)
					}
				}
			} else {
				Warn("skipped %s", bin.Name)
			}
		} else {
			manual, hasManual := bin.Install["manual"]
			fmt.Printf("  %s requires manual installation\n", StyleBold.Render(bin.Name))
			if hasManual {
				fmt.Printf("  Install at: %s\n", StyleGreen.Render(manual))
			}
			if s.Instructions != "" {
				fmt.Printf("\n  %s\n%s\n",
					StyleBold.Render("Setup instructions:"),
					indent(s.Instructions, "    "),
				)
			}
		}
	}

	// Env vars
	missingEnv := []string{}
	for _, envVar := range s.Requires.Env {
		if os.Getenv(envVar) == "" {
			missingEnv = append(missingEnv, envVar)
		}
	}

	if len(missingEnv) > 0 {
		fmt.Printf("\n%s\n", StyleBold.Render("Environment variables needed:"))

		if s.Instructions != "" {
			fmt.Printf("\n%s\n%s\n\n",
				StyleBold.Render("  Instructions:"),
				indent(s.Instructions, "    "),
			)
		}

		for _, envVar := range missingEnv {
			fmt.Printf("  %s %s\n", StyleRed.Render("✗"), envVar)

			var value string
			if err := RunForm(huh.NewGroup(
				SecretInput(
					fmt.Sprintf("Enter value for %s (leave blank to skip)", envVar),
					&value,
				),
			)); err != nil {
				return err
			}

			if value == "" {
				Warn("skipped %s", envVar)
				continue
			}

			// Write to .env
			if err := SetEnvVar(envPath, envVar, value); err != nil {
				Warn("failed to save %s: %v", envVar, err)
				continue
			}

			// Also set in current process so validate check passes immediately
			err := os.Setenv(envVar, value) //nolint:errcheck
			if err != nil {
				Warn("failed to set %s: %v", envVar, err)
				continue
			}
			Success("%s saved to .env", envVar)
		}
	}

	fmt.Println()
	failed, _ := PrintSkillValidation(s)
	if failed {
		return fmt.Errorf("skill %q has unmet requirements", s.Name)
	}
	return nil
}

// ── Display ───────────────────────────────────────────────────────────────────

// SkillSummaryLine returns a single-line summary for table output.
func SkillSummaryLine(s config.SkillConfig) string {
	return fmt.Sprintf("%-20s %s", s.Name, EnabledLabel(s.Enabled))
}

func SkillConfigPath(skillsDir, name string) string {
	return filepath.Join(skillsDir, name+".md")
}

// SkillWithTools holds frontmatter metadata plus parsed commands.
type SkillWithTools struct {
	config.SkillConfig
	Commands []*config.SkillToolEntry
}

func PrintSkillValidation(s config.SkillConfig) (failed bool, noRequirements bool) {
	// Check bins
	if len(s.Requires.Bins) == 0 && len(s.Requires.Env) == 0 {
		return false, true
	}

	fmt.Printf("\n%s\n", StyleBold.Render(string(s.Name)+" skill"))
	for _, bin := range s.Requires.Bins {
		path, err := exec.LookPath(bin.Name)
		if err != nil {
			failed = true
			hint := formatInstallHint(bin)
			if hint != "" {
				fmt.Printf("  %s %-16s not found — %s\n",
					StyleRed.Render("✗"),
					bin.Name,
					StyleDim.Render(hint),
				)
			} else {
				fmt.Printf("  %s %-16s not found\n",
					StyleRed.Render("✗"),
					bin.Name,
				)
			}
		} else {
			fmt.Printf("  %s %-16s %s\n",
				StyleGreen.Render("✓"),
				bin.Name,
				StyleDim.Render(path),
			)
		}
	}

	// Check env vars
	for _, envVar := range s.Requires.Env {
		val := os.Getenv(envVar)
		if val == "" {
			failed = true
			fmt.Printf("  %s %-16s not set — add to ~/.onlyagents/.env\n",
				StyleRed.Render("✗"),
				envVar,
			)
		} else {
			fmt.Printf("  %s %-16s set\n",
				StyleGreen.Render("✓"),
				envVar,
			)
		}
	}

	return failed, false
}

func runInstallCommand(command string) error {
	fmt.Println()
	cmd := exec.Command("bash", "-c", command) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin // sudo needs stdin for password
	return cmd.Run()
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func formatInstallHint(bin config.BinRequirement) string {
	for _, pm := range []string{"brew", "apt", "apt-get", "yum", "pacman", "scoop"} {
		if _, err := exec.LookPath(pm); err == nil {
			if cmd, ok := bin.Install[pm]; ok {
				return cmd
			}
		}
	}
	if manual, ok := bin.Install["manual"]; ok {
		return manual
	}
	return ""
}
