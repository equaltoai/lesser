# GraphQL Implementation Plan: 85% → 100%

**Current Status**: 70% Complete (60+ operations functional)  
**Target**: 100% Complete (All schema operations implemented)  

---

## Gap Analysis Summary

### Critical Gaps (Blocking 100%)
- ❌ Hashtag Following System (8 operations)
- ❌ Thread Synchronization (3 operations)
- ✅ Severed Relationships (4 operations) - COMPLETE (Phase 2.4)
- ❌ Phase 2 Alert Subscriptions (4 operations)
- ❌ Media Streaming (3 operations incomplete)
- ❌ Federation Visualization (3 operations)
- ❌ Streaming Analytics (3 operations)
- ❌ Performance Monitoring (3 operations)
- ❌ Moderation Dashboard (3 operations)
- ✅ Advanced Moderation ML (2 operations) - COMPLETE (Phase 2.3)

**Total Missing**: 30 operations across 9 feature areas (3 completed)

---

## Implementation Phases

### PHASE 1: Critical Mastodon Parity (Week 1-2)
**Goal**: Achieve 100% Mastodon API compatibility  
**Effort**: 6-9 days  
**Priority**: CRITICAL

#### 1.1 Hashtag Following System ⭐ HIGHEST PRIORITY
**Status**: NOT STARTED (stubs only)  
**Effort**: 2-3 days  
**Dependencies**: None  
**Blocking**: Mastodon compatibility

**Missing Operations**:
1. `Query.hashtag(name: String!)` → Hashtag
2. `Query.followedHashtags(first, after)` → HashtagConnection
3. `Query.hashtagTimeline(hashtag, first, after)` → PostConnection
4. `Query.multiHashtagTimeline(hashtags, mode, first, after)` → PostConnection
5. `Query.suggestedHashtags(limit)` → [HashtagSuggestion]
6. `Mutation.followHashtag(hashtag, notifyLevel)` → HashtagFollowPayload
7. `Mutation.unfollowHashtag(hashtag)` → UnfollowHashtagPayload
8. `Mutation.updateHashtagNotifications(hashtag, settings)` → UpdateHashtagNotificationsPayload
9. `Mutation.muteHashtag(hashtag, until)` → MuteHashtagPayload
10. `Subscription.hashtagActivity(hashtags)` → HashtagActivityUpdate

**Implementation Plan**:

```
Day 1: Storage Layer
├── Create pkg/services/hashtags/service.go
├── Add storage models:
│   ├── HashtagFollow (PK: user#{userID}, SK: hashtag#{name})
│   ├── HashtagMute (PK: user#{userID}, SK: muted_hashtag#{name})
│   └── HashtagStats (PK: hashtag#{name}, SK: stats)
├── Add repository methods:
│   ├── FollowHashtag(ctx, userID, hashtag, settings)
│   ├── UnfollowHashtag(ctx, userID, hashtag)
│   ├── GetFollowedHashtags(ctx, userID, pagination)
│   ├── IsFollowingHashtag(ctx, userID, hashtag)
│   └── MuteHashtag(ctx, userID, hashtag, until)
└── Add GSI: hashtag-followers-index (PK: hashtag, SK: userID)

Day 2: Query Implementations
├── Implement hashtag(name) resolver
│   ├── Get hashtag stats from existing hashtag indexer
│   ├── Check if current user follows it
│   ├── Get related hashtags
│   └── Get notification settings
├── Implement followedHashtags resolver
│   ├── Query user's followed hashtags
│   ├── Enrich with current stats
│   └── Apply pagination
├── Implement hashtagTimeline resolver
│   ├── Reuse existing timeline service
│   ├── Filter by hashtag
│   └── Apply user's hashtag filters
├── Implement multiHashtagTimeline resolver
│   ├── Support ANY mode (union)
│   ├── Support ALL mode (intersection)
│   └── Merge and sort results
└── Implement suggestedHashtags resolver
    ├── Get trending hashtags
    ├── Get hashtags from followed users
    ├── Get related to user's interests
    └── Score and rank suggestions

Day 3: Mutations & Subscriptions
├── Implement followHashtag mutation
│   ├── Create HashtagFollow record
│   ├── Set notification preferences
│   ├── Update user's followed count
│   └── Emit followHashtag event
├── Implement unfollowHashtag mutation
│   ├── Delete HashtagFollow record
│   ├── Update user's followed count
│   └── Emit unfollowHashtag event
├── Implement updateHashtagNotifications mutation
│   ├── Update notification settings
│   └── Validate settings
├── Implement muteHashtag mutation
│   ├── Create HashtagMute record
│   └── Set expiration if provided
├── Implement hashtagActivity subscription
│   ├── Subscribe to hashtag events
│   ├── Filter by followed hashtags
│   ├── Filter by notification settings
│   └── Stream real-time posts
└── Add tests for all operations
```

**Acceptance Criteria**:
- [ ] Can follow/unfollow hashtags
- [ ] Can view timeline of followed hashtags
- [ ] Can combine multiple hashtags
- [ ] Notifications respect user preferences
- [ ] Real-time updates work
- [ ] Muting works correctly
- [ ] Suggestions are relevant

---

#### 1.2 Thread Synchronization
**Status**: NOT STARTED  
**Effort**: 3-4 days  
**Dependencies**: Federation service  
**Blocking**: Federation completeness

**Missing Operations**:
1. `Query.threadContext(noteId)` → ThreadContext
2. `Mutation.syncThread(noteUrl, depth)` → SyncThreadPayload
3. `Mutation.syncMissingReplies(noteId)` → SyncRepliesPayload

**Implementation Plan**:

```
Day 1-2: Thread Service & Storage
├── Create pkg/services/threads/service.go
├── Add storage models:
│   ├── ThreadSync (PK: thread#{rootID}, SK: sync#{timestamp})
│   ├── ThreadNode (PK: note#{noteID}, SK: thread_meta)
│   └── MissingReply (PK: thread#{rootID}, SK: missing#{noteID})
├── Implement thread traversal:
│   ├── FindThreadRoot(ctx, noteID) - walk up inReplyTo chain
│   ├── GetThreadTree(ctx, rootID, depth) - build tree structure
│   ├── FindMissingReplies(ctx, rootID) - detect gaps
│   └── CalculateSyncStatus(ctx, rootID) - compute completeness
├── Add federation integration:
│   ├── FetchRemoteNote(ctx, noteURL) - get note via ActivityPub
│   ├── FetchRemoteReplies(ctx, noteURL) - get replies collection
│   └── ValidateThreadIntegrity(ctx, notes) - check consistency
└── Add background sync job queue

Day 3: Query Implementation
├── Implement threadContext resolver
│   ├── Find thread root
│   ├── Build thread structure
│   ├── Count participants
│   ├── Find missing posts
│   ├── Get sync status
│   └── Track last activity
└── Add caching for thread contexts

Day 4: Mutation Implementations & Testing
├── Implement syncThread mutation
│   ├── Parse note URL
│   ├── Fetch remote thread
│   ├── Traverse up to root
│   ├── Traverse down to leaves
│   ├── Respect depth limit
│   ├── Store all notes
│   ├── Update thread context
│   └── Return sync results
├── Implement syncMissingReplies mutation
│   ├── Get thread context
│   ├── Identify missing replies
│   ├── Fetch missing notes
│   ├── Update thread
│   └── Return sync count
├── Add error handling:
│   ├── Handle unreachable instances
│   ├── Handle deleted notes
│   ├── Handle authorization failures
│   └── Handle circular references
└── Add comprehensive tests
```

**Acceptance Criteria**:
- [ ] Can identify thread root
- [ ] Can detect missing replies
- [ ] Can sync remote threads
- [ ] Respects depth limits
- [ ] Handles federation errors gracefully
- [ ] Updates thread context correctly
- [ ] Background sync works

---

### PHASE 2: Federation & Monitoring (Week 3-4)
**Goal**: Complete Phase 2 features  
**Effort**: 8-11 days  
**Priority**: HIGH

#### 2.1 Phase 2 Alert Subscriptions
**Status**: NOT STARTED  
**Effort**: 1-2 days  
**Dependencies**: Existing metrics services

**Missing Operations**:
1. `Subscription.moderationAlerts(severity)` → ModerationAlert
2. `Subscription.costAlerts(thresholdUSD)` → CostAlert
3. `Subscription.budgetAlerts(domain)` → BudgetAlert
4. `Subscription.federationHealthUpdates(domain)` → FederationHealthUpdate

**Implementation Plan**:

```
Day 1: Alert Infrastructure
├── Extend subscription manager for alerts
├── Add alert threshold checking
├── Add alert deduplication (prevent spam)
├── Add alert priority routing
└── Connect to existing services:
    ├── Moderation service
    ├── Cost tracking service
    └── Federation health service

Day 2: Subscription Implementations
├── Implement moderationAlerts
│   ├── Subscribe to moderation events
│   ├── Filter by severity
│   ├── Include context (content, pattern)
│   └── Stream to subscribers
├── Implement costAlerts
│   ├── Subscribe to cost tracking events
│   ├── Check against threshold
│   ├── Aggregate by time window
│   └── Include cost breakdown
├── Implement budgetAlerts
│   ├── Subscribe to budget tracking
│   ├── Filter by domain
│   ├── Calculate % of budget used
│   └── Predict overspend
├── Implement federationHealthUpdates
│   ├── Subscribe to health check events
│   ├── Filter by domain
│   ├── Track status changes
│   └── Include issues and recommendations
└── Add tests for all subscriptions
```

**Notes**:
- The ModerationQueueUpdate subscription still uses the legacy direct event-bus wiring. Harmonizing it with the SubscriptionManager can be scheduled as a follow-up (e.g., during the Phase 3 moderation dashboard work).

---

#### 2.2 Media Streaming Completion ✅ COMPLETE
**Status**: 100% COMPLETE  
**Completed**: October 16, 2025  
**Dependencies**: AWS MediaConvert, CloudFront

**Implemented Operations**:
1. ✅ `Query.mediaStreamUrl(mediaId)` - Returns HLS/DASH manifests with variants
2. ✅ `Query.supportedBitrates(mediaId)` - Lists all transcoded quality levels
3. ✅ `Mutation.preloadMedia(mediaIds)` - Preloads manifests and primes CDN cache
4. ✅ `Mutation.requestStreamingUrl(mediaId, quality)` - Generates signed streaming URLs

**Implementation Summary**:

✅ **Completed Components**:

1. **AWS MediaConvert Integration** (`pkg/services/media/transcoding/service.go`)
   - Job submission with multiple quality levels (240p-4K)
   - HLS and DASH output generation
   - Thumbnail generation (frame capture)
   - Job status tracking and monitoring
   - Cost estimation ($0.0075-$0.060/min based on quality)
   - Duration estimation (1/5 of video length per quality)

2. **HLS/DASH Manifest Service** (`pkg/services/media/transcoding/manifest.go`)
   - HLS master playlist generation
   - Per-quality variant playlists
   - DASH manifest generation (MPD format)
   - 6-second segment duration
   - S3 storage integration
   - Manifest discovery and metadata

3. **CloudFront Signed URLs** (`pkg/services/media/transcoding/cloudfront.go`)
   - RSA-based URL signing
   - Policy generation and encoding
   - Streaming URL generation (HLS/DASH)
   - Batch signing for preload
   - Configurable TTL (default 24h)

4. **Media Service Pipeline** (`pkg/services/media/streaming_pipeline.go`)
   - `SubmitTranscodeJob()` - Queue transcoding with quality selection
   - `GetMediaRenditions()` - Retrieve available transcoded variants
   - `GenerateSignedStreamURL()` - Generate signed URLs with quality filter
   - `PreloadMedia()` - Prime CDN cache for multiple media items
   - Job state tracking in DynamoDB

5. **GraphQL Resolvers** (Production-ready)
   - `Query.mediaStreamUrl` - Returns HLS/DASH manifests with all variants
   - `Query.supportedBitrates` - Lists actual transcoded quality levels
   - `Mutation.requestStreamingUrl` - Signed URL with quality selection
   - `Mutation.preloadMedia` - Batch preload with manifest priming

6. **Storage & Repository**
   - TranscodingJob model with cost tracking
   - MediaSession model for streaming sessions
   - Repository methods for job persistence
   - GSI indexes for user/media queries

7. **Testing**
   - Unit tests for quality params, cost estimation, validation
   - CloudFront URL signing tests (with test keys)
   - Manifest generation logic tests
   - Business logic coverage (no AWS dependencies)

**Production Deployment Notes**:
- AWS MediaConvert endpoint must be configured per region
- CloudFront distribution and key pair required for signed URLs
- Private key must be securely stored (AWS Secrets Manager recommended)
- S3 buckets for source and streaming output must be configured
- Repository methods already integrated with existing MediaRepository
- Services are optional - can be enabled incrementally via service setters

**Follow-up Work** (tracked separately):
- MediaConvert job templates configuration in AWS
- CloudFront distribution setup with origin access identity
- Lambda function for processing MediaConvert job completion events
- Streaming analytics collection (view tracking, quality metrics)
- CDN cache invalidation on content updates

**Infrastructure Wiring** ✅ COMPLETE (October 16, 2025):
- Registry extended to wire transcoding, manifest, and CloudFront services
- Config fields added for media streaming (buckets, MediaConvert endpoint, CloudFront settings)
- CDK resources provisioned:
  - S3 buckets: `lesser-streaming-{env}` for HLS/DASH outputs
  - IAM role: `MediaConvertRole` with S3/CloudWatch permissions
  - Secrets Manager: CloudFront private key auto-generated at deployment
  - CloudFront distribution configured with OAI
  - Outputs: Bucket names, role ARNs, secret ARNs
- Environment configs updated (development/staging/production)
- Services are optional and lazy-initialized via registry setters
- Secrets Manager integration: Runtime retrieves CloudFront key automatically
- AWS config caching: Single SDK initialization shared across services

**Post-Deployment Steps** (semi-automated):
1. **Build and deploy stack**:
   ```bash
   # Build all binaries (includes cloudfront-keygen)
   make build-lambdas build-cloudfront-keygen
   
   # Or use the deploy target (builds automatically)
   make deploy ENV=production DOMAIN=your-domain.com
   ```
   - CDK provisions all infrastructure (S3, IAM, MediaConvert role, etc.)
   - Deployment triggers Go Lambda custom resource (`cmd/cloudfront-keygen`)
   - Lambda generates RSA-2048 key pair at deployment time (not synth time)
   - Private key stored in Secrets Manager as JSON: `{"privateKey": "...", "publicKey": "..."}`
   - Public key output in stack outputs for manual CloudFront upload
   
   **Note**: The `bin/cloudfront-keygen` binary is built automatically by `make deploy`, `make deploy-dev`, `make deploy-test`, and `make deploy-live` targets.

2. **Extract public key**:
   ```bash
   PUBLIC_KEY=$(aws cloudformation describe-stacks \
     --stack-name LesserStack-production \
     --query 'Stacks[0].Outputs[?OutputKey==`CloudFrontKeyPairPublicKey`].OutputValue' \
     --output text)
   ```

3. **Upload to CloudFront**:
   ```bash
   # Upload public key to CloudFront Key Management
   aws cloudfront create-public-key --public-key-config \
     Name=lesser-production-key,EncodedKey="$PUBLIC_KEY",CallerReference=$(date +%s)
   
   # Note the KeyId from response
   ```

4. **Create key group and update env**:
   ```bash
   # Upload public key and capture the Key ID
   KEY_ID=$(aws cloudfront create-public-key \
     --public-key-config Name=lesser-production-key,EncodedKey="$PUBLIC_KEY",CallerReference=$(date +%s) \
     --query 'PublicKey.Id' --output text)
   
   echo "CloudFront Key ID: $KEY_ID"
   
   # Create CloudFront key group with the Key ID
   aws cloudfront create-key-group --key-group-config \
     Name=lesser-production-keygroup,Items=$KEY_ID
   
   # Update CDK config YAML with the Key ID
   # Edit infra/cdk/config/production.yaml:
   #   media:
   #     cloudfrontKeyPairId: "K1234567EXAMPLE"
   
   # Or update Lambda environment variables directly (temporary until next deploy)
   for func in graphql api streaming media-processor; do
     aws lambda update-function-configuration \
       --function-name lesser-production-$func \
       --environment Variables="{CLOUDFRONT_KEY_PAIR_ID=$KEY_ID}" \
       --no-cli-pager 2>/dev/null || true
   done
   
   echo "✓ CloudFront key pair configured"
   echo "⚠️  Update infra/cdk/config/production.yaml with cloudfrontKeyPairId: $KEY_ID"
   echo "⚠️  Run 'cdk deploy' to persist the env var change"
   ```
   
   **For staging environment**, replace `production` with `staging` in all commands.
   
   **Automated Script**: Use `scripts/configure-cloudfront-keys.sh` to automate steps 2-4:
   ```bash
   # For production
   ./scripts/configure-cloudfront-keys.sh production
   
   # For staging
   ./scripts/configure-cloudfront-keys.sh staging
   ```
   
   The script will:
   - Extract the public key from CloudFormation outputs
   - Upload the public key to CloudFront
   - Create a CloudFront key group
   - Update the CDK config YAML file with the Key ID
   - Update all Lambda environment variables (temporary until next deploy)
   - Display a summary with the Key ID and next steps

**Environment Variable Wiring** ✅ COMPLETE:
- All Lambda functions receive media/ML config via `infra/cdk/constructs/lambda_functions.go`
- Wired environment variables:
  - `MEDIA_SOURCE_BUCKET_NAME`, `MEDIA_STREAMING_BUCKET_NAME` (from CDK-created buckets)
  - `MEDIA_CONVERT_ENDPOINT` (from YAML config)
  - `MEDIA_CONVERT_ROLE_ARN` (from CDK-created IAM role)
  - `CLOUDFRONT_DOMAIN` (from YAML config)
  - `CLOUDFRONT_PRIVATE_KEY_PATH` (Secrets Manager ARN, auto-wired)
  - `CLOUDFRONT_KEY_PAIR_ID` (from YAML config, set after manual upload)
  - `MANIFEST_TTL_HOURS` (from YAML config, default: 24)
  - `MODERATION_TRAINING_BUCKET_NAME` (from CDK-created bucket)
  - `MODERATION_MODEL_METADATA_TABLE` (from main DynamoDB table with GSI9)
  - `BEDROCK_TRAINING_REGION`, `BEDROCK_INFERENCE_MODEL_ID` (from YAML config)
- Runtime `pkg/config/config.go` loads these env vars into `Config` struct
- Registry services lazy-initialize using config values

**Secret Retrieval** ✅ COMPLETE:
- `pkg/services/registry.go` retrieves CloudFront key from Secrets Manager with caching
- `pkg/media/streaming/storage.go` also supports Secrets Manager retrieval
- Both implementations parse JSON secrets and extract `privateKey` field automatically
- ARN format (`arn:aws:secretsmanager:...`) and path format (`lesser/...`) both supported
- No secrets embedded in CloudFormation templates or CDK artifacts

**Deployment-Time Key Generation** ✅ COMPLETE:
- Go Lambda custom resource in `cmd/cloudfront-keygen/main.go`
- Compiles to ARM64 binary: `bin/cloudfront-keygen`
- Triggered by CloudFormation during stack deployment
- Generates RSA-2048 key pair using Go stdlib (`crypto/rsa`, `crypto/x509`)
- Creates/updates Secrets Manager secret with JSON payload
- Returns public key as CloudFormation output attribute
- Idempotent: safe to run multiple times (Create/Update events)

---

#### 2.3 Advanced Moderation ML
**Status**: ✅ COMPLETE (Async/Event-Driven Architecture)  
**Completed**: October 16, 2025  
**Refactored**: October 16, 2025 (Async Model)  
**Dependencies**: AWS Bedrock (Claude/Titan), Guardrails, S3, DynamoDB, EventBridge (optional)

**Infrastructure** ✅ COMPLETE:
- S3 bucket: `lesser-training-{env}` for ML training datasets
- DynamoDB main table stores all training job state
- Bedrock IAM policies extended:
  - `CreateModelCustomizationJob`, `GetModelCustomizationJob`, `ListModelCustomizationJobs`
  - `StopModelCustomizationJob`, `GetFoundationModel`, `ListFoundationModels`
  - `InvokeModel` with guardrail support
- **Bedrock Customization Role ARN**: Configured via `BEDROCK_CUSTOMIZATION_ROLE_ARN` env var
- Lambda role granted read/write access to training bucket
- Environment configs include Bedrock region, model IDs, guardrail configuration, and IAM role

**Implemented Operations** ✅:
1. `Mutation.trainModerationModel(samples, options)` → TrainingResult (returns SUBMITTED status immediately)
2. `Query.moderationEffectiveness(patternId, period)` → ModerationEffectiveness

**Implementation Details**:

**2.3.1 Feature Flags & Access Control** ✅
- `MODERATION_ML_ENABLED` - Global feature flag
- `MODERATION_ML_TENANTS` - Comma-separated list of allowed tenant IDs
- `pkg/common/feature_flags.go` - Centralized feature flag checking
- Admin-only access enforced in GraphQL resolvers

**2.3.2 Async Training Pipeline** ✅ REFACTORED
- **Sample Queuing**: `QueueSamples()` returns sample IDs after storing to DynamoDB
- **Dataset Preparation**: `prepareTrainingDataset()` fetches samples, formats as JSONL for Bedrock, **returns S3 key**
- **S3 Upload**: Training datasets uploaded to configured S3 bucket with unique timestamped+UUID paths
- **Bedrock Training**: `launchBedrockTraining()` calls `CreateModelCustomizationJob`, **uses config role ARN**, returns job ARN + name
- **Job Tracking**: `ModelTrainingJob` records created in DynamoDB with status SUBMITTED
- **Event Publishing**: `MODEL_TRAINING_SUBMITTED` events published via EventPublisher interface
- **No Polling**: Removed synchronous polling (`pollTrainingStatus` deprecated)
- **Completion Handling**: Job completion tracked via:
  - Option 1: DynamoDB Streams → Lambda → update job status to COMPLETED/FAILED
  - Option 2: EventBridge scheduled polling → Lambda checks Bedrock status → updates DynamoDB
  - Option 3: External workflow orchestration (Step Functions)
- **Model Versioning**: Training results stored in `ModerationModelVersion` when job completes

**2.3.3 Inference & Guardrails** ✅
- **ScoreContent**: Invokes active model via `InvokeModel` API
- **Guardrail Integration**: Bedrock guardrails applied when configured
- **Guardrail Handling**: Blocked content returns max risk score with reason (detects guardrail rejection in error messages)
- **Risk Scoring**: Label-based risk calculation (0.0=safe, 1.0=high risk)
- **Model Selection**: Automatically uses active model from DynamoDB

**2.3.4 GraphQL Integration** ✅
- **trainModerationModel**: Queues samples, launches training, **returns immediately with SUBMITTED status**
- **TrainingResult**: Includes `Status`, `JobID`, `JobName`, `DatasetS3Key`, with metrics=0 when SUBMITTED
- **moderationEffectiveness**: Computes TP/FP/TN/FN metrics, returns precision/recall/F1
- **Sample ID Fix**: Resolvers now use repository-assigned IDs instead of ObjectIDs
- **Authentication**: Both operations require admin authentication

**2.3.5 Storage Models** ✅
- **ModerationSample**: Stores labeled training samples with GSI for lookups by ID, label, reviewer
- **MLPrediction**: Tracks ML predictions vs human labels for effectiveness metrics (created, TODO integration)
- **ModerationModelVersion**: Tracks trained models, metrics, and active status
- **ModelTrainingJob**: NEW - Tracks training job lifecycle with status, metrics, timestamps, and TTL (90 days)
- **ModerationEffectivenessMetric**: Stores computed effectiveness metrics by period

**2.3.6 Real Bedrock APIs** ✅
- `CreateModelCustomizationJob` - Launches fine-tuning jobs
- `EventBridge` + completion Lambda/Step Functions - React to Bedrock job state changes (`InProgress` → `Completed`/`Failed`)
- `InvokeModel` - Performs inference on content
- Proper error handling for API failures and guardrail rejections

**Production Considerations & Future Work**:
- ✅ **Completion Handler**: Fully implemented using DynamoDB Streams on `ModelTrainingJob` + `MLPollRequest` for async polling
  - `ml-training-processor` Lambda handles job status changes and poll requests
  - Creates `ModerationModelVersion` on job completion with deactivation of previous versions
- ⚠️ **IAM Role ARN**: Configure `BEDROCK_CUSTOMIZATION_ROLE_ARN` per environment (currently uses placeholder with warning)
- ✅ **Training Metrics**: Real metrics extracted from Bedrock `GetModelCustomizationJob.TrainingMetrics` with S3 fallback and comprehensive tests
- ✅ **Event Publisher**: Training events emitted via DynamoDB `StreamingEvent` records for stream processor consumption
- ⚠️ **Effectiveness Computation**: Currently simplified - integrate `MLPrediction` model to track real predictions vs labels (deferred to Phase 2.4)
- ⚠️ **Content Extraction**: Sample queuing uses metadata fallback - should fetch actual content from Object/Status repositories (TODO marked in code, deferred to Phase 2.4)
- ⚠️ **Cost Tracking**: Track Bedrock API costs (training jobs, inference invocations) for budget monitoring (deferred to Phase 2.4)
- ⚠️ **Job TTL**: Training jobs expire after 90 days - consider archiving to S3 for audit trail (deferred to Phase 2.4)

---

#### 2.4 Severed Relationships
**Status**: ✅ COMPLETE  
**Completed**: October 17, 2025  
**Effort**: 4 days  

**Implemented Operations**:
1. ✅ `Query.severedRelationships(instance, first, after)` → SeveredRelationshipConnection
2. ✅ `Query.affectedRelationships(severedRelationshipId)` → AffectedRelationshipConnection
3. ✅ `Mutation.acknowledgeSeverance(id)` → AcknowledgePayload
4. ✅ `Mutation.attemptReconnection(id)` → ReconnectionPayload

**Key Components**:
- Service layer: `pkg/services/severance/service.go`
- Storage models: `pkg/storage/models/severed_relationship.go`
- Repository: `pkg/storage/repositories/severance_repository.go`
- GraphQL resolvers: `graph/*_resolvers_federation.go`
- Event processor: `cmd/severance-processor/` (auto-detection Lambda)
- Infrastructure: CDK constructs wired with DynamoDB streams

**Event-Driven Architecture**:
- Auto-detection via DynamoDB streams for domain blocks, federation issues, instance health
- Event emission: SEVERANCE_DETECTED, SEVERANCE_ACKNOWLEDGED, RECONNECTION_ATTEMPTED
- Severity calculation: High (>1000 affected), Medium (100-1000), Low (<100)

**Verification**: See `docs/PHASE_2_4_VERIFICATION.md` for complete implementation details.

**Implementation Plan**:

```
Day 1: Detection & Storage
├── Create pkg/services/severance/service.go
├── Add storage models:
│   ├── SeveredRelationship
│   ├── AffectedRelationship
│   └── ReconnectionAttempt
├── Implement detection:
│   ├── Monitor federation health
│   ├── Detect instance blocks
│   ├── Detect defederation
│   ├── Track instance downtime
│   └── Identify policy violations
├── Add automatic severance tracking
└── Implement notification system

Day 2: Query Implementations
├── Implement severedRelationships query
│   ├── Filter by instance
│   ├── Show reason and timestamp
│   ├── Include affected counts
│   ├── Apply pagination
│   └── Sort by severity
├── Implement affectedRelationships query
│   ├── Get followers affected
│   ├── Get following affected
│   ├── Show relationship history
│   └── Include last interaction
└── Add caching for severance data

Day 3: Reconnection Logic
├── Implement acknowledgeSeverance mutation
│   ├── Mark as acknowledged by user
│   ├── Update notification status
│   └── Track acknowledgment
├── Implement attemptReconnection mutation
│   ├── Check instance reachability
│   ├── Attempt to restore follows
│   ├── Attempt to restore followers
│   ├── Track success/failure
│   ├── Handle partial recovery
│   └── Update severance status
└── Add reconnection job queue

Day 4: Monitoring & Testing
├── Add automatic detection job
├── Add user notifications
├── Add admin dashboard data
├── Comprehensive testing
└── Edge case handling
```

---

### PHASE 3: Visualization & Analytics (Week 5-6)
**Goal**: Complete Phase 3 features  
**Effort**: 10-13 days  
**Priority**: MEDIUM

#### 3.1 Federation Graph Visualization
**Status**: ✅ COMPLETE  
**Completed**: October 17, 2025  
**Effort**: 1 day  
**Dependencies**: Graph computation, Federation repository

**Implemented Operations**:
1. ✅ `Query.federationMap(depth)` → FederationGraph - Returns nodes, edges, clusters with health scores
2. ✅ `Query.instanceRelationships(domain)` → InstanceRelations - Returns connections, score, recommendations
3. ✅ `Query.federationFlow(period)` → FederationFlow - Returns flow analysis with costs and volumes

**Implementation Plan**:

```
Day 1-2: Graph Data Collection
├── Create graph aggregation job
├── Collect relationship data:
│   ├── Follow relationships across instances
│   ├── Activity volume between instances
│   ├── Response times and errors
│   └── Shared content metrics
├── Calculate connection weights:
│   ├── Bidirectional activity
│   ├── Interaction frequency
│   ├── Content similarity
│   └── Health score
├── Store graph snapshot in DynamoDB
└── Schedule periodic updates (hourly)

Day 3: Clustering & Layout
├── Implement clustering algorithm:
│   ├── Group by software type
│   ├── Group by language
│   ├── Group by activity patterns
│   └── Identify communities
├── Calculate layout coordinates:
│   ├── Force-directed layout
│   ├── Minimize edge crossings
│   ├── Respect hierarchies
│   └── Optimize for visualization
└── Cache computed layouts

Day 4: Query Implementations
├── Implement federationMap query
│   ├── Get graph snapshot
│   ├── Apply depth filter
│   ├── Return nodes (instances)
│   ├── Return edges (relationships)
│   ├── Return clusters
│   └── Include health scores
├── Implement instanceRelationships query
│   ├── Get direct connections
│   ├── Get indirect connections
│   ├── Calculate connection strength
│   ├── Find mutual connections
│   └── Generate recommendations
└── Add caching layer

Day 5: Flow Analysis
├── Implement federationFlow query
│   ├── Aggregate activity by time
│   ├── Identify top sources
│   ├── Identify top destinations
│   ├── Calculate volume trends
│   ├── Analyze cost by instance
│   └── Show hourly patterns
└── Add flow visualization data

Day 6: Optimization & Testing
├── Optimize graph queries
├── Add incremental updates
├── Test with large datasets
└── Performance tuning
```

---

#### 3.2 Streaming Analytics
**Status**: ✅ COMPLETE  
**Completed**: October 17, 2025  
**Effort**: 1 day  
**Dependencies**: Media streaming (Phase 2.2)

**Implemented Operations**:
1. ✅ `Query.streamingAnalytics(mediaId)` → StreamingAnalytics - Real-time metrics aggregation
2. ✅ `Query.popularStreams(first, after)` → StreamConnection - Cursor-paginated popular content
3. ✅ `Query.bandwidthUsage(period)` → BandwidthReport - Time-windowed bandwidth analytics

**Implementation Summary**:

```
Day 1: Analytics Collection
├── Set up CloudWatch streaming metrics
├── Track streaming events:
│   ├── View starts
│   ├── View completions
│   ├── Quality changes
│   ├── Buffering events
│   ├── Bandwidth consumption
│   └── Watch time
├── Store analytics in time-series DB
└── Add real-time aggregation

Day 2: Query Implementations
├── Implement streamingAnalytics query
│   ├── Get total views
│   ├── Get unique viewers
│   ├── Calculate avg watch time
│   ├── Get quality distribution
│   ├── Count buffering events
│   └── Calculate completion rate
├── Implement popularStreams query
│   ├── Rank by view count
│   ├── Weight by recency
│   ├── Filter by time period
│   └── Apply pagination
└── Add caching for analytics

Day 3: Bandwidth Reporting
├── Implement bandwidthUsage query
│   ├── Aggregate by time period
│   ├── Break down by quality
│   ├── Show hourly patterns
│   ├── Calculate peak usage
│   ├── Estimate costs
│   └── Project future usage
└── Add bandwidth optimization hints

Day 4: Testing & Optimization
├── Test analytics accuracy
├── Optimize time-series queries
├── Add data retention policies
└── Performance tuning
```

---

#### 3.3 Performance Monitoring
**Status**: STUB (infrastructure exists)  
**Effort**: 2-3 days  
**Dependencies**: Observability package

**Missing Operations**:
1. `Query.performanceMetrics(service)` → PerformanceReport
2. `Query.slowQueries(threshold)` → [QueryPerformance]
3. `Query.infrastructureHealth` → InfrastructureStatus (needs real data)

**Implementation Plan**:

```
Day 1: Metrics Integration
├── Connect to existing observability package
├── Query EMF metrics from CloudWatch
├── Aggregate Lambda metrics:
│   ├── Duration percentiles (p50, p95, p99)
│   ├── Error rates
│   ├── Throughput
│   ├── Cold starts
│   └── Memory usage
├── Aggregate DynamoDB metrics
└── Aggregate service-specific metrics

Day 2: Query Implementations
├── Implement performanceMetrics query
│   ├── Get service-specific metrics
│   ├── Calculate percentiles
│   ├── Show error rates
│   ├── Track throughput
│   └── Count cold starts
├── Implement slowQueries query
│   ├── Parse GraphQL query logs
│   ├── Identify slow operations
│   ├── Track frequency
│   ├── Calculate percentiles
│   └── Show recent occurrences
├── Implement infrastructureHealth query
│   ├── Check service health
│   ├── Check database health
│   ├── Check queue health
│   ├── List active alerts
│   └── Calculate overall status
└── Add metric caching

Day 3: Dashboard Integration & Testing
├── Create health check aggregator
├── Add alert correlation
├── Test metric accuracy
└── Performance optimization
```

---

#### 3.4 Moderation Dashboard
**Status**: NOT STARTED  
**Effort**: 3-4 days  
**Dependencies**: Moderation service

**Missing Operations**:
1. `Query.moderationDashboard(filter)` → ModerationDashboard
2. `Query.patternEffectiveness(patternId)` → PatternStats (needs real data)
3. `Query.moderatorActivity(moderatorId, period)` → ModeratorStats

**Implementation Plan**:

```
Day 1: Dashboard Data Aggregation
├── Create moderation metrics aggregator
├── Collect pending review counts
├── Get recent decisions
├── Track pattern statistics
├── Calculate response times
├── Identify threat trends
└── Store aggregated data

Day 2: Query Implementations
├── Implement moderationDashboard query
│   ├── Count pending reviews
│   ├── Get recent decisions
│   ├── Show top patterns
│   ├── Calculate false positive rate
│   ├── Show avg response time
│   ├── Display threat trends
│   └── Apply filters
├── Implement patternEffectiveness query
│   ├── Get pattern stats (from Phase 2.3)
│   ├── Calculate match count
│   ├── Track accuracy
│   ├── Show last match
│   └── Display trend
└── Add dashboard caching

Day 3: Moderator Analytics
├── Implement moderatorActivity query
│   ├── Count decisions by moderator
│   ├── Calculate avg response time
│   ├── Track accuracy (overturned decisions)
│   ├── Break down by category
│   ├── Show performance trends
│   └── Compare to team average
└── Add moderator leaderboard data

Day 4: Testing & Optimization
├── Test dashboard accuracy
├── Optimize aggregation queries
├── Add real-time updates
└── Performance tuning
```

---

#### 3.5 Phase 3 Subscriptions
**Status**: NOT STARTED  
**Effort**: 1-2 days  
**Dependencies**: Phase 3 features

**Missing Operations**:
1. `Subscription.moderationQueueUpdate(priority)` → ModerationItem
2. `Subscription.threatIntelligence` → ThreatAlert
3. `Subscription.performanceAlert(severity)` → PerformanceAlert
4. `Subscription.infrastructureEvent` → InfrastructureEvent

**Implementation Plan**:

```
Day 1: Subscription Implementations
├── Implement moderationQueueUpdate
│   ├── Subscribe to queue changes
│   ├── Filter by priority
│   ├── Stream new items
│   └── Include assignment changes
├── Implement threatIntelligence
│   ├── Subscribe to threat detection
│   ├── Include threat details
│   ├── Show mitigation steps
│   └── Track affected instances
├── Implement performanceAlert
│   ├── Subscribe to metric breaches
│   ├── Filter by severity
│   ├── Include threshold info
│   └── Show actual values
├── Implement infrastructureEvent
│   ├── Subscribe to infrastructure changes
│   ├── Track deployments
│   ├── Track scaling events
│   ├── Track failures/recoveries
│   └── Show impact assessment
└── Add subscription tests

Day 2: Integration & Testing
├── Connect to event sources
├── Test real-time delivery
├── Add error handling
└── Performance testing
```

---

## Testing Strategy

### Unit Tests (Throughout Implementation)
- Test each service method
- Test edge cases
- Test error handling
- Target: 80%+ coverage

### Integration Tests (End of Each Phase)
- Test GraphQL resolvers
- Test with real dependencies
- Test pagination
- Test subscriptions
- Target: 70%+ coverage

### End-to-End Tests (End of Each Phase)
- Test complete workflows
- Test user scenarios
- Test federation scenarios
- Test real-time features
- Target: 60%+ coverage

### Performance Tests (End of Phase 3)
- Load testing
- Stress testing
- Subscription scaling
- Database query optimization
- Target: <200ms p95 latency

---

## Documentation Requirements

### API Documentation (Continuous)
- Update GraphQL schema docs
- Add operation examples
- Document error codes
- Add best practices

### Integration Guides (End of Each Phase)
- Client SDK examples
- Subscription setup
- Authentication flows
- Error handling

### Architecture Documentation (End of Project)
- System design updates
- Data flow diagrams
- Scalability considerations
- Cost optimization guide

---

## Rollout Strategy

### Phase 1 Rollout
- Deploy hashtag following to beta
- Deploy thread sync to beta
- Monitor for issues
- Collect feedback
- Roll out to production

### Phase 2 Rollout
- Feature flags for streaming
- Gradual rollout of ML features
- Monitoring dashboard for admins only
- Collect metrics
- Full production rollout

### Phase 3 Rollout
- Analytics for admins first
- Beta test visualization tools
- Gradual user access
- Performance monitoring
- Full rollout

---

## Risk Mitigation

### Technical Risks
1. **GraphQL query complexity** → Add complexity limits
2. **Subscription scaling** → Use connection pooling
3. **ML model accuracy** → Continuous training
4. **Graph computation performance** → Pre-compute and cache
5. **Streaming bandwidth costs** → Implement adaptive bitrate

### Schedule Risks
1. **Dependencies on AWS services** → Have fallbacks ready
2. **ML training time** → Start training pipeline early
3. **Graph computation** → Can use simplified algorithms initially
4. **Testing time** → Automate where possible

---

## Success Metrics

### Functional Completeness
- ✅ All 36 missing operations implemented
- ✅ All tests passing
- ✅ All documentation complete
- ✅ All features deployed

### Quality Metrics
- Unit test coverage: >80%
- Integration test coverage: >70%
- E2E test coverage: >60%
- Performance: p95 latency <200ms
- Error rate: <0.1%

### User Metrics (Post-Launch)
- Hashtag follow usage: >10% of users
- Thread sync requests: >100/day
- Streaming adoption: >5% of media
- Alert subscriptions: >20 admins

---

## Resource Requirements

### Development
- 1 Senior Backend Engineer: Full-time (7 weeks)
- OR 2 Mid-level Engineers: Full-time (4 weeks)
- 1 QA Engineer: Part-time (2 weeks)

### Infrastructure
- AWS MediaConvert: For streaming
- AWS SageMaker or Bedrock: For ML training
- CloudWatch: Enhanced monitoring
- Additional DynamoDB capacity: 20-30%
- Additional S3 storage: For streaming

### Budget
- Development: 7-8 weeks engineering time
- Infrastructure: $500-1000/month additional
- Testing: 2 weeks QA time
- Total: ~$40,000-60,000 (depending on team rates)

---

## Timeline Summary

| Phase | Features | Duration | End Date (1 Dev) | End Date (2 Devs) |
|-------|----------|----------|------------------|-------------------|
| Phase 1 | Mastodon Parity | 6-9 days | Week 2 | Week 1 |
| Phase 2 | Federation & Monitoring | 8-11 days | Week 4 | Week 2.5 |
| Phase 3 | Visualization & Analytics | 10-13 days | Week 6.5 | Week 4.5 |
| Testing | Integration & E2E | 3-4 days | Week 7 | Week 5 |
| **Total** | **100% Complete** | **27-37 days** | **7 weeks** | **5 weeks** |

---

## Execution Order (Optimized for Dependencies)

### Week 1
1. Hashtag Following System (Day 1-3)
2. Thread Synchronization (Day 4-7)

### Week 2
3. Severed Relationships (Day 8-11)
4. Phase 2 Alert Subscriptions (Day 12-13)
5. Start Media Streaming (Day 14)

### Week 3
6. Complete Media Streaming (Day 15-18)
7. Advanced Moderation ML (Day 19-21)

### Week 4
8. Federation Graph Visualization (Day 22-27)

### Week 5
9. Streaming Analytics (Day 28-31)
10. Performance Monitoring (Day 32-34)

### Week 6
11. Moderation Dashboard (Day 35-38)
12. Phase 3 Subscriptions (Day 39-40)

### Week 7
13. Integration Testing (Day 41-43)
14. E2E Testing (Day 44-45)
15. Documentation & Deployment (Day 46-47)

---

## Deliverables Checklist

### Code
- [ ] All 36 missing operations implemented
- [ ] All services created
- [ ] All storage models added
- [ ] All resolvers complete
- [ ] All subscriptions working

### Tests
- [ ] Unit tests (80%+ coverage)
- [ ] Integration tests (70%+ coverage)
- [ ] E2E tests (60%+ coverage)
- [ ] Performance tests passed
- [ ] Load tests passed

### Documentation
- [ ] API documentation updated
- [ ] Integration guides written
- [ ] Architecture docs updated
- [ ] Deployment guide updated
- [ ] Troubleshooting guide created

### Deployment
- [ ] Feature flags configured
- [ ] Monitoring dashboards created
- [ ] Alerts configured
- [ ] Rollback plan documented
- [ ] Production deployment successful

---

## PLAN STATUS: READY FOR APPROVAL

**Prepared by**: AI Assistant  
**Date**: October 15, 2025  
**Next Step**: Approval and Phase 1 execution

**Estimated to reach 100%**: 7 weeks (1 developer) or 5 weeks (2 developers)

