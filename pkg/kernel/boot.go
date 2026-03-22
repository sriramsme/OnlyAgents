package kernel

func kernelInitializers() []Initializer {
	return []Initializer{
		connectorsInitializer{},
		agentsInitializer{},  // assigns skills, deps, user context, available agents
		promptsInitializer{}, // always last — all data must be set before this
		memoryInitializer{},
	}
}

func (k *Kernel) Boot() error {
	for _, init := range kernelInitializers() {
		if err := init.Init(k.ctx, k); err != nil {
			return err
		}
	}

	return nil
}
