package agents

func (a *Agent) SetHandleFindSkill(fn handleFindSkillFunc) {
	a.handleFindSkill = fn
}
