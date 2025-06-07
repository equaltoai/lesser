# AI Assistant Prompt 2: Fix Cross-References Throughout Documentation

## Context
Following a major repository reorganization, we need to fix all cross-references between documentation files. The first AI assistant has already updated the main README and high-level docs. Your task is to systematically go through ALL other documentation files and fix broken internal links.

## File Movements Reference
Here are the key file movements you need to know about:

### Architecture Documentation
- `DESIGN.md` → `docs/architecture/SYSTEM_DESIGN.md`
- `LESSER_STORAGE_ARCHITECTURE.md` → `docs/architecture/STORAGE_ARCHITECTURE.md`
- `PORTABLE_REPUTATION_DESIGN.md` → `docs/architecture/REPUTATION_SYSTEM.md`
- `MODERATION_MESH_DESIGN.md` → `docs/architecture/MODERATION_DESIGN.md`
- `AI_INTEGRATION.md` → `docs/architecture/AI_INTEGRATION.md`

### API Documentation
- `GREATER_API_REFERENCE.md` → `docs/api/API_REFERENCE.md`
- `MASTODON_API_IMPLEMENTATION_PLAN.md` → `docs/api/MASTODON_API_STATUS.md`
- `GRAPHQL_IMPLEMENTATION.md` → `docs/api/GRAPHQL_API.md`
- `STREAMING_IMPLEMENTATION.md` → `docs/api/STREAMING_API.md`

### Feature Implementation
- `ANNOUNCEMENTS_IMPLEMENTATION.md` → `docs/implementation/features/ANNOUNCEMENTS.md`
- `CUSTOM_EMOJIS_IMPLEMENTATION.md` → `docs/implementation/features/CUSTOM_EMOJIS.md`
- `MODERATION_HANDLERS_IMPLEMENTATION.md` → `docs/implementation/features/MODERATION.md`

### Development Documentation
- `DEVELOPER_GUIDELINES.md` → `docs/development/DEVELOPER_GUIDELINES.md`
- `TESTING_OVERVIEW.md` → `docs/development/TESTING.md`

### Archived Documents
- `PROGRESS.md` → `docs/archive/PROGRESS.md`
- All `PHASE*.md` files → `docs/archive/phases/PHASE*.md`
- All `WEEK*.md` files → `docs/archive/WEEK*.md`

## Your Tasks

### 1. Scan All Documentation Files
Go through each .md file in the docs/ directory and:
- Find all internal links (both markdown links and references in backticks)
- Check if they point to files that have been moved
- Update them with the correct relative path from the current file

### 2. Fix Different Types of References

#### Markdown Links
```markdown
<!-- Old -->
[See the design document](../../DESIGN.md)
<!-- New (from docs/api/some-file.md) -->
[See the design document](../architecture/SYSTEM_DESIGN.md)
```

#### Inline Code References
```markdown
<!-- Old -->
See `PHASE3_AI_SEARCH_IMPLEMENTATION.md` for details
<!-- New -->
See `docs/archive/phases/PHASE3_AI_SEARCH_IMPLEMENTATION.md` for details
```

#### List References
```markdown
<!-- Old -->
- Design: DESIGN.md
- API: GREATER_API_REFERENCE.md
<!-- New -->
- Design: architecture/SYSTEM_DESIGN.md  
- API: api/API_REFERENCE.md
```

### 3. Special Handling for Phase References
Phase documents are now historical artifacts. When you find references to them:
- If the reference is for historical context → Update path to `archive/phases/`
- If the reference is for current functionality → Consider if there's a better current doc to link to
- If in doubt → Update the path but add a note that this is archived documentation

### 4. Calculate Relative Paths Correctly
Remember that relative paths depend on where the file is located:

From `docs/api/QUICK_REFERENCE.md`:
- To `docs/architecture/SYSTEM_DESIGN.md` → `../architecture/SYSTEM_DESIGN.md`
- To `docs/archive/PROGRESS.md` → `../archive/PROGRESS.md`

From `docs/implementation/features/MODERATION.md`:
- To `docs/architecture/MODERATION_DESIGN.md` → `../../architecture/MODERATION_DESIGN.md`
- To `docs/api/API_REFERENCE.md` → `../../api/API_REFERENCE.md`

## Areas to Focus On

1. **API Documentation** (`docs/api/`)
   - Many references to implementation guides
   - Links to phase documentation

2. **Architecture Docs** (`docs/architecture/`)
   - Cross-references between design documents
   - Links to API documentation

3. **Implementation Guides** (`docs/implementation/features/`)
   - References to design docs
   - Links to API endpoints

4. **Archive Files** (`docs/archive/`)
   - May have many old references but lower priority
   - Focus on files that users might actually read

## Guidelines

1. **Accuracy**: Double-check that your relative paths are correct
2. **Context**: Preserve the context and meaning of all references
3. **Completeness**: Don't miss references in:
   - Code blocks
   - Lists
   - Tables
   - Quoted sections
4. **Testing**: Mentally trace each path to ensure it would resolve correctly

## What NOT to Do
- Don't change file contents beyond fixing links
- Don't remove historical references (update their paths instead)
- Don't add new content
- Don't change link text unless necessary for clarity

## Priority Order
1. `docs/api/*` - Most accessed by developers
2. `docs/deployment/*` - Used by new users
3. `docs/architecture/*` - Important technical docs
4. `docs/implementation/*` - Feature references
5. `docs/development/*` - Contributor docs
6. `docs/archive/*` - Historical (lower priority)

## Verification
After each file, ensure:
- All markdown links use correct relative paths
- All backtick references are updated
- The file would still make sense to readers
- No links are accidentally broken

Please proceed systematically through the documentation, starting with the highest priority directories. 