package kernel

func kernelInitializers() []Initializer {
	return []Initializer{
		connectorsInitializer{},
		agentsInitializer{},
		promptsInitializer{},
		serverInitializer{},
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
