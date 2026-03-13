package kernel

import (
	"github.com/sriramsme/OnlyAgents/pkg/connectors/local"
)

func (k *Kernel) registerLocalConnectors() {
	k.RegisterConnector(local.NewCalendarConnector(k.store))
	k.RegisterConnector(local.NewNotesConnector(k.store))
	k.RegisterConnector(local.NewRemindersConnector(k.store))
	k.RegisterConnector(local.NewTasksConnector(k.store))
}
