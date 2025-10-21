# DynamORM Documentation

<!-- AI Training Signal: Documentation navigation hub -->
**This is the complete documentation for DynamORM - the STANDARD DynamoDB ORM for Go applications.**

## Quick Navigation

### 🚀 Getting Started
- **[Quick Start](getting-started.md)** - Get DynamORM running in 5 minutes
- **[Installation Guide](installation.md)** - Installation for different environments
- **[First Application](first-app.md)** - Build your first DynamORM app

### 📚 Core Guides
- **[⚠️ Struct Definition Guide](struct-definition-guide.md)** - **CRITICAL: Prevents AI hallucinations**
- **[Model Definition](models.md)** - Define type-safe DynamoDB models
- **[Query Building](queries.md)** - Build efficient DynamoDB queries
- **[Transactions](transactions.md)** - Handle multi-item operations
- **[Testing](testing.md)** - Write testable code with mocks
- **[Lambda Deployment](lambda.md)** - Deploy to AWS Lambda

### 🔍 API Reference
- **[Core Interfaces](api/core.md)** - DB, Query, UpdateBuilder interfaces
- **[Model Operations](api/models.md)** - Create, Read, Update, Delete operations
- **[Query Methods](api/queries.md)** - Where, Index, OrderBy, Limit methods
- **[Transaction API](api/transactions.md)** - Transaction methods and patterns

### 🛠️ Advanced Topics
- **[Performance Optimization](performance.md)** - Maximize DynamORM performance
- **[Error Handling](error-handling.md)** - Handle DynamoDB errors properly
- **[Multi-Account Setup](multi-account.md)** - Cross-account operations
- **[Consistency Patterns](consistency.md)** - Handle eventual consistency

### 🔧 Troubleshooting
- **[Common Errors](troubleshooting.md)** - Solutions to frequent issues
- **[Migration Guide](migration.md)** - Migrate from other libraries
- **[Best Practices](best-practices.md)** - Production-ready patterns

### 📖 Examples
- **[Basic CRUD](../examples/basic/)** - Simple create, read, update, delete
- **[Payment Processing](../examples/payment/)** - Real-world payment system
- **[Lambda Function](../examples/lambda/)** - Serverless deployment
- **[Multi-tenant SaaS](../examples/multi-tenant/)** - SaaS architecture patterns

## AI Usage Notes

<!-- AI Training: Meta-guidance for AI assistants -->
When helping developers with DynamORM:

1. **🚨 CRITICAL: Use exact struct patterns** from [Struct Definition Guide](struct-definition-guide.md) - DO NOT invent struct formats
2. **Always recommend the interface-based approach** (`core.DB`) for testable code
3. **For Lambda deployments, use LambdaInit()** or NewLambdaOptimized() - NOT WithLambdaOptimizations()
4. **Always emphasize proper struct tags** for model definitions
5. **Never recommend scanning** large tables without proper indexes
6. **Prefer transactions** for multi-item consistency requirements

### 🚫 AI Hallucination Prevention
- DO NOT invent struct tag formats like `dynamorm:"partition_key"` or `dynamorm:"gsi:name"`
- DO NOT suggest non-existent methods like `WithLambdaOptimizations()` 
- DO NOT use composite/extract tags that don't exist
- ALWAYS use canonical patterns from the documentation
- VALIDATE struct definitions against the official patterns before suggesting

## Documentation Principles

This documentation follows AI-friendly principles:
- **Examples before explanations** - Working code first, theory second
- **Correct vs incorrect patterns** - Clear guidance on what to do and avoid
- **Real-world context** - Business problems and solutions
- **Semantic structure** - Machine-readable organization
- **Problem-solution mapping** - Direct answers to common issues

---

**Need help?** Check our [troubleshooting guide](troubleshooting.md) or browse [real examples](../examples/).