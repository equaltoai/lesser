# Agent Prompt: Create `_patterns.yaml`

## Objective

Create the machine-readable patterns file `docs/_patterns.yaml` by analyzing the Lesser source code for correct vs incorrect usage patterns.

## Core Principle

> **Extract patterns directly from source code.** All examples must come from actual implementations.

---

## Source Code to Analyze

### 1. Handler Patterns

Read `pkg/lift/` to understand the Lift framework patterns:

```
pkg/lift/
├── context.go          # Lift context usage
├── handler.go          # Handler patterns
├── middleware.go       # Middleware patterns
├── response.go         # Response patterns
├── router.go           # Routing patterns
```

Then read examples in `cmd/api/` handlers:

```
cmd/api/
├── handlers/           # Handler implementations
├── routes.go           # Route definitions
├── main.go             # App setup
```

**Extract:**
- How handlers receive context
- How handlers return responses
- How errors are handled
- How middleware is applied

### 2. Service Patterns

Read `pkg/services/registry.go` and service implementations:

```
pkg/services/
├── registry.go         # Service registration
├── accounts/           # Account service pattern
├── notes/              # Notes service pattern
├── media/              # Media service pattern
├── cms/                # CMS service pattern
```

**Extract:**
- How services are registered
- How services depend on repositories
- How services are injected into handlers

### 3. Repository Patterns

Read `pkg/storage/repositories/`:

```
pkg/storage/repositories/
├── accounts.go         # Account repository
├── notes.go            # Notes repository
├── relationships.go    # Relationship repository
├── timelines.go        # Timeline repository
```

**Extract:**
- How repositories use DynamORM
- Query patterns
- Batch operation patterns

### 4. GraphQL Resolver Patterns

Read `graph/` resolver files:

```
graph/
├── resolver.go                  # Root resolver
├── query_resolvers_*.go         # Query patterns
├── mutation_resolvers_*.go      # Mutation patterns
├── subscription_resolvers_*.go  # Subscription patterns
```

**Extract:**
- How resolvers access services
- How resolvers handle errors
- How resolvers return data

### 5. Middleware Patterns

Read `pkg/middleware/`:

```
pkg/middleware/
├── auth.go             # Auth middleware
├── cors.go             # CORS middleware
├── logging.go          # Logging middleware
├── ratelimit.go        # Rate limiting
├── tenant.go           # Multi-tenant middleware
```

---

## Output Format

Create `docs/_patterns.yaml` using this structure:

```yaml
# _patterns.yaml - Usage patterns for Lesser
# Generated from source code analysis
# Last updated: [YYYY-MM-DD]

patterns:
  handler_pattern:
    name: "Lift Handler Pattern"
    problem: "How to implement a Lambda handler"
    solution: "Use Lift context for request/response handling"
    source_file: "cmd/api/handlers/[example].go"
    correct_example: |
      // CORRECT: Actual code from source
      // [Copy relevant handler from cmd/api/handlers/]
      func GetStatus(ctx *lift.Context) error {
          // ... actual implementation
      }
    anti_patterns:
      - name: "Raw Lambda Handler"
        why: "Bypasses Lift error handling and middleware"
        incorrect_example: |
          // INCORRECT: Don't use raw Lambda handlers
          func handler(ctx context.Context, event events.APIGatewayV2HTTPRequest) (
              events.APIGatewayV2HTTPResponse, error) {
              // ... bypasses Lift
          }
        consequences:
          - no_middleware_support
          - inconsistent_error_handling
          - no_automatic_logging

  service_pattern:
    name: "Service Layer Pattern"
    problem: "How to implement business logic"
    solution: "Use registered services with repository injection"
    source_file: "pkg/services/registry.go"
    correct_example: |
      // CORRECT: From pkg/services/registry.go
      // [Copy actual service registration pattern]
    anti_patterns:
      - name: "Direct Repository Access"
        why: "Bypasses business logic and validation"
        incorrect_example: |
          // INCORRECT: Don't access repositories directly in handlers
          func Handler(ctx *lift.Context) error {
              repo := repositories.NewAccountsRepo(db)  // Wrong!
              // ...
          }

  repository_pattern:
    name: "Repository Access Pattern"
    problem: "How to access DynamoDB data"
    solution: "Use DynamORM repositories with proper queries"
    source_file: "pkg/storage/repositories/accounts.go"
    correct_example: |
      // CORRECT: From actual repository
      // [Copy query pattern from code]

  resolver_pattern:
    name: "GraphQL Resolver Pattern"
    problem: "How to implement GraphQL operations"
    solution: "Use services through resolver context"
    source_file: "graph/query_resolvers_accounts.go"
    correct_example: |
      // CORRECT: From actual resolver
      // [Copy resolver pattern from code]

  error_pattern:
    name: "Error Handling Pattern"
    problem: "How to return errors to clients"
    solution: "Use typed errors from pkg/errors"
    source_file: "pkg/errors/errors.go"
    correct_example: |
      // CORRECT: Use typed errors
      return lift.NotFoundError("status not found")
    anti_patterns:
      - name: "Generic Errors"
        why: "Loses error context and type information"
        incorrect_example: |
          // INCORRECT: Generic errors
          return errors.New("not found")

  federation_pattern:
    name: "Federation Delivery Pattern"
    problem: "How to deliver ActivityPub activities"
    solution: "Use federation service with retry queue"
    source_file: "pkg/federation/delivery.go"
    correct_example: |
      // CORRECT: From federation package
      // [Copy delivery pattern]

  middleware_pattern:
    name: "Middleware Application Pattern"
    problem: "How to apply cross-cutting concerns"
    solution: "Use middleware chain in router"
    source_file: "cmd/api/main.go"
    correct_example: |
      // CORRECT: From actual app setup
      app := lift.New()
      app.Use(middleware.Logger())
      app.Use(middleware.Auth(config))
```

---

## Validation Criteria

- [ ] All `correct_example` code copied from actual source files
- [ ] `source_file` paths are valid
- [ ] Anti-patterns show what NOT to do and why
- [ ] At least 8 patterns documented
- [ ] YAML syntax is valid

## Output

Create file: `docs/_patterns.yaml`

Target size: 400-800 lines
