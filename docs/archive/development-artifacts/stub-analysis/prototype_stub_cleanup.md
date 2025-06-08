# Prototype Stub Cleanup Plan

## Context
This is an unreleased prototype with ~27 stub implementations. This is actually pretty normal for prototype development where you're trying to prove concepts and build the architecture.

## Quick Wins (1-2 days)

### 1. Stop the GraphQL Panics
Just replace the panics with errors so the GraphQL endpoint doesn't crash:
```go
// Quick fix in schema.resolvers.go
return nil, fmt.Errorf("not yet implemented")
```

### 2. Fix Import/Export Lists
These are probably the most annoying stubs since they make testing harder:
- `getUserImportJobs()` 
- `getUserExportJobs()`

Just need basic DynamoDB queries - maybe 2-3 hours each.

### 3. Pick One Export Type to Make Real
Instead of fixing all 12 export functions, just pick one (like followers) to implement properly so you can test the full export flow.

## Nice to Haves (When You Get Time)

- Media processing (or just document that it returns fake data)
- Some of the more useful GraphQL queries
- Fill in a few more export types as needed

## What to Skip

- Full GraphQL implementation (that's a whole project)
- Perfect test coverage (it's a prototype)
- Process improvements (save for production)

## Practical Approach

1. Fix what's blocking you from testing other features
2. Document what's not implemented in the README
3. Use GitHub issues to track what needs doing before release
4. Don't stress about "for now" comments - they're fine in prototypes

## Simple Tracking

```markdown
## Stubs that actually matter:
- [ ] Import/Export lists (blocking testing)
- [ ] At least one export type (for e2e testing)
- [ ] GraphQL panics (annoying)

## Would be nice:
- [ ] Video/audio duration
- [ ] More export types
- [ ] Some GraphQL queries
```

That's it. Keep it simple. It's a prototype - some stubs are expected! 