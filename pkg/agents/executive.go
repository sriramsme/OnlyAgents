package agents

// RegisterPeer adds a peer agent to the executive's manifest and rebuilds.
// No-op on non-executive agents.
func (a *Agent) RegisterPeer(agentInfo AgentInfo) {
	if !a.isExecutive {
		return
	}
	a.stateMu.Lock()
	defer a.stateMu.Unlock()

	if a.availableAgents == nil {
		a.availableAgents = make(map[string]AgentInfo)
	}
	a.availableAgents[agentInfo.ID] = agentInfo
	a.RebuildSystemPrompt()
	a.logger.Info("peer registered at runtime", "agent", a.id, "peer", agentInfo.ID)
}
