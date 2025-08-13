#!/bin/bash
# organize_repository.sh - Reorganize Lesser repository structure
# This script moves files to their proper locations according to the cleanup plan

set -e  # Exit on error

echo "🧹 Starting Lesser repository reorganization..."

# Create new directory structure
echo "📁 Creating new directory structure..."
mkdir -p docs/{api,architecture,security/authentication,implementation/features,implementation/archive/phases,deployment,development,legal,archive}
mkdir -p tests/{integration,utilities,load,federation}
mkdir -p examples/{frontend-react,frontend-vue,bots,demos}
mkdir -p scripts
mkdir -p .github/{workflows,ISSUE_TEMPLATE}

# Move architecture documentation
echo "📚 Moving architecture documentation..."
[ -f "DESIGN.md" ] && mv DESIGN.md docs/architecture/SYSTEM_DESIGN.md
[ -f "LESSER_STORAGE_ARCHITECTURE.md" ] && mv LESSER_STORAGE_ARCHITECTURE.md docs/architecture/STORAGE_ARCHITECTURE.md
[ -f "PORTABLE_REPUTATION_DESIGN.md" ] && mv PORTABLE_REPUTATION_DESIGN.md docs/architecture/REPUTATION_SYSTEM.md
[ -f "MODERATION_MESH_DESIGN.md" ] && mv MODERATION_MESH_DESIGN.md docs/architecture/MODERATION_DESIGN.md
[ -f "TIMELINE_DESIGN.md" ] && mv TIMELINE_DESIGN.md docs/architecture/TIMELINE_DESIGN.md
[ -f "SEARCH_DESIGN.md" ] && mv SEARCH_DESIGN.md docs/architecture/SEARCH_DESIGN.md
[ -f "AI_INTEGRATION.md" ] && mv AI_INTEGRATION.md docs/architecture/AI_INTEGRATION.md
[ -f "ARCHITECTURE_DECISIONS.md" ] && mv ARCHITECTURE_DECISIONS.md docs/architecture/

# Move API documentation
echo "📚 Moving API documentation..."
[ -f "MASTODON_API_IMPLEMENTATION_PLAN.md" ] && mv MASTODON_API_IMPLEMENTATION_PLAN.md docs/api/MASTODON_API_STATUS.md
[ -f "GREATER_API_REFERENCE.md" ] && mv GREATER_API_REFERENCE.md docs/api/API_REFERENCE.md
[ -f "GRAPHQL_IMPLEMENTATION.md" ] && mv GRAPHQL_IMPLEMENTATION.md docs/api/GRAPHQL_API.md
[ -f "STREAMING_IMPLEMENTATION.md" ] && mv STREAMING_IMPLEMENTATION.md docs/api/STREAMING_API.md
[ -f "SERVER_IMPLEMENTATION_PLAN.md" ] && mv SERVER_IMPLEMENTATION_PLAN.md docs/api/

# Move security documentation
echo "🔒 Moving security documentation..."
mv MODERN_AUTH_*.md docs/security/authentication/ 2>/dev/null || true
mv AUTH_*.md docs/security/authentication/ 2>/dev/null || true
mv EMAIL_FREE_AUTH_*.md docs/security/authentication/ 2>/dev/null || true
mv WALLET_AUTH_*.md docs/security/authentication/ 2>/dev/null || true
mv WEBAUTHN_*.md docs/security/authentication/ 2>/dev/null || true
[ -f "INFRASTRUCTURE_SECURITY_ENHANCEMENTS.md" ] && mv INFRASTRUCTURE_SECURITY_ENHANCEMENTS.md docs/security/INFRASTRUCTURE_SECURITY.md
[ -f "API_SECURITY_QUICK_START.md" ] && mv API_SECURITY_QUICK_START.md docs/security/API_SECURITY.md
[ -f "QUICK_START_CONTENT_SIGNATURES.md" ] && mv QUICK_START_CONTENT_SIGNATURES.md docs/security/CONTENT_SIGNATURES.md
[ -f "SECURITY_ENHANCEMENT_SUMMARY.md" ] && mv SECURITY_ENHANCEMENT_SUMMARY.md docs/security/

# Move feature implementation guides
echo "✨ Moving feature implementation guides..."
[ -f "ANNOUNCEMENTS_IMPLEMENTATION.md" ] && mv ANNOUNCEMENTS_IMPLEMENTATION.md docs/implementation/features/ANNOUNCEMENTS.md
[ -f "CUSTOM_EMOJIS_IMPLEMENTATION.md" ] && mv CUSTOM_EMOJIS_IMPLEMENTATION.md docs/implementation/features/CUSTOM_EMOJIS.md
[ -f "TRENDS_DISCOVERY_IMPLEMENTATION.md" ] && mv TRENDS_DISCOVERY_IMPLEMENTATION.md docs/implementation/features/TRENDS.md
[ -f "PUSH_NOTIFICATIONS_IMPLEMENTATION.md" ] && mv PUSH_NOTIFICATIONS_IMPLEMENTATION.md docs/implementation/features/PUSH_NOTIFICATIONS.md
[ -f "MODERATION_HANDLERS_IMPLEMENTATION.md" ] && mv MODERATION_HANDLERS_IMPLEMENTATION.md docs/implementation/features/MODERATION.md

# Move deployment documentation
echo "🚀 Moving deployment documentation..."
mv INSTANCE_CONFIG*.md docs/deployment/ 2>/dev/null || true
[ -f "MIGRATION_GUIDE.md" ] && mv MIGRATION_GUIDE.md docs/deployment/
[ -f "DEVELOPER_GUIDELINES.md" ] && mv DEVELOPER_GUIDELINES.md docs/development/
[ -f "TESTING_OVERVIEW.md" ] && mv TESTING_OVERVIEW.md docs/development/TESTING.md
[ -f "TEST_README.md" ] && mv TEST_README.md docs/development/TEST_GUIDE.md

# Move legal documentation
echo "⚖️ Moving legal documentation..."
[ -f "PRIVACY_POLICY.md" ] && mv PRIVACY_POLICY.md docs/legal/
[ -f "TERMS_OF_SERVICE.md" ] && mv TERMS_OF_SERVICE.md docs/legal/
[ -f "LEGAL_SUMMARY.md" ] && mv LEGAL_SUMMARY.md docs/legal/

# Archive phase documentation
echo "📦 Archiving phase documentation..."
mv PHASE*.md docs/archive/phases/ 2>/dev/null || true
mv WEEK*.md docs/archive/ 2>/dev/null || true
mv *_PROGRESS.md docs/archive/ 2>/dev/null || true
mv *_COMPLETION*.md docs/archive/ 2>/dev/null || true
mv *_COMPLETE.md docs/archive/ 2>/dev/null || true
mv MONTH*.md docs/archive/ 2>/dev/null || true

# Move OpenSearch removal docs to archive
echo "📦 Archiving OpenSearch documentation..."
mv OPENSEARCH_*.md docs/archive/ 2>/dev/null || true

# Move test files
echo "🧪 Moving test files..."
mv test_*.py tests/integration/ 2>/dev/null || true
mv *_test.py tests/integration/ 2>/dev/null || true
mv federation_test_harness.py tests/utilities/ 2>/dev/null || true
mv test_data_generator.py tests/utilities/ 2>/dev/null || true
mv performance_benchmark.py tests/load/ 2>/dev/null || true
mv requirements-test.txt tests/ 2>/dev/null || true

# Move demo files
echo "🎨 Moving demo files..."
mv *_demo.html examples/demos/ 2>/dev/null || true

# Move utility scripts
echo "🔧 Moving utility scripts..."
[ -f "register_user.sh" ] && mv register_user.sh scripts/
[ -f "test-deployment.sh" ] && mv test-deployment.sh scripts/
[ -f "verify_lambdas.sh" ] && mv verify_lambdas.sh scripts/
[ -f "migrate_to_v2.sh" ] && mv migrate_to_v2.sh scripts/
[ -f "extract_handlers.py" ] && mv extract_handlers.py scripts/
[ -f "add_test_data.py" ] && mv add_test_data.py scripts/

# Move other documentation
echo "📚 Moving miscellaneous documentation..."
[ -f "AI_ASSISTANT_PROMPT.md" ] && mv AI_ASSISTANT_PROMPT.md docs/development/
[ -f "AI_IMPLEMENTATION_GUIDE.md" ] && mv AI_IMPLEMENTATION_GUIDE.md docs/development/
[ -f "FEDERATION_PROGRESS.md" ] && mv FEDERATION_PROGRESS.md docs/archive/
[ -f "PITCH.md" ] && mv PITCH.md docs/

# Create .env.example if it doesn't exist
if [ ! -f ".env.example" ]; then
    echo "🔧 Creating .env.example..."
    cat > .env.example << 'EOF'
# Lesser Environment Configuration Example
# Copy this file to .env and fill in your values

# AWS Configuration
AWS_REGION=us-east-1
AWS_ACCOUNT_ID=your-account-id

# Instance Configuration
INSTANCE_DOMAIN=your-instance.social
INSTANCE_NAME="Your Instance"
INSTANCE_DESCRIPTION="A Lesser-powered ActivityPub instance"

# Authentication
JWT_SECRET=generate-a-secure-secret
OAUTH_CLIENT_ID=your-oauth-client-id
OAUTH_CLIENT_SECRET=your-oauth-client-secret

# Optional: AI Integration
OPENAI_API_KEY=your-openai-key
ANTHROPIC_API_KEY=your-anthropic-key

# Optional: Media Storage
S3_MEDIA_BUCKET=your-media-bucket
CLOUDFRONT_DISTRIBUTION_ID=your-distribution-id

# Optional: Email (for notifications)
SES_FROM_EMAIL=noreply@your-instance.social
EOF
fi

# Update .gitignore
echo "📝 Updating .gitignore..."
cat > .gitignore << 'EOF'
# Binaries
bin/
*.exe
*.dll
*.so
*.dylib
*.test
main
api
auth-api
activity-processor.test

# Go
vendor/
go.work

# Python
__pycache__/
*.py[cod]
*$py.class
*.so
.Python
test_venv/
venv/
.env

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# AWS
.aws/
.aws-sam/

# Terraform/Pulumi
*.tfstate
*.tfstate.*
.terraform/
Pulumi.*.yaml

# Test outputs
*.log
*.out
coverage.txt
coverage.html
results.csv

# Temporary files
tmp/
temp/
*.tmp
*.bak

# API documentation
docs/api/openapi.json
docs/api/swagger-ui/
EOF

# Clean up empty directories
echo "🧹 Cleaning up empty directories..."
find . -type d -empty -delete 2>/dev/null || true

# Create documentation index
echo "📚 Creating documentation index..."
cat > docs/README.md << 'EOF'
# Lesser Documentation

Welcome to the Lesser documentation! Lesser is a serverless ActivityPub implementation that runs at 1/100th the cost of traditional solutions.

## 📖 Documentation Structure

### Getting Started
- [Quick Start Guide](deployment/QUICK_START.md) - Deploy Lesser in 15 minutes
- [Configuration Guide](deployment/CONFIGURATION.md) - Configure your instance
- [Migration Guide](deployment/MIGRATION.md) - Migrate from Mastodon

### Architecture
- [System Design](architecture/SYSTEM_DESIGN.md) - Overall architecture
- [Storage Architecture](architecture/STORAGE_ARCHITECTURE.md) - DynamoDB design
- [Cost Model](architecture/COST_MODEL.md) - Why Lesser is 100x cheaper
- [Security Model](security/README.md) - Security architecture

### API Reference
- [Mastodon API Status](api/MASTODON_API_STATUS.md) - API compatibility
- [API Reference](api/API_REFERENCE.md) - Complete API documentation
- [GraphQL API](api/GRAPHQL_API.md) - GraphQL schema and queries
- [Streaming API](api/STREAMING_API.md) - WebSocket events

### Development
- [Developer Setup](development/DEVELOPER_GUIDELINES.md) - Set up your dev environment
- [Testing Guide](development/TESTING.md) - How to test Lesser
- [Frontend Guide](development/FRONTEND_GUIDE.md) - Build UIs for Lesser
- [Contributing](../CONTRIBUTING.md) - How to contribute

### Features
- [Authentication](security/authentication/) - Modern auth system
- [Moderation](implementation/features/MODERATION.md) - Trust-based moderation
- [Search](architecture/SEARCH_DESIGN.md) - Semantic search
- [Federation](architecture/FEDERATION_DESIGN.md) - ActivityPub federation

### Legal
- [Privacy Policy](legal/PRIVACY_POLICY.md)
- [Terms of Service](legal/TERMS_OF_SERVICE.md)
- [License](../LICENSE) - GNU AGPL v3

## 🚀 Quick Links

- **Deploy Now**: [Quick Start Guide](deployment/QUICK_START.md)
- **API Docs**: [API Reference](api/API_REFERENCE.md)
- **Get Help**: [GitHub Issues](https://github.com/equaltoai/lesser/issues)

## 📊 Project Status

Lesser is 98% feature complete with full Mastodon API compatibility. See our [Progress Tracker](../PROGRESS.md) for details.
EOF

# Create CONTRIBUTING.md if it doesn't exist
if [ ! -f "CONTRIBUTING.md" ]; then
    echo "📝 Creating CONTRIBUTING.md..."
    cat > CONTRIBUTING.md << 'EOF'
# Contributing to Lesser

Thank you for your interest in contributing to Lesser! We welcome contributions of all kinds.

## How to Contribute

### Reporting Issues
- Use the GitHub issue tracker
- Include steps to reproduce
- Include error messages and logs

### Code Contributions
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

### Code Style
- Run `gofmt` before committing
- Follow existing patterns
- Add comments for complex logic
- Keep functions small and focused

### Testing
- Write tests for new features
- Ensure all tests pass
- Add integration tests for API changes

## Development Setup

See [Developer Guidelines](docs/development/DEVELOPER_GUIDELINES.md) for setup instructions.

## Questions?

Join our [Discord](https://discord.gg/lesser) or open a GitHub issue.
EOF
fi

# Final summary
echo ""
echo "✅ Repository reorganization complete!"
echo ""
echo "📊 Summary:"
echo "  - Created organized directory structure"
echo "  - Moved $(find docs -name "*.md" 2>/dev/null | wc -l) documentation files"
echo "  - Moved $(find tests -name "*.py" 2>/dev/null | wc -l) test files"
echo "  - Created .env.example and .gitignore"
echo "  - Created documentation index"
echo ""
echo "🎯 Next steps:"
echo "  1. Review the changes with: git status"
echo "  2. Commit the reorganization: git add . && git commit -m 'Reorganize repository structure'"
echo "  3. Update any broken imports in the code"
echo "  4. Create a clean README.md for the root directory" 