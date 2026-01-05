# Agent Prompt: Create `_concepts.yaml`

## Objective

Create the machine-readable concept map file `docs/_concepts.yaml` by analyzing the Lesser source code.

## Core Principle

> **Do NOT reference existing documentation.** Extract all concepts directly from source code analysis.

---

## Source Code to Analyze

### 1. Lambda Functions

Read `cmd/` directory to extract all Lambda function purposes:

```
cmd/
├── activity-processor/main.go   # DynamoDB stream handler
├── actor/main.go                # Actor profile serving
├── api/                         # REST API (read routes.go or main.go)
├── graphql/main.go              # GraphQL API
├── inbox/main.go                # ActivityPub inbox
├── outbox/main.go               # ActivityPub outbox
├── federation-delivery/main.go  # Outbound federation
├── media-processor/main.go      # Media processing
├── moderation-processor/main.go # Content moderation
├── streaming/main.go            # WebSocket connections
├── webfinger/main.go            # WebFinger endpoint
... (all 38+ Lambda directories)
```

**Extract for each Lambda:**
- Purpose from package doc comments
- Handler function signatures
- Key dependencies imported

### 2. Core Packages

Read `pkg/` directory structure and key files:

```
pkg/
├── activitypub/        # Read activitypub.go or types.go
├── auth/               # Read auth.go, oauth.go, webauthn.go
├── federation/         # Read federation.go, delivery.go
├── lift/               # Read lift.go or context.go
├── mastodon/           # Read mastodon.go or types.go
├── services/registry.go # Service registry
├── storage/            # Read types.go, repositories/
├── streaming/          # Read streaming.go
```

**Extract for each package:**
- Package purpose from doc comment
- Key exported types
- Key exported functions

### 3. GraphQL Schema

Read `docs/contracts/graphql-schema.graphql` to extract:
- Query types
- Mutation types
- Subscription types
- Key domain types (Status, Account, etc.)

### 4. Infrastructure

Read `infra/cdk/constructs/` and `infra/cdk/stacks/`:
- CDK construct names and purposes
- Stack organization
- Key AWS resources

---

## Output Format

Create `docs/_concepts.yaml` using this structure:

```yaml
# _concepts.yaml - Machine-readable concept map for Lesser
# Generated from source code analysis
# Last updated: [YYYY-MM-DD]

concepts:
  lesser:
    type: platform
    language: go
    purpose: "[Extract from README.md or main.go]"
    tagline: "Serverless ActivityPub implementation on AWS"
    provides:
      - mastodon_api_compatibility
      - activitypub_federation
      - graphql_api
      - websocket_streaming
      # ... extract from actual capabilities
    requires:
      - aws_account
      - route53_hosted_zone
      # ... extract from infra requirements
    use_when:
      - need_federated_social
      - prefer_serverless
    dont_use_when:
      - need_mastodon_plugins

  # Lambda concepts - one entry per cmd/ directory
  lambdas:
    type: component_group
    purpose: "AWS Lambda functions implementing Lesser functionality"
    components:
      activity_processor:
        type: lambda
        purpose: "[Extract from cmd/activity-processor/main.go]"
        triggers: dynamodb_stream
        key_functions:
          - "[Function names from code]"
      api:
        type: lambda
        purpose: "[Extract from cmd/api/main.go]"
        triggers: http_api
        routes: "[Route count from code]"
      # ... one entry per Lambda

  # Package concepts - one entry per pkg/ directory
  packages:
    type: component_group
    purpose: "Go packages implementing Lesser core logic"
    components:
      activitypub:
        type: package
        purpose: "[Extract from pkg/activitypub/ doc comment]"
        provides:
          - "[Key types]"
          - "[Key functions]"
      auth:
        type: package
        purpose: "[Extract from pkg/auth/ doc comment]"
        provides:
          - oauth_flows
          - webauthn_support
          - jwt_handling
      # ... one entry per package

  # GraphQL concepts
  graphql:
    type: api
    purpose: "GraphQL API for client applications"
    schema_file: docs/contracts/graphql-schema.graphql
    query_count: "[Count from schema]"
    mutation_count: "[Count from schema]"
    subscription_count: "[Count from schema]"

  # Infrastructure concepts
  infrastructure:
    type: component_group
    purpose: "AWS infrastructure defined via CDK"
    components:
      dynamodb:
        type: database
        purpose: "Single-table design for all data"
        gsi_count: "[Count from code]"
      s3:
        type: storage
        purpose: "Media and static assets"
      sqs:
        type: queue
        purpose: "Async message processing"
      eventbridge:
        type: scheduler
        purpose: "Scheduled tasks"
```

---

## Validation Criteria

- [ ] Lambda count matches Makefile (38 functions)
- [ ] Package list matches `pkg/` directories
- [ ] All purposes extracted from actual code
- [ ] YAML syntax is valid
- [ ] No references to external documentation

## Output

Create file: `docs/_concepts.yaml`

Target size: 200-400 lines
