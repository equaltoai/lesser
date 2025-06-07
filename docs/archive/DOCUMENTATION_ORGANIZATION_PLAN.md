# Lesser Documentation Organization Plan

## Current Documentation Inventory

We have **70+ documentation files** in the root directory! Here's how to organize them:

## 📁 Documentation Reorganization

### `/docs/architecture/`
Move these architecture and design documents:
- `DESIGN.md` → `architecture/SYSTEM_DESIGN.md`
- `LESSER_STORAGE_ARCHITECTURE.md` → `architecture/STORAGE_ARCHITECTURE.md`
- `PORTABLE_REPUTATION_DESIGN.md` → `architecture/REPUTATION_SYSTEM.md`
- `MODERATION_MESH_DESIGN.md` → `architecture/MODERATION_DESIGN.md`
- `TIMELINE_DESIGN.md` → `architecture/TIMELINE_DESIGN.md`
- `SEARCH_DESIGN.md` → `architecture/SEARCH_DESIGN.md`
- `AI_INTEGRATION.md` → `architecture/AI_INTEGRATION.md`

### `/docs/api/`
Move API-related documentation:
- `MASTODON_API_IMPLEMENTATION_PLAN.md` → `api/MASTODON_API_STATUS.md`
- `GREATER_API_REFERENCE.md` → `api/API_REFERENCE.md`
- `GRAPHQL_IMPLEMENTATION.md` → `api/GRAPHQL_API.md`
- `STREAMING_IMPLEMENTATION.md` → `api/STREAMING_API.md`

### `/docs/security/`
Move security documentation:
- `MODERN_AUTH_*.md` → `security/authentication/`
- `AUTH_*.md` → `security/authentication/`
- `INFRASTRUCTURE_SECURITY_ENHANCEMENTS.md` → `security/INFRASTRUCTURE_SECURITY.md`
- `API_SECURITY_QUICK_START.md` → `security/API_SECURITY.md`
- `QUICK_START_CONTENT_SIGNATURES.md` → `security/CONTENT_SIGNATURES.md`

### `/docs/implementation/`
Move implementation guides:
- `ANNOUNCEMENTS_IMPLEMENTATION.md` → `implementation/features/ANNOUNCEMENTS.md`
- `CUSTOM_EMOJIS_IMPLEMENTATION.md` → `implementation/features/CUSTOM_EMOJIS.md`
- `TRENDS_DISCOVERY_IMPLEMENTATION.md` → `implementation/features/TRENDS.md`
- `PHASE*_*.md` → `implementation/archive/phases/`

### `/docs/deployment/`
Move deployment and operations:
- `INSTANCE_CONFIG*.md` → `deployment/CONFIGURATION.md`
- `MIGRATION_GUIDE.md` → `deployment/MIGRATION.md`
- `DEVELOPER_GUIDELINES.md` → `deployment/DEVELOPER_SETUP.md`
- `TESTING_OVERVIEW.md` → `deployment/TESTING.md`

### `/docs/legal/`
Move legal documents:
- `PRIVACY_POLICY.md` → `legal/PRIVACY_POLICY.md`
- `TERMS_OF_SERVICE.md` → `legal/TERMS_OF_SERVICE.md`
- `LEGAL_SUMMARY.md` → `legal/SUMMARY.md`
- `LICENSE` → Keep in root (required by GitHub)

### `/docs/archive/`
Archive completed phase documentation:
- All `PHASE*_*.md` files
- All `WEEK*_*.md` files
- All `*_PROGRESS.md` files
- All `*_COMPLETION*.md` files

## 🧹 Test File Organization

### Move to `/tests/`
```bash
# Python test files
mv test_*.py tests/integration/
mv *_test.py tests/integration/

# Test utilities
mv federation_test_harness.py tests/utilities/
mv test_data_generator.py tests/utilities/
mv requirements-test.txt tests/

# Demo files
mv *_demo.html examples/demos/
```

## 📝 New Documentation to Create

### 1. `/docs/README.md` - Documentation Index
```markdown
# Lesser Documentation

## Quick Links
- [Getting Started](deployment/QUICK_START.md)
- [API Reference](api/API_REFERENCE.md)
- [Architecture Overview](architecture/SYSTEM_DESIGN.md)
- [Security Model](security/README.md)

## For Developers
- [Building a Frontend](development/FRONTEND_GUIDE.md)
- [Contributing](../CONTRIBUTING.md)
- [Testing Guide](deployment/TESTING.md)

## For Operators
- [Deployment Guide](deployment/README.md)
- [Configuration](deployment/CONFIGURATION.md)
- [Monitoring](deployment/MONITORING.md)
```

### 2. Clean Root `README.md`
```markdown
# Lesser

**Serverless ActivityPub at 1/100th the cost**

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue)](go.mod)
[![Mastodon API](https://img.shields.io/badge/Mastodon%20API-Compatible-purple)](docs/api/MASTODON_API_STATUS.md)

## What is Lesser?

Lesser is headless ActivityPub infrastructure that runs entirely on AWS serverless services, achieving 100x cost reduction compared to traditional hosting.

### Key Features

🚀 **100% Serverless** - Runs on Lambda, DynamoDB, and S3  
💰 **Pay What It Costs** - $0.01-0.05 per active user/month  
🔐 **Modern Auth** - Passkeys, crypto wallets, OAuth2  
🛡️ **Reactive Moderation** - Community-driven trust system  
📊 **Cost Transparency** - Know exactly what you're paying for  
🌐 **Federation First** - Full ActivityPub support  

## Quick Start

```bash
# Clone the repository
git clone https://github.com/yourusername/lesser.git
cd lesser

# Deploy to AWS (15 minutes)
cd infra
pulumi up
```

[Full deployment guide →](docs/deployment/QUICK_START.md)

## Documentation

📖 **[View Full Documentation](docs/README.md)**

- [Architecture Overview](docs/architecture/SYSTEM_DESIGN.md)
- [API Reference](docs/api/API_REFERENCE.md)
- [Security Model](docs/security/README.md)
- [Building Frontends](docs/development/FRONTEND_GUIDE.md)

## Why Lesser?

Traditional Mastodon hosting costs $100-500/month for 1,000 users. Lesser costs $10-50/month for the same load, while providing better scalability and modern features.

[Learn more about our architecture →](docs/architecture/COST_MODEL.md)

## Community

- 🌟 [Star us on GitHub](https://github.com/yourusername/lesser)
- 💬 [Join our Discord](https://discord.gg/lesser)
- 🐘 [Follow updates](https://fosstodon.org/@lesser)

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

GNU Affero General Public License v3.0 - see [LICENSE](LICENSE)
```

### 3. `/docs/deployment/QUICK_START.md`
Consolidate all deployment information into one clear guide.

### 4. `/docs/api/OPENAPI.yaml`
Generate complete OpenAPI specification from the code.

## 🗂️ File Movement Script

```bash
#!/bin/bash
# organize_docs.sh

# Create directory structure
mkdir -p docs/{api,architecture,security/authentication,implementation/features,implementation/archive/phases,deployment,legal,archive}
mkdir -p tests/{integration,utilities,load,federation}
mkdir -p examples/{frontend-react,frontend-vue,bots,demos}
mkdir -p scripts

# Move architecture docs
mv DESIGN.md docs/architecture/SYSTEM_DESIGN.md 2>/dev/null || true
mv LESSER_STORAGE_ARCHITECTURE.md docs/architecture/STORAGE_ARCHITECTURE.md 2>/dev/null || true
# ... continue for all files

# Move test files
mv test_*.py tests/integration/ 2>/dev/null || true
mv *_test_*.py tests/integration/ 2>/dev/null || true

# Move phase documentation to archive
mv PHASE*.md docs/archive/phases/ 2>/dev/null || true
mv WEEK*.md docs/archive/ 2>/dev/null || true

# Clean up empty directories
find . -type d -empty -delete

echo "Documentation reorganization complete!"
```

## 📊 Documentation Metrics

### Before Reorganization
- 70+ files in root directory
- No clear structure
- Mixed test files and docs
- Hard to find information

### After Reorganization
- Clean root with only essential files
- Logical documentation structure
- All tests in `/tests/`
- Easy navigation with clear hierarchy

## 🎯 Implementation Timeline

### Day 1-2: File Organization
- Run organization script
- Verify all files moved correctly
- Update any broken links
- Commit with clear message

### Day 3-4: Documentation Updates
- Create new README.md files
- Update navigation/index files
- Generate API documentation
- Add missing guides

### Day 5: Testing & Polish
- Test all documentation links
- Ensure examples work
- Update import paths if needed
- Create documentation site

## 📚 Documentation Site

Consider using:
- **Docusaurus** - React-based, great for technical docs
- **MkDocs** - Python-based, simple and clean
- **GitBook** - Easy to use, good search
- **GitHub Pages** - Simple, free hosting

## ✅ Success Criteria

1. **Findability**: Can locate any document in <30 seconds
2. **Clarity**: New developers understand structure immediately
3. **Completeness**: All features documented
4. **Maintainability**: Easy to add new docs
5. **Searchability**: Full-text search available

This organization will transform Lesser from a project with great code but chaotic docs into a professional, well-documented platform ready for widespread adoption! 