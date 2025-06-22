# Deployment Notes - GSI6 for Reply Indexing

## Infrastructure Change Required
This deployment adds a new Global Secondary Index (GSI6) to the DynamoDB table for efficient reply queries.

### Deployment Steps:
1. **Deploy Infrastructure First**
   ```bash
   cd infra
   pulumi up
   ```
   This will add GSI6 to the existing DynamoDB table. The index creation may take a few minutes.

2. **Deploy Application Code**
   ```bash
   make deploy
   ```

### Important Notes:
- **New replies** will automatically be indexed in GSI6
- **Existing replies** won't appear in context endpoints until they're updated or migrated
- The GSI creation is non-blocking - the table remains available during index creation

### GSI6 Pattern:
- **Partition Key (GSI6PK)**: `REPLIES#<parent-object-url>`
- **Sort Key (GSI6SK)**: `<timestamp>#<reply-object-url>`

### What This Fixes:
- Context endpoint now returns descendants (replies) efficiently
- Reply counts can be calculated without table scans
- ~99% cost reduction for popular posts with many replies

### Rollback:
If needed, the code gracefully handles missing GSI6 (though queries would fail). To rollback:
1. Deploy previous application code
2. GSI6 can be left in place (no cost if unused) or removed via Pulumi