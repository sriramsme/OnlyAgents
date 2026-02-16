---
name: Skill Submission
about: Submit a new skill for OnlyAgents
title: '[SKILL] '
labels: skill, community
assignees: ''
---

## Skill Name
What is the name of your skill?

## Skill Description
What does this skill do?

## Domain/Category
- [ ] Calendar
- [ ] Communication
- [ ] Finance
- [ ] Research
- [ ] Development
- [ ] Other: ___________

## Required Capabilities
What permissions does this skill need?
- [ ] read:calendar
- [ ] write:calendar
- [ ] read:email
- [ ] write:email
- [ ] network:web_search
- [ ] Other: ___________

## Required Platforms
What platform connectors are needed?
- [ ] Google Calendar
- [ ] Gmail
- [ ] Slack
- [ ] Other: ___________

## Implementation
- Repository/Branch: [link to your fork/branch]
- Tests included: [ ] Yes [ ] No
- Documentation: [ ] Yes [ ] No

## Example Usage
```yaml
# Example agent.yaml configuration
skills:
  - name: "your_skill_name"
    enabled: true
    config:
      option1: "value1"
```

## Demo/Proof of Concept
If possible, provide a demo video or screenshots showing your skill in action.

## Checklist
- [ ] Code follows project style guidelines
- [ ] Tests pass (`go test ./...`)
- [ ] Documentation is complete
- [ ] Security review considered
- [ ] Works with latest OnlyAgents release
