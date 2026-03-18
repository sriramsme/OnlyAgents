package kernel

func (k *Kernel) assignSystemPrompts() error {
	for _, agent := range k.agents.All() {
		agent.RebuildSystemPrompt()
	}
	return nil
}
