package agents

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

func (a *Agent) delegationAck(agentID string) string {
	name := a.resolveAgentName(agentID)

	msgs := a.soul.DelegationAcknowledgments()
	if len(msgs) == 0 {
		return fmt.Sprintf("I've handed that off to %s.", name)
	}

	template := msgs[rand.Intn(len(msgs))] // nolint:gosec
	return strings.ReplaceAll(template, "{agent_name}", name)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

func workflowAck(wf *workflow.WorkflowDefinition) string {
	var b strings.Builder
	fmt.Fprintf(&b, "On it — kicking off a %d-step workflow:\n", len(wf.Tasks))
	for i, s := range wf.Tasks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, s.Description)
	}
	b.WriteString("\nI'll report back once it's done.")
	return b.String()
}

func (a *Agent) turnLockFor(sessionID string) *sync.Mutex {
	v, _ := a.executeMu.LoadOrStore(sessionID, &sync.Mutex{})
	return v.(*sync.Mutex)
}
