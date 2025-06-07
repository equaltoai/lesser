# AI Assistant Prompt 1: Update Main README and High-Level Documentation

## Context
We have just completed a major repository reorganization for the Lesser project. All documentation has been moved from the root directory into a proper hierarchy under `docs/`. Your task is to update the main README.md and other high-level documentation to reflect these changes.

## Current Structure
```
lesser/
├── docs/
│   ├── api/                    # API documentation
│   ├── architecture/           # System design docs
│   ├── security/              # Security documentation
│   │   └── authentication/    # Auth-specific docs
│   ├── deployment/            # Deployment guides
│   ├── development/           # Developer documentation
│   ├── implementation/        # Implementation guides
│   │   └── features/         # Feature-specific guides
│   ├── legal/                # Legal documents
│   └── archive/              # Historical documentation
│       └── phases/           # Old phase documents
├── tests/                     # Test files
├── examples/                  # Example implementations
└── scripts/                   # Utility scripts
```

## Your Tasks

### 1. Update the Main README.md (root directory)
- Fix all broken links to documentation that has been moved
- Update the "Documentation" section to reflect the new structure
- Ensure all references use relative paths from the root
- Remove or update any references to PHASE*.md files (now in archive)
- Keep the README focused and user-friendly

Example transformations:
- `[Design Document](DESIGN.md)` → `[Design Document](docs/architecture/SYSTEM_DESIGN.md)`
- `[Progress Tracker](PROGRESS.md)` → `[Progress Tracker](docs/archive/PROGRESS.md)`
- `[Developer Guidelines](DEVELOPER_GUIDELINES.md)` → `[Developer Guidelines](docs/development/DEVELOPER_GUIDELINES.md)`

### 2. Update docs/README.md
- This should be the main documentation index
- Ensure all links work from this location
- Organize sections logically
- Add any missing important documents

### 3. Check High-Level Entry Points
Review and update these key files:
- `docs/deployment/QUICK_START.md` - New users start here
- `docs/api/API_REFERENCE.md` - Developers start here
- `docs/architecture/OVERVIEW.md` - Technical overview
- `CONTRIBUTING.md` - Contributor entry point

## Guidelines

1. **Preserve Intent**: Keep the original meaning and purpose of all documentation
2. **User Experience**: Make it easy for new users to find what they need
3. **Relative Paths**: Use relative paths that work from each file's location
4. **Archive References**: References to PHASE*.md files should either:
   - Be removed if not essential
   - Updated to point to `docs/archive/phases/` if historically important
   - Replaced with references to the relevant feature documentation
5. **Test Links**: Mentally verify each link would work from its file location

## What NOT to Do
- Don't change the actual content/meaning of documentation
- Don't add new features or promises
- Don't remove important information
- Don't use absolute paths

## Verification Checklist
After updates, ensure:
- [ ] Main README.md has no broken links
- [ ] Documentation section clearly shows the new structure
- [ ] New users can easily find the Quick Start guide
- [ ] Developers can easily find API documentation
- [ ] Contributors can find contributing guidelines
- [ ] All paths work from their respective file locations

## Example Link Updates

From root README.md:
```markdown
<!-- Old -->
See [DESIGN.md](DESIGN.md) for architecture details

<!-- New -->
See [System Design](docs/architecture/SYSTEM_DESIGN.md) for architecture details
```

From docs/deployment/QUICK_START.md:
```markdown
<!-- Old -->
[Enable AI Features](../../AI_INTEGRATION.md)

<!-- New -->
[Enable AI Features](../architecture/AI_INTEGRATION.md)
```

Please proceed with updating these files, focusing on accuracy and maintaining a great user experience. 