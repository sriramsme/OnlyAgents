# Contributing to OnlyAgents

First off, thank you for considering contributing to OnlyAgents!

## Code of Conduct

This project and everyone participating in it is governed by our Code of Conduct. By participating, you are expected to uphold this code.

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues. When you create a bug report, include as many details as possible:

- Use a clear and descriptive title
- Describe the exact steps to reproduce
- Provide specific examples
- Describe the behavior you observed and what you expected
- Include logs and configuration files

### Suggesting Features

Feature suggestions are welcome! Please:

- Use a clear and descriptive title
- Provide a detailed description of the feature
- Explain why this feature would be useful
- Include examples of how the feature would be used

### Pull Requests

1. Fork the repo and create your branch from `main`
2. If you've added code, add tests
3. Ensure the test suite passes
4. Make sure your code lints
5. Write a clear commit message

## Development Setup
```bash
# Clone your fork
git clone https://github.com/sriamsme/onlyagents.git
cd onlyagents

# Install dependencies
go mod download

# Run tests
go test ./...

# Run linter
golangci-lint run

# Build
go build ./cmd/agent
```

## Coding Standards

- Follow standard Go conventions (gofmt, go vet)
- Write tests for new features
- Keep functions focused and small
- Comment exported functions and types
- Use meaningful variable names
- Handle errors explicitly

## Commit Messages

- Use the present tense ("Add feature" not "Added feature")
- Use the imperative mood ("Move cursor to..." not "Moves cursor to...")
- Limit the first line to 72 characters
- Reference issues and PRs liberally

Examples:
```
Add calendar skill for Google Calendar integration
Fix message signing verification bug
Update README with installation instructions
```

## Project Structure
```
onlyagents/
├── cmd/           # Command-line tools
├── pkg/           # Public packages
├── internal/      # Private packages (future)
├── examples/      # Example agents and configurations
├── docs/          # Documentation
└── tests/         # Integration tests
```

## Testing

We use Go's built-in testing framework:
```bash
# Run all tests
go test ./...

# Run with race detection
go test -race ./...

# Run with coverage
go test -coverprofile=coverage.txt ./...
```

## Documentation

- Document all exported functions, types, and packages
- Keep README.md up to date
- Add examples for new features
- Update CHANGELOG.md

## Questions?

Feel free to open an issue with the label `question` or reach out on Discord.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
