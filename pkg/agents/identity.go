package agents

// ID returns agent ID.
func (a *Agent) ID() string { return a.id }

func (a *Agent) Name() string { return a.name }

func (a *Agent) Description() string { return a.description }

func (a *Agent) IsExecutive() bool { return a.isExecutive }

func (a *Agent) IsGeneral() bool { return a.isGeneral }
