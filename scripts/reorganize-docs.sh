#!/bin/bash

# Lesser Documentation Reorganization Script
# This script helps migrate existing documentation to the new structure

set -e

echo "🚀 Starting Lesser documentation reorganization..."

# Create new directory structure
echo "📁 Creating new directory structure..."

mkdir -p docs/getting-started
mkdir -p docs/guides/{deployment,administration,moderation,customization,migration,security,troubleshooting}
mkdir -p docs/reference/api/{rest,graphql,websocket}
mkdir -p docs/reference/{architecture,configuration,cli,features}
mkdir -p docs/concepts
mkdir -p docs/tutorials
mkdir -p docs/use-cases
mkdir -p docs/development
mkdir -p docs/archive/{development,audits,artifacts,implementation,old-docs}

echo "✅ Directory structure created"

# Function to safely move files
move_if_exists() {
    if [ -f "$1" ]; then
        echo "  Moving $1 to $2"
        mv "$1" "$2"
    fi
}

# Function to safely copy files
copy_if_exists() {
    if [ -f "$1" ]; then
        echo "  Copying $1 to $2"
        cp "$1" "$2"
    fi
}

echo ""
echo "📋 Migrating Getting Started content..."

# Migrate getting started content
copy_if_exists "docs/deployment/QUICK_START.md" "docs/getting-started/quick-deploy.md"
copy_if_exists "docs/deployment/INSTANCE_CONFIG_EXAMPLE.md" "docs/getting-started/first-instance.md"

echo ""
echo "📚 Migrating API documentation..."

# Migrate API documentation
copy_if_exists "docs/api/API_REFERENCE.md" "docs/reference/api/rest/README.md"
copy_if_exists "docs/api/GRAPHQL_API.md" "docs/reference/api/graphql/README.md"
copy_if_exists "docs/api/STREAMING_API.md" "docs/reference/api/websocket/README.md"
copy_if_exists "docs/api/MASTODON_API_STATUS.md" "docs/reference/api/rest/mastodon-compatibility.md"

echo ""
echo "🏗️ Migrating architecture documentation..."

# Migrate architecture docs
copy_if_exists "docs/architecture/SYSTEM_DESIGN.md" "docs/reference/architecture/system-design.md"
copy_if_exists "docs/architecture/STORAGE_ARCHITECTURE.md" "docs/reference/architecture/storage.md"
copy_if_exists "docs/architecture/SEARCH_DESIGN.md" "docs/reference/architecture/search.md"
copy_if_exists "docs/architecture/TIMELINE_DESIGN.md" "docs/reference/architecture/timelines.md"
copy_if_exists "docs/architecture/MODERATION_DESIGN.md" "docs/reference/architecture/moderation.md"
copy_if_exists "docs/architecture/AI_INTEGRATION.md" "docs/concepts/ai-features.md"

echo ""
echo "🔐 Migrating security documentation..."

# Migrate security docs
copy_if_exists "docs/security/API_SECURITY.md" "docs/guides/security/api-security.md"
copy_if_exists "docs/security/INFRASTRUCTURE_SECURITY.md" "docs/guides/security/infrastructure.md"
copy_if_exists "docs/security/CONTENT_SIGNATURES.md" "docs/guides/security/signatures.md"

echo ""
echo "👥 Migrating use case documentation..."

# Migrate use cases
copy_if_exists "docs/use-cases/COMMUNITY_ORGANIZATIONS.md" "docs/use-cases/community.md"
copy_if_exists "docs/use-cases/GOVERNMENT_DIGITAL_SERVICES.md" "docs/use-cases/government.md"
copy_if_exists "docs/use-cases/RESEARCH_PLATFORM.md" "docs/use-cases/education.md"

echo ""
echo "🛠️ Migrating development documentation..."

# Migrate development docs
copy_if_exists "docs/development/DEVELOPER_GUIDELINES.md" "docs/development/setup.md"
copy_if_exists "docs/development/TESTING.md" "docs/development/testing.md"
copy_if_exists "CONTRIBUTING.md" "docs/development/contributing.md"

echo ""
echo "🗄️ Archiving old documentation..."

# Archive old docs
move_if_exists "docs/archive/phases" "docs/archive/development/"
move_if_exists "docs/archive/development-artifacts" "docs/archive/artifacts/"
move_if_exists "docs/audits" "docs/archive/audits/"

# Archive files that will be replaced
copy_if_exists "README.md" "docs/archive/old-docs/README_OLD.md"
copy_if_exists "docs/DOCUMENTATION_INDEX.md" "docs/archive/old-docs/DOCUMENTATION_INDEX_OLD.md"

echo ""
echo "📝 Creating placeholder files for new content..."

# Create placeholder files for new content that doesn't exist yet
touch docs/getting-started/connect-apps.md
touch docs/getting-started/join-fediverse.md
touch docs/guides/deployment/production.md
touch docs/guides/deployment/dns.md
touch docs/guides/deployment/ssl.md
touch docs/guides/deployment/scaling.md
touch docs/guides/administration/README.md
touch docs/guides/administration/users.md
touch docs/guides/administration/federation.md
touch docs/guides/administration/cost-monitoring.md
touch docs/guides/administration/backups.md
touch docs/guides/administration/ai-setup.md
touch docs/guides/administration/cost-optimization.md
touch docs/guides/administration/invites.md
touch docs/guides/administration/sso.md
touch docs/guides/moderation/basic-setup.md
touch docs/guides/moderation/policies.md
touch docs/guides/moderation/tools.md
touch docs/guides/moderation/federation-moderation.md
touch docs/guides/customization/branding.md
touch docs/guides/customization/themes.md
touch docs/guides/customization/features.md
touch docs/guides/troubleshooting/README.md
touch docs/guides/troubleshooting/auth.md
touch docs/concepts/federation.md
touch docs/concepts/serverless.md
touch docs/concepts/cost-model.md
touch docs/concepts/security-model.md
touch docs/concepts/five-day-story.md
touch docs/tutorials/build-client.md
touch docs/tutorials/create-bot.md
touch docs/tutorials/extend-api.md
touch docs/tutorials/custom-search.md
touch docs/tutorials/migrate-mastodon.md
touch docs/tutorials/custom-domain.md
touch docs/tutorials/enable-ai.md
touch docs/tutorials/configure-sso.md
touch docs/use-cases/business.md
touch docs/reference/faq.md
touch docs/reference/features/compatibility-matrix.md

echo ""
echo "✅ Documentation reorganization complete!"
echo ""
echo "📋 Next steps:"
echo "1. Review the new structure in docs/"
echo "2. Fill in placeholder files with content"
echo "3. Update internal links in all documents"
echo "4. Replace README.md with README_NEW.md when ready"
echo "5. Delete old/archived files after verification"
echo ""
echo "📊 Summary:"
echo "  - New directories created: 15+"
echo "  - Files migrated: $(find docs/reference docs/getting-started docs/guides -type f -name "*.md" 2>/dev/null | wc -l)"
echo "  - Placeholder files created: $(find docs -type f -size 0 -name "*.md" 2>/dev/null | wc -l)"
echo "  - Files archived: $(find docs/archive -type f -name "*.md" 2>/dev/null | wc -l)"
