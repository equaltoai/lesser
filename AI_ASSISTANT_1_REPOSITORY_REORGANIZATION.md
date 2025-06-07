# AI Assistant 1: Repository Reorganization Task

## Your Mission
You are responsible for reorganizing the Lesser repository structure. Lesser is now 100% feature-complete and needs its repository cleaned up for public release.

## Task Overview
Execute the repository reorganization by moving files to their proper locations according to our cleanup plan. You'll be working in parallel with another AI assistant who is creating documentation.

## Specific Tasks

### 1. Execute the Organization Script
```bash
# First, make it executable
chmod +x scripts/organize_repository.sh

# Then run it
./scripts/organize_repository.sh
```

### 2. Fix Any Issues
- If the script encounters errors, fix them
- Ensure all files are moved to correct locations
- Update any broken symlinks

### 3. Update Import Paths
After moving files, you need to:
- Search for any Go imports that reference moved files
- Update import paths in Go code if needed
- Ensure the project still compiles

### 4. Clean Up Remaining Files
- Move any files the script missed
- Delete empty directories
- Ensure test binaries (*.test, main, api, auth-api) are in .gitignore

### 5. Verify Organization
Check that the new structure matches:
```
lesser/
├── cmd/                    # Lambda functions
├── pkg/                    # Core packages
├── infra/                  # Infrastructure as Code
├── docs/                   # ALL documentation
│   ├── api/               # API docs
│   ├── architecture/      # System design
│   ├── security/          # Security docs
│   ├── deployment/        # Deployment guides
│   ├── development/       # Developer guides
│   ├── legal/            # Terms, privacy
│   └── archive/          # Historical docs
├── tests/                 # All test files
├── examples/              # Example implementations
├── scripts/              # Utility scripts
├── .github/              # GitHub specific
├── README.md             # Keep existing for now
├── CONTRIBUTING.md       # Will be created by other assistant
└── LICENSE              # Keep in root
```

## Important Notes

1. **Preserve Git History**: Use `git mv` when moving files manually
2. **Test Compilation**: Run `go build ./...` after moving files
3. **Don't Touch**: 
   - The other AI assistant is creating new documentation
   - Don't modify README.md yet (other assistant will handle)
   - Keep go.mod, go.sum, Makefile in root

4. **Archive These Patterns**:
   - `PHASE*.md` → `docs/archive/phases/`
   - `WEEK*.md` → `docs/archive/`
   - `*_PROGRESS.md` → `docs/archive/`
   - `*_COMPLETION*.md` → `docs/archive/`
   - `OPENSEARCH_*.md` → `docs/archive/`

5. **Handle Test Files**:
   - `test_*.py` → `tests/integration/`
   - `*_test_harness.py` → `tests/utilities/`
   - `*_demo.html` → `examples/demos/`

## Success Criteria

- [ ] All documentation files moved to `docs/` hierarchy
- [ ] All test files moved to `tests/`
- [ ] Root directory contains only essential files
- [ ] Project still compiles successfully
- [ ] No broken imports
- [ ] Git history preserved for moved files

## Final Step
Once complete, prepare a summary of:
1. Files moved (count by category)
2. Any issues encountered
3. Any files that need manual review
4. Confirmation that `go build ./...` succeeds

Do NOT commit changes yet - wait for the other assistant to complete documentation tasks. 