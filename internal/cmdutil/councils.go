package cmdutil

import (
	"fmt"

	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/internal/paths"
)

// CouncilRegistry loads all council configs from the councils dir.
func CouncilRegistry(councilsDir string) ([]config.CouncilConfig, error) {
	councils, err := LoadDir[config.CouncilConfig](councilsDir)
	if err != nil {
		return nil, fmt.Errorf("council registry: %w", err)
	}
	return councils, nil
}

// FindCouncil returns the first council matching name.
func FindCouncil(councils []config.CouncilConfig, name string) (config.CouncilConfig, error) {
	for _, c := range councils {
		if c.Name == name {
			return c, nil
		}
	}
	return config.CouncilConfig{}, fmt.Errorf("council %q not found", name)
}

// EnabledCouncils returns only enabled councils.
func EnabledCouncils(councils []config.CouncilConfig) []config.CouncilConfig {
	var out []config.CouncilConfig
	for _, c := range councils {
		if c.Enabled {
			out = append(out, c)
		}
	}
	return out
}

// EnableCouncil activates all agents, skills, and connectors in the council.
// Returns warnings for components that couldn't be found.
func EnableCouncil(cfg config.CouncilConfig, paths *paths.Paths) []string {
	var warnings []string

	for _, name := range cfg.Agents {
		if !AgentExists(paths.Agents, name) {
			warnings = append(warnings, fmt.Sprintf("agent %q not found — skipped", name))
			continue
		}
		if err := AgentSetEnabled(paths.Agents, name, true); err != nil {
			warnings = append(warnings, fmt.Sprintf("agent %q: %v", name, err))
		}
	}

	for _, name := range cfg.Skills {
		if err := SkillSetEnabled(paths.Skills, name, true); err != nil {
			warnings = append(warnings, fmt.Sprintf("skill %q: %v", name, err))
		}
	}

	for _, name := range cfg.Connectors {
		if err := ConnectorSetEnabled(paths.Connectors, name, true); err != nil {
			warnings = append(warnings, fmt.Sprintf("connector %q: %v", name, err))
		}
	}

	// Mark council itself as enabled
	if err := councilSetEnabled(paths.Councils, cfg.Name, true); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not mark council enabled: %v", err))
	}

	return warnings
}

// DisableCouncil deactivates resources that are not claimed by any other
// active council. Resources shared with another active council are left alone.
func DisableCouncil(cfg config.CouncilConfig, paths *paths.Paths) []string {
	var warnings []string

	// Load all councils to compute what other active councils need
	all, err := CouncilRegistry(paths.Councils)
	if err != nil {
		return []string{fmt.Sprintf("could not load councils: %v", err)}
	}

	protected := protectedResources(cfg.Name, all)

	for _, name := range cfg.Agents {
		if protected.agents[name] {
			continue // another active council needs this agent
		}
		if !AgentExists(paths.Agents, name) {
			continue
		}
		if err := AgentSetEnabled(paths.Agents, name, false); err != nil {
			warnings = append(warnings, fmt.Sprintf("agent %q: %v", name, err))
		}
	}

	for _, name := range cfg.Skills {
		if protected.skills[name] {
			continue
		}
		if err := SkillSetEnabled(paths.Skills, name, false); err != nil {
			warnings = append(warnings, fmt.Sprintf("skill %q: %v", name, err))
		}
	}

	for _, name := range cfg.Connectors {
		if protected.connectors[name] {
			continue
		}
		if err := ConnectorSetEnabled(paths.Connectors, name, false); err != nil {
			warnings = append(warnings, fmt.Sprintf("connector %q: %v", name, err))
		}
	}

	if err := councilSetEnabled(paths.Councils, cfg.Name, false); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not mark council disabled: %v", err))
	}

	return warnings
}

// ── Resource protection ───────────────────────────────────────────────────────

type resourceSet struct {
	agents     map[string]bool
	skills     map[string]bool
	connectors map[string]bool
}

// protectedResources builds the union of resources claimed by all active
// councils except the one being disabled.
func protectedResources(disabling string, all []config.CouncilConfig) resourceSet {
	rs := resourceSet{
		agents:     make(map[string]bool),
		skills:     make(map[string]bool),
		connectors: make(map[string]bool),
	}
	for _, c := range all {
		if c.Name == disabling || !c.Enabled {
			continue
		}
		for _, a := range c.Agents {
			rs.agents[a] = true
		}
		for _, s := range c.Skills {
			rs.skills[s] = true
		}
		for _, cn := range c.Connectors {
			rs.connectors[cn] = true
		}
	}
	return rs
}

// ── Council file helpers ──────────────────────────────────────────────────────

func councilSetEnabled(councilsDir, name string, enabled bool) error {
	path := CouncilConfigPath(councilsDir, name)
	var raw map[string]any
	if err := ReadYAML(path, &raw); err != nil {
		return err
	}
	raw["enabled"] = enabled
	return WriteYAML(path, raw)
}

func CouncilConfigPath(councilsDir, name string) string {
	return councilsDir + "/" + name + ".yaml"
}

// CouncilStatus returns per-resource status for the info command.
type CouncilStatus struct {
	Name       string
	Active     bool
	Agents     []ResourceStatus
	Skills     []ResourceStatus
	Connectors []ResourceStatus
}

type ResourceStatus struct {
	Name    string
	Present bool
	Enabled bool
}

func CouncilInfo(cfg config.CouncilConfig, paths *paths.Paths) CouncilStatus {
	status := CouncilStatus{
		Name:   cfg.Name,
		Active: cfg.Enabled,
	}

	for _, name := range cfg.Agents {
		rs := ResourceStatus{Name: name, Present: AgentExists(paths.Agents, name)}
		if rs.Present {
			agents, err := AgentRegistry(paths.Agents)
			if err != nil {
				rs.Present = false
				continue
			}
			if a, err := FindAgent(agents, name); err == nil {
				rs.Enabled = a.Enabled
			}
		}
		status.Agents = append(status.Agents, rs)
	}

	for _, name := range cfg.Skills {
		rs := ResourceStatus{Name: name}
		skills, err := SkillRegistry(paths.Skills)
		if err != nil {
			rs.Present = false
			continue
		}
		if s, err := FindSkill(skills, name); err == nil {
			rs.Present = true
			rs.Enabled = s.Enabled
		}
		status.Skills = append(status.Skills, rs)
	}

	for _, name := range cfg.Connectors {
		rs := ResourceStatus{Name: name}
		connectors, err := ConnectorRegistry(paths.Connectors)
		if err != nil {
			rs.Present = false
			continue
		}
		if c, err := FindConnector(connectors, name); err == nil {
			rs.Present = true
			rs.Enabled = c.Enabled
		}
		status.Connectors = append(status.Connectors, rs)
	}

	return status
}
