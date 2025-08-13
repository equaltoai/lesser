#!/bin/bash

# Lesser Documentation Cleanup Script
# Archives old documentation and creates clean structure

set -e

echo "🧹 Starting Lesser documentation cleanup..."

# Create archive structure
echo "📁 Creating archive structure..."
mkdir -p docs/archive/old-2024
mkdir -p docs/archive/old-2024/{api,architecture,implementation,phases,testing,services,observability,features,audits}

# Function to archive directory
archive_dir() {
    if [ -d "$1" ]; then
        echo "  Archiving $1..."
        mv "$1" "$2" 2>/dev/null || true
    fi
}

# Function to archive file
archive_file() {
    if [ -f "$1" ]; then
        echo "  Archiving $1..."
        mv "$1" "$2" 2>/dev/null || true
    fi
}

echo ""
echo "🗄️ Archiving old development artifacts..."

# Archive old implementation docs (these are development artifacts)
archive_dir "docs/implementation" "docs/archive/old-2024/"
archive_dir "docs/phases" "docs/archive/old-2024/"
archive_dir "docs/audits" "docs/archive/old-2024/"
archive_dir "docs/services" "docs/archive/old-2024/"
archive_dir "docs/observability" "docs/archive/old-2024/"
archive_dir "docs/features" "docs/archive/old-2024/"

# Archive old API docs that have been reorganized
if [ -d "docs/api" ]; then
    # Keep only essential API docs in reference
    mkdir -p docs/archive/old-2024/api
    find docs/api -name "*.md" -type f ! -name "README.md" -exec mv {} docs/archive/old-2024/api/ \; 2>/dev/null || true
fi

# Archive old architecture docs that have been migrated
if [ -d "docs/architecture" ]; then
    mkdir -p docs/archive/old-2024/architecture
    # Keep only files that aren't in reference/architecture
    find docs/architecture -name "*.md" -type f -exec mv {} docs/archive/old-2024/architecture/ \; 2>/dev/null || true
fi

# Archive old testing docs
if [ -d "docs/testing" ]; then
    archive_dir "docs/testing" "docs/archive/old-2024/"
fi

echo ""
echo "📝 Archiving duplicate and outdated files..."

# Archive old root-level docs that are duplicates or outdated
archive_file "docs/DOCUMENTATION_INDEX.md" "docs/archive/old-2024/DOCUMENTATION_INDEX_OLD.md"
archive_file "docs/MVP_COMPLETE_SUMMARY.md" "docs/archive/old-2024/MVP_COMPLETE_SUMMARY.md"
archive_file "docs/FEATURES.md" "docs/archive/old-2024/FEATURES.md"
archive_file "docs/PITCH.md" "docs/archive/old-2024/PITCH.md"
archive_file "docs/PROGRESS.md" "docs/archive/old-2024/PROGRESS.md"
archive_file "docs/AI_ORCHESTRATION_METHODOLOGY.md" "docs/archive/old-2024/AI_ORCHESTRATION_METHODOLOGY.md"
archive_file "docs/FEDERATION_ANALYTICS_IMPLEMENTATION.md" "docs/archive/old-2024/FEDERATION_ANALYTICS_IMPLEMENTATION.md"
archive_file "docs/STORAGE_CONSOLIDATION_PLAN.md" "docs/archive/old-2024/STORAGE_CONSOLIDATION_PLAN.md"
archive_file "docs/README.md" "docs/archive/old-2024/README_OLD.md"

# Archive planning and development docs
for file in docs/*PLAN*.md docs/*IMPLEMENTATION*.md docs/*SUMMARY*.md docs/*PROGRESS*.md docs/*CHECKLIST*.md; do
    if [ -f "$file" ]; then
        filename=$(basename "$file")
        archive_file "$file" "docs/archive/old-2024/$filename"
    fi
done

echo ""
echo "🧹 Cleaning up empty directories..."

# Remove empty directories
find docs -type d -empty -delete 2>/dev/null || true

echo ""
echo "✅ Creating clean structure..."

# Ensure clean structure exists
mkdir -p docs/{getting-started,guides,reference,concepts,tutorials,use-cases}
mkdir -p docs/guides/{deployment,administration,moderation,customization,security,troubleshooting}
mkdir -p docs/reference/{api,architecture,configuration,features}
mkdir -p docs/reference/api/{rest,graphql,websocket}

echo ""
echo "📊 Documentation cleanup complete!"
echo ""
echo "Clean structure:"
echo "docs/"
echo "├── index.md                 # Documentation hub"
echo "├── getting-started/         # New user guides"
echo "├── guides/                  # How-to guides"
echo "│   ├── deployment/"
echo "│   ├── administration/"
echo "│   ├── moderation/"
echo "│   ├── customization/"
echo "│   ├── security/"
echo "│   └── troubleshooting/"
echo "├── reference/               # Technical reference"
echo "│   ├── api/"
echo "│   ├── architecture/"
echo "│   ├── configuration/"
echo "│   └── features/"
echo "├── concepts/                # Conceptual guides"
echo "├── tutorials/               # Step-by-step tutorials"
echo "├── use-cases/              # Industry solutions"
echo "└── archive/                # Historical docs"
echo ""
echo "📋 Statistics:"
echo "  - Markdown files in clean structure: $(find docs -name "*.md" -not -path "*/archive/*" 2>/dev/null | wc -l)"
echo "  - Archived files: $(find docs/archive -name "*.md" 2>/dev/null | wc -l)"
echo "  - Space saved: $(du -sh docs/archive 2>/dev/null | cut -f1)"
