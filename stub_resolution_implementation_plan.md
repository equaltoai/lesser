# Stub Resolution Implementation Plan

## Executive Summary

This plan outlines a systematic approach to resolve ~27 critical stub implementations that were incorrectly marked as complete. The plan is divided into immediate fixes (Week 1), critical features (Weeks 2-3), and long-term process improvements.

## Current State Assessment

### Critical Broken Features
1. **Import/Export System** - Returns empty data
2. **Export Data Generation** - Generates empty files
3. **GraphQL API** - 97% non-functional (panics)
4. **Media Processing** - Returns fake data for video/audio

### Impact
- Users cannot access their data
- GDPR compliance at risk (data portability)
- API advertised features don't work
- Trust in system reliability compromised

## Phase 1: Immediate Stabilization (Week 1)

### Day 1-2: Communication & Assessment

**Tasks:**
1. **Internal Communication**
   - Hold all-hands meeting to discuss the issue
   - Establish "no more stubs" policy
   - Create shame-free environment for reporting other stubs

2. **External Communication**
   - Update documentation to reflect actual functionality
   - Disable/hide non-functional features in UI
   - Prepare transparent communication for users

3. **Complete Audit**
   ```bash
   # Run comprehensive stub detection
   ./check_stub_implementations.sh > stub_audit_results.txt
   
   # Generate detailed report
   python analyze_stub_implementations.py
   ```

4. **Create Tracking System**
   ```markdown
   ## Stub Tracking Sheet
   | Feature | File | Function | Severity | Owner | Status |
   |---------|------|----------|----------|-------|--------|
   | Import List | imports.go | getUserImportJobs | CRITICAL | TBD | Not Started |
   | Export List | exports.go | getUserExportJobs | CRITICAL | TBD | Not Started |
   ...
   ```

### Day 3-5: Critical Fixes

**1. Import/Export Listing Functions**

```go
// Replace stub in cmd/api/handlers/imports.go
func (h *Handler) getUserImportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]interface{}, error) {
    // Build filter expression for statuses if provided
    filterExpr := ""
    exprAttrValues := map[string]types.AttributeValue{
        ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
    }
    
    if len(statuses) > 0 {
        statusConditions := make([]string, len(statuses))
        for i, status := range statuses {
            statusConditions[i] = fmt.Sprintf("#status = :status%d", i)
            exprAttrValues[fmt.Sprintf(":status%d", i)] = &types.AttributeValueMemberS{Value: status}
        }
        filterExpr = strings.Join(statusConditions, " OR ")
    }
    
    input := &dynamodb.QueryInput{
        TableName: aws.String(h.cfg.TableName),
        IndexName: aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk"),
        ExpressionAttributeValues: exprAttrValues,
        ScanIndexForward: aws.Bool(false), // Most recent first
    }
    
    if filterExpr != "" {
        input.FilterExpression = aws.String(filterExpr)
        input.ExpressionAttributeNames = map[string]string{
            "#status": "Status",
        }
    }
    
    result, err := h.store.GetClient().Query(ctx, input)
    if err != nil {
        h.logger.Error("failed to query import jobs", 
            zap.String("username", username),
            zap.Error(err))
        return nil, fmt.Errorf("failed to query import jobs: %w", err)
    }
    
    jobs := make([]map[string]interface{}, 0, len(result.Items))
    for _, item := range result.Items {
        job := make(map[string]interface{})
        if err := attributevalue.UnmarshalMap(item, &job); err != nil {
            h.logger.Warn("failed to unmarshal job", zap.Error(err))
            continue
        }
        jobs = append(jobs, job)
    }
    
    return jobs, nil
}
```

**2. Error Handling for GraphQL**

```go
// Quick fix: Replace panics with error returns
func (r *activityResolver) Type(ctx context.Context, obj *activitypub.Activity) (model.ActivityType, error) {
    // TODO: Implement actual resolver
    return "", fmt.Errorf("GraphQL API is currently under development")
}
```

## Phase 2: Core Feature Implementation (Weeks 2-3)

### Week 2: Export Data Generation

**Implementation Order:**
1. `getFollowers()` and `getFollowing()` - Most commonly used
2. `getOutbox()` - Critical for data portability
3. `getLikes()` and `getBookmarks()` - User content
4. `getBlocks()` and `getMutes()` - Safety features
5. Remaining functions

**Example Implementation:**
```go
func getFollowers(ctx context.Context, username string) ([]string, error) {
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(tableName),
        KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
            ":sk": &types.AttributeValueMemberS{Value: "FOLLOWER#"},
        },
    }
    
    var followers []string
    var lastKey map[string]types.AttributeValue
    
    for {
        if lastKey != nil {
            queryInput.ExclusiveStartKey = lastKey
        }
        
        result, err := dynamoClient.Query(ctx, queryInput)
        if err != nil {
            return nil, fmt.Errorf("failed to query followers: %w", err)
        }
        
        for _, item := range result.Items {
            if sk, ok := item["SK"].(*types.AttributeValueMemberS); ok {
                // Extract follower username from SK
                follower := strings.TrimPrefix(sk.Value, "FOLLOWER#")
                followers = append(followers, follower)
            }
        }
        
        if result.LastEvaluatedKey == nil {
            break
        }
        lastKey = result.LastEvaluatedKey
    }
    
    return followers, nil
}
```

### Week 3: Testing Infrastructure

**1. Integration Test Suite**
```python
# tests/integration/test_import_export_cycle.py
class TestImportExportCycle:
    def test_complete_cycle(self):
        # Create test data
        user = create_test_user()
        followers = create_followers(user, ["follower1", "follower2"])
        posts = create_posts(user, ["post1", "post2"])
        
        # Export data
        export_job = api.create_export(user.token, type="archive")
        export_job = wait_for_job_completion(export_job.id)
        
        # Download and verify export
        export_data = api.download_export(export_job.download_url)
        assert "follower1" in export_data
        assert "post1" in export_data
        
        # Import to new account
        new_user = create_test_user()
        import_job = api.create_import(new_user.token, data=export_data)
        import_job = wait_for_job_completion(import_job.id)
        
        # Verify imported data
        imported_followers = api.get_followers(new_user.token)
        assert len(imported_followers) == 2
```

**2. Stub Detection Tests**
```python
# tests/quality/test_no_stubs.py
def test_no_stub_implementations():
    """Ensure no stub implementations in codebase"""
    stub_patterns = [
        r"// For now.*return.*empty",
        r"return \[\].*\{\}, nil.*// For now",
        r"panic.*not implemented"
    ]
    
    violations = []
    for pattern in stub_patterns:
        results = grep_codebase(pattern)
        violations.extend(results)
    
    assert len(violations) == 0, f"Found {len(violations)} stub implementations"
```

## Phase 3: Process Improvements (Ongoing)

### 1. Definition of Done Checklist

```markdown
## Feature Completion Checklist
- [ ] Implementation complete (no stubs, no "for now" comments)
- [ ] Unit tests written and passing
- [ ] Integration tests written and passing
- [ ] Manual testing completed with real data
- [ ] Code reviewed by senior developer
- [ ] Documentation updated
- [ ] Performance tested under load
- [ ] Error cases handled appropriately
- [ ] Logging and monitoring in place
```

### 2. Code Review Standards

```yaml
# .github/review-checklist.yml
code_review_checklist:
  - name: "No stub implementations"
    patterns_to_reject:
      - "for now.*return.*empty"
      - "TODO.*implement"
      - "panic.*not implemented"
    message: "Stub implementations are not allowed in production code"
    
  - name: "Integration tests required"
    required_files:
      - "tests/integration/test_*.py"
    message: "New features must include integration tests"
```

### 3. Automated Checks

```yaml
# .github/workflows/stub-detection.yml
name: Stub Detection
on: [push, pull_request]

jobs:
  detect-stubs:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Check for stub implementations
        run: |
          ./check_stub_implementations.sh
          if [ $? -ne 0 ]; then
            echo "Stub implementations detected!"
            exit 1
          fi
      
      - name: Run integration tests
        run: |
          python -m pytest tests/integration/
```

## Phase 4: Media Processing Implementation (Week 4)

### Video Processing
```go
func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []interface{}) (ProcessingResult, error) {
    result := ProcessingResult{
        Sizes: make(map[string]SizeInfo),
    }
    
    // Write video to temp file
    tmpFile, err := os.CreateTemp("", "video-*.mp4")
    if err != nil {
        return result, fmt.Errorf("failed to create temp file: %w", err)
    }
    defer os.Remove(tmpFile.Name())
    
    if _, err := tmpFile.Write(data); err != nil {
        return result, fmt.Errorf("failed to write video: %w", err)
    }
    tmpFile.Close()
    
    // Use ffprobe to get video info
    cmd := exec.Command("ffprobe", 
        "-v", "error",
        "-select_streams", "v:0",
        "-show_entries", "stream=width,height,duration",
        "-of", "json",
        tmpFile.Name())
    
    output, err := cmd.Output()
    if err != nil {
        return result, fmt.Errorf("failed to probe video: %w", err)
    }
    
    var probeResult struct {
        Streams []struct {
            Width    int    `json:"width"`
            Height   int    `json:"height"`
            Duration string `json:"duration"`
        } `json:"streams"`
    }
    
    if err := json.Unmarshal(output, &probeResult); err != nil {
        return result, fmt.Errorf("failed to parse probe result: %w", err)
    }
    
    if len(probeResult.Streams) > 0 {
        stream := probeResult.Streams[0]
        result.Width = stream.Width
        result.Height = stream.Height
        
        if dur, err := strconv.ParseFloat(stream.Duration, 64); err == nil {
            result.Duration = int(dur * 1000) // Convert to milliseconds
        }
    }
    
    // Generate thumbnail
    // ... thumbnail generation code ...
    
    return result, nil
}
```

## Success Metrics

### Week 1
- [ ] All critical stubs identified and documented
- [ ] Import/Export listing functions implemented
- [ ] GraphQL panics replaced with errors

### Week 2-3
- [ ] All export data functions implemented
- [ ] Integration test suite running
- [ ] 0 "for now" comments in production code

### Week 4
- [ ] Media processing fully implemented
- [ ] All features manually tested
- [ ] User documentation updated

### Long-term
- [ ] 0 stub implementations in codebase
- [ ] 100% feature integration test coverage
- [ ] Automated stub detection preventing new stubs
- [ ] Team trained on new standards

## Communication Plan

### Internal
- Daily standups focusing on stub resolution progress
- Weekly progress reports to stakeholders
- Shared dashboard showing completion status

### External
- Blog post about our commitment to quality
- Transparent changelog showing fixed features
- User notification when features are actually working

## Risk Mitigation

### Technical Risks
- **Risk**: Fixing stubs breaks existing workarounds
- **Mitigation**: Careful testing, gradual rollout

### Process Risks
- **Risk**: Team reverts to old habits
- **Mitigation**: Automated checks, regular training

### Timeline Risks
- **Risk**: Fixes take longer than estimated
- **Mitigation**: Focus on highest-impact fixes first

## Conclusion

This plan will systematically eliminate all stub implementations over 4 weeks while establishing processes to prevent future occurrences. The key is combining immediate fixes with long-term process improvements and maintaining transparency throughout. 