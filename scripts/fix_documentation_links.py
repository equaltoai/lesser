#!/usr/bin/env python3
"""
Fix documentation links after repository reorganization.
This script updates all markdown files to use the new file paths.
"""

import os
import re
from pathlib import Path

# Define link mappings from old to new paths
LINK_MAPPINGS = {
    # Architecture docs
    "DESIGN.md": "docs/architecture/SYSTEM_DESIGN.md",
    "LESSER_STORAGE_ARCHITECTURE.md": "docs/architecture/STORAGE_ARCHITECTURE.md",
    "PORTABLE_REPUTATION_DESIGN.md": "docs/architecture/REPUTATION_SYSTEM.md",
    "MODERATION_MESH_DESIGN.md": "docs/architecture/MODERATION_DESIGN.md",
    "TIMELINE_DESIGN.md": "docs/architecture/TIMELINE_DESIGN.md",
    "SEARCH_DESIGN.md": "docs/architecture/SEARCH_DESIGN.md",
    "AI_INTEGRATION.md": "docs/architecture/AI_INTEGRATION.md",
    
    # API docs
    "GREATER_API_REFERENCE.md": "docs/api/API_REFERENCE.md",
    "MASTODON_API_IMPLEMENTATION_PLAN.md": "docs/api/MASTODON_API_STATUS.md",
    "GRAPHQL_IMPLEMENTATION.md": "docs/api/GRAPHQL_API.md",
    "STREAMING_IMPLEMENTATION.md": "docs/api/STREAMING_API.md",
    "SERVER_IMPLEMENTATION_PLAN.md": "docs/api/SERVER_IMPLEMENTATION_PLAN.md",
    
    # Security docs
    "AUTH_IMPLEMENTATION_QUICK_START.md": "docs/security/authentication/AUTH_IMPLEMENTATION_QUICK_START.md",
    "AUTH_INFRASTRUCTURE_SECURITY.md": "docs/security/authentication/AUTH_INFRASTRUCTURE_SECURITY.md",
    
    # Development docs
    "DEVELOPER_GUIDELINES.md": "docs/development/DEVELOPER_GUIDELINES.md",
    "TESTING_OVERVIEW.md": "docs/development/TESTING.md",
    "TEST_README.md": "docs/development/TEST_GUIDE.md",
    
    # Archive docs
    "PROGRESS.md": "docs/archive/PROGRESS.md",
    "FEDERATION_PROGRESS.md": "docs/archive/FEDERATION_PROGRESS.md",
    
    # Phase docs (all moved to archive)
    "PHASE1_COST_TRACKING_STATUS.md": "docs/archive/phases/PHASE1_COST_TRACKING_STATUS.md",
    "PHASE2_MODERATION_COMPLETE.md": "docs/archive/phases/PHASE2_MODERATION_COMPLETE.md",
    "PHASE3_AI_SEARCH_IMPLEMENTATION.md": "docs/archive/phases/PHASE3_AI_SEARCH_IMPLEMENTATION.md",
    "PHASE3_DEBUG_ENDPOINTS.md": "docs/archive/phases/PHASE3_DEBUG_ENDPOINTS.md",
    "PHASE3_IMPLEMENTATION_PLAN.md": "docs/archive/phases/PHASE3_IMPLEMENTATION_PLAN.md",
    "PHASE3_TESTING_UTILITIES.md": "docs/archive/phases/PHASE3_TESTING_UTILITIES.md",
    "PHASE4_INSTANCE_FEATURES.md": "docs/archive/phases/PHASE4_INSTANCE_FEATURES.md",
    "PHASE4_2_COMMUNITY_NOTES.md": "docs/archive/phases/PHASE4_2_COMMUNITY_NOTES.md",
    "PHASE5_USER_FEATURES_IMPLEMENTATION.md": "docs/archive/phases/PHASE5_USER_FEATURES_IMPLEMENTATION.md",
    "PHASE6_MEDIA_IMPORT_EXPORT.md": "docs/archive/phases/PHASE6_MEDIA_IMPORT_EXPORT.md",
    "PHASE7_ADMIN_API_IMPLEMENTATION.md": "docs/archive/phases/PHASE7_ADMIN_API_IMPLEMENTATION.md",
    "PHASE7_DOMAIN_FEDERATION_IMPLEMENTATION.md": "docs/archive/phases/PHASE7_DOMAIN_FEDERATION_IMPLEMENTATION.md",
    
    # Instance config
    "INSTANCE_CONFIG.md": "docs/deployment/INSTANCE_CONFIG.md",
    "MIGRATION_GUIDE.md": "docs/deployment/MIGRATION_GUIDE.md",
}

def calculate_relative_path(from_file, to_file):
    """Calculate the relative path from one file to another."""
    from_path = Path(from_file).parent
    to_path = Path(to_file)
    
    try:
        return os.path.relpath(to_path, from_path)
    except ValueError:
        # If on different drives on Windows, use absolute path
        return "/" + to_file

def fix_markdown_links(file_path, content):
    """Fix markdown links in the content based on the file's location."""
    modified = False
    
    # Pattern to match markdown links
    link_pattern = re.compile(r'\[([^\]]+)\]\(([^)]+)\)')
    
    def replace_link(match):
        nonlocal modified
        link_text = match.group(1)
        link_url = match.group(2)
        
        # Skip external links
        if link_url.startswith(('http://', 'https://', '#', 'mailto:')):
            return match.group(0)
        
        # Check if this is a link to a moved file
        link_file = os.path.basename(link_url)
        
        if link_file in LINK_MAPPINGS:
            new_path = LINK_MAPPINGS[link_file]
            # Calculate relative path from current file to new location
            relative_path = calculate_relative_path(file_path, new_path)
            modified = True
            return f'[{link_text}]({relative_path})'
        
        return match.group(0)
    
    new_content = link_pattern.sub(replace_link, content)
    
    # Also fix references in backticks
    for old_file, new_file in LINK_MAPPINGS.items():
        if f'`{old_file}`' in new_content:
            new_content = new_content.replace(f'`{old_file}`', f'`{new_file}`')
            modified = True
    
    return new_content, modified

def process_file(file_path):
    """Process a single markdown file."""
    print(f"Processing: {file_path}")
    
    with open(file_path, 'r', encoding='utf-8') as f:
        content = f.read()
    
    new_content, modified = fix_markdown_links(file_path, content)
    
    if modified:
        with open(file_path, 'w', encoding='utf-8') as f:
            f.write(new_content)
        print(f"  ✅ Updated links in {file_path}")
        return True
    
    return False

def main():
    """Main function to process all markdown files."""
    print("🔍 Scanning for markdown files...")
    
    # Find all markdown files
    markdown_files = []
    for root, dirs, files in os.walk('.'):
        # Skip hidden directories and common non-doc directories
        dirs[:] = [d for d in dirs if not d.startswith('.') and d not in ['node_modules', 'vendor', 'bin']]
        
        for file in files:
            if file.endswith('.md'):
                markdown_files.append(os.path.join(root, file))
    
    print(f"📄 Found {len(markdown_files)} markdown files")
    
    # Process each file
    updated_count = 0
    for file_path in markdown_files:
        if process_file(file_path):
            updated_count += 1
    
    print(f"\n✅ Updated {updated_count} files")
    print("🎉 Documentation links fixed!")

if __name__ == "__main__":
    main() 