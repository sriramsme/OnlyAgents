package agents

// ID returns agent ID.
func (a *Agent) ID() string { return a.id }

func (a *Agent) Name() string { return a.name }

func (a *Agent) Description() string { return a.description }

func (a *Agent) IsExecutive() bool { return a.isExecutive }

func (a *Agent) IsGeneral() bool { return a.isGeneral }

// SetUserContext stores the user profile / preference section injected by kernel.
func (a *Agent) SetUserContext(userContext string) {
	a.userContext = userContext
}

// SetAvailableAgents stores the peer agent manifest — executive agents only.
// Calling this on a non-executive agent is a no-op.
func (a *Agent) SetAvailableAgents(agents map[string]AgentInfo) {
	if !a.isExecutive {
		return
	}
	a.availableAgents = agents
}
