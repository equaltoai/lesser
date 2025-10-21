# Lift Development Guidelines

This document outlines the development standards and best practices for the Lift framework to ensure code quality, maintainability, and production readiness.

## Production Code Standards

### 1. No Panic Statements
- **Always return errors** instead of panicking in production code
- Use structured errors with `lift.NewLiftError()` for consistent error responses
- **Exception**: CDK construct initialization may use panic as these run at infrastructure definition time, not runtime
  - Must include clear error messages: `panic(fmt.Sprintf("ConstructName validation failed: %v", err))`
  - Document why panic is acceptable in a comment
  - **Only acceptable for**: Required parameter validation, configuration errors, and construct initialization failures
  - **Never acceptable for**: Runtime errors, business logic failures, or recoverable conditions

### 2. No Debug Prints
- **Never use** `fmt.Print`, `fmt.Printf`, `log.Print`, or similar in production code
- Use structured logging via `ctx.Logger` with appropriate log levels
- Never log sensitive information (tokens, passwords, keys, PII)
- Use proper log levels:
  - `Debug`: Detailed information for debugging
  - `Info`: General informational messages
  - `Warn`: Warning messages for potentially harmful situations
  - `Error`: Error events that might still allow the application to continue

### 3. No Stub Implementations
- All functions must have real implementations
- Use feature flags for gradual rollout of new features
- Mock implementations only in test files or behind feature flags
- No "TODO: implement this" comments in production code

### 4. Proper Error Handling
```go
// ❌ Wrong - Stub implementation
func GetData() interface{} {
    // TODO: implement this
    return nil
}

// ✅ Correct - Real implementation with error handling
func GetData(ctx *lift.Context) (*Data, error) {
    result, err := db.Query(ctx, query)
    if err != nil {
        return nil, fmt.Errorf("failed to query data: %w", err)
    }
    return result, nil
}
```

### 5. Feature Flags for New Features
```go
// Use feature flags to control feature rollout
ff := middleware.GetFeatureFlags(ctx)
if ff != nil && ff.IsEnabled("new_feature_name") {
    return newImplementation(ctx)
}
return stableImplementation(ctx)
```

### 6. Context Usage
- Always pass `*lift.Context` as the first parameter
- Use context for cancellation, timeouts, and request-scoped values
- Never store mutable state in context

## Testing Standards

### 1. Test Isolation
- Test helpers must be in `*_test.go` files only
- Use interfaces for mockability
- No test code in production paths
- Use `lifttesting.NewTestApp()` for isolated test environments

### 2. Integration Tests
- Use DynamoDB Local for database tests
- Use LocalStack for AWS service tests
- Clean up all resources after tests
- Use unique identifiers to prevent test conflicts

### 3. Benchmarks
- Benchmark critical paths
- Track performance over time
- Alert on performance regressions
- Include memory allocation stats

```go
func BenchmarkCriticalPath(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        // benchmark code
    }
}
```

## Code Organization

### 1. Package Structure
- Keep packages focused and cohesive
- Avoid circular dependencies
- Use internal packages for implementation details
- Export only what's necessary

### 2. Interface Design
- Define interfaces where they're used, not where they're implemented
- Keep interfaces small and focused
- Use type embedding to compose interfaces

### 3. Error Handling
- Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Define custom error types for domain-specific errors
- Use `errors.Is()` and `errors.As()` for error checking

## Security Standards

### 1. No Hardcoded Credentials
- Never commit secrets, API keys, or passwords
- Use environment variables or AWS Secrets Manager
- Add `.env` files to `.gitignore`

### 2. Input Validation
- Always validate and sanitize user input
- Use the validation package for struct validation
- Implement rate limiting for public endpoints

### 3. Secure Defaults
- Enable security features by default
- Require explicit opt-out for security features
- Document security implications of configuration options
- Require non-empty data protection encryption keys; initialization fails fast when missing
- Configure runtime guardrails (`MaxRequestSize`, `MaxResponseSize`, `Timeout`, `RequireTenantID`) in `lift.Config`; Lift enforces them before handler execution and emits guardrail metrics automatically.

## Development Workflow

### 1. Before Implementing
- Search existing code for similar patterns
- Check if functionality already exists
- Review relevant documentation
- Consider security implications

### 2. Follow Conventions
- Match existing code style and patterns
- Use consistent naming conventions
- Follow Go idioms and best practices
- Run `go fmt` and `go vet` before committing
- Tie middleware background workers to the app lifecycle (e.g., wrap load shedding config with `middleware.ConfigureLoadSheddingForApp(app, cfg)`)

### 3. Documentation
- Document all exported types and functions
- Include examples for complex functionality
- Keep documentation up-to-date with code changes
- Remove outdated comments

### 4. Testing Requirements
- Write tests for all new functionality
- Maintain or improve code coverage
- Include both positive and negative test cases
- Test edge cases and error conditions

## Code Review Checklist

Before submitting code for review, ensure:

- [ ] No panic statements in production code (except CDK constructs)
- [ ] No fmt.Print/log.Print statements
- [ ] All TODOs have associated tracking issues
- [ ] Proper error handling throughout
- [ ] Feature flags for new/experimental features
- [ ] Tests cover happy and error paths
- [ ] Documentation is complete and accurate
- [ ] Security review for sensitive operations
- [ ] No hardcoded credentials or secrets
- [ ] Code follows established patterns
- [ ] Performance implications considered
- [ ] Backward compatibility maintained

## Monitoring and Observability

### 1. Structured Logging
- Include request ID in all logs
- Add relevant context (user ID, tenant ID)
- Use consistent field names
- Avoid excessive logging in hot paths

### 2. Metrics
- Record key business metrics
- Monitor performance metrics
- Set up alerts for anomalies
- Use consistent metric naming
- Prefer `middleware.EnhancedObservabilityMiddleware` for unified logging/metrics/tracing. Use `SampleRate` for probabilistic sampling and `DisableSampling` when instrumentation must be fully suppressed; tenant and user identifiers are added automatically.

### 3. Tracing
- Implement distributed tracing for complex flows
- Include relevant metadata in traces
- Ensure trace context propagation
- Monitor trace sampling rates

## Performance Guidelines

### 1. Avoid Premature Optimization
- Profile before optimizing
- Focus on algorithmic improvements
- Consider caching for expensive operations
- Document performance-critical code

### 2. Resource Management
- Close resources in defer statements
- Use connection pooling
- Implement proper timeouts
- Handle backpressure appropriately
- Close `performance.ConnectionPool` instances during shutdown. The pool now enforces `MaxConnections`, reports safe utilisation stats, and closing it stops background health checks without deadlocking.

### 3. Concurrency
- Use goroutines judiciously
- Protect shared state with appropriate synchronization
- Prefer channels over shared memory
- Avoid goroutine leaks

## Backward Compatibility

### 1. API Changes
- Maintain backward compatibility
- Deprecate before removing
- Version APIs appropriately
- Document breaking changes

### 2. Configuration
- Support old configuration formats
- Provide migration guides
- Log deprecation warnings
- Set reasonable defaults
- `disaster.DRConfig` validates testing cadences at startup. Leave `Frequency` or health `Interval` at zero to accept defaults, and ensure `NotifyBefore` is less than `Frequency` or monitoring will fail fast.

## Tools and Automation

### 1. Pre-commit Hooks
```bash
# Install pre-commit hooks
git config core.hooksPath .githooks
```

### 2. CI/CD Checks
- Run tests on every commit
- Check code coverage
- Run security scans
- Validate documentation

### 3. Linting
```bash
# Run all linters
make lint

# Specific linters
golangci-lint run
go vet ./...
```

## Getting Help

- Review existing code for examples
- Check documentation in `/docs`
- Ask questions in code reviews
- Consult team for architectural decisions

## Updating These Guidelines

These guidelines are living documentation. To propose changes:

1. Create a GitHub issue describing the proposed change
2. Discuss with the team
3. Submit a pull request with the update
4. Get approval from maintainers

Remember: These guidelines exist to help us build better software. When in doubt, prioritize code clarity, safety, and maintainability.
