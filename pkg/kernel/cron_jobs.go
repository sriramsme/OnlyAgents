package kernel

func (k *Kernel) loadCronJobs() {
	jobs, err := k.store.ListCronJobs(k.ctx)
	if err != nil {
		k.logger.Error("failed to load cron jobs", "err", err)
		return
	}
	for _, job := range jobs {
		if job.Enabled {
			k.scheduler.RegisterDynamic(job)
		}
	}
	k.logger.Info("cron jobs loaded", "count", len(jobs))
}
