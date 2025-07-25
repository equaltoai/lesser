# Comprehensive Stub Implementation Plan

## Overview
This document provides specific implementation details for every stub in the Lesser codebase, with emphasis on properly using the existing storage layer.

## Table of Contents

1. [Storage Layer Access Pattern](#storage-layer-access-pattern)
2. [Phase 1: Export Generator Fixes](#phase-1-export-generator-fixes-priority-critical)
3. [Phase 2: Import/Export Job Management](#phase-2-importexport-job-management-priority-high)
4. [Phase 3: Media Processing](#phase-3-media-processing-priority-medium)
5. [Phase 4: GraphQL Implementation](#phase-4-graphql-implementation-priority-medium)
6. [Phase 5: Minor Features](#phase-5-minor-features-priority-low)
7. [Implementation Checklist](#implementation-checklist)
8. [Testing Strategy](#testing-strategy)

## Storage Layer Access Pattern

### For Lambda Functions
```go
// 1. Import the storage package
import (
    "github.com/aron23/lesser/pkg/storage"
    "github.com/aron23/lesser/pkg/storage/dynamodb"
)

// 2. Add storage client to globals
var (
    storageClient storage.Interface
    // ... other globals
)

// 3. Initialize in main or init function
func initStorage(ctx context.Context) error {
    cfg := &dynamodb.Config{
        TableName:     tableName,
        Client:        dynamoClient,
        Domain:        os.Getenv("DOMAIN"),
        JWTSecret:     os.Getenv("JWT_SECRET"),
        Region:        os.Getenv("AWS_REGION"),
    }
    
    storageClient = dynamodb.NewStorage(cfg)
    return nil
}
```

### For API Handlers
```go
// Storage is already available via h.store
func (h *Handler) someFunction(ctx context.Context) {
    // Use h.store directly
    result, err := h.store.GetFollowers(ctx, username, limit, cursor)
}
```

## Phase 1: Export Generator Fixes (Priority: CRITICAL)

### Location: `cmd/export-generator/main.go`

#### 1. getFollowers()
```go
// CURRENT (line 573)
func getFollowers(ctx context.Context, username string) ([]string, error) {
    return []string{}, nil
}

// IMPLEMENTATION
func getFollowers(ctx context.Context, username string) ([]string, error) {
    // Get all followers (paginated if needed)
    var allFollowers []string
    cursor := ""
    
    for {
        followers, nextCursor, err := storageClient.GetFollowers(ctx, username, 100, cursor)
        if err != nil {
            return nil, fmt.Errorf("failed to get followers: %w", err)
        }
        
        allFollowers = append(allFollowers, followers...)
        
        if nextCursor == "" {
            break
        }
        cursor = nextCursor
    }
    
    return allFollowers, nil
}
```

#### 2. getFollowing()
```go
// IMPLEMENTATION
func getFollowing(ctx context.Context, username string) ([]string, error) {
    var allFollowing []string
    cursor := ""
    
    for {
        following, nextCursor, err := storageClient.GetFollowing(ctx, username, 100, cursor)
        if err != nil {
            return nil, fmt.Errorf("failed to get following: %w", err)
        }
        
        allFollowing = append(allFollowing, following...)
        
        if nextCursor == "" {
            break
        }
        cursor = nextCursor
    }
    
    return allFollowing, nil
}
```

#### 3. getBlocks()
```go
// IMPLEMENTATION
func getBlocks(ctx context.Context, username string) ([]string, error) {
    var allBlocks []string
    cursor := ""
    
    for {
        blocks, nextCursor, err := storageClient.GetBlockedActors(ctx, username, 100, cursor)
        if err != nil {
            return nil, fmt.Errorf("failed to get blocks: %w", err)
        }
        
        // Extract usernames from block records
        for _, block := range blocks {
            allBlocks = append(allBlocks, block.BlockedActor)
        }
        
        if nextCursor == "" {
            break
        }
        cursor = nextCursor
    }
    
    return allBlocks, nil
}
```

#### 4. getMutes()
```go
// IMPLEMENTATION
func getMutes(ctx context.Context, username string) ([]MuteInfo, error) {
    // Note: Check if mutes are implemented in storage layer
    // If not, this becomes a new storage layer implementation task
    
    // For now, query the mutes collection directly
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(tableName),
        KeyConditionExpression: aws.String("PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#MUTES", username)},
        },
    }
    
    result, err := dynamoClient.Query(ctx, queryInput)
    if err != nil {
        return nil, fmt.Errorf("failed to query mutes: %w", err)
    }
    
    var mutes []MuteInfo
    for _, item := range result.Items {
        mute := MuteInfo{
            AccountID: item["AccountID"].(*types.AttributeValueMemberS).Value,
            HideNotifications: item["HideNotifications"].(*types.AttributeValueMemberBOOL).Value,
        }
        mutes = append(mutes, mute)
    }
    
    return mutes, nil
}
```

#### 5. getListsWithMembers()
```go
// IMPLEMENTATION
func getListsWithMembers(ctx context.Context, username string) (map[string][]string, error) {
    // First get all lists for the user
    listsResult, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
        TableName: aws.String(tableName),
        KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
            ":sk": &types.AttributeValueMemberS{Value: "LIST#"},
        },
    })
    if err != nil {
        return nil, fmt.Errorf("failed to get lists: %w", err)
    }
    
    listsWithMembers := make(map[string][]string)
    
    // For each list, get members
    for _, item := range listsResult.Items {
        listID := item["ListID"].(*types.AttributeValueMemberS).Value
        listName := item["Name"].(*types.AttributeValueMemberS).Value
        
        // Get list members
        membersResult, err := dynamoClient.Query(ctx, &dynamodb.QueryInput{
            TableName: aws.String(tableName),
            KeyConditionExpression: aws.String("PK = :pk"),
            ExpressionAttributeValues: map[string]types.AttributeValue{
                ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("LIST#%s", listID)},
            },
        })
        if err != nil {
            continue // Skip this list on error
        }
        
        var members []string
        for _, member := range membersResult.Items {
            if accountID, ok := member["AccountID"].(*types.AttributeValueMemberS); ok {
                members = append(members, accountID.Value)
            }
        }
        
        listsWithMembers[listName] = members
    }
    
    return listsWithMembers, nil
}
```

#### 6. getBookmarks()
```go
// IMPLEMENTATION
func getBookmarks(ctx context.Context, username string) ([]BookmarkInfo, error) {
    // Query bookmarks using GSI
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(tableName),
        IndexName: aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#BOOKMARKS", username)},
        },
    }
    
    result, err := dynamoClient.Query(ctx, queryInput)
    if err != nil {
        return nil, fmt.Errorf("failed to query bookmarks: %w", err)
    }
    
    var bookmarks []BookmarkInfo
    for _, item := range result.Items {
        createdAt, _ := time.Parse(time.RFC3339, item["CreatedAt"].(*types.AttributeValueMemberS).Value)
        bookmark := BookmarkInfo{
            StatusURL: item["StatusURL"].(*types.AttributeValueMemberS).Value,
            CreatedAt: createdAt,
        }
        bookmarks = append(bookmarks, bookmark)
    }
    
    return bookmarks, nil
}
```

#### 7. getOutbox()
```go
// IMPLEMENTATION
func getOutbox(ctx context.Context, username string, dateRange *DateRange) ([]any, int, error) {
    // Get user's actor ID first
    actorID := fmt.Sprintf("https://%s/users/%s", baseURL, username)
    
    // Query for objects attributed to this actor
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(tableName),
        IndexName: aws.String("GSI2"), // Assuming GSI2 indexes by AttributedTo
        KeyConditionExpression: aws.String("GSI2PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTOR#%s#OBJECTS", actorID)},
        },
    }
    
    // Add date range filter if provided
    if dateRange != nil {
        queryInput.FilterExpression = aws.String("CreatedAt BETWEEN :start AND :end")
        queryInput.ExpressionAttributeValues[":start"] = &types.AttributeValueMemberS{
            Value: dateRange.Start.Format(time.RFC3339),
        }
        queryInput.ExpressionAttributeValues[":end"] = &types.AttributeValueMemberS{
            Value: dateRange.End.Format(time.RFC3339),
        }
    }
    
    var allObjects []any
    var lastEvaluatedKey map[string]types.AttributeValue
    
    for {
        if lastEvaluatedKey != nil {
            queryInput.ExclusiveStartKey = lastEvaluatedKey
        }
        
        result, err := dynamoClient.Query(ctx, queryInput)
        if err != nil {
            return nil, 0, fmt.Errorf("failed to query outbox: %w", err)
        }
        
        for _, item := range result.Items {
            // Convert DynamoDB item to object
            var obj map[string]any
            if err := attributevalue.UnmarshalMap(item, &obj); err != nil {
                continue
            }
            allObjects = append(allObjects, obj)
        }
        
        if result.LastEvaluatedKey == nil {
            break
        }
        lastEvaluatedKey = result.LastEvaluatedKey
    }
    
    return allObjects, len(allObjects), nil
}
```

#### 8. getFollowingActors() and getFollowersActors()
```go
// IMPLEMENTATION for getFollowingActors
func getFollowingActors(ctx context.Context, username string) ([]string, error) {
    // First get following usernames
    followingUsernames, err := getFollowing(ctx, username)
    if err != nil {
        return nil, err
    }
    
    // Convert to actor IDs
    var actorIDs []string
    for _, followedUsername := range followingUsernames {
        // Get the actor details to get proper ID
        actor, err := storageClient.GetActor(ctx, followedUsername)
        if err != nil {
            // If local actor not found, construct the ID
            actorID := fmt.Sprintf("https://%s/users/%s", baseURL, followedUsername)
            actorIDs = append(actorIDs, actorID)
            continue
        }
        actorIDs = append(actorIDs, actor.ID)
    }
    
    return actorIDs, nil
}

// IMPLEMENTATION for getFollowersActors
func getFollowersActors(ctx context.Context, username string) ([]string, error) {
    // First get follower usernames
    followerUsernames, err := getFollowers(ctx, username)
    if err != nil {
        return nil, err
    }
    
    // Convert to actor IDs
    var actorIDs []string
    for _, followerUsername := range followerUsernames {
        actor, err := storageClient.GetActor(ctx, followerUsername)
        if err != nil {
            actorID := fmt.Sprintf("https://%s/users/%s", baseURL, followerUsername)
            actorIDs = append(actorIDs, actorID)
            continue
        }
        actorIDs = append(actorIDs, actor.ID)
    }
    
    return actorIDs, nil
}
```

#### 9. getLikes()
```go
// IMPLEMENTATION
func getLikes(ctx context.Context, username string) ([]any, error) {
    actorID := fmt.Sprintf("https://%s/users/%s", baseURL, username)
    
    // Get all likes by this actor
    var allLikes []any
    cursor := ""
    
    for {
        likes, nextCursor, err := storageClient.GetActorLikes(ctx, actorID, 100, cursor)
        if err != nil {
            return nil, fmt.Errorf("failed to get likes: %w", err)
        }
        
        // Convert likes to ActivityPub Like activities
        for _, like := range likes {
            likeActivity := map[string]any{
                "@context": "https://www.w3.org/ns/activitystreams",
                "type":     "Like",
                "actor":    like.Actor,
                "object":   like.Object,
                "published": like.CreatedAt.Format(time.RFC3339),
            }
            allLikes = append(allLikes, likeActivity)
        }
        
        if nextCursor == "" {
            break
        }
        cursor = nextCursor
    }
    
    return allLikes, nil
}
```

#### 10. getActorPreferences()
```go
// IMPLEMENTATION
func getActorPreferences(ctx context.Context, username string) (map[string]any, error) {
    // Get actor preferences from storage
    actor, err := storageClient.GetActor(ctx, username)
    if err != nil {
        return nil, fmt.Errorf("failed to get actor: %w", err)
    }
    
    // Build preferences map
    prefs := map[string]any{
        "posting:default:visibility": "public",
        "posting:default:sensitive": false,
        "posting:default:language": "en",
        "reading:expand:media": "default",
        "reading:expand:spoilers": false,
        "reading:autoplay:gifs": true,
    }
    
    // Override with stored preferences
    if actor.Preferences != nil {
        for key, value := range actor.Preferences {
            prefs[key] = value
        }
    }
    
    return prefs, nil
}
```

#### 11. getDomainBlocks()
```go
// IMPLEMENTATION
func getDomainBlocks(ctx context.Context, username string) ([]string, error) {
    // Query domain blocks
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(tableName),
        KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :sk)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
            ":sk": &types.AttributeValueMemberS{Value: "DOMAINBLOCK#"},
        },
    }
    
    result, err := dynamoClient.Query(ctx, queryInput)
    if err != nil {
        return nil, fmt.Errorf("failed to query domain blocks: %w", err)
    }
    
    var domains []string
    for _, item := range result.Items {
        if domain, ok := item["Domain"].(*types.AttributeValueMemberS); ok {
            domains = append(domains, domain.Value)
        }
    }
    
    return domains, nil
}
```

#### 12. exportToS3()
```go
// FIX - This is trying to use a nil s3Client
func exportToS3(ctx context.Context, exportData map[string]any, username, exportID string) error {
    // Ensure S3 client is initialized
    if s3Client == nil {
        cfg, err := config.LoadDefaultConfig(ctx)
        if err != nil {
            return fmt.Errorf("failed to load AWS config: %w", err)
        }
        s3Client = s3.NewFromConfig(cfg)
    }
    
    // ... rest of the function remains the same
}
```

## Phase 2: Import/Export Job Management (Priority: HIGH)

### Location: `cmd/api/handlers/imports.go` and `exports.go`

#### getUserImportJobs()
```go
// CURRENT (imports.go:342)
func (h *Handler) getUserImportJobs(_ context.Context, _ string, _ ...string) ([]map[string]any, error) {
    return []map[string]any{}, nil
}

// IMPLEMENTATION
func (h *Handler) getUserImportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]any, error) {
    // Query GSI1 for user's imports
    input := &dynamodb.QueryInput{
        TableName: aws.String(h.cfg.TableName),
        IndexName: aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk AND begins_with(GSI1SK, :sk)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
            ":sk": &types.AttributeValueMemberS{Value: "CREATED#"},
        },
        ScanIndexForward: aws.Bool(false), // Most recent first
    }
    
    // Add filter for specific statuses if provided
    if len(statuses) > 0 {
        filterParts := make([]string, len(statuses))
        for i, status := range statuses {
            filterParts[i] = fmt.Sprintf("#status = :status%d", i)
            input.ExpressionAttributeValues[fmt.Sprintf(":status%d", i)] = &types.AttributeValueMemberS{
                Value: status,
            }
        }
        input.FilterExpression = aws.String(strings.Join(filterParts, " OR "))
        input.ExpressionAttributeNames = map[string]string{
            "#status": "Status",
        }
    }
    
    // Execute query
    result, err := h.store.GetClient().Query(ctx, input)
    if err != nil {
        h.logger.Error("failed to query import jobs", 
            zap.String("username", username),
            zap.Error(err))
        return nil, fmt.Errorf("failed to query import jobs: %w", err)
    }
    
    // Convert items to map format
    jobs := make([]map[string]any, 0, len(result.Items))
    for _, item := range result.Items {
        // Only include import jobs (check PK prefix)
        if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
            if strings.HasPrefix(pk.Value, "IMPORT#") {
                job := make(map[string]any)
                if err := attributevalue.UnmarshalMap(item, &job); err != nil {
                    h.logger.Warn("failed to unmarshal job", zap.Error(err))
                    continue
                }
                jobs = append(jobs, job)
            }
        }
    }
    
    return jobs, nil
}
```

#### getUserExportJobs()
```go
// CURRENT (exports.go:342)
func (h *Handler) getUserExportJobs(_ context.Context, _ string, _ ...string) ([]map[string]any, error) {
    return []map[string]any{}, nil
}

// IMPLEMENTATION
func (h *Handler) getUserExportJobs(ctx context.Context, username string, statuses ...string) ([]map[string]any, error) {
    // Query GSI1 for user's exports - identical pattern to imports
    input := &dynamodb.QueryInput{
        TableName: aws.String(h.cfg.TableName),
        IndexName: aws.String("GSI1"),
        KeyConditionExpression: aws.String("GSI1PK = :pk AND begins_with(GSI1SK, :sk)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
            ":sk": &types.AttributeValueMemberS{Value: "CREATED#"},
        },
        ScanIndexForward: aws.Bool(false), // Most recent first
    }
    
    // Add filter for specific statuses if provided
    if len(statuses) > 0 {
        filterParts := make([]string, len(statuses))
        for i, status := range statuses {
            filterParts[i] = fmt.Sprintf("#status = :status%d", i)
            input.ExpressionAttributeValues[fmt.Sprintf(":status%d", i)] = &types.AttributeValueMemberS{
                Value: status,
            }
        }
        input.FilterExpression = aws.String(strings.Join(filterParts, " OR "))
        input.ExpressionAttributeNames = map[string]string{
            "#status": "Status",
        }
    }
    
    // Execute query
    result, err := h.store.GetClient().Query(ctx, input)
    if err != nil {
        h.logger.Error("failed to query export jobs", 
            zap.String("username", username),
            zap.Error(err))
        return nil, fmt.Errorf("failed to query export jobs: %w", err)
    }
    
    // Convert items to map format, filtering for exports only
    jobs := make([]map[string]any, 0, len(result.Items))
    for _, item := range result.Items {
        // Only include export jobs (check PK prefix)
        if pk, ok := item["PK"].(*types.AttributeValueMemberS); ok {
            if strings.HasPrefix(pk.Value, "EXPORT#") {
                job := make(map[string]any)
                if err := attributevalue.UnmarshalMap(item, &job); err != nil {
                    h.logger.Warn("failed to unmarshal job", zap.Error(err))
                    continue
                }
                jobs = append(jobs, job)
            }
        }
    }
    
    return jobs, nil
}
```

### Additional Support Functions for Job Management

#### updateJobStatus()
```go
// Helper function to update job status
func (h *Handler) updateJobStatus(ctx context.Context, jobID, status string, metadata map[string]any) error {
    update := &dynamodb.UpdateItemInput{
        TableName: aws.String(h.cfg.TableName),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: jobID},
            "SK": &types.AttributeValueMemberS{Value: "METADATA"},
        },
        UpdateExpression: aws.String("SET #status = :status, UpdatedAt = :now"),
        ExpressionAttributeNames: map[string]string{
            "#status": "Status",
        },
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":status": &types.AttributeValueMemberS{Value: status},
            ":now": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
        },
    }
    
    // Add metadata fields if provided
    if metadata != nil {
        for key, value := range metadata {
            update.UpdateExpression = aws.String(fmt.Sprintf("%s, %s = :%s", 
                *update.UpdateExpression, key, key))
            
            // Convert value to AttributeValue
            av, err := attributevalue.Marshal(value)
            if err == nil {
                update.ExpressionAttributeValues[":"+key] = av
            }
        }
    }
    
    _, err := h.store.GetClient().UpdateItem(ctx, update)
    return err
}
```

#### getJobDetails()
```go
// Helper to get full job details
func (h *Handler) getJobDetails(ctx context.Context, jobID string) (map[string]any, error) {
    result, err := h.store.GetClient().GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(h.cfg.TableName),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: jobID},
            "SK": &types.AttributeValueMemberS{Value: "METADATA"},
        },
    })
    
    if err != nil {
        return nil, fmt.Errorf("failed to get job details: %w", err)
    }
    
    if result.Item == nil {
        return nil, fmt.Errorf("job not found: %s", jobID)
    }
    
    var job map[string]any
    if err := attributevalue.UnmarshalMap(result.Item, &job); err != nil {
        return nil, fmt.Errorf("failed to unmarshal job: %w", err)
    }
    
    return job, nil
}
```

## Phase 3: Media Processing (Priority: MEDIUM)

### Location: `cmd/media-processor/main.go`

#### processVideo()
```go
// CURRENT (line 277)
func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    result := ProcessingResult{
        Sizes: make(map[string]SizeInfo),
    }
    logger.Warn("video processing not yet implemented")
    result.Width = 1920
    result.Height = 1080
    result.Duration = 30000
    return result, nil
}

// IMPLEMENTATION
func processVideo(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    result := ProcessingResult{
        Sizes: make(map[string]SizeInfo),
    }
    
    // Write to temp file
    tmpFile, err := os.CreateTemp("", "video-*.mp4")
    if err != nil {
        return result, fmt.Errorf("failed to create temp file: %w", err)
    }
    defer os.Remove(tmpFile.Name())
    
    if _, err := tmpFile.Write(data); err != nil {
        return result, fmt.Errorf("failed to write video: %w", err)
    }
    tmpFile.Close()
    
    // Get video metadata using ffprobe
    cmd := exec.Command("ffprobe",
        "-v", "error",
        "-select_streams", "v:0",
        "-count_packets",
        "-show_entries", "stream=width,height,duration,nb_read_packets",
        "-of", "json",
        tmpFile.Name())
    
    output, err := cmd.Output()
    if err != nil {
        return result, fmt.Errorf("ffprobe failed: %w", err)
    }
    
    var probeData struct {
        Streams []struct {
            Width         int    `json:"width"`
            Height        int    `json:"height"`
            Duration      string `json:"duration"`
            NbReadPackets string `json:"nb_read_packets"`
        } `json:"streams"`
    }
    
    if err := json.Unmarshal(output, &probeData); err != nil {
        return result, fmt.Errorf("failed to parse probe data: %w", err)
    }
    
    if len(probeData.Streams) > 0 {
        stream := probeData.Streams[0]
        result.Width = stream.Width
        result.Height = stream.Height
        
        // Parse duration
        if duration, err := strconv.ParseFloat(stream.Duration, 64); err == nil {
            result.Duration = int(duration * 1000) // Convert to milliseconds
        }
    }
    
    // Generate thumbnail at 1 second mark
    thumbnailCmd := exec.Command("ffmpeg",
        "-i", tmpFile.Name(),
        "-ss", "00:00:01",
        "-vframes", "1",
        "-f", "image2pipe",
        "-vcodec", "png",
        "-")
    
    thumbnailData, err := thumbnailCmd.Output()
    if err == nil {
        // Process thumbnail as image
        processedThumbnail, _ := media.ProcessImage(thumbnailData, "image/png")
        if small, ok := processedThumbnail["small"]; ok {
            // Upload thumbnail
            thumbKey := fmt.Sprintf("media/%s/%s/thumbnail.png", event.Username, event.MediaID)
            uploadToS3(ctx, thumbKey, small.Data, "image/png")
            result.PreviewURL = buildMediaURL(thumbKey)
        }
    }
    
    // For now, keep original video (later: add transcoding)
    videoKey := fmt.Sprintf("media/%s/%s/video.mp4", event.Username, event.MediaID)
    if err := uploadToS3(ctx, videoKey, data, "video/mp4"); err != nil {
        return result, fmt.Errorf("failed to upload video: %w", err)
    }
    
    result.Sizes["original"] = SizeInfo{
        Width:  result.Width,
        Height: result.Height,
        URL:    buildMediaURL(videoKey),
        S3Key:  videoKey,
    }
    
    return result, nil
}
```

#### processAudio()
```go
// CURRENT (line 324)
func processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    result := ProcessingResult{}
    logger.Warn("audio processing not yet implemented")
    result.Duration = 180000 // 3 minutes
    return result, nil
}

// IMPLEMENTATION
func processAudio(ctx context.Context, data []byte, event MediaProcessingEvent, tasks []any) (ProcessingResult, error) {
    result := ProcessingResult{}
    
    // Write to temp file
    tmpFile, err := os.CreateTemp("", "audio-*.mp3")
    if err != nil {
        return result, fmt.Errorf("failed to create temp file: %w", err)
    }
    defer os.Remove(tmpFile.Name())
    
    if _, err := tmpFile.Write(data); err != nil {
        return result, fmt.Errorf("failed to write audio: %w", err)
    }
    tmpFile.Close()
    
    // Get audio metadata
    cmd := exec.Command("ffprobe",
        "-v", "error",
        "-show_entries", "format=duration",
        "-of", "json",
        tmpFile.Name())
    
    output, err := cmd.Output()
    if err != nil {
        return result, fmt.Errorf("ffprobe failed: %w", err)
    }
    
    var probeData struct {
        Format struct {
            Duration string `json:"duration"`
        } `json:"format"`
    }
    
    if err := json.Unmarshal(output, &probeData); err != nil {
        return result, fmt.Errorf("failed to parse probe data: %w", err)
    }
    
    // Parse duration
    if duration, err := strconv.ParseFloat(probeData.Format.Duration, 64); err == nil {
        result.Duration = int(duration * 1000) // Convert to milliseconds
    }
    
    // Generate waveform visualization (simplified)
    // In production, use a proper waveform library
    waveformCmd := exec.Command("ffmpeg",
        "-i", tmpFile.Name(),
        "-filter_complex", "showwavespic=s=640x120",
        "-frames:v", "1",
        "-f", "image2pipe",
        "-vcodec", "png",
        "-")
    
    waveformData, err := waveformCmd.Output()
    if err == nil {
        // Upload waveform
        waveformKey := fmt.Sprintf("media/%s/%s/waveform.png", event.Username, event.MediaID)
        uploadToS3(ctx, waveformKey, waveformData, "image/png")
        result.PreviewURL = buildMediaURL(waveformKey)
    }
    
    // Upload original audio
    audioKey := fmt.Sprintf("media/%s/%s/audio.mp3", event.Username, event.MediaID)
    if err := uploadToS3(ctx, audioKey, data, "audio/mpeg"); err != nil {
        return result, fmt.Errorf("failed to upload audio: %w", err)
    }
    
    result.Sizes = map[string]SizeInfo{
        "original": {
            URL:   buildMediaURL(audioKey),
            S3Key: audioKey,
        },
    }
    
    return result, nil
}
```

### Additional Media Processing Enhancements

#### Advanced Video Processing

```go
// Add video transcoding for better compatibility
func transcodeVideo(ctx context.Context, inputFile string, event MediaProcessingEvent) (map[string]SizeInfo, error) {
    sizes := make(map[string]SizeInfo)
    
    // Define quality variants
    variants := []struct {
        name   string
        width  int
        height int
        bitrate string
    }{
        {"1080p", 1920, 1080, "5000k"},
        {"720p", 1280, 720, "2500k"},
        {"480p", 854, 480, "1000k"},
    }
    
    for _, variant := range variants {
        outputFile := fmt.Sprintf("/tmp/video-%s-%s.mp4", event.MediaID, variant.name)
        
        // Transcode command
        cmd := exec.Command("ffmpeg",
            "-i", inputFile,
            "-vf", fmt.Sprintf("scale=%d:%d", variant.width, variant.height),
            "-c:v", "libx264",
            "-preset", "fast",
            "-crf", "23",
            "-b:v", variant.bitrate,
            "-c:a", "aac",
            "-b:a", "128k",
            "-movflags", "+faststart",
            outputFile)
        
        if err := cmd.Run(); err != nil {
            logger.Warn("failed to transcode video variant", 
                zap.String("variant", variant.name),
                zap.Error(err))
            continue
        }
        
        // Read transcoded file
        data, err := os.ReadFile(outputFile)
        if err != nil {
            continue
        }
        defer os.Remove(outputFile)
        
        // Upload to S3
        key := fmt.Sprintf("media/%s/%s/video-%s.mp4", 
            event.Username, event.MediaID, variant.name)
        
        if err := uploadToS3(ctx, key, data, "video/mp4"); err != nil {
            continue
        }
        
        sizes[variant.name] = SizeInfo{
            Width:  variant.width,
            Height: variant.height,
            URL:    buildMediaURL(key),
            S3Key:  key,
        }
    }
    
    return sizes, nil
}
```

#### Enhanced Audio Processing

```go
// Add audio format conversion
func convertAudioFormat(ctx context.Context, inputFile string, format string) ([]byte, error) {
    outputFile := fmt.Sprintf("/tmp/audio-converted.%s", format)
    defer os.Remove(outputFile)
    
    // Convert audio format
    cmd := exec.Command("ffmpeg",
        "-i", inputFile,
        "-c:a", getAudioCodec(format),
        "-b:a", "192k",
        outputFile)
    
    if err := cmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to convert audio: %w", err)
    }
    
    return os.ReadFile(outputFile)
}

func getAudioCodec(format string) string {
    switch format {
    case "mp3":
        return "libmp3lame"
    case "ogg":
        return "libvorbis"
    case "m4a":
        return "aac"
    default:
        return "copy"
    }
}
```

## Phase 4: GraphQL Implementation (Priority: MEDIUM)

### Location: `graph/schema.resolvers.go`

#### Quick Fix - Replace All Panics

```bash
# Run this sed command to replace all panics with proper error returns
sed -i 's/panic(fmt\.Errorf("not implemented: \(.*\)"))/return nil, fmt.Errorf("GraphQL resolver not yet implemented: \1")/' graph/schema.resolvers.go
```

#### Implement Key Resolvers

##### Actor Field Resolvers

```go
// Username resolver
func (r *actorResolver) Username(ctx context.Context, obj *activitypub.Actor) (string, error) {
    if obj == nil {
        return "", fmt.Errorf("actor is nil")
    }
    return obj.PreferredUsername, nil
}

// Followers count
func (r *actorResolver) Followers(ctx context.Context, obj *activitypub.Actor) (int, error) {
    if obj == nil {
        return 0, fmt.Errorf("actor is nil")
    }
    
    // Get follower count from storage
    count, err := r.Storage.GetFollowerCount(ctx, obj.PreferredUsername)
    if err != nil {
        return 0, err
    }
    
    return count, nil
}

// Following count
func (r *actorResolver) Following(ctx context.Context, obj *activitypub.Actor) (int, error) {
    if obj == nil {
        return 0, fmt.Errorf("actor is nil")
    }
    
    // Get following count from storage
    count, err := r.Storage.GetFollowingCount(ctx, obj.PreferredUsername)
    if err != nil {
        return 0, err
    }
    
    return count, nil
}

// Posts count
func (r *actorResolver) PostsCount(ctx context.Context, obj *activitypub.Actor) (int, error) {
    if obj == nil {
        return 0, fmt.Errorf("actor is nil")
    }
    
    // Query for posts count
    count, err := r.Storage.GetActorPostCount(ctx, obj.ID)
    if err != nil {
        return 0, err
    }
    
    return count, nil
}
```

##### Query Resolvers

```go
// Timeline query
func (r *queryResolver) Timeline(ctx context.Context, typeArg model.TimelineType, hashtag *string, listID *string, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
    limit := 20
    if first != nil {
        limit = *first
    }
    
    cursor := ""
    if after != nil {
        cursor = string(*after)
    }
    
    var entries []*storage.TimelineEntry
    var nextCursor string
    var err error
    
    switch typeArg {
    case model.TimelineTypeHome:
        // Get authenticated user from context
        username := r.GetUsernameFromContext(ctx)
        entries, nextCursor, err = r.Storage.GetHomeTimeline(ctx, username, limit, cursor)
        
    case model.TimelineTypePublic:
        entries, nextCursor, err = r.Storage.GetPublicTimeline(ctx, false, limit, cursor)
        
    case model.TimelineTypeLocal:
        entries, nextCursor, err = r.Storage.GetPublicTimeline(ctx, true, limit, cursor)
        
    case model.TimelineTypeHashtag:
        if hashtag == nil {
            return nil, fmt.Errorf("hashtag required for hashtag timeline")
        }
        entries, nextCursor, err = r.Storage.GetHashtagTimeline(ctx, *hashtag, false, limit, cursor)
        
    case model.TimelineTypeList:
        if listID == nil {
            return nil, fmt.Errorf("listID required for list timeline")
        }
        entries, nextCursor, err = r.Storage.GetListTimeline(ctx, *listID, limit, cursor)
    }
    
    if err != nil {
        return nil, err
    }
    
    // Convert to GraphQL format
    edges := make([]*model.ObjectEdge, len(entries))
    for i, entry := range entries {
        // Get the actual object
        obj, err := r.Storage.GetObject(ctx, entry.ObjectID)
        if err != nil {
            continue
        }
        
        edges[i] = &model.ObjectEdge{
            Node:   &model.Object{ID: entry.ObjectID, Raw: obj},
            Cursor: model.Cursor(entry.ID),
        }
    }
    
    return &model.ObjectConnection{
        Edges: edges,
        PageInfo: &model.PageInfo{
            HasNextPage: nextCursor != "",
            EndCursor:   (*model.Cursor)(&nextCursor),
        },
    }, nil
}

// Actor search query
func (r *queryResolver) SearchActors(ctx context.Context, query string, first *int, after *model.Cursor) (*model.ActorConnection, error) {
    limit := 20
    if first != nil {
        limit = *first
    }
    
    cursor := ""
    if after != nil {
        cursor = string(*after)
    }
    
    // Use storage search
    results, nextCursor, err := r.Storage.SearchActors(ctx, query, limit, cursor)
    if err != nil {
        return nil, err
    }
    
    // Convert to GraphQL format
    edges := make([]*model.ActorEdge, len(results))
    for i, actor := range results {
        edges[i] = &model.ActorEdge{
            Node:   actor,
            Cursor: model.Cursor(fmt.Sprintf("%d", i)),
        }
    }
    
    return &model.ActorConnection{
        Edges: edges,
        PageInfo: &model.PageInfo{
            HasNextPage: nextCursor != "",
            EndCursor:   (*model.Cursor)(&nextCursor),
        },
    }, nil
}

// Current user query
func (r *queryResolver) Me(ctx context.Context) (*activitypub.Actor, error) {
    username := r.GetUsernameFromContext(ctx)
    if username == "" {
        return nil, fmt.Errorf("not authenticated")
    }
    
    return r.Storage.GetActor(ctx, username)
}
```

##### Mutation Resolvers

```go
// Follow mutation
func (r *mutationResolver) FollowActor(ctx context.Context, id string) (*activitypub.Activity, error) {
    // Get authenticated user
    username := r.GetUsernameFromContext(ctx)
    
    // Parse target actor ID to get username
    targetUsername := r.ExtractUsernameFromActorID(id)
    
    // Create follow activity
    activity := &activitypub.Activity{
        BaseObject: activitypub.BaseObject{
            Type: "Follow",
            ID:   fmt.Sprintf("https://%s/activities/%s", r.Domain, uuid.New().String()),
        },
        Actor:  fmt.Sprintf("https://%s/users/%s", r.Domain, username),
        Object: id,
    }
    
    // Store the follow relationship
    err := r.Storage.CreateFollow(ctx, username, targetUsername, activity.ID)
    if err != nil {
        return nil, fmt.Errorf("failed to create follow: %w", err)
    }
    
    // Queue for federation
    if err := r.FederationQueue.QueueActivity(ctx, activity); err != nil {
        r.Logger.Error("failed to queue follow activity", zap.Error(err))
    }
    
    return activity, nil
}

// Unfollow mutation
func (r *mutationResolver) UnfollowActor(ctx context.Context, id string) (*activitypub.Activity, error) {
    username := r.GetUsernameFromContext(ctx)
    targetUsername := r.ExtractUsernameFromActorID(id)
    
    // Create undo activity
    activity := &activitypub.Activity{
        BaseObject: activitypub.BaseObject{
            Type: "Undo",
            ID:   fmt.Sprintf("https://%s/activities/%s", r.Domain, uuid.New().String()),
        },
        Actor: fmt.Sprintf("https://%s/users/%s", r.Domain, username),
        Object: map[string]any{
            "type":   "Follow",
            "actor":  fmt.Sprintf("https://%s/users/%s", r.Domain, username),
            "object": id,
        },
    }
    
    // Remove the follow relationship
    err := r.Storage.RemoveFollow(ctx, username, targetUsername)
    if err != nil {
        return nil, fmt.Errorf("failed to remove follow: %w", err)
    }
    
    // Queue for federation
    if err := r.FederationQueue.QueueActivity(ctx, activity); err != nil {
        r.Logger.Error("failed to queue unfollow activity", zap.Error(err))
    }
    
    return activity, nil
}

// Like mutation
func (r *mutationResolver) LikeObject(ctx context.Context, id string) (*activitypub.Activity, error) {
    username := r.GetUsernameFromContext(ctx)
    actorID := fmt.Sprintf("https://%s/users/%s", r.Domain, username)
    
    // Create like
    like := &storage.Like{
        Actor:     actorID,
        Object:    id,
        CreatedAt: time.Now(),
    }
    
    if err := r.Storage.CreateLike(ctx, like); err != nil {
        return nil, fmt.Errorf("failed to create like: %w", err)
    }
    
    // Create activity
    activity := &activitypub.Activity{
        BaseObject: activitypub.BaseObject{
            Type: "Like",
            ID:   fmt.Sprintf("https://%s/activities/%s", r.Domain, uuid.New().String()),
        },
        Actor:  actorID,
        Object: id,
    }
    
    // Queue for federation
    if err := r.FederationQueue.QueueActivity(ctx, activity); err != nil {
        r.Logger.Error("failed to queue like activity", zap.Error(err))
    }
    
    return activity, nil
}

// Create note mutation
func (r *mutationResolver) CreateNote(ctx context.Context, content string, inReplyTo *string, visibility *model.Visibility) (*activitypub.Object, error) {
    username := r.GetUsernameFromContext(ctx)
    actorID := fmt.Sprintf("https://%s/users/%s", r.Domain, username)
    
    // Create note object
    note := &activitypub.Object{
        BaseObject: activitypub.BaseObject{
            Type: "Note",
            ID:   fmt.Sprintf("https://%s/notes/%s", r.Domain, uuid.New().String()),
        },
        AttributedTo: actorID,
        Content:      content,
        Published:    time.Now(),
    }
    
    if inReplyTo != nil {
        note.InReplyTo = *inReplyTo
    }
    
    // Set visibility
    switch visibility {
    case &model.VisibilityPublic:
        note.To = []string{"https://www.w3.org/ns/activitystreams#Public"}
        note.Cc = []string{fmt.Sprintf("%s/followers", actorID)}
    case &model.VisibilityUnlisted:
        note.To = []string{fmt.Sprintf("%s/followers", actorID)}
        note.Cc = []string{"https://www.w3.org/ns/activitystreams#Public"}
    case &model.VisibilityFollowers:
        note.To = []string{fmt.Sprintf("%s/followers", actorID)}
    case &model.VisibilityDirect:
        // Extract mentions and set as To
        mentions := r.ExtractMentions(content)
        note.To = mentions
    }
    
    // Store the note
    if err := r.Storage.CreateObject(ctx, note); err != nil {
        return nil, fmt.Errorf("failed to create note: %w", err)
    }
    
    // Create and queue activity
    createActivity := &activitypub.Activity{
        BaseObject: activitypub.BaseObject{
            Type: "Create",
            ID:   fmt.Sprintf("https://%s/activities/%s", r.Domain, uuid.New().String()),
        },
        Actor:  actorID,
        Object: note,
    }
    
    if err := r.FederationQueue.QueueActivity(ctx, createActivity); err != nil {
        r.Logger.Error("failed to queue create activity", zap.Error(err))
    }
    
    return note, nil
}
```

##### Subscription Resolvers

```go
// Timeline subscription
func (r *subscriptionResolver) TimelineUpdates(ctx context.Context, timelineType model.TimelineType, hashtag *string, listID *string) (<-chan *activitypub.Object, error) {
    username := r.GetUsernameFromContext(ctx)
    ch := make(chan *activitypub.Object, 100)
    
    // Create subscription key based on timeline type
    var subKey string
    switch timelineType {
    case model.TimelineTypeHome:
        subKey = fmt.Sprintf("timeline:home:%s", username)
    case model.TimelineTypePublic:
        subKey = "timeline:public"
    case model.TimelineTypeLocal:
        subKey = "timeline:local"
    case model.TimelineTypeHashtag:
        if hashtag == nil {
            return nil, fmt.Errorf("hashtag required")
        }
        subKey = fmt.Sprintf("timeline:hashtag:%s", *hashtag)
    case model.TimelineTypeList:
        if listID == nil {
            return nil, fmt.Errorf("listID required")
        }
        subKey = fmt.Sprintf("timeline:list:%s", *listID)
    }
    
    // Subscribe to updates
    go func() {
        defer close(ch)
        
        // Use Redis pubsub or similar for real-time updates
        updates := r.PubSub.Subscribe(ctx, subKey)
        
        for {
            select {
            case <-ctx.Done():
                return
            case update := <-updates:
                if obj, ok := update.(*activitypub.Object); ok {
                    ch <- obj
                }
            }
        }
    }()
    
    return ch, nil
}
```

## Phase 5: Minor Features (Priority: LOW)

### Search Enhancements

Location: Various search-related files

```go
// In pkg/storage/dynamodb/search_service.go
// Implement hashtag search
func (s *SearchService) SearchHashtags(ctx context.Context, query string, limit int) ([]*HashtagResult, error) {
    // Use GSI to search hashtags
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(s.tableName),
        IndexName: aws.String("GSI3"), // Hashtag index
        KeyConditionExpression: aws.String("GSI3PK = :pk AND begins_with(GSI3SK, :query)"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk":    &types.AttributeValueMemberS{Value: "HASHTAGS"},
            ":query": &types.AttributeValueMemberS{Value: strings.ToLower(query)},
        },
        Limit: aws.Int32(int32(limit)),
    }
    
    result, err := s.client.Query(ctx, queryInput)
    if err != nil {
        return nil, fmt.Errorf("failed to search hashtags: %w", err)
    }
    
    // Convert results
    var hashtags []*HashtagResult
    for _, item := range result.Items {
        hashtag := &HashtagResult{
            Name:  item["Name"].(*types.AttributeValueMemberS).Value,
            URL:   item["URL"].(*types.AttributeValueMemberS).Value,
            Count: 0, // Would need to query usage count
        }
        hashtags = append(hashtags, hashtag)
    }
    
    return hashtags, nil
}

// Implement full-text search
func (s *SearchService) SearchPosts(ctx context.Context, query string, limit int, cursor string) ([]*PostResult, string, error) {
    // Note: Full-text search requires OpenSearch/ElasticSearch integration
    // For now, implement basic hashtag and mention search
    
    var results []*PostResult
    var nextCursor string
    
    // If query starts with #, search hashtags
    if strings.HasPrefix(query, "#") {
        hashtag := strings.TrimPrefix(query, "#")
        posts, cursor, err := s.GetHashtagPosts(ctx, hashtag, limit, cursor)
        if err != nil {
            return nil, "", err
        }
        
        for _, post := range posts {
            results = append(results, &PostResult{
                ID:      post.ID,
                Content: post.Content,
                Author:  post.AttributedTo,
                Created: post.Published,
            })
        }
        nextCursor = cursor
    }
    
    return results, nextCursor, nil
}
```

### Translation Cache Implementation

Location: `pkg/translation/aws_translate.go`

```go
// Add caching layer to translation service
type CachedTranslateService struct {
    *AWSTranslateService
    dynamoClient *dynamodb.Client
    tableName    string
}

// Implement DynamoDB caching
func (t *CachedTranslateService) getCachedTranslation(ctx context.Context, text, sourceLang, targetLang string) (*Translation, error) {
    // Create cache key
    h := sha256.New()
    h.Write([]byte(text))
    cacheKey := fmt.Sprintf("%s:%s:%x", sourceLang, targetLang, h.Sum(nil))
    
    result, err := t.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(t.tableName),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: "TRANSLATION"},
            "SK": &types.AttributeValueMemberS{Value: cacheKey},
        },
    })
    
    if err != nil || result.Item == nil {
        return nil, nil // Cache miss
    }
    
    var cached Translation
    if err := attributevalue.UnmarshalMap(result.Item, &cached); err != nil {
        return nil, err
    }
    
    return &cached, nil
}

func (t *CachedTranslateService) setCachedTranslation(ctx context.Context, text, sourceLang, targetLang, translated string) error {
    h := sha256.New()
    h.Write([]byte(text))
    cacheKey := fmt.Sprintf("%s:%s:%x", sourceLang, targetLang, h.Sum(nil))
    
    item := map[string]any{
        "PK":             "TRANSLATION",
        "SK":             cacheKey,
        "SourceText":     text,
        "TranslatedText": translated,
        "SourceLang":     sourceLang,
        "TargetLang":     targetLang,
        "CreatedAt":      time.Now().Format(time.RFC3339),
        "TTL":            time.Now().Add(30 * 24 * time.Hour).Unix(), // 30 day cache
    }
    
    itemAV, err := attributevalue.MarshalMap(item)
    if err != nil {
        return err
    }
    
    _, err = t.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(t.tableName),
        Item:      itemAV,
    })
    
    return err
}

// Override Translate method to use cache
func (t *CachedTranslateService) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
    // Check cache first
    cached, err := t.getCachedTranslation(ctx, text, sourceLang, targetLang)
    if err == nil && cached != nil {
        return cached.TranslatedText, nil
    }
    
    // Cache miss - call AWS Translate
    translated, err := t.AWSTranslateService.Translate(ctx, text, sourceLang, targetLang)
    if err != nil {
        return "", err
    }
    
    // Cache the result (ignore cache errors)
    _ = t.setCachedTranslation(ctx, text, sourceLang, targetLang, translated)
    
    return translated, nil
}
```

### Admin Features

Location: `cmd/api/handlers/admin.go`

```go
// Implement account silencing
func (h *Handler) silenceAccount(ctx context.Context, username string, reason string) error {
    // Update actor metadata
    updates := map[string]any{
        "Silenced":       true,
        "SilencedAt":     time.Now().Format(time.RFC3339),
        "SilencedReason": reason,
        "UpdatedAt":      time.Now().Format(time.RFC3339),
    }
    
    err := h.store.UpdateActor(ctx, username, updates)
    if err != nil {
        return fmt.Errorf("failed to silence account: %w", err)
    }
    
    // Log admin action
    h.logger.Info("account silenced",
        zap.String("username", username),
        zap.String("reason", reason),
        zap.String("admin", h.GetAdminFromContext(ctx)))
    
    return nil
}

// Mark media as sensitive
func (h *Handler) markMediaSensitive(ctx context.Context, username string) error {
    // Update all media items for user
    // Query all media for user
    queryInput := &dynamodb.QueryInput{
        TableName: aws.String(h.cfg.TableName),
        IndexName: aws.String("GSI2"),
        KeyConditionExpression: aws.String("GSI2PK = :pk"),
        ExpressionAttributeValues: map[string]types.AttributeValue{
            ":pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s#MEDIA", username)},
        },
    }
    
    paginator := dynamodb.NewQueryPaginator(h.store.GetClient(), queryInput)
    updateCount := 0
    
    for paginator.HasMorePages() {
        page, err := paginator.NextPage(ctx)
        if err != nil {
            return fmt.Errorf("failed to query media: %w", err)
        }
        
        // Update each media item
        for _, item := range page.Items {
            if mediaID, ok := item["MediaID"].(*types.AttributeValueMemberS); ok {
                updateInput := &dynamodb.UpdateItemInput{
                    TableName: aws.String(h.cfg.TableName),
                    Key: map[string]types.AttributeValue{
                        "PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("MEDIA#%s", mediaID.Value)},
                        "SK": &types.AttributeValueMemberS{Value: "METADATA"},
                    },
                    UpdateExpression: aws.String("SET Sensitive = :true, UpdatedAt = :now"),
                    ExpressionAttributeValues: map[string]types.AttributeValue{
                        ":true": &types.AttributeValueMemberBOOL{Value: true},
                        ":now":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
                    },
                }
                
                if _, err := h.store.GetClient().UpdateItem(ctx, updateInput); err == nil {
                    updateCount++
                }
            }
        }
    }
    
    h.logger.Info("marked media as sensitive",
        zap.String("username", username),
        zap.Int("count", updateCount))
    
    return nil
}

// Implement instance-wide blocks
func (h *Handler) blockDomain(ctx context.Context, domain string, severity string, reason string) error {
    // Create domain block record
    item := map[string]any{
        "PK":         "INSTANCE#BLOCKS",
        "SK":         fmt.Sprintf("DOMAIN#%s", domain),
        "Domain":     domain,
        "Severity":   severity, // "silence" or "suspend"
        "Reason":     reason,
        "CreatedAt":  time.Now().Format(time.RFC3339),
        "CreatedBy":  h.GetAdminFromContext(ctx),
    }
    
    itemAV, err := attributevalue.MarshalMap(item)
    if err != nil {
        return fmt.Errorf("failed to marshal domain block: %w", err)
    }
    
    _, err = h.store.GetClient().PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(h.cfg.TableName),
        Item:      itemAV,
    })
    
    if err != nil {
        return fmt.Errorf("failed to create domain block: %w", err)
    }
    
    // Trigger federation updates
    h.federationService.HandleDomainBlock(ctx, domain, severity)
    
    return nil
}
```

### Notification Features

Location: `pkg/notifications/service.go`

```go
// Implement notification preferences
func (s *NotificationService) UpdatePreferences(ctx context.Context, username string, prefs NotificationPreferences) error {
    item := map[string]any{
        "PK":              fmt.Sprintf("USER#%s", username),
        "SK":              "NOTIFICATION#PREFERENCES",
        "FollowEnabled":   prefs.FollowEnabled,
        "LikeEnabled":     prefs.LikeEnabled,
        "MentionEnabled":  prefs.MentionEnabled,
        "ReblogEnabled":   prefs.ReblogEnabled,
        "PollEnabled":     prefs.PollEnabled,
        "UpdatedAt":       time.Now().Format(time.RFC3339),
    }
    
    itemAV, err := attributevalue.MarshalMap(item)
    if err != nil {
        return err
    }
    
    _, err = s.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
        TableName: aws.String(s.tableName),
        Item:      itemAV,
    })
    
    return err
}

// Get notification preferences
func (s *NotificationService) GetPreferences(ctx context.Context, username string) (*NotificationPreferences, error) {
    result, err := s.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
        TableName: aws.String(s.tableName),
        Key: map[string]types.AttributeValue{
            "PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USER#%s", username)},
            "SK": &types.AttributeValueMemberS{Value: "NOTIFICATION#PREFERENCES"},
        },
    })
    
    if err != nil {
        return nil, err
    }
    
    // Default preferences if not found
    if result.Item == nil {
        return &NotificationPreferences{
            FollowEnabled:  true,
            LikeEnabled:    true,
            MentionEnabled: true,
            ReblogEnabled:  true,
            PollEnabled:    true,
        }, nil
    }
    
    var prefs NotificationPreferences
    if err := attributevalue.UnmarshalMap(result.Item, &prefs); err != nil {
        return nil, err
    }
    
    return &prefs, nil
}
```

## Implementation Checklist

### Week 1 - Critical Export Generator Fixes
- [ ] Initialize storage client in export-generator Lambda
- [ ] Implement getFollowers() with storage layer
- [ ] Implement getFollowing() with storage layer
- [ ] Implement getBlocks() with storage layer
- [ ] Implement getMutes() query
- [ ] Implement getListsWithMembers() query
- [ ] Implement getBookmarks() query
- [ ] Implement getOutbox() with date filtering
- [ ] Implement getFollowingActors() and getFollowersActors()
- [ ] Implement getLikes() with pagination
- [ ] Implement getActorPreferences()
- [ ] Implement getDomainBlocks()
- [ ] Fix exportToS3() S3 client initialization
- [ ] Test complete export generation flow

### Week 2 - Job Management & Media Processing
- [ ] Implement getUserImportJobs() with GSI query
- [ ] Implement getUserExportJobs() with GSI query
- [ ] Add job status update helpers
- [ ] Implement video processing with ffmpeg
- [ ] Implement audio processing with ffmpeg
- [ ] Add video thumbnail generation
- [ ] Add audio waveform generation
- [ ] Test media processing with various formats
- [ ] Add error handling and retry logic

### Week 3 - GraphQL Implementation
- [ ] Replace all GraphQL panics with errors
- [ ] Implement Actor field resolvers
- [ ] Implement Timeline query resolver
- [ ] Implement Search query resolvers
- [ ] Implement Follow/Unfollow mutations
- [ ] Implement Like/Unlike mutations
- [ ] Implement CreateNote mutation
- [ ] Implement subscription resolvers
- [ ] Add authentication middleware
- [ ] Test GraphQL API endpoints

### Week 4 - Minor Features & Polish
- [ ] Implement hashtag search
- [ ] Add translation caching
- [ ] Implement admin account silencing
- [ ] Implement media sensitivity marking
- [ ] Add domain blocking
- [ ] Implement notification preferences
- [ ] Performance optimization
- [ ] Documentation updates
- [ ] Deploy completed features

## Testing Strategy

### For Each Implementation:

1. **Unit Tests** - Test function in isolation
2. **Integration Tests** - Test with real DynamoDB
3. **End-to-End Tests** - Test complete user flows
4. **Load Tests** - Ensure performance at scale

### Example Test Pattern:

```go
func TestGetFollowers(t *testing.T) {
    // Setup
    ctx := context.Background()
    username := "testuser"
    
    // Create test followers
    for i := 0; i < 5; i++ {
        follower := fmt.Sprintf("follower%d", i)
        err := storageClient.CreateFollow(ctx, follower, username, "test-activity")
        require.NoError(t, err)
        err = storageClient.AcceptFollow(ctx, follower, username)
        require.NoError(t, err)
    }
    
    // Test
    followers, err := getFollowers(ctx, username)
    
    // Verify
    assert.NoError(t, err)
    assert.Len(t, followers, 5)
    assert.Contains(t, followers, "follower0")
}
```

## Key Success Factors

1. **Always use the storage layer** - Never query DynamoDB directly in application code
2. **Handle pagination** - Most queries can return large result sets
3. **Error handling** - Log errors but don't fail silently
4. **Test with real data** - Create test data that mirrors production
5. **Monitor performance** - Add metrics for each implementation
6. **Document changes** - Update API docs as features are completed
7. **Incremental deployment** - Deploy and test each phase separately

## Common Pitfalls to Avoid

1. **Don't forget to initialize clients** - Ensure AWS clients are initialized before use
2. **Check for nil pointers** - Many stub functions try to use nil clients
3. **Handle empty results** - Don't assume queries will always return data
4. **Use proper indexes** - Query using GSIs where appropriate
5. **Respect rate limits** - Add backoff for external services
6. **Consider costs** - Some operations (like full table scans) can be expensive

This comprehensive plan provides specific implementation details for every stub in the Lesser codebase, emphasizing proper use of the storage layer and existing functionality. 