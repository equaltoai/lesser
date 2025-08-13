# Linting Guide for Lesser

This project uses [golangci-lint](https://golangci-lint.run/) for code quality and consistency. The configuration is defined in `.golangci.yml`.

## Quick Start

```bash
# Run all linters
make lint

# Auto-fix issues where possible
make lint-fix

# Run linters only on new code
make lint-new

# Run a specific linter
make lint-gosec
```

## Enabled Linters

### Default Linters
- **errcheck**: Checks for unchecked errors
- **govet**: Reports suspicious constructs
- **ineffassign**: Detects unused assignments
- **staticcheck**: Advanced static analysis
- **unused**: Checks for unused code

### Code Quality
- **bodyclose**: Ensures HTTP response bodies are closed
- **dupl**: Detects duplicate code (threshold: 100 tokens)
- **copyloopvar**: Checks for loop variable copies
- **goconst**: Finds repeated strings that could be constants
- **gocyclo**: Cyclomatic complexity (max: 15)
- **gosec**: Security issues
- **misspell**: Spelling mistakes
- **nakedret**: Naked returns (max function lines: 30)
- **prealloc**: Suggests slice preallocations
- **revive**: Comprehensive style checker
- **unconvert**: Unnecessary type conversions
- **unparam**: Unused function parameters
- **whitespace**: Whitespace issues

### Best Practices
- **nilerr**: Nil error checking
- **exhaustive**: Enum exhaustiveness
- **noctx**: HTTP requests without context
- **asciicheck**: Non-ASCII identifiers
- **bidichk**: Dangerous unicode sequences

## Common Issues and Fixes

### 1. Unchecked Errors
```go
// Bad
resp, _ := http.Get(url)

// Good
resp, err := http.Get(url)
if err != nil {
    return fmt.Errorf("failed to get URL: %w", err)
}
```

### 2. HTTP Response Body Not Closed
```go
// Bad
resp, err := http.Get(url)
if err != nil {
    return err
}

// Good
resp, err := http.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()
```

### 3. Repeated Strings
```go
// Bad
log.Printf("processing user %s", userID)
log.Printf("failed for user %s", userID)
log.Printf("completed for user %s", userID)

// Good
const userLogFormat = "user %s"
log.Printf("processing " + userLogFormat, userID)
log.Printf("failed for " + userLogFormat, userID)
log.Printf("completed for " + userLogFormat, userID)
```

### 4. Package Comments
```go
// Bad
package mypackage

// Good
// Package mypackage provides functionality for...
package mypackage
```

### 5. Exported Types Without Comments
```go
// Bad
type MyType struct {
    Field string
}

// Good
// MyType represents...
type MyType struct {
    Field string
}
```

## Excluded Patterns

The following patterns are excluded from linting:

1. **Test files** (`_test.go`): Relaxed rules for cyclomatic complexity, error checking, and duplication
2. **Generated files** (`generated.go`): Most linters disabled
3. **Lambda handlers** (`cmd/*/main.go`): `init()` functions are allowed
4. **GraphQL generated code**: Complexity and duplication checks disabled
5. **Common patterns**: 
   - Error checking for `Close()`, `Flush()`, print functions
   - Shadowing in DynamoDB operations
   - AWS SDK error patterns

## Project-Specific Settings

- **Cyclomatic Complexity**: Maximum 15
- **Duplicate Code**: Minimum 100 tokens
- **String Constants**: Minimum length 3, minimum 3 occurrences
- **Naked Returns**: Maximum 30 lines per function
- **Security Exclusions**: 
  - G101: Hardcoded credentials (too many false positives)
  - G401, G501, G505: Weak crypto algorithms (legacy compatibility)

## Running Specific Checks

```bash
# Security-focused check
make lint-gosec

# Performance check
make lint-prealloc

# Style check
make lint-revive

# Complexity check
make lint-gocyclo
```

## Fixing Issues

Many issues can be automatically fixed:

```bash
# Format code
make fmt

# Auto-fix linting issues
make lint-fix
```

For issues that can't be auto-fixed:
1. Read the linter output carefully
2. Check the examples above
3. Consult the [golangci-lint documentation](https://golangci-lint.run/)

## Disabling Linters

If you need to disable a linter for a specific line:

```go
//nolint:errcheck // Reason why this is acceptable
resp, _ := http.Get(url)
```

For a block of code:
```go
//nolint:gosec // These credentials are for testing only
const (
    testUser = "admin"
    testPass = "password"
)
```

**Note**: Always provide a reason when disabling linters.

## Adding New Linters

To add a new linter:
1. Add it to the `enable` list in `.golangci.yml`
2. Configure its settings in `linters-settings`
3. Test it: `make lint-<linter-name>`
4. Update this documentation

## Continuous Integration

The linter runs automatically on:
- Pull requests (only on changed files)
- Main branch pushes (full codebase)

To replicate CI behavior locally:
```bash
# Check only changed files
make lint-new
```