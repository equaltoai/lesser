# Contributing to Lesser

First off, thank you for considering contributing to Lesser! It's people like you that make Lesser such a great tool for democratizing social media hosting.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How Can I Contribute?](#how-can-i-contribute)
- [Development Setup](#development-setup)
- [Code Style Guidelines](#code-style-guidelines)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)
- [Reporting Issues](#reporting-issues)
- [Community](#community)

## Code of Conduct

This project and everyone participating in it is governed by our Code of Conduct. By participating, you are expected to uphold this code.

### Our Standards

- **Be respectful** - Disagreements happen, but keep it professional
- **Be inclusive** - Welcome newcomers and help them get started
- **Be collaborative** - Work together to solve problems
- **Be patient** - Not everyone has the same experience level
- **Be thoughtful** - Consider how your words and actions affect others

## How Can I Contribute?

### Reporting Bugs

Before creating bug reports, please check existing issues to avoid duplicates. When creating a bug report, include:

1. **Clear title** - Summarize the issue
2. **Description** - What happened vs what you expected
3. **Steps to reproduce** - Be specific!
4. **Environment** - OS, Go version, AWS region
5. **Logs/Screenshots** - If applicable
6. **Possible fix** - If you have ideas

**Template:**
```markdown
**Describe the bug**
A clear description of what the bug is.

**To Reproduce**
1. Deploy Lesser with configuration '...'
2. Send request to '...'
3. See error

**Expected behavior**
What you expected to happen.

**Environment:**
- OS: [e.g. Ubuntu 22.04]
- Go version: [e.g. 1.21]
- AWS Region: [e.g. us-east-1]
- Deployment method: [e.g. Pulumi]

**Additional context**
Any other relevant information.
```

### Suggesting Enhancements

Enhancement suggestions are tracked as GitHub issues. Include:

1. **Use case** - Why is this needed?
2. **Current behavior** - What happens now?
3. **Desired behavior** - What should happen?
4. **Alternatives** - Other ways to solve it?

### Your First Code Contribution

Unsure where to begin? Look for these labels:

- `good first issue` - Simple fixes to get started
- `help wanted` - More complex but guidance available
- `documentation` - Help improve our docs

## Development Setup

### Prerequisites

```bash
# Required
- Go 1.21 or higher
- AWS CLI configured
- Pulumi CLI
- Docker (for local DynamoDB)
- Make

# Optional but recommended
- golangci-lint
- mockgen
- pre-commit
```

### Local Development

1. **Fork and clone the repository**
   ```bash
   git clone https://github.com/yourusername/lesser.git
   cd lesser
   ```

2. **Install dependencies**
   ```bash
   make deps
   ```

3. **Set up local environment**
   ```bash
   # Copy example configuration
   cp .env.example .env
   
   # Start local DynamoDB
   make local-db
   
   # Run migrations
   make migrate-local
   ```

4. **Run tests**
   ```bash
   make test
   ```

5. **Start development server**
   ```bash
   make dev
   ```

### Project Structure

```
lesser/
├── cmd/           # Lambda function entry points
├── pkg/           # Shared packages
│   ├── activitypub/   # ActivityPub implementation
│   ├── auth/          # Authentication
│   ├── storage/       # Storage interfaces
│   └── ...
├── internal/      # Internal packages
├── infra/         # Infrastructure as Code
└── test/          # Integration tests
```

## Code Style Guidelines

### Go Code Style

We follow standard Go conventions with some additions:

1. **Format code** with `gofmt -s`
2. **Lint code** with `golangci-lint run`
3. **Comments** on all exported functions/types
4. **Error handling** - always check errors
5. **Context** - pass context.Context as first parameter

**Example:**
```go
// CreateActor creates a new ActivityPub actor in the system.
// It validates the input and stores the actor in DynamoDB.
func CreateActor(ctx context.Context, input *CreateActorInput) (*Actor, error) {
    // Validate input
    if err := input.Validate(); err != nil {
        return nil, fmt.Errorf("invalid input: %w", err)
    }
    
    // Create actor
    actor := &Actor{
        ID:        generateID(),
        Username:  input.Username,
        CreatedAt: time.Now(),
    }
    
    // Store in database
    if err := storage.PutActor(ctx, actor); err != nil {
        return nil, fmt.Errorf("failed to store actor: %w", err)
    }
    
    return actor, nil
}
```

### Naming Conventions

- **Packages** - lowercase, no underscores
- **Files** - lowercase with underscores
- **Functions** - CamelCase for exported, camelCase for internal
- **Constants** - CamelCase for exported, camelCase for internal
- **Interfaces** - End with `-er` suffix when possible

### Import Grouping

```go
import (
    // Standard library
    "context"
    "fmt"
    
    // Third party
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    
    // Internal packages
    "github.com/yourusername/lesser/pkg/storage"
)
```

## Testing Requirements

### Test Coverage

- Minimum 80% coverage for new code
- Unit tests for all public functions
- Integration tests for API endpoints
- Edge cases and error conditions

### Writing Tests

```go
func TestCreateActor(t *testing.T) {
    tests := []struct {
        name    string
        input   *CreateActorInput
        wantErr bool
    }{
        {
            name: "valid input",
            input: &CreateActorInput{
                Username: "testuser",
            },
            wantErr: false,
        },
        {
            name:    "empty username",
            input:   &CreateActorInput{},
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := context.Background()
            _, err := CreateActor(ctx, tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateActor() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests
make test-integration

# Specific package
go test ./pkg/activitypub/...

# With coverage
make test-coverage
```

## Pull Request Process

### Before Submitting

1. **Update documentation** - Keep docs in sync with code
2. **Add tests** - Cover new functionality
3. **Run checks** - `make lint test`
4. **Update CHANGELOG** - Note your changes
5. **Sign commits** - Use `git commit -s`

### PR Guidelines

1. **One feature per PR** - Keep PRs focused
2. **Clear title** - Summarize the change
3. **Description** - Explain what and why
4. **Reference issues** - Link related issues
5. **Small commits** - Logical, atomic changes

**PR Template:**
```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests added/updated
- [ ] CHANGELOG updated
```

### Review Process

1. **Automated checks** must pass
2. **One approval** required from maintainers
3. **Address feedback** promptly
4. **Squash commits** before merge
5. **Delete branch** after merge

## Reporting Issues

### Security Issues

**DO NOT** report security vulnerabilities publicly. Email security@lesser.app instead.

### Bug Reports

Use the issue template and include:
- Clear reproduction steps
- Expected vs actual behavior
- Environment details
- Relevant logs/screenshots

### Feature Requests

Explain:
- The problem you're trying to solve
- Your proposed solution
- Alternative solutions considered
- Impact on existing features

## Community

### Getting Help

- **Discord** - Real-time chat and support
- **GitHub Discussions** - Long-form discussions
- **Stack Overflow** - Tag with `lesser`

### Staying Updated

- **Blog** - Major updates and tutorials
- **Twitter/Mastodon** - Quick updates
- **Newsletter** - Monthly summary

## Recognition

Contributors are recognized in:
- CONTRIBUTORS.md file
- Release notes
- Project website
- Annual contributor spotlight

## Development Tips

### Debugging

```bash
# Enable debug logging
export LESSER_DEBUG=true

# Use delve debugger
dlv debug ./cmd/api

# Trace requests
export AWS_XRAY_TRACE=true
```

### Performance

- Profile before optimizing
- Benchmark critical paths
- Consider DynamoDB costs
- Minimize Lambda cold starts

### Common Pitfalls

1. **Context cancellation** - Always respect context
2. **Error wrapping** - Use `%w` for errors
3. **Goroutine leaks** - Clean up properly
4. **Large responses** - Paginate results

## Questions?

Feel free to:
- Ask in Discord
- Open a discussion
- Email maintainers

Thank you for contributing to Lesser! 🚀
