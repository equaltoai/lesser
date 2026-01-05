import { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type Maybe<T> = T | null | undefined;
export type InputMaybe<T> = T | null;
export type Exact<T extends {
    [key: string]: unknown;
}> = {
    [K in keyof T]: T[K];
};
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & {
    [SubKey in K]?: Maybe<T[SubKey]>;
};
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & {
    [SubKey in K]: Maybe<T[SubKey]>;
};
export type MakeEmpty<T extends {
    [key: string]: unknown;
}, K extends keyof T> = {
    [_ in K]?: never;
};
export type Incremental<T> = T | {
    [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never;
};
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
    ID: {
        input: string;
        output: string;
    };
    String: {
        input: string;
        output: string;
    };
    Boolean: {
        input: boolean;
        output: boolean;
    };
    Int: {
        input: number;
        output: number;
    };
    Float: {
        input: number;
        output: number;
    };
    Cursor: {
        input: string;
        output: string;
    };
    Duration: {
        input: string;
        output: string;
    };
    Time: {
        input: string;
        output: string;
    };
    Upload: {
        input: File | Blob;
        output: File | Blob;
    };
};
/** AI Analysis types */
export type AiAnalysis = {
    readonly __typename: 'AIAnalysis';
    readonly aiDetection?: Maybe<AiDetection>;
    readonly analyzedAt: Scalars['Time']['output'];
    readonly confidence: Scalars['Float']['output'];
    readonly id: Scalars['ID']['output'];
    readonly imageAnalysis?: Maybe<ImageAnalysis>;
    readonly moderationAction: ModerationAction;
    readonly objectId: Scalars['ID']['output'];
    readonly objectType: Scalars['String']['output'];
    readonly overallRisk: Scalars['Float']['output'];
    readonly spamAnalysis?: Maybe<SpamAnalysis>;
    readonly textAnalysis?: Maybe<TextAnalysis>;
};
export type AiAnalysisRequest = {
    readonly __typename: 'AIAnalysisRequest';
    readonly estimatedTime: Scalars['String']['output'];
    readonly message: Scalars['String']['output'];
    readonly objectId: Scalars['ID']['output'];
};
export type AiCapabilities = {
    readonly __typename: 'AICapabilities';
    readonly aiDetection: AiDetectionCapabilities;
    readonly costPerAnalysis: CostBreakdown;
    readonly imageAnalysis: ImageAnalysisCapabilities;
    readonly moderationActions: ReadonlyArray<Scalars['String']['output']>;
    readonly textAnalysis: TextAnalysisCapabilities;
};
export type AiDetection = {
    readonly __typename: 'AIDetection';
    readonly aiGeneratedProbability: Scalars['Float']['output'];
    readonly generationModel?: Maybe<Scalars['String']['output']>;
    readonly patternConsistency: Scalars['Float']['output'];
    readonly semanticCoherence: Scalars['Float']['output'];
    readonly styleDeviation: Scalars['Float']['output'];
    readonly suspiciousPatterns: ReadonlyArray<Scalars['String']['output']>;
};
export type AiDetectionCapabilities = {
    readonly __typename: 'AIDetectionCapabilities';
    readonly aiGeneratedContent: Scalars['Boolean']['output'];
    readonly patternAnalysis: Scalars['Boolean']['output'];
    readonly styleConsistency: Scalars['Boolean']['output'];
};
export type AiStats = {
    readonly __typename: 'AIStats';
    readonly aiContentRate: Scalars['Float']['output'];
    readonly aiGenerated: Scalars['Int']['output'];
    readonly moderationActions: ModerationActionCounts;
    readonly nsfwContent: Scalars['Int']['output'];
    readonly nsfwRate: Scalars['Float']['output'];
    readonly period: Scalars['String']['output'];
    readonly piiDetected: Scalars['Int']['output'];
    readonly spamDetected: Scalars['Int']['output'];
    readonly spamRate: Scalars['Float']['output'];
    readonly totalAnalyses: Scalars['Int']['output'];
    readonly toxicContent: Scalars['Int']['output'];
    readonly toxicityRate: Scalars['Float']['output'];
};
export type AccessLog = {
    readonly __typename: 'AccessLog';
    readonly cost: Scalars['Int']['output'];
    readonly operation: Scalars['String']['output'];
    readonly timestamp: Scalars['Time']['output'];
};
export type AccountSuggestion = {
    readonly __typename: 'AccountSuggestion';
    readonly account: Actor;
    readonly reason?: Maybe<Scalars['String']['output']>;
    readonly source: SuggestionSource;
};
export type AcknowledgePayload = {
    readonly __typename: 'AcknowledgePayload';
    readonly acknowledged: Scalars['Boolean']['output'];
    readonly severedRelationship: SeveredRelationship;
    readonly success: Scalars['Boolean']['output'];
};
export type Activity = {
    readonly __typename: 'Activity';
    readonly actor: Actor;
    readonly cost: Scalars['Int']['output'];
    readonly id: Scalars['ID']['output'];
    readonly object?: Maybe<Object>;
    readonly published: Scalars['Time']['output'];
    readonly target?: Maybe<Object>;
    readonly type: ActivityType;
};
export type ActivityType = 'ACCEPT' | 'ANNOUNCE' | 'CREATE' | 'DELETE' | 'FLAG' | 'FOLLOW' | 'LIKE' | 'REJECT' | 'UNDO' | 'UPDATE';
export type Actor = {
    readonly __typename: 'Actor';
    readonly avatar?: Maybe<Scalars['String']['output']>;
    readonly bot: Scalars['Boolean']['output'];
    readonly createdAt: Scalars['Time']['output'];
    readonly displayName?: Maybe<Scalars['String']['output']>;
    readonly domain?: Maybe<Scalars['String']['output']>;
    readonly fields: ReadonlyArray<Field>;
    readonly followers: Scalars['Int']['output'];
    readonly following: Scalars['Int']['output'];
    readonly header?: Maybe<Scalars['String']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly locked: Scalars['Boolean']['output'];
    readonly reputation?: Maybe<Reputation>;
    readonly statusesCount: Scalars['Int']['output'];
    readonly summary?: Maybe<Scalars['String']['output']>;
    readonly trustScore: Scalars['Float']['output'];
    readonly updatedAt: Scalars['Time']['output'];
    readonly username: Scalars['String']['output'];
    readonly vouches: ReadonlyArray<Vouch>;
};
export type ActorListPage = {
    readonly __typename: 'ActorListPage';
    readonly actors: ReadonlyArray<Actor>;
    readonly nextCursor?: Maybe<Scalars['Cursor']['output']>;
    readonly totalCount: Scalars['Int']['output'];
};
export type ActorType = 'APPLICATION' | 'GROUP' | 'ORGANIZATION' | 'PERSON' | 'SERVICE';
export type AffectedRelationship = {
    readonly __typename: 'AffectedRelationship';
    readonly actor: Actor;
    readonly establishedAt: Scalars['Time']['output'];
    readonly lastInteraction?: Maybe<Scalars['Time']['output']>;
    readonly relationshipType: Scalars['String']['output'];
};
export type AffectedRelationshipConnection = {
    readonly __typename: 'AffectedRelationshipConnection';
    readonly edges: ReadonlyArray<AffectedRelationshipEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type AffectedRelationshipEdge = {
    readonly __typename: 'AffectedRelationshipEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: AffectedRelationship;
};
export type AlertLevel = 'CRITICAL' | 'INFO' | 'WARNING';
export type AlertSeverity = 'CRITICAL' | 'ERROR' | 'INFO' | 'WARNING';
export type Attachment = {
    readonly __typename: 'Attachment';
    readonly blurhash?: Maybe<Scalars['String']['output']>;
    readonly description?: Maybe<Scalars['String']['output']>;
    readonly duration?: Maybe<Scalars['Float']['output']>;
    readonly height?: Maybe<Scalars['Int']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly preview?: Maybe<Scalars['String']['output']>;
    readonly type: Scalars['String']['output'];
    readonly url: Scalars['String']['output'];
    readonly width?: Maybe<Scalars['Int']['output']>;
};
export type BandwidthReport = {
    readonly __typename: 'BandwidthReport';
    readonly avgMbps: Scalars['Float']['output'];
    readonly byHour: ReadonlyArray<HourlyBandwidth>;
    readonly byQuality: ReadonlyArray<QualityBandwidth>;
    readonly cost: Scalars['Float']['output'];
    readonly peakMbps: Scalars['Float']['output'];
    readonly period: TimePeriod;
    readonly totalGB: Scalars['Float']['output'];
};
export type BedrockTrainingOptions = {
    readonly baseModelId?: InputMaybe<Scalars['String']['input']>;
    readonly datasetS3Path?: InputMaybe<Scalars['String']['input']>;
    readonly earlyStoppingEnabled?: InputMaybe<Scalars['Boolean']['input']>;
    readonly maxTrainingTime?: InputMaybe<Scalars['Int']['input']>;
    readonly outputS3Path?: InputMaybe<Scalars['String']['input']>;
};
export type Bitrate = {
    readonly __typename: 'Bitrate';
    readonly bitsPerSecond: Scalars['Int']['output'];
    readonly codec: Scalars['String']['output'];
    readonly height: Scalars['Int']['output'];
    readonly quality: StreamQuality;
    readonly width: Scalars['Int']['output'];
};
export type BudgetAlert = {
    readonly __typename: 'BudgetAlert';
    readonly alertLevel: AlertLevel;
    readonly budgetUSD: Scalars['Float']['output'];
    readonly domain: Scalars['String']['output'];
    readonly id: Scalars['ID']['output'];
    readonly percentUsed: Scalars['Float']['output'];
    readonly projectedOverspend?: Maybe<Scalars['Float']['output']>;
    readonly spentUSD: Scalars['Float']['output'];
    readonly timestamp: Scalars['Time']['output'];
};
export type CategoryStats = {
    readonly __typename: 'CategoryStats';
    readonly accuracy: Scalars['Float']['output'];
    readonly category: Scalars['String']['output'];
    readonly count: Scalars['Int']['output'];
};
export type Celebrity = {
    readonly __typename: 'Celebrity';
    readonly confidence: Scalars['Float']['output'];
    readonly name: Scalars['String']['output'];
    readonly urls: ReadonlyArray<Scalars['String']['output']>;
};
export type CommunityNote = {
    readonly __typename: 'CommunityNote';
    readonly author: Actor;
    readonly content: Scalars['String']['output'];
    readonly createdAt: Scalars['Time']['output'];
    readonly helpful: Scalars['Int']['output'];
    readonly id: Scalars['ID']['output'];
    readonly notHelpful: Scalars['Int']['output'];
};
export type CommunityNoteInput = {
    readonly content: Scalars['String']['input'];
    readonly objectId: Scalars['ID']['input'];
};
export type CommunityNotePayload = {
    readonly __typename: 'CommunityNotePayload';
    readonly note: CommunityNote;
    readonly object: Object;
};
export type ConnectionType = 'BOOSTS' | 'FOLLOWS' | 'MENTIONS' | 'MIXED' | 'QUOTES' | 'REPLIES';
export type ContentMap = {
    readonly __typename: 'ContentMap';
    readonly content: Scalars['String']['output'];
    readonly language: Scalars['String']['output'];
};
export type ContentMapInput = {
    readonly content: Scalars['String']['input'];
    readonly language: Scalars['String']['input'];
};
export type Conversation = {
    readonly __typename: 'Conversation';
    readonly accounts: ReadonlyArray<Actor>;
    readonly createdAt: Scalars['Time']['output'];
    readonly id: Scalars['ID']['output'];
    readonly lastStatus?: Maybe<Object>;
    readonly unread: Scalars['Boolean']['output'];
    readonly updatedAt: Scalars['Time']['output'];
};
export type Coordinates = {
    readonly __typename: 'Coordinates';
    readonly x: Scalars['Float']['output'];
    readonly y: Scalars['Float']['output'];
};
export type CostAlert = {
    readonly __typename: 'CostAlert';
    readonly amount: Scalars['Float']['output'];
    readonly domain?: Maybe<Scalars['String']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly message: Scalars['String']['output'];
    readonly threshold: Scalars['Float']['output'];
    readonly timestamp: Scalars['Time']['output'];
    readonly type: Scalars['String']['output'];
};
export type CostBreakdown = {
    readonly __typename: 'CostBreakdown';
    readonly breakdown: ReadonlyArray<CostItem>;
    readonly dataTransferCost: Scalars['Float']['output'];
    readonly dynamoDBCost: Scalars['Float']['output'];
    readonly lambdaCost: Scalars['Float']['output'];
    readonly period: Period;
    readonly s3StorageCost: Scalars['Float']['output'];
    readonly totalCost: Scalars['Float']['output'];
};
export type CostItem = {
    readonly __typename: 'CostItem';
    readonly cost: Scalars['Float']['output'];
    readonly count: Scalars['Int']['output'];
    readonly operation: Scalars['String']['output'];
};
export type CostOptimizationResult = {
    readonly __typename: 'CostOptimizationResult';
    readonly actions: ReadonlyArray<OptimizationAction>;
    readonly optimized: Scalars['Int']['output'];
    readonly savedMonthlyUSD: Scalars['Float']['output'];
};
export type CostOrderBy = 'DOMAIN_ASC' | 'ERROR_RATE_DESC' | 'REQUEST_COUNT_DESC' | 'TOTAL_COST_ASC' | 'TOTAL_COST_DESC';
export type CostProjection = {
    readonly __typename: 'CostProjection';
    readonly currentCost: Scalars['Float']['output'];
    readonly period: Period;
    readonly projectedCost: Scalars['Float']['output'];
    readonly recommendations: ReadonlyArray<Scalars['String']['output']>;
    readonly topDrivers: ReadonlyArray<Driver>;
    readonly variance: Scalars['Float']['output'];
};
export type CostUpdate = {
    readonly __typename: 'CostUpdate';
    readonly dailyTotal: Scalars['Float']['output'];
    readonly monthlyProjection: Scalars['Float']['output'];
    readonly operationCost: Scalars['Int']['output'];
};
export type CreateEmojiInput = {
    readonly category?: InputMaybe<Scalars['String']['input']>;
    readonly image: Scalars['String']['input'];
    readonly shortcode: Scalars['String']['input'];
    readonly visibleInPicker?: InputMaybe<Scalars['Boolean']['input']>;
};
export type CreateListInput = {
    readonly exclusive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly repliesPolicy?: InputMaybe<RepliesPolicy>;
    readonly title: Scalars['String']['input'];
};
export type CreateNoteInput = {
    readonly attachmentIds?: InputMaybe<ReadonlyArray<Scalars['ID']['input']>>;
    readonly content: Scalars['String']['input'];
    readonly contentMap?: InputMaybe<ReadonlyArray<ContentMapInput>>;
    readonly inReplyToId?: InputMaybe<Scalars['ID']['input']>;
    readonly mentions?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    readonly poll?: InputMaybe<PollParamsInput>;
    readonly quoteId?: InputMaybe<Scalars['ID']['input']>;
    readonly sensitive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly spoilerText?: InputMaybe<Scalars['String']['input']>;
    readonly tags?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    readonly visibility: Visibility;
};
export type CreateNotePayload = {
    readonly __typename: 'CreateNotePayload';
    readonly activity: Activity;
    readonly cost: CostUpdate;
    readonly object: Object;
};
export type CreateQuoteNoteInput = {
    readonly content: Scalars['String']['input'];
    readonly mediaIds?: InputMaybe<ReadonlyArray<Scalars['ID']['input']>>;
    readonly quoteType?: InputMaybe<QuoteType>;
    readonly quoteUrl: Scalars['String']['input'];
    readonly quoteable?: InputMaybe<Scalars['Boolean']['input']>;
    readonly sensitive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly spoilerText?: InputMaybe<Scalars['String']['input']>;
    readonly visibility?: InputMaybe<Visibility>;
};
export type CustomEmoji = {
    readonly __typename: 'CustomEmoji';
    readonly category?: Maybe<Scalars['String']['output']>;
    readonly createdAt: Scalars['Time']['output'];
    readonly domain?: Maybe<Scalars['String']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly shortcode: Scalars['String']['output'];
    readonly staticUrl: Scalars['String']['output'];
    readonly updatedAt: Scalars['Time']['output'];
    readonly url: Scalars['String']['output'];
    readonly visibleInPicker: Scalars['Boolean']['output'];
};
export type DatabaseStatus = {
    readonly __typename: 'DatabaseStatus';
    readonly connections: Scalars['Int']['output'];
    readonly latency: Scalars['Duration']['output'];
    readonly name: Scalars['String']['output'];
    readonly status: HealthStatus;
    readonly throughput: Scalars['Float']['output'];
    readonly type: Scalars['String']['output'];
};
export type DigestFrequency = 'DAILY' | 'MONTHLY' | 'NEVER' | 'WEEKLY';
export type DirectoryFiltersInput = {
    readonly active?: InputMaybe<Scalars['Boolean']['input']>;
    readonly local?: InputMaybe<Scalars['Boolean']['input']>;
    readonly order?: InputMaybe<DirectoryOrder>;
    readonly remote?: InputMaybe<Scalars['Boolean']['input']>;
};
export type DirectoryOrder = 'ACTIVE' | 'NEW';
export type DiscoveryPreferences = {
    readonly __typename: 'DiscoveryPreferences';
    readonly personalizedSearchEnabled: Scalars['Boolean']['output'];
    readonly searchSuggestionsEnabled: Scalars['Boolean']['output'];
    readonly showFollowCounts: Scalars['Boolean']['output'];
};
export type Driver = {
    readonly __typename: 'Driver';
    readonly cost: Scalars['Float']['output'];
    readonly domain?: Maybe<Scalars['String']['output']>;
    readonly percentOfTotal: Scalars['Float']['output'];
    readonly trend: Trend;
    readonly type: Scalars['String']['output'];
};
export type Entity = {
    readonly __typename: 'Entity';
    readonly score: Scalars['Float']['output'];
    readonly text: Scalars['String']['output'];
    readonly type: Scalars['String']['output'];
};
export type ExpandMediaPreference = 'DEFAULT' | 'HIDE_ALL' | 'SHOW_ALL';
export type FederationCost = {
    readonly __typename: 'FederationCost';
    readonly breakdown: CostBreakdown;
    readonly domain: Scalars['String']['output'];
    readonly egressBytes: Scalars['Int']['output'];
    readonly errorRate: Scalars['Float']['output'];
    readonly healthScore: Scalars['Float']['output'];
    readonly ingressBytes: Scalars['Int']['output'];
    readonly lastUpdated: Scalars['Time']['output'];
    readonly monthlyCostUSD: Scalars['Float']['output'];
    readonly recommendation?: Maybe<Scalars['String']['output']>;
    readonly requestCount: Scalars['Int']['output'];
};
export type FederationCostConnection = {
    readonly __typename: 'FederationCostConnection';
    readonly edges: ReadonlyArray<FederationCostEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type FederationCostEdge = {
    readonly __typename: 'FederationCostEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: FederationCost;
};
export type FederationEdge = {
    readonly __typename: 'FederationEdge';
    readonly bidirectional: Scalars['Boolean']['output'];
    readonly errorRate: Scalars['Float']['output'];
    readonly healthScore: Scalars['Float']['output'];
    readonly latency: Scalars['Float']['output'];
    readonly source: Scalars['String']['output'];
    readonly target: Scalars['String']['output'];
    readonly volumePerDay: Scalars['Int']['output'];
    readonly weight: Scalars['Float']['output'];
};
export type FederationFlow = {
    readonly __typename: 'FederationFlow';
    readonly costByInstance: ReadonlyArray<InstanceCost>;
    readonly topDestinations: ReadonlyArray<FlowNode>;
    readonly topSources: ReadonlyArray<FlowNode>;
    readonly volumeByHour: ReadonlyArray<HourlyVolume>;
};
export type FederationGraph = {
    readonly __typename: 'FederationGraph';
    readonly clusters: ReadonlyArray<InstanceCluster>;
    readonly edges: ReadonlyArray<FederationEdge>;
    readonly healthScore: Scalars['Float']['output'];
    readonly nodes: ReadonlyArray<InstanceNode>;
};
export type FederationHealthUpdate = {
    readonly __typename: 'FederationHealthUpdate';
    readonly currentStatus: InstanceHealthStatus;
    readonly domain: Scalars['String']['output'];
    readonly issues: ReadonlyArray<HealthIssue>;
    readonly previousStatus: InstanceHealthStatus;
    readonly timestamp: Scalars['Time']['output'];
};
export type FederationLimit = {
    readonly __typename: 'FederationLimit';
    readonly active: Scalars['Boolean']['output'];
    readonly createdAt: Scalars['Time']['output'];
    readonly domain: Scalars['String']['output'];
    readonly egressLimitMB: Scalars['Int']['output'];
    readonly ingressLimitMB: Scalars['Int']['output'];
    readonly monthlyBudgetUSD?: Maybe<Scalars['Float']['output']>;
    readonly requestsPerMinute: Scalars['Int']['output'];
    readonly updatedAt: Scalars['Time']['output'];
};
export type FederationLimitInput = {
    readonly egressLimitMB?: InputMaybe<Scalars['Int']['input']>;
    readonly ingressLimitMB?: InputMaybe<Scalars['Int']['input']>;
    readonly monthlyBudgetUSD?: InputMaybe<Scalars['Float']['input']>;
    readonly requestsPerMinute?: InputMaybe<Scalars['Int']['input']>;
};
export type FederationManagementStatus = {
    readonly __typename: 'FederationManagementStatus';
    readonly domain: Scalars['String']['output'];
    readonly limits?: Maybe<FederationLimit>;
    readonly metrics: FederationMetrics;
    readonly pausedUntil?: Maybe<Scalars['Time']['output']>;
    readonly reason?: Maybe<Scalars['String']['output']>;
    readonly status: FederationState;
};
export type FederationMetrics = {
    readonly __typename: 'FederationMetrics';
    readonly averageResponseTime: Scalars['Float']['output'];
    readonly currentMonthBandwidthMB: Scalars['Int']['output'];
    readonly currentMonthCostUSD: Scalars['Float']['output'];
    readonly currentMonthRequests: Scalars['Int']['output'];
    readonly errorRate: Scalars['Float']['output'];
};
export type FederationRecommendation = {
    readonly __typename: 'FederationRecommendation';
    readonly action: Scalars['String']['output'];
    readonly domain?: Maybe<Scalars['String']['output']>;
    readonly potentialImpact: Scalars['String']['output'];
    readonly priority: Priority;
    readonly reason: Scalars['String']['output'];
    readonly type: RecommendationType;
};
export type FederationState = 'ACTIVE' | 'BLOCKED' | 'ERROR' | 'LIMITED' | 'PAUSED';
export type FederationStatus = {
    readonly __typename: 'FederationStatus';
    readonly domain: Scalars['String']['output'];
    readonly lastContact?: Maybe<Scalars['Time']['output']>;
    readonly publicKey?: Maybe<Scalars['String']['output']>;
    readonly reachable: Scalars['Boolean']['output'];
    readonly sharedInbox?: Maybe<Scalars['String']['output']>;
    readonly software?: Maybe<Scalars['String']['output']>;
    readonly version?: Maybe<Scalars['String']['output']>;
};
export type Field = {
    readonly __typename: 'Field';
    readonly name: Scalars['String']['output'];
    readonly value: Scalars['String']['output'];
    readonly verifiedAt?: Maybe<Scalars['Time']['output']>;
};
export type FlagInput = {
    readonly evidence?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    readonly objectId: Scalars['ID']['input'];
    readonly reason: Scalars['String']['input'];
};
export type FlagPayload = {
    readonly __typename: 'FlagPayload';
    readonly moderationId: Scalars['ID']['output'];
    readonly queued: Scalars['Boolean']['output'];
};
export type FlowNode = {
    readonly __typename: 'FlowNode';
    readonly avgMessageSize: Scalars['Int']['output'];
    readonly domain: Scalars['String']['output'];
    readonly percentage: Scalars['Float']['output'];
    readonly trend: Trend;
    readonly volume: Scalars['Int']['output'];
};
export type FocusInput = {
    readonly x: Scalars['Float']['input'];
    readonly y: Scalars['Float']['input'];
};
export type Hashtag = {
    readonly __typename: 'Hashtag';
    readonly analytics: HashtagAnalytics;
    readonly displayName: Scalars['String']['output'];
    readonly followedAt?: Maybe<Scalars['Time']['output']>;
    readonly followerCount: Scalars['Int']['output'];
    readonly isFollowing: Scalars['Boolean']['output'];
    readonly name: Scalars['String']['output'];
    readonly notificationSettings?: Maybe<HashtagNotificationSettings>;
    readonly postCount: Scalars['Int']['output'];
    readonly posts: PostConnection;
    readonly relatedHashtags: ReadonlyArray<Hashtag>;
    readonly trendingScore: Scalars['Float']['output'];
    readonly url: Scalars['String']['output'];
};
export type HashtagPostsArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type HashtagActivityUpdate = {
    readonly __typename: 'HashtagActivityUpdate';
    readonly author: Actor;
    readonly hashtag: Scalars['String']['output'];
    readonly post: Object;
    readonly timestamp: Scalars['Time']['output'];
};
export type HashtagAnalytics = {
    readonly __typename: 'HashtagAnalytics';
    readonly dailyPosts: ReadonlyArray<Scalars['Int']['output']>;
    readonly engagement: Scalars['Float']['output'];
    readonly hourlyPosts: ReadonlyArray<Scalars['Int']['output']>;
    readonly sentiment: Scalars['Float']['output'];
    readonly topPosters: ReadonlyArray<Actor>;
};
export type HashtagConnection = {
    readonly __typename: 'HashtagConnection';
    readonly edges: ReadonlyArray<HashtagEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type HashtagEdge = {
    readonly __typename: 'HashtagEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Hashtag;
};
export type HashtagFollowPayload = {
    readonly __typename: 'HashtagFollowPayload';
    readonly hashtag: Hashtag;
    readonly success: Scalars['Boolean']['output'];
};
export type HashtagMode = 'ALL' | 'ANY';
export type HashtagNotificationSettings = {
    readonly __typename: 'HashtagNotificationSettings';
    readonly filters: ReadonlyArray<NotificationFilter>;
    readonly level: NotificationLevel;
    readonly muted: Scalars['Boolean']['output'];
    readonly mutedUntil?: Maybe<Scalars['Time']['output']>;
};
export type HashtagNotificationSettingsInput = {
    readonly filters?: InputMaybe<ReadonlyArray<NotificationFilterInput>>;
    readonly level: NotificationLevel;
    readonly muted?: InputMaybe<Scalars['Boolean']['input']>;
    readonly mutedUntil?: InputMaybe<Scalars['Time']['input']>;
};
export type HashtagSuggestion = {
    readonly __typename: 'HashtagSuggestion';
    readonly hashtag: Hashtag;
    readonly reason: Scalars['String']['output'];
    readonly score: Scalars['Float']['output'];
};
export type HealthIssue = {
    readonly __typename: 'HealthIssue';
    readonly description: Scalars['String']['output'];
    readonly detectedAt: Scalars['Time']['output'];
    readonly impact: Scalars['String']['output'];
    readonly severity: IssueSeverity;
    readonly type: Scalars['String']['output'];
};
export type HealthStatus = 'DEGRADED' | 'DOWN' | 'HEALTHY' | 'UNKNOWN';
export type HourlyBandwidth = {
    readonly __typename: 'HourlyBandwidth';
    readonly hour: Scalars['Time']['output'];
    readonly peakMbps: Scalars['Float']['output'];
    readonly totalGB: Scalars['Float']['output'];
};
export type HourlyVolume = {
    readonly __typename: 'HourlyVolume';
    readonly avgLatency: Scalars['Float']['output'];
    readonly errors: Scalars['Int']['output'];
    readonly hour: Scalars['Time']['output'];
    readonly inbound: Scalars['Int']['output'];
    readonly outbound: Scalars['Int']['output'];
};
export type ImageAnalysis = {
    readonly __typename: 'ImageAnalysis';
    readonly celebrityFaces: ReadonlyArray<Celebrity>;
    readonly deepfakeScore: Scalars['Float']['output'];
    readonly detectedText: ReadonlyArray<Scalars['String']['output']>;
    readonly isNSFW: Scalars['Boolean']['output'];
    readonly moderationLabels: ReadonlyArray<ModerationLabel>;
    readonly nsfwConfidence: Scalars['Float']['output'];
    readonly textToxicity: Scalars['Float']['output'];
    readonly violenceScore: Scalars['Float']['output'];
    readonly weaponsDetected: Scalars['Boolean']['output'];
};
export type ImageAnalysisCapabilities = {
    readonly __typename: 'ImageAnalysisCapabilities';
    readonly celebrityRecognition: Scalars['Boolean']['output'];
    readonly deepfakeDetection: Scalars['Boolean']['output'];
    readonly nsfwDetection: Scalars['Boolean']['output'];
    readonly textExtraction: Scalars['Boolean']['output'];
    readonly violenceDetection: Scalars['Boolean']['output'];
};
export type InfrastructureAlert = {
    readonly __typename: 'InfrastructureAlert';
    readonly id: Scalars['ID']['output'];
    readonly message: Scalars['String']['output'];
    readonly resolved: Scalars['Boolean']['output'];
    readonly service: Scalars['String']['output'];
    readonly severity: AlertSeverity;
    readonly timestamp: Scalars['Time']['output'];
};
export type InfrastructureEvent = {
    readonly __typename: 'InfrastructureEvent';
    readonly description: Scalars['String']['output'];
    readonly id: Scalars['ID']['output'];
    readonly impact: Scalars['String']['output'];
    readonly service: Scalars['String']['output'];
    readonly timestamp: Scalars['Time']['output'];
    readonly type: InfrastructureEventType;
};
export type InfrastructureEventType = 'DEPLOYMENT' | 'FAILURE' | 'MAINTENANCE' | 'RECOVERY' | 'SCALING';
export type InfrastructureStatus = {
    readonly __typename: 'InfrastructureStatus';
    readonly alerts: ReadonlyArray<InfrastructureAlert>;
    readonly databases: ReadonlyArray<DatabaseStatus>;
    readonly healthy: Scalars['Boolean']['output'];
    readonly queues: ReadonlyArray<QueueStatus>;
    readonly services: ReadonlyArray<ServiceStatus>;
};
export type InstanceBudget = {
    readonly __typename: 'InstanceBudget';
    readonly alertThreshold: Scalars['Float']['output'];
    readonly autoLimit: Scalars['Boolean']['output'];
    readonly currentSpendUSD: Scalars['Float']['output'];
    readonly domain: Scalars['String']['output'];
    readonly monthlyBudgetUSD: Scalars['Float']['output'];
    readonly period: Scalars['String']['output'];
    readonly projectedOverspend?: Maybe<Scalars['Float']['output']>;
    readonly remainingBudgetUSD: Scalars['Float']['output'];
};
export type InstanceCluster = {
    readonly __typename: 'InstanceCluster';
    readonly avgHealthScore: Scalars['Float']['output'];
    readonly commonality: Scalars['String']['output'];
    readonly description: Scalars['String']['output'];
    readonly id: Scalars['ID']['output'];
    readonly members: ReadonlyArray<Scalars['String']['output']>;
    readonly name: Scalars['String']['output'];
    readonly totalVolume: Scalars['Int']['output'];
};
export type InstanceConnection = {
    readonly __typename: 'InstanceConnection';
    readonly connectionType: ConnectionType;
    readonly domain: Scalars['String']['output'];
    readonly lastActivity: Scalars['Time']['output'];
    readonly sharedUsers: Scalars['Int']['output'];
    readonly strength: Scalars['Float']['output'];
    readonly volumeIn: Scalars['Int']['output'];
    readonly volumeOut: Scalars['Int']['output'];
};
export type InstanceCost = {
    readonly __typename: 'InstanceCost';
    readonly breakdown: CostBreakdown;
    readonly costUSD: Scalars['Float']['output'];
    readonly domain: Scalars['String']['output'];
    readonly percentage: Scalars['Float']['output'];
};
export type InstanceHealthMetrics = {
    readonly __typename: 'InstanceHealthMetrics';
    readonly costEfficiency: Scalars['Float']['output'];
    readonly errorRate: Scalars['Float']['output'];
    readonly federationDelay: Scalars['Float']['output'];
    readonly queueDepth: Scalars['Int']['output'];
    readonly responseTime: Scalars['Float']['output'];
};
export type InstanceHealthReport = {
    readonly __typename: 'InstanceHealthReport';
    readonly domain: Scalars['String']['output'];
    readonly issues: ReadonlyArray<HealthIssue>;
    readonly lastChecked: Scalars['Time']['output'];
    readonly metrics: InstanceHealthMetrics;
    readonly recommendations: ReadonlyArray<Scalars['String']['output']>;
    readonly status: InstanceHealthStatus;
};
export type InstanceHealthStatus = 'CRITICAL' | 'HEALTHY' | 'OFFLINE' | 'UNKNOWN' | 'WARNING';
export type InstanceMetadata = {
    readonly __typename: 'InstanceMetadata';
    readonly approvalRequired: Scalars['Boolean']['output'];
    readonly description?: Maybe<Scalars['String']['output']>;
    readonly firstSeen: Scalars['Time']['output'];
    readonly lastActivity: Scalars['Time']['output'];
    readonly monthlyActiveUsers: Scalars['Int']['output'];
    readonly primaryLanguage: Scalars['String']['output'];
    readonly registrationsOpen: Scalars['Boolean']['output'];
};
export type InstanceMetrics = {
    readonly __typename: 'InstanceMetrics';
    readonly activeUsers: Scalars['Int']['output'];
    readonly averageLatencyMs: Scalars['Float']['output'];
    readonly estimatedMonthlyCost: Scalars['Float']['output'];
    readonly lastUpdated: Scalars['Time']['output'];
    readonly requestsPerMinute: Scalars['Int']['output'];
    readonly storageUsedGB: Scalars['Float']['output'];
};
export type InstanceNode = {
    readonly __typename: 'InstanceNode';
    readonly coordinates: Coordinates;
    readonly displayName: Scalars['String']['output'];
    readonly domain: Scalars['String']['output'];
    readonly federatingWith: Scalars['Int']['output'];
    readonly healthStatus: InstanceHealthStatus;
    readonly metadata: InstanceMetadata;
    readonly software: Scalars['String']['output'];
    readonly statusCount: Scalars['Int']['output'];
    readonly userCount: Scalars['Int']['output'];
    readonly version: Scalars['String']['output'];
};
export type InstanceRelations = {
    readonly __typename: 'InstanceRelations';
    readonly blockedBy: ReadonlyArray<Scalars['String']['output']>;
    readonly blocking: ReadonlyArray<Scalars['String']['output']>;
    readonly directConnections: ReadonlyArray<InstanceConnection>;
    readonly domain: Scalars['String']['output'];
    readonly federationScore: Scalars['Float']['output'];
    readonly indirectConnections: ReadonlyArray<InstanceConnection>;
    readonly recommendations: ReadonlyArray<FederationRecommendation>;
};
export type IssueSeverity = 'CRITICAL' | 'HIGH' | 'LOW' | 'MEDIUM';
export type List = {
    readonly __typename: 'List';
    readonly accountCount: Scalars['Int']['output'];
    readonly accounts: ReadonlyArray<Actor>;
    readonly createdAt: Scalars['Time']['output'];
    readonly exclusive: Scalars['Boolean']['output'];
    readonly id: Scalars['ID']['output'];
    readonly repliesPolicy: RepliesPolicy;
    readonly title: Scalars['String']['output'];
    readonly updatedAt: Scalars['Time']['output'];
};
export type ListUpdate = {
    readonly __typename: 'ListUpdate';
    readonly account?: Maybe<Actor>;
    readonly list: List;
    readonly timestamp: Scalars['Time']['output'];
    readonly type: Scalars['String']['output'];
};
export type Media = {
    readonly __typename: 'Media';
    readonly blurhash?: Maybe<Scalars['String']['output']>;
    readonly createdAt: Scalars['Time']['output'];
    readonly description?: Maybe<Scalars['String']['output']>;
    readonly duration?: Maybe<Scalars['Float']['output']>;
    readonly height?: Maybe<Scalars['Int']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly mediaCategory: MediaCategory;
    readonly mimeType: Scalars['String']['output'];
    readonly previewUrl?: Maybe<Scalars['String']['output']>;
    readonly sensitive: Scalars['Boolean']['output'];
    readonly size: Scalars['Int']['output'];
    readonly spoilerText?: Maybe<Scalars['String']['output']>;
    readonly type: MediaType;
    readonly uploadedBy: Actor;
    readonly url: Scalars['String']['output'];
    readonly width?: Maybe<Scalars['Int']['output']>;
};
export type MediaCategory = 'AUDIO' | 'DOCUMENT' | 'GIFV' | 'IMAGE' | 'UNKNOWN' | 'VIDEO';
export type MediaConnection = {
    readonly __typename: 'MediaConnection';
    readonly edges: ReadonlyArray<MediaEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type MediaEdge = {
    readonly __typename: 'MediaEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Media;
};
export type MediaFilterInput = {
    readonly mediaType?: InputMaybe<MediaType>;
    readonly mimeType?: InputMaybe<Scalars['String']['input']>;
    readonly ownerId?: InputMaybe<Scalars['ID']['input']>;
    readonly ownerUsername?: InputMaybe<Scalars['String']['input']>;
    readonly since?: InputMaybe<Scalars['Time']['input']>;
    readonly until?: InputMaybe<Scalars['Time']['input']>;
};
export type MediaStream = {
    readonly __typename: 'MediaStream';
    readonly bitrates: ReadonlyArray<Bitrate>;
    readonly dashManifestUrl?: Maybe<Scalars['String']['output']>;
    readonly duration: Scalars['Int']['output'];
    readonly expiresAt: Scalars['Time']['output'];
    readonly hlsPlaylistUrl?: Maybe<Scalars['String']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly thumbnailUrl: Scalars['String']['output'];
    readonly url: Scalars['String']['output'];
};
export type MediaType = 'AUDIO' | 'IMAGE' | 'UNKNOWN' | 'VIDEO';
export type Mention = {
    readonly __typename: 'Mention';
    readonly domain?: Maybe<Scalars['String']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly url: Scalars['String']['output'];
    readonly username: Scalars['String']['output'];
};
export type MetricsDimension = {
    readonly __typename: 'MetricsDimension';
    readonly key: Scalars['String']['output'];
    readonly value: Scalars['String']['output'];
};
export type MetricsUpdate = {
    readonly __typename: 'MetricsUpdate';
    readonly aggregationLevel: Scalars['String']['output'];
    readonly average: Scalars['Float']['output'];
    readonly count: Scalars['Int']['output'];
    readonly dimensions: ReadonlyArray<MetricsDimension>;
    readonly instanceDomain?: Maybe<Scalars['String']['output']>;
    readonly max: Scalars['Float']['output'];
    readonly metricId: Scalars['String']['output'];
    readonly metricType: Scalars['String']['output'];
    readonly min: Scalars['Float']['output'];
    readonly p50?: Maybe<Scalars['Float']['output']>;
    readonly p95?: Maybe<Scalars['Float']['output']>;
    readonly p99?: Maybe<Scalars['Float']['output']>;
    readonly serviceName: Scalars['String']['output'];
    readonly subscriptionCategory: Scalars['String']['output'];
    readonly sum: Scalars['Float']['output'];
    readonly tenantId?: Maybe<Scalars['String']['output']>;
    readonly timestamp: Scalars['Time']['output'];
    readonly totalCostMicrocents?: Maybe<Scalars['Int']['output']>;
    readonly unit?: Maybe<Scalars['String']['output']>;
    readonly userCostMicrocents?: Maybe<Scalars['Int']['output']>;
    readonly userId?: Maybe<Scalars['String']['output']>;
};
export type ModerationAction = 'FLAG' | 'HIDE' | 'NONE' | 'REMOVE' | 'REVIEW' | 'SHADOW_BAN';
export type ModerationActionCounts = {
    readonly __typename: 'ModerationActionCounts';
    readonly flag: Scalars['Int']['output'];
    readonly hide: Scalars['Int']['output'];
    readonly none: Scalars['Int']['output'];
    readonly remove: Scalars['Int']['output'];
    readonly review: Scalars['Int']['output'];
    readonly shadowBan: Scalars['Int']['output'];
};
export type ModerationAlert = {
    readonly __typename: 'ModerationAlert';
    readonly confidence: Scalars['Float']['output'];
    readonly content: Object;
    readonly handled: Scalars['Boolean']['output'];
    readonly id: Scalars['ID']['output'];
    readonly matchedText: Scalars['String']['output'];
    readonly pattern?: Maybe<ModerationPattern>;
    readonly severity: ModerationSeverity;
    readonly suggestedAction: ModerationAction;
    readonly timestamp: Scalars['Time']['output'];
};
export type ModerationDashboard = {
    readonly __typename: 'ModerationDashboard';
    readonly averageResponseTime: Scalars['Duration']['output'];
    readonly falsePositiveRate: Scalars['Float']['output'];
    readonly pendingReviews: Scalars['Int']['output'];
    readonly recentDecisions: ReadonlyArray<ModerationDecision>;
    readonly threatTrends: ReadonlyArray<ThreatTrend>;
    readonly topPatterns: ReadonlyArray<PatternStats>;
};
export type ModerationDecision = {
    readonly __typename: 'ModerationDecision';
    readonly confidence: Scalars['Float']['output'];
    readonly decision: Scalars['String']['output'];
    readonly evidence: ReadonlyArray<Scalars['String']['output']>;
    readonly id: Scalars['ID']['output'];
    readonly object: Object;
    readonly reviewers: ReadonlyArray<Actor>;
    readonly timestamp: Scalars['Time']['output'];
};
export type ModerationEffectiveness = {
    readonly __typename: 'ModerationEffectiveness';
    readonly f1Score: Scalars['Float']['output'];
    readonly falsePositives: Scalars['Int']['output'];
    readonly matchCount: Scalars['Int']['output'];
    readonly missedCount: Scalars['Int']['output'];
    readonly patternId: Scalars['ID']['output'];
    readonly precision: Scalars['Float']['output'];
    readonly recall: Scalars['Float']['output'];
    readonly truePositives: Scalars['Int']['output'];
};
export type ModerationFilter = {
    readonly assignedTo?: InputMaybe<Scalars['ID']['input']>;
    readonly priority?: InputMaybe<Priority>;
    readonly severity?: InputMaybe<ModerationSeverity>;
    readonly unhandled?: InputMaybe<Scalars['Boolean']['input']>;
};
export type ModerationItem = {
    readonly __typename: 'ModerationItem';
    readonly assignedTo?: Maybe<Actor>;
    readonly content: Object;
    readonly deadline: Scalars['Time']['output'];
    readonly id: Scalars['ID']['output'];
    readonly priority: Priority;
    readonly reportCount: Scalars['Int']['output'];
    readonly severity: ModerationSeverity;
};
export type ModerationLabel = {
    readonly __typename: 'ModerationLabel';
    readonly confidence: Scalars['Float']['output'];
    readonly name: Scalars['String']['output'];
    readonly parentName?: Maybe<Scalars['String']['output']>;
};
export type ModerationPattern = {
    readonly __typename: 'ModerationPattern';
    readonly active: Scalars['Boolean']['output'];
    readonly createdAt: Scalars['Time']['output'];
    readonly createdBy: Actor;
    readonly falsePositiveRate: Scalars['Float']['output'];
    readonly id: Scalars['ID']['output'];
    readonly matchCount: Scalars['Int']['output'];
    readonly pattern: Scalars['String']['output'];
    readonly severity: ModerationSeverity;
    readonly type: PatternType;
    readonly updatedAt: Scalars['Time']['output'];
};
export type ModerationPatternInput = {
    readonly active?: InputMaybe<Scalars['Boolean']['input']>;
    readonly pattern: Scalars['String']['input'];
    readonly severity: ModerationSeverity;
    readonly type: PatternType;
};
export type ModerationPeriod = 'DAILY' | 'HOURLY' | 'MONTHLY' | 'WEEKLY';
export type ModerationSampleInput = {
    readonly confidence: Scalars['Float']['input'];
    readonly label: Scalars['String']['input'];
    readonly objectId: Scalars['ID']['input'];
    readonly objectType: Scalars['String']['input'];
};
export type ModerationSeverity = 'CRITICAL' | 'HIGH' | 'INFO' | 'LOW' | 'MEDIUM';
export type ModeratorStats = {
    readonly __typename: 'ModeratorStats';
    readonly accuracy: Scalars['Float']['output'];
    readonly avgResponseTime: Scalars['Duration']['output'];
    readonly categories: ReadonlyArray<CategoryStats>;
    readonly decisionsCount: Scalars['Int']['output'];
    readonly moderatorId: Scalars['ID']['output'];
    readonly overturned: Scalars['Int']['output'];
    readonly period: TimePeriod;
};
export type Mutation = {
    readonly __typename: 'Mutation';
    readonly acknowledgeSeverance: AcknowledgePayload;
    readonly addAccountsToList: List;
    readonly addCommunityNote: CommunityNotePayload;
    readonly attemptReconnection: ReconnectionPayload;
    readonly blockActor: Relationship;
    readonly bookmarkObject: Object;
    readonly cancelScheduledStatus: Scalars['Boolean']['output'];
    readonly clearNotifications: Scalars['Boolean']['output'];
    readonly createEmoji: CustomEmoji;
    readonly createList: List;
    readonly createModerationPattern: ModerationPattern;
    readonly createNote: CreateNotePayload;
    readonly createQuoteNote: CreateNotePayload;
    readonly deleteConversation: Scalars['Boolean']['output'];
    readonly deleteEmoji: Scalars['Boolean']['output'];
    readonly deleteList: Scalars['Boolean']['output'];
    readonly deleteModerationPattern: Scalars['Boolean']['output'];
    readonly deleteObject: Scalars['Boolean']['output'];
    readonly deletePushSubscription: Scalars['Boolean']['output'];
    readonly dismissNotification: Scalars['Boolean']['output'];
    readonly flagObject: FlagPayload;
    readonly followActor: Activity;
    readonly followHashtag: HashtagFollowPayload;
    readonly likeObject: Activity;
    readonly markConversationAsRead: Conversation;
    readonly muteActor: Relationship;
    readonly muteHashtag: MuteHashtagPayload;
    readonly optimizeFederationCosts: CostOptimizationResult;
    readonly pauseFederation: FederationManagementStatus;
    readonly pinObject: Object;
    readonly preloadMedia: ReadonlyArray<MediaStream>;
    readonly registerPushSubscription: PushSubscription;
    readonly removeAccountsFromList: List;
    readonly reportStreamingQuality: StreamingQualityReport;
    readonly requestAIAnalysis: AiAnalysisRequest;
    readonly requestStreamingUrl: MediaStream;
    readonly resumeFederation: FederationManagementStatus;
    readonly scheduleStatus: ScheduledStatus;
    readonly setFederationLimit: FederationLimit;
    readonly setInstanceBudget: InstanceBudget;
    readonly shareObject: Object;
    readonly syncMissingReplies: SyncRepliesPayload;
    readonly syncThread: SyncThreadPayload;
    readonly trainModerationModel: TrainingResult;
    readonly unblockActor: Scalars['Boolean']['output'];
    readonly unbookmarkObject: Scalars['Boolean']['output'];
    readonly unfollowActor: Scalars['Boolean']['output'];
    readonly unfollowHashtag: UnfollowHashtagPayload;
    readonly unlikeObject: Scalars['Boolean']['output'];
    readonly unmuteActor: Scalars['Boolean']['output'];
    readonly unpinObject: Scalars['Boolean']['output'];
    readonly unshareObject: Scalars['Boolean']['output'];
    readonly updateEmoji: CustomEmoji;
    readonly updateHashtagNotifications: UpdateHashtagNotificationsPayload;
    readonly updateList: List;
    readonly updateMedia: Media;
    readonly updateModerationPattern: ModerationPattern;
    readonly updateProfile: Actor;
    readonly updatePushSubscription: PushSubscription;
    readonly updateQuotePermissions: UpdateQuotePermissionsPayload;
    readonly updateRelationship: Relationship;
    readonly updateScheduledStatus: ScheduledStatus;
    readonly updateStreamingPreferences: UserPreferences;
    readonly updateTrust: TrustEdge;
    readonly updateUserPreferences: UserPreferences;
    readonly uploadMedia: UploadMediaPayload;
    readonly voteCommunityNote: CommunityNote;
    readonly withdrawFromQuotes: WithdrawQuotePayload;
};
export type MutationAcknowledgeSeveranceArgs = {
    id: Scalars['ID']['input'];
};
export type MutationAddAccountsToListArgs = {
    accountIds: ReadonlyArray<Scalars['ID']['input']>;
    id: Scalars['ID']['input'];
};
export type MutationAddCommunityNoteArgs = {
    input: CommunityNoteInput;
};
export type MutationAttemptReconnectionArgs = {
    id: Scalars['ID']['input'];
};
export type MutationBlockActorArgs = {
    id: Scalars['ID']['input'];
};
export type MutationBookmarkObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationCancelScheduledStatusArgs = {
    id: Scalars['ID']['input'];
};
export type MutationCreateEmojiArgs = {
    input: CreateEmojiInput;
};
export type MutationCreateListArgs = {
    input: CreateListInput;
};
export type MutationCreateModerationPatternArgs = {
    input: ModerationPatternInput;
};
export type MutationCreateNoteArgs = {
    input: CreateNoteInput;
};
export type MutationCreateQuoteNoteArgs = {
    input: CreateQuoteNoteInput;
};
export type MutationDeleteConversationArgs = {
    id: Scalars['ID']['input'];
};
export type MutationDeleteEmojiArgs = {
    shortcode: Scalars['String']['input'];
};
export type MutationDeleteListArgs = {
    id: Scalars['ID']['input'];
};
export type MutationDeleteModerationPatternArgs = {
    id: Scalars['ID']['input'];
};
export type MutationDeleteObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationDismissNotificationArgs = {
    id: Scalars['ID']['input'];
};
export type MutationFlagObjectArgs = {
    input: FlagInput;
};
export type MutationFollowActorArgs = {
    id: Scalars['ID']['input'];
};
export type MutationFollowHashtagArgs = {
    hashtag: Scalars['String']['input'];
    notifyLevel?: InputMaybe<NotificationLevel>;
};
export type MutationLikeObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationMarkConversationAsReadArgs = {
    id: Scalars['ID']['input'];
};
export type MutationMuteActorArgs = {
    id: Scalars['ID']['input'];
    notifications?: InputMaybe<Scalars['Boolean']['input']>;
};
export type MutationMuteHashtagArgs = {
    hashtag: Scalars['String']['input'];
    until?: InputMaybe<Scalars['Time']['input']>;
};
export type MutationOptimizeFederationCostsArgs = {
    threshold: Scalars['Float']['input'];
};
export type MutationPauseFederationArgs = {
    domain: Scalars['String']['input'];
    reason: Scalars['String']['input'];
    until?: InputMaybe<Scalars['Time']['input']>;
};
export type MutationPinObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationPreloadMediaArgs = {
    mediaIds: ReadonlyArray<Scalars['ID']['input']>;
};
export type MutationRegisterPushSubscriptionArgs = {
    input: RegisterPushSubscriptionInput;
};
export type MutationRemoveAccountsFromListArgs = {
    accountIds: ReadonlyArray<Scalars['ID']['input']>;
    id: Scalars['ID']['input'];
};
export type MutationReportStreamingQualityArgs = {
    input: StreamingQualityInput;
};
export type MutationRequestAiAnalysisArgs = {
    force?: InputMaybe<Scalars['Boolean']['input']>;
    objectId: Scalars['ID']['input'];
    objectType?: InputMaybe<Scalars['String']['input']>;
};
export type MutationRequestStreamingUrlArgs = {
    mediaId: Scalars['ID']['input'];
    quality?: InputMaybe<StreamQuality>;
};
export type MutationResumeFederationArgs = {
    domain: Scalars['String']['input'];
};
export type MutationScheduleStatusArgs = {
    input: ScheduleStatusInput;
};
export type MutationSetFederationLimitArgs = {
    domain: Scalars['String']['input'];
    limit: FederationLimitInput;
};
export type MutationSetInstanceBudgetArgs = {
    autoLimit?: InputMaybe<Scalars['Boolean']['input']>;
    domain: Scalars['String']['input'];
    monthlyUSD: Scalars['Float']['input'];
};
export type MutationShareObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationSyncMissingRepliesArgs = {
    noteId: Scalars['ID']['input'];
};
export type MutationSyncThreadArgs = {
    depth?: InputMaybe<Scalars['Int']['input']>;
    noteUrl: Scalars['String']['input'];
};
export type MutationTrainModerationModelArgs = {
    options?: InputMaybe<BedrockTrainingOptions>;
    samples: ReadonlyArray<ModerationSampleInput>;
};
export type MutationUnblockActorArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUnbookmarkObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUnfollowActorArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUnfollowHashtagArgs = {
    hashtag: Scalars['String']['input'];
};
export type MutationUnlikeObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUnmuteActorArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUnpinObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUnshareObjectArgs = {
    id: Scalars['ID']['input'];
};
export type MutationUpdateEmojiArgs = {
    input: UpdateEmojiInput;
    shortcode: Scalars['String']['input'];
};
export type MutationUpdateHashtagNotificationsArgs = {
    hashtag: Scalars['String']['input'];
    settings: HashtagNotificationSettingsInput;
};
export type MutationUpdateListArgs = {
    id: Scalars['ID']['input'];
    input: UpdateListInput;
};
export type MutationUpdateMediaArgs = {
    id: Scalars['ID']['input'];
    input: UpdateMediaInput;
};
export type MutationUpdateModerationPatternArgs = {
    id: Scalars['ID']['input'];
    input: ModerationPatternInput;
};
export type MutationUpdateProfileArgs = {
    input: UpdateProfileInput;
};
export type MutationUpdatePushSubscriptionArgs = {
    input: UpdatePushSubscriptionInput;
};
export type MutationUpdateQuotePermissionsArgs = {
    noteId: Scalars['ID']['input'];
    permission: QuotePermission;
    quoteable: Scalars['Boolean']['input'];
};
export type MutationUpdateRelationshipArgs = {
    id: Scalars['ID']['input'];
    input: UpdateRelationshipInput;
};
export type MutationUpdateScheduledStatusArgs = {
    id: Scalars['ID']['input'];
    input: UpdateScheduledStatusInput;
};
export type MutationUpdateStreamingPreferencesArgs = {
    input: StreamingPreferencesInput;
};
export type MutationUpdateTrustArgs = {
    input: TrustInput;
};
export type MutationUpdateUserPreferencesArgs = {
    input: UpdateUserPreferencesInput;
};
export type MutationUploadMediaArgs = {
    input: UploadMediaInput;
};
export type MutationVoteCommunityNoteArgs = {
    helpful: Scalars['Boolean']['input'];
    id: Scalars['ID']['input'];
};
export type MutationWithdrawFromQuotesArgs = {
    noteId: Scalars['ID']['input'];
};
export type MuteHashtagPayload = {
    readonly __typename: 'MuteHashtagPayload';
    readonly hashtag: Hashtag;
    readonly mutedUntil?: Maybe<Scalars['Time']['output']>;
    readonly success: Scalars['Boolean']['output'];
};
export type Notification = {
    readonly __typename: 'Notification';
    readonly account: Actor;
    readonly createdAt: Scalars['Time']['output'];
    readonly id: Scalars['ID']['output'];
    readonly read: Scalars['Boolean']['output'];
    readonly status?: Maybe<Object>;
    readonly type: Scalars['String']['output'];
};
export type NotificationConnection = {
    readonly __typename: 'NotificationConnection';
    readonly edges: ReadonlyArray<NotificationEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type NotificationEdge = {
    readonly __typename: 'NotificationEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Notification;
};
export type NotificationFilter = {
    readonly __typename: 'NotificationFilter';
    readonly type: Scalars['String']['output'];
    readonly value: Scalars['String']['output'];
};
export type NotificationFilterInput = {
    readonly type: Scalars['String']['input'];
    readonly value: Scalars['String']['input'];
};
export type NotificationLevel = 'ALL' | 'FOLLOWING' | 'MUTUALS' | 'NONE';
export type NotificationPreferences = {
    readonly __typename: 'NotificationPreferences';
    readonly digest: DigestFrequency;
    readonly email: Scalars['Boolean']['output'];
    readonly inApp: Scalars['Boolean']['output'];
    readonly push: Scalars['Boolean']['output'];
};
export type Object = {
    readonly __typename: 'Object';
    readonly actor: Actor;
    readonly attachments: ReadonlyArray<Attachment>;
    readonly boostedObject?: Maybe<Object>;
    readonly communityNotes: ReadonlyArray<CommunityNote>;
    readonly content: Scalars['String']['output'];
    readonly contentMap: ReadonlyArray<ContentMap>;
    readonly createdAt: Scalars['Time']['output'];
    readonly estimatedCost: Scalars['Int']['output'];
    readonly id: Scalars['ID']['output'];
    readonly inReplyTo?: Maybe<Object>;
    readonly likesCount: Scalars['Int']['output'];
    readonly mentions: ReadonlyArray<Mention>;
    readonly moderationScore?: Maybe<Scalars['Float']['output']>;
    readonly poll?: Maybe<Poll>;
    readonly quoteContext?: Maybe<QuoteContext>;
    readonly quoteCount: Scalars['Int']['output'];
    readonly quotePermissions: QuotePermission;
    readonly quoteUrl?: Maybe<Scalars['String']['output']>;
    readonly quoteable: Scalars['Boolean']['output'];
    readonly quotes: QuoteConnection;
    readonly repliesCount: Scalars['Int']['output'];
    readonly sensitive: Scalars['Boolean']['output'];
    readonly sharesCount: Scalars['Int']['output'];
    readonly spoilerText?: Maybe<Scalars['String']['output']>;
    readonly tags: ReadonlyArray<Tag>;
    readonly type: ObjectType;
    readonly updatedAt: Scalars['Time']['output'];
    readonly visibility: Visibility;
};
export type ObjectQuotesArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type ObjectConnection = {
    readonly __typename: 'ObjectConnection';
    readonly edges: ReadonlyArray<ObjectEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type ObjectEdge = {
    readonly __typename: 'ObjectEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Object;
};
export type ObjectExplanation = {
    readonly __typename: 'ObjectExplanation';
    readonly accessPattern: ReadonlyArray<AccessLog>;
    readonly object: Object;
    readonly sizeBytes: Scalars['Int']['output'];
    readonly storageCost: Scalars['Float']['output'];
    readonly storageLocation: Scalars['String']['output'];
};
export type ObjectType = 'ARTICLE' | 'EVENT' | 'IMAGE' | 'NOTE' | 'PAGE' | 'QUESTION' | 'VIDEO';
export type OptimizationAction = {
    readonly __typename: 'OptimizationAction';
    readonly action: Scalars['String']['output'];
    readonly domain: Scalars['String']['output'];
    readonly impact: Scalars['String']['output'];
    readonly savingsUSD: Scalars['Float']['output'];
};
export type PageInfo = {
    readonly __typename: 'PageInfo';
    readonly endCursor?: Maybe<Scalars['Cursor']['output']>;
    readonly hasNextPage: Scalars['Boolean']['output'];
    readonly hasPreviousPage: Scalars['Boolean']['output'];
    readonly startCursor?: Maybe<Scalars['Cursor']['output']>;
};
export type PatternStats = {
    readonly __typename: 'PatternStats';
    readonly accuracy: Scalars['Float']['output'];
    readonly lastMatch: Scalars['Time']['output'];
    readonly matchCount: Scalars['Int']['output'];
    readonly pattern: ModerationPattern;
    readonly trend: Trend;
};
export type PatternType = 'KEYWORD' | 'ML_PATTERN' | 'PHRASE' | 'REGEX';
export type PerformanceAlert = {
    readonly __typename: 'PerformanceAlert';
    readonly actualValue: Scalars['Float']['output'];
    readonly id: Scalars['ID']['output'];
    readonly metric: Scalars['String']['output'];
    readonly service: ServiceCategory;
    readonly severity: AlertSeverity;
    readonly threshold: Scalars['Float']['output'];
    readonly timestamp: Scalars['Time']['output'];
};
export type PerformanceReport = {
    readonly __typename: 'PerformanceReport';
    readonly coldStarts: Scalars['Int']['output'];
    readonly errorRate: Scalars['Float']['output'];
    readonly p50Latency: Scalars['Duration']['output'];
    readonly p95Latency: Scalars['Duration']['output'];
    readonly p99Latency: Scalars['Duration']['output'];
    readonly period: TimePeriod;
    readonly service: ServiceCategory;
    readonly throughput: Scalars['Float']['output'];
};
export type Period = 'DAY' | 'HOUR' | 'MONTH' | 'WEEK' | 'YEAR';
export type Poll = {
    readonly __typename: 'Poll';
    readonly expired: Scalars['Boolean']['output'];
    readonly expiresAt?: Maybe<Scalars['Time']['output']>;
    readonly hideTotals: Scalars['Boolean']['output'];
    readonly id: Scalars['ID']['output'];
    readonly multiple: Scalars['Boolean']['output'];
    readonly options: ReadonlyArray<PollOption>;
    readonly ownVotes?: Maybe<ReadonlyArray<Scalars['Int']['output']>>;
    readonly voted: Scalars['Boolean']['output'];
    readonly votersCount: Scalars['Int']['output'];
    readonly votesCount: Scalars['Int']['output'];
};
export type PollOption = {
    readonly __typename: 'PollOption';
    readonly title: Scalars['String']['output'];
    readonly votesCount: Scalars['Int']['output'];
};
export type PollParams = {
    readonly __typename: 'PollParams';
    readonly expiresIn: Scalars['Int']['output'];
    readonly hideTotals?: Maybe<Scalars['Boolean']['output']>;
    readonly multiple?: Maybe<Scalars['Boolean']['output']>;
    readonly options: ReadonlyArray<Scalars['String']['output']>;
};
export type PollParamsInput = {
    readonly expiresIn: Scalars['Int']['input'];
    readonly hideTotals?: InputMaybe<Scalars['Boolean']['input']>;
    readonly multiple?: InputMaybe<Scalars['Boolean']['input']>;
    readonly options: ReadonlyArray<Scalars['String']['input']>;
};
export type PortableReputation = {
    readonly __typename: 'PortableReputation';
    readonly actor: Scalars['String']['output'];
    readonly context: ReadonlyArray<Scalars['String']['output']>;
    readonly expiresAt: Scalars['Time']['output'];
    readonly issuedAt: Scalars['Time']['output'];
    readonly issuer: Scalars['String']['output'];
    readonly issuerProof: Scalars['String']['output'];
    readonly reputation: Reputation;
    readonly type: Scalars['String']['output'];
    readonly vouches: ReadonlyArray<Vouch>;
};
export type PostConnection = {
    readonly __typename: 'PostConnection';
    readonly edges: ReadonlyArray<PostEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type PostEdge = {
    readonly __typename: 'PostEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Object;
};
export type PostingPreferences = {
    readonly __typename: 'PostingPreferences';
    readonly defaultLanguage: Scalars['String']['output'];
    readonly defaultSensitive: Scalars['Boolean']['output'];
    readonly defaultVisibility: Visibility;
};
export type Priority = 'CRITICAL' | 'HIGH' | 'LOW' | 'MEDIUM';
export type PrivacyPreferences = {
    readonly __typename: 'PrivacyPreferences';
    readonly defaultVisibility: Visibility;
    readonly indexable: Scalars['Boolean']['output'];
    readonly showOnlineStatus: Scalars['Boolean']['output'];
};
export type ProfileDirectory = {
    readonly __typename: 'ProfileDirectory';
    readonly accounts: ReadonlyArray<Actor>;
    readonly totalCount: Scalars['Int']['output'];
};
export type ProfileFieldInput = {
    readonly name: Scalars['String']['input'];
    readonly value: Scalars['String']['input'];
    readonly verifiedAt?: InputMaybe<Scalars['Time']['input']>;
};
export type PushSubscription = {
    readonly __typename: 'PushSubscription';
    readonly alerts: PushSubscriptionAlerts;
    readonly createdAt?: Maybe<Scalars['Time']['output']>;
    readonly endpoint: Scalars['String']['output'];
    readonly id: Scalars['ID']['output'];
    readonly keys: PushSubscriptionKeys;
    readonly policy: Scalars['String']['output'];
    readonly serverKey?: Maybe<Scalars['String']['output']>;
    readonly updatedAt?: Maybe<Scalars['Time']['output']>;
};
export type PushSubscriptionAlerts = {
    readonly __typename: 'PushSubscriptionAlerts';
    readonly adminReport: Scalars['Boolean']['output'];
    readonly adminSignUp: Scalars['Boolean']['output'];
    readonly favourite: Scalars['Boolean']['output'];
    readonly follow: Scalars['Boolean']['output'];
    readonly followRequest: Scalars['Boolean']['output'];
    readonly mention: Scalars['Boolean']['output'];
    readonly poll: Scalars['Boolean']['output'];
    readonly reblog: Scalars['Boolean']['output'];
    readonly status: Scalars['Boolean']['output'];
    readonly update: Scalars['Boolean']['output'];
};
export type PushSubscriptionAlertsInput = {
    readonly adminReport?: InputMaybe<Scalars['Boolean']['input']>;
    readonly adminSignUp?: InputMaybe<Scalars['Boolean']['input']>;
    readonly favourite?: InputMaybe<Scalars['Boolean']['input']>;
    readonly follow?: InputMaybe<Scalars['Boolean']['input']>;
    readonly followRequest?: InputMaybe<Scalars['Boolean']['input']>;
    readonly mention?: InputMaybe<Scalars['Boolean']['input']>;
    readonly poll?: InputMaybe<Scalars['Boolean']['input']>;
    readonly reblog?: InputMaybe<Scalars['Boolean']['input']>;
    readonly status?: InputMaybe<Scalars['Boolean']['input']>;
    readonly update?: InputMaybe<Scalars['Boolean']['input']>;
};
export type PushSubscriptionKeys = {
    readonly __typename: 'PushSubscriptionKeys';
    readonly auth: Scalars['String']['output'];
    readonly p256dh: Scalars['String']['output'];
};
export type PushSubscriptionKeysInput = {
    readonly auth: Scalars['String']['input'];
    readonly p256dh: Scalars['String']['input'];
};
export type QualityBandwidth = {
    readonly __typename: 'QualityBandwidth';
    readonly percentage: Scalars['Float']['output'];
    readonly quality: StreamQuality;
    readonly totalGB: Scalars['Float']['output'];
};
export type QualityStats = {
    readonly __typename: 'QualityStats';
    readonly avgBandwidth: Scalars['Float']['output'];
    readonly percentage: Scalars['Float']['output'];
    readonly quality: StreamQuality;
    readonly viewCount: Scalars['Int']['output'];
};
export type Query = {
    readonly __typename: 'Query';
    readonly actor?: Maybe<Actor>;
    readonly affectedRelationships: AffectedRelationshipConnection;
    readonly aiAnalysis?: Maybe<AiAnalysis>;
    readonly aiCapabilities: AiCapabilities;
    readonly aiStats: AiStats;
    readonly bandwidthUsage: BandwidthReport;
    readonly conversation?: Maybe<Conversation>;
    readonly conversations: ReadonlyArray<Conversation>;
    readonly costBreakdown: CostBreakdown;
    readonly costProjections: CostProjection;
    readonly customEmojis: ReadonlyArray<CustomEmoji>;
    readonly explainObject: ObjectExplanation;
    readonly federationCosts: FederationCostConnection;
    readonly federationFlow: FederationFlow;
    readonly federationHealth: ReadonlyArray<FederationManagementStatus>;
    readonly federationLimits: ReadonlyArray<FederationLimit>;
    readonly federationMap: FederationGraph;
    readonly federationStatus: FederationStatus;
    readonly followedHashtags: HashtagConnection;
    readonly followers: ActorListPage;
    readonly following: ActorListPage;
    readonly hashtag?: Maybe<Hashtag>;
    readonly hashtagTimeline: PostConnection;
    readonly infrastructureHealth: InfrastructureStatus;
    readonly instanceBudgets: ReadonlyArray<InstanceBudget>;
    readonly instanceHealthReport: InstanceHealthReport;
    readonly instanceMetrics: InstanceMetrics;
    readonly instanceRelationships: InstanceRelations;
    readonly list?: Maybe<List>;
    readonly listAccounts: ReadonlyArray<Actor>;
    readonly lists: ReadonlyArray<List>;
    readonly media?: Maybe<Media>;
    readonly mediaLibrary: MediaConnection;
    readonly mediaStreamUrl: MediaStream;
    readonly moderationDashboard: ModerationDashboard;
    readonly moderationEffectiveness: ModerationEffectiveness;
    readonly moderationPatterns: ReadonlyArray<ModerationPattern>;
    readonly moderationQueue: ReadonlyArray<ModerationDecision>;
    readonly moderatorActivity: ModeratorStats;
    readonly multiHashtagTimeline: PostConnection;
    readonly notifications: NotificationConnection;
    readonly object?: Maybe<Object>;
    readonly patternEffectiveness: PatternStats;
    readonly performanceMetrics: PerformanceReport;
    readonly popularStreams: StreamConnection;
    readonly profileDirectory: ProfileDirectory;
    readonly pushSubscription?: Maybe<PushSubscription>;
    readonly relationship?: Maybe<Relationship>;
    readonly relationships: ReadonlyArray<Relationship>;
    readonly removeSuggestion: Scalars['Boolean']['output'];
    readonly scheduledStatus?: Maybe<ScheduledStatus>;
    readonly scheduledStatuses: ReadonlyArray<ScheduledStatus>;
    readonly search: SearchResult;
    readonly severedRelationships: SeveredRelationshipConnection;
    readonly slowQueries: ReadonlyArray<QueryPerformance>;
    readonly streamingAnalytics: StreamingAnalytics;
    readonly suggestedHashtags: ReadonlyArray<HashtagSuggestion>;
    readonly suggestions: ReadonlyArray<AccountSuggestion>;
    readonly supportedBitrates: ReadonlyArray<Bitrate>;
    readonly threadContext?: Maybe<ThreadContext>;
    readonly timeline: ObjectConnection;
    readonly trustGraph: ReadonlyArray<TrustEdge>;
    readonly userPreferences: UserPreferences;
};
export type QueryActorArgs = {
    id?: InputMaybe<Scalars['ID']['input']>;
    username?: InputMaybe<Scalars['String']['input']>;
};
export type QueryAffectedRelationshipsArgs = {
    severedRelationshipId: Scalars['ID']['input'];
};
export type QueryAiAnalysisArgs = {
    objectId: Scalars['ID']['input'];
};
export type QueryAiStatsArgs = {
    period: Period;
};
export type QueryBandwidthUsageArgs = {
    period: TimePeriod;
};
export type QueryConversationArgs = {
    id: Scalars['ID']['input'];
};
export type QueryConversationsArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryCostBreakdownArgs = {
    period?: InputMaybe<Period>;
};
export type QueryCostProjectionsArgs = {
    period: Period;
};
export type QueryExplainObjectArgs = {
    id: Scalars['ID']['input'];
};
export type QueryFederationCostsArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    orderBy?: InputMaybe<CostOrderBy>;
};
export type QueryFederationFlowArgs = {
    period: TimePeriod;
};
export type QueryFederationHealthArgs = {
    threshold?: InputMaybe<Scalars['Float']['input']>;
};
export type QueryFederationLimitsArgs = {
    active?: InputMaybe<Scalars['Boolean']['input']>;
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryFederationMapArgs = {
    depth?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryFederationStatusArgs = {
    domain: Scalars['String']['input'];
};
export type QueryFollowedHashtagsArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryFollowersArgs = {
    cursor?: InputMaybe<Scalars['Cursor']['input']>;
    limit?: InputMaybe<Scalars['Int']['input']>;
    username: Scalars['String']['input'];
};
export type QueryFollowingArgs = {
    cursor?: InputMaybe<Scalars['Cursor']['input']>;
    limit?: InputMaybe<Scalars['Int']['input']>;
    username: Scalars['String']['input'];
};
export type QueryHashtagArgs = {
    name: Scalars['String']['input'];
};
export type QueryHashtagTimelineArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    hashtag: Scalars['String']['input'];
    mediaOnly?: InputMaybe<Scalars['Boolean']['input']>;
};
export type QueryInstanceBudgetsArgs = {
    exceeded?: InputMaybe<Scalars['Boolean']['input']>;
};
export type QueryInstanceHealthReportArgs = {
    domain: Scalars['String']['input'];
};
export type QueryInstanceRelationshipsArgs = {
    domain: Scalars['String']['input'];
};
export type QueryListArgs = {
    id: Scalars['ID']['input'];
};
export type QueryListAccountsArgs = {
    id: Scalars['ID']['input'];
};
export type QueryMediaArgs = {
    id: Scalars['ID']['input'];
};
export type QueryMediaLibraryArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    filter?: InputMaybe<MediaFilterInput>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryMediaStreamUrlArgs = {
    mediaId: Scalars['ID']['input'];
};
export type QueryModerationDashboardArgs = {
    filter?: InputMaybe<ModerationFilter>;
};
export type QueryModerationEffectivenessArgs = {
    patternId: Scalars['ID']['input'];
    period: ModerationPeriod;
};
export type QueryModerationPatternsArgs = {
    active?: InputMaybe<Scalars['Boolean']['input']>;
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    severity?: InputMaybe<ModerationSeverity>;
};
export type QueryModerationQueueArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryModeratorActivityArgs = {
    moderatorId: Scalars['ID']['input'];
    period: TimePeriod;
};
export type QueryMultiHashtagTimelineArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    hashtags: ReadonlyArray<Scalars['String']['input']>;
    mode: HashtagMode;
};
export type QueryNotificationsArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    excludeTypes?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    first?: InputMaybe<Scalars['Int']['input']>;
    types?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
};
export type QueryObjectArgs = {
    id: Scalars['ID']['input'];
};
export type QueryPatternEffectivenessArgs = {
    patternId: Scalars['ID']['input'];
};
export type QueryPerformanceMetricsArgs = {
    service: ServiceCategory;
};
export type QueryPopularStreamsArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first: Scalars['Int']['input'];
};
export type QueryProfileDirectoryArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    filters?: InputMaybe<DirectoryFiltersInput>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QueryRelationshipArgs = {
    id: Scalars['ID']['input'];
};
export type QueryRelationshipsArgs = {
    ids: ReadonlyArray<Scalars['ID']['input']>;
};
export type QueryRemoveSuggestionArgs = {
    accountId: Scalars['ID']['input'];
};
export type QueryScheduledStatusArgs = {
    id: Scalars['ID']['input'];
};
export type QueryScheduledStatusesArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
};
export type QuerySearchArgs = {
    after?: InputMaybe<Scalars['Cursor']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    query: Scalars['String']['input'];
    type?: InputMaybe<Scalars['String']['input']>;
};
export type QuerySeveredRelationshipsArgs = {
    after?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    instance?: InputMaybe<Scalars['String']['input']>;
};
export type QuerySlowQueriesArgs = {
    threshold: Scalars['Duration']['input'];
};
export type QueryStreamingAnalyticsArgs = {
    mediaId: Scalars['ID']['input'];
};
export type QuerySuggestedHashtagsArgs = {
    limit?: InputMaybe<Scalars['Int']['input']>;
};
export type QuerySuggestionsArgs = {
    limit?: InputMaybe<Scalars['Int']['input']>;
};
export type QuerySupportedBitratesArgs = {
    mediaId: Scalars['ID']['input'];
};
export type QueryThreadContextArgs = {
    noteId: Scalars['ID']['input'];
};
export type QueryTimelineArgs = {
    actorId?: InputMaybe<Scalars['ID']['input']>;
    after?: InputMaybe<Scalars['Cursor']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    hashtag?: InputMaybe<Scalars['String']['input']>;
    listId?: InputMaybe<Scalars['ID']['input']>;
    mediaOnly?: InputMaybe<Scalars['Boolean']['input']>;
    type: TimelineType;
};
export type QueryTrustGraphArgs = {
    actorId: Scalars['ID']['input'];
    category?: InputMaybe<TrustCategory>;
};
export type QueryPerformance = {
    readonly __typename: 'QueryPerformance';
    readonly avgDuration: Scalars['Duration']['output'];
    readonly count: Scalars['Int']['output'];
    readonly errorCount: Scalars['Int']['output'];
    readonly lastSeen: Scalars['Time']['output'];
    readonly p95Duration: Scalars['Duration']['output'];
    readonly query: Scalars['String']['output'];
};
export type QueueStatus = {
    readonly __typename: 'QueueStatus';
    readonly depth: Scalars['Int']['output'];
    readonly dlqCount: Scalars['Int']['output'];
    readonly name: Scalars['String']['output'];
    readonly oldestMessage?: Maybe<Scalars['Time']['output']>;
    readonly processingRate: Scalars['Float']['output'];
};
export type QuoteActivityUpdate = {
    readonly __typename: 'QuoteActivityUpdate';
    readonly quote?: Maybe<Object>;
    readonly quoter?: Maybe<Actor>;
    readonly timestamp: Scalars['Time']['output'];
    readonly type: Scalars['String']['output'];
};
export type QuoteConnection = {
    readonly __typename: 'QuoteConnection';
    readonly edges: ReadonlyArray<QuoteEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type QuoteContext = {
    readonly __typename: 'QuoteContext';
    readonly originalAuthor: Actor;
    readonly originalNote?: Maybe<Object>;
    readonly quoteAllowed: Scalars['Boolean']['output'];
    readonly quoteType: QuoteType;
    readonly withdrawn: Scalars['Boolean']['output'];
};
export type QuoteEdge = {
    readonly __typename: 'QuoteEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Object;
};
export type QuotePermission = 'EVERYONE' | 'FOLLOWERS' | 'NONE';
export type QuoteType = 'COMMENTARY' | 'FULL' | 'PARTIAL' | 'REACTION';
export type ReadingPreferences = {
    readonly __typename: 'ReadingPreferences';
    readonly autoplayGifs: Scalars['Boolean']['output'];
    readonly expandMedia: ExpandMediaPreference;
    readonly expandSpoilers: Scalars['Boolean']['output'];
    readonly timelineOrder: TimelineOrder;
};
export type ReblogFilter = {
    readonly __typename: 'ReblogFilter';
    readonly enabled: Scalars['Boolean']['output'];
    readonly key: Scalars['String']['output'];
};
export type ReblogFilterInput = {
    readonly enabled: Scalars['Boolean']['input'];
    readonly key: Scalars['String']['input'];
};
export type RecommendationType = 'CONNECTIVITY' | 'CONTENT' | 'COST' | 'PERFORMANCE' | 'SECURITY';
export type ReconnectionPayload = {
    readonly __typename: 'ReconnectionPayload';
    readonly errors?: Maybe<ReadonlyArray<Scalars['String']['output']>>;
    readonly failed: Scalars['Int']['output'];
    readonly reconnected: Scalars['Int']['output'];
    readonly severedRelationship: SeveredRelationship;
    readonly success: Scalars['Boolean']['output'];
};
export type RegisterPushSubscriptionInput = {
    readonly alerts: PushSubscriptionAlertsInput;
    readonly endpoint: Scalars['String']['input'];
    readonly keys: PushSubscriptionKeysInput;
};
export type Relationship = {
    readonly __typename: 'Relationship';
    readonly blockedBy: Scalars['Boolean']['output'];
    readonly blocking: Scalars['Boolean']['output'];
    readonly domainBlocking: Scalars['Boolean']['output'];
    readonly followedBy: Scalars['Boolean']['output'];
    readonly following: Scalars['Boolean']['output'];
    readonly id: Scalars['ID']['output'];
    readonly languages?: Maybe<ReadonlyArray<Scalars['String']['output']>>;
    readonly muting: Scalars['Boolean']['output'];
    readonly mutingNotifications: Scalars['Boolean']['output'];
    readonly note?: Maybe<Scalars['String']['output']>;
    readonly notifying: Scalars['Boolean']['output'];
    readonly requested: Scalars['Boolean']['output'];
    readonly showingReblogs: Scalars['Boolean']['output'];
};
export type RelationshipUpdate = {
    readonly __typename: 'RelationshipUpdate';
    readonly actor: Actor;
    readonly relationship: Relationship;
    readonly timestamp: Scalars['Time']['output'];
    readonly type: Scalars['String']['output'];
};
export type RepliesPolicy = 'FOLLOWED' | 'LIST' | 'NONE';
export type Reputation = {
    readonly __typename: 'Reputation';
    readonly activityScore: Scalars['Int']['output'];
    readonly actorId: Scalars['ID']['output'];
    readonly calculatedAt: Scalars['Time']['output'];
    readonly communityScore: Scalars['Int']['output'];
    readonly evidence: ReputationEvidence;
    readonly instance: Scalars['String']['output'];
    readonly moderationScore: Scalars['Int']['output'];
    readonly signature?: Maybe<Scalars['String']['output']>;
    readonly totalScore: Scalars['Int']['output'];
    readonly trustScore: Scalars['Int']['output'];
    readonly version: Scalars['String']['output'];
};
export type ReputationEvidence = {
    readonly __typename: 'ReputationEvidence';
    readonly accountAge: Scalars['Int']['output'];
    readonly averageTrustScore: Scalars['Float']['output'];
    readonly totalFollowers: Scalars['Int']['output'];
    readonly totalPosts: Scalars['Int']['output'];
    readonly trustingActors: Scalars['Int']['output'];
    readonly vouchCount: Scalars['Int']['output'];
};
export type ReputationImportResult = {
    readonly __typename: 'ReputationImportResult';
    readonly actorId: Scalars['String']['output'];
    readonly error?: Maybe<Scalars['String']['output']>;
    readonly importedScore: Scalars['Int']['output'];
    readonly message?: Maybe<Scalars['String']['output']>;
    readonly previousScore: Scalars['Int']['output'];
    readonly success: Scalars['Boolean']['output'];
    readonly vouchesImported: Scalars['Int']['output'];
};
export type ReputationVerificationResult = {
    readonly __typename: 'ReputationVerificationResult';
    readonly actorId: Scalars['String']['output'];
    readonly error?: Maybe<Scalars['String']['output']>;
    readonly expiresAt: Scalars['Time']['output'];
    readonly issuedAt: Scalars['Time']['output'];
    readonly issuer: Scalars['String']['output'];
    readonly issuerTrusted: Scalars['Boolean']['output'];
    readonly notExpired: Scalars['Boolean']['output'];
    readonly signatureValid: Scalars['Boolean']['output'];
    readonly valid: Scalars['Boolean']['output'];
};
export type ScheduleStatusInput = {
    readonly inReplyToId?: InputMaybe<Scalars['ID']['input']>;
    readonly language?: InputMaybe<Scalars['String']['input']>;
    readonly mediaIds?: InputMaybe<ReadonlyArray<Scalars['ID']['input']>>;
    readonly poll?: InputMaybe<PollParamsInput>;
    readonly scheduledAt: Scalars['Time']['input'];
    readonly sensitive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly spoilerText?: InputMaybe<Scalars['String']['input']>;
    readonly text: Scalars['String']['input'];
    readonly visibility?: InputMaybe<Visibility>;
};
export type ScheduledStatus = {
    readonly __typename: 'ScheduledStatus';
    readonly createdAt: Scalars['Time']['output'];
    readonly id: Scalars['ID']['output'];
    readonly mediaAttachments: ReadonlyArray<Media>;
    readonly params: StatusParams;
    readonly scheduledAt: Scalars['Time']['output'];
};
export type SearchResult = {
    readonly __typename: 'SearchResult';
    readonly accounts: ReadonlyArray<Actor>;
    readonly hashtags: ReadonlyArray<Tag>;
    readonly statuses: ReadonlyArray<Object>;
};
export type Sentiment = 'MIXED' | 'NEGATIVE' | 'NEUTRAL' | 'POSITIVE';
export type SentimentScores = {
    readonly __typename: 'SentimentScores';
    readonly mixed: Scalars['Float']['output'];
    readonly negative: Scalars['Float']['output'];
    readonly neutral: Scalars['Float']['output'];
    readonly positive: Scalars['Float']['output'];
};
export type ServiceCategory = 'FEDERATION_DELIVERY' | 'GRAPHQL_API' | 'MEDIA_PROCESSOR' | 'MODERATION_ENGINE' | 'SEARCH_INDEXER' | 'STREAMING_SERVICE';
export type ServiceStatus = {
    readonly __typename: 'ServiceStatus';
    readonly errorRate: Scalars['Float']['output'];
    readonly lastRestart?: Maybe<Scalars['Time']['output']>;
    readonly name: Scalars['String']['output'];
    readonly status: HealthStatus;
    readonly type: ServiceCategory;
    readonly uptime: Scalars['Float']['output'];
};
export type SeveranceDetails = {
    readonly __typename: 'SeveranceDetails';
    readonly adminNotes?: Maybe<Scalars['String']['output']>;
    readonly autoDetected: Scalars['Boolean']['output'];
    readonly description: Scalars['String']['output'];
    readonly metadata: ReadonlyArray<Scalars['String']['output']>;
};
export type SeveranceReason = 'DEFEDERATION' | 'DOMAIN_BLOCK' | 'INSTANCE_DOWN' | 'OTHER' | 'POLICY_VIOLATION';
export type SeveredRelationship = {
    readonly __typename: 'SeveredRelationship';
    readonly affectedFollowers: Scalars['Int']['output'];
    readonly affectedFollowing: Scalars['Int']['output'];
    readonly details?: Maybe<SeveranceDetails>;
    readonly id: Scalars['ID']['output'];
    readonly localInstance: Scalars['String']['output'];
    readonly reason: SeveranceReason;
    readonly remoteInstance: Scalars['String']['output'];
    readonly reversible: Scalars['Boolean']['output'];
    readonly timestamp: Scalars['Time']['output'];
};
export type SeveredRelationshipConnection = {
    readonly __typename: 'SeveredRelationshipConnection';
    readonly edges: ReadonlyArray<SeveredRelationshipEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type SeveredRelationshipEdge = {
    readonly __typename: 'SeveredRelationshipEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: SeveredRelationship;
};
export type SpamAnalysis = {
    readonly __typename: 'SpamAnalysis';
    readonly accountAgeDays: Scalars['Int']['output'];
    readonly followerRatio: Scalars['Float']['output'];
    readonly interactionRate: Scalars['Float']['output'];
    readonly linkDensity: Scalars['Float']['output'];
    readonly postingVelocity: Scalars['Float']['output'];
    readonly repetitionScore: Scalars['Float']['output'];
    readonly spamIndicators: ReadonlyArray<SpamIndicator>;
    readonly spamScore: Scalars['Float']['output'];
};
export type SpamIndicator = {
    readonly __typename: 'SpamIndicator';
    readonly description: Scalars['String']['output'];
    readonly severity: Scalars['Float']['output'];
    readonly type: Scalars['String']['output'];
};
export type StatusParams = {
    readonly __typename: 'StatusParams';
    readonly inReplyToId?: Maybe<Scalars['ID']['output']>;
    readonly language?: Maybe<Scalars['String']['output']>;
    readonly poll?: Maybe<PollParams>;
    readonly sensitive: Scalars['Boolean']['output'];
    readonly spoilerText?: Maybe<Scalars['String']['output']>;
    readonly text: Scalars['String']['output'];
    readonly visibility: Visibility;
};
export type Stream = {
    readonly __typename: 'Stream';
    readonly createdAt: Scalars['Time']['output'];
    readonly duration: Scalars['Duration']['output'];
    readonly id: Scalars['ID']['output'];
    readonly mediaId: Scalars['ID']['output'];
    readonly popularity: Scalars['Float']['output'];
    readonly quality: StreamQuality;
    readonly thumbnail: Scalars['String']['output'];
    readonly title: Scalars['String']['output'];
    readonly viewCount: Scalars['Int']['output'];
};
export type StreamConnection = {
    readonly __typename: 'StreamConnection';
    readonly edges: ReadonlyArray<StreamEdge>;
    readonly pageInfo: PageInfo;
    readonly totalCount: Scalars['Int']['output'];
};
export type StreamEdge = {
    readonly __typename: 'StreamEdge';
    readonly cursor: Scalars['Cursor']['output'];
    readonly node: Stream;
};
export type StreamQuality = 'AUTO' | 'HIGH' | 'LOW' | 'MEDIUM' | 'ULTRA';
export type StreamingAnalytics = {
    readonly __typename: 'StreamingAnalytics';
    readonly averageWatchTime: Scalars['Duration']['output'];
    readonly bufferingEvents: Scalars['Int']['output'];
    readonly completionRate: Scalars['Float']['output'];
    readonly qualityDistribution: ReadonlyArray<QualityStats>;
    readonly totalViews: Scalars['Int']['output'];
    readonly uniqueViewers: Scalars['Int']['output'];
};
export type StreamingPreferences = {
    readonly __typename: 'StreamingPreferences';
    readonly autoQuality: Scalars['Boolean']['output'];
    readonly dataSaver: Scalars['Boolean']['output'];
    readonly defaultQuality: StreamQuality;
    readonly preloadNext: Scalars['Boolean']['output'];
};
export type StreamingPreferencesInput = {
    readonly autoQuality?: InputMaybe<Scalars['Boolean']['input']>;
    readonly dataSaver?: InputMaybe<Scalars['Boolean']['input']>;
    readonly defaultQuality?: InputMaybe<StreamQuality>;
    readonly preloadNext?: InputMaybe<Scalars['Boolean']['input']>;
};
export type StreamingQualityInput = {
    readonly bufferingEvents: Scalars['Int']['input'];
    readonly mediaId: Scalars['ID']['input'];
    readonly quality: StreamQuality;
    readonly watchTime: Scalars['Int']['input'];
};
export type StreamingQualityReport = {
    readonly __typename: 'StreamingQualityReport';
    readonly mediaId: Scalars['ID']['output'];
    readonly quality: StreamQuality;
    readonly reportId: Scalars['ID']['output'];
    readonly success: Scalars['Boolean']['output'];
};
export type Subscription = {
    readonly __typename: 'Subscription';
    readonly activityStream: Activity;
    readonly aiAnalysisUpdates: AiAnalysis;
    readonly budgetAlerts: BudgetAlert;
    readonly conversationUpdates: Conversation;
    readonly costAlerts: CostAlert;
    readonly costUpdates: CostUpdate;
    readonly federationHealthUpdates: FederationHealthUpdate;
    readonly hashtagActivity: HashtagActivityUpdate;
    readonly infrastructureEvent: InfrastructureEvent;
    readonly listUpdates: ListUpdate;
    readonly metricsUpdates: MetricsUpdate;
    readonly moderationAlerts: ModerationAlert;
    readonly moderationEvents: ModerationDecision;
    readonly moderationQueueUpdate: ModerationItem;
    readonly notificationStream: Notification;
    readonly performanceAlert: PerformanceAlert;
    readonly quoteActivity: QuoteActivityUpdate;
    readonly relationshipUpdates: RelationshipUpdate;
    readonly threatIntelligence: ThreatAlert;
    readonly timelineUpdates: Object;
    readonly trustUpdates: TrustEdge;
};
export type SubscriptionActivityStreamArgs = {
    types?: InputMaybe<ReadonlyArray<ActivityType>>;
};
export type SubscriptionAiAnalysisUpdatesArgs = {
    objectId?: InputMaybe<Scalars['ID']['input']>;
};
export type SubscriptionBudgetAlertsArgs = {
    domain?: InputMaybe<Scalars['String']['input']>;
};
export type SubscriptionCostAlertsArgs = {
    thresholdUSD: Scalars['Float']['input'];
};
export type SubscriptionCostUpdatesArgs = {
    threshold?: InputMaybe<Scalars['Int']['input']>;
};
export type SubscriptionFederationHealthUpdatesArgs = {
    domain?: InputMaybe<Scalars['String']['input']>;
};
export type SubscriptionHashtagActivityArgs = {
    hashtags: ReadonlyArray<Scalars['String']['input']>;
};
export type SubscriptionListUpdatesArgs = {
    listId: Scalars['ID']['input'];
};
export type SubscriptionMetricsUpdatesArgs = {
    categories?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    services?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    threshold?: InputMaybe<Scalars['Float']['input']>;
};
export type SubscriptionModerationAlertsArgs = {
    severity?: InputMaybe<ModerationSeverity>;
};
export type SubscriptionModerationEventsArgs = {
    actorId?: InputMaybe<Scalars['ID']['input']>;
};
export type SubscriptionModerationQueueUpdateArgs = {
    priority?: InputMaybe<Priority>;
};
export type SubscriptionNotificationStreamArgs = {
    types?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
};
export type SubscriptionPerformanceAlertArgs = {
    severity: AlertSeverity;
};
export type SubscriptionQuoteActivityArgs = {
    noteId: Scalars['ID']['input'];
};
export type SubscriptionRelationshipUpdatesArgs = {
    actorId?: InputMaybe<Scalars['ID']['input']>;
};
export type SubscriptionTimelineUpdatesArgs = {
    listId?: InputMaybe<Scalars['ID']['input']>;
    type: TimelineType;
};
export type SubscriptionTrustUpdatesArgs = {
    actorId: Scalars['ID']['input'];
};
export type SuggestionSource = 'GLOBAL' | 'PAST_INTERACTIONS' | 'SIMILAR_PROFILES' | 'STAFF';
export type SyncRepliesPayload = {
    readonly __typename: 'SyncRepliesPayload';
    readonly success: Scalars['Boolean']['output'];
    readonly syncedReplies: Scalars['Int']['output'];
    readonly thread: ThreadContext;
};
export type SyncStatus = 'COMPLETE' | 'FAILED' | 'PARTIAL' | 'SYNCING';
export type SyncThreadPayload = {
    readonly __typename: 'SyncThreadPayload';
    readonly errors?: Maybe<ReadonlyArray<Scalars['String']['output']>>;
    readonly success: Scalars['Boolean']['output'];
    readonly syncedPosts: Scalars['Int']['output'];
    readonly thread: ThreadContext;
};
export type Tag = {
    readonly __typename: 'Tag';
    readonly name: Scalars['String']['output'];
    readonly url: Scalars['String']['output'];
};
export type TextAnalysis = {
    readonly __typename: 'TextAnalysis';
    readonly containsPII: Scalars['Boolean']['output'];
    readonly dominantLanguage: Scalars['String']['output'];
    readonly entities: ReadonlyArray<Entity>;
    readonly keyPhrases: ReadonlyArray<Scalars['String']['output']>;
    readonly sentiment: Sentiment;
    readonly sentimentScores: SentimentScores;
    readonly toxicityLabels: ReadonlyArray<Scalars['String']['output']>;
    readonly toxicityScore: Scalars['Float']['output'];
};
export type TextAnalysisCapabilities = {
    readonly __typename: 'TextAnalysisCapabilities';
    readonly entityExtraction: Scalars['Boolean']['output'];
    readonly languageDetection: Scalars['Boolean']['output'];
    readonly piiDetection: Scalars['Boolean']['output'];
    readonly sentimentAnalysis: Scalars['Boolean']['output'];
    readonly spamDetection: Scalars['Boolean']['output'];
    readonly toxicityDetection: Scalars['Boolean']['output'];
};
export type ThreadContext = {
    readonly __typename: 'ThreadContext';
    readonly lastActivity: Scalars['Time']['output'];
    readonly missingPosts: Scalars['Int']['output'];
    readonly participantCount: Scalars['Int']['output'];
    readonly replyCount: Scalars['Int']['output'];
    readonly rootNote: Object;
    readonly syncStatus: SyncStatus;
};
export type ThreatAlert = {
    readonly __typename: 'ThreatAlert';
    readonly affectedInstances: ReadonlyArray<Scalars['String']['output']>;
    readonly description: Scalars['String']['output'];
    readonly id: Scalars['ID']['output'];
    readonly mitigationSteps: ReadonlyArray<Scalars['String']['output']>;
    readonly severity: ModerationSeverity;
    readonly source: Scalars['String']['output'];
    readonly timestamp: Scalars['Time']['output'];
    readonly type: Scalars['String']['output'];
};
export type ThreatTrend = {
    readonly __typename: 'ThreatTrend';
    readonly change: Scalars['Float']['output'];
    readonly count: Scalars['Int']['output'];
    readonly instances: ReadonlyArray<Scalars['String']['output']>;
    readonly severity: ModerationSeverity;
    readonly type: Scalars['String']['output'];
};
export type TimePeriod = 'DAY' | 'HOUR' | 'MONTH' | 'WEEK';
export type TimelineOrder = 'NEWEST' | 'OLDEST';
export type TimelineType = 'ACTOR' | 'DIRECT' | 'HASHTAG' | 'HOME' | 'LIST' | 'LOCAL' | 'PUBLIC';
export type TrainingResult = {
    readonly __typename: 'TrainingResult';
    readonly accuracy: Scalars['Float']['output'];
    readonly datasetS3Key: Scalars['String']['output'];
    readonly f1Score: Scalars['Float']['output'];
    readonly improvements: ReadonlyArray<Scalars['String']['output']>;
    readonly jobId: Scalars['String']['output'];
    readonly jobName: Scalars['String']['output'];
    readonly modelVersion: Scalars['String']['output'];
    readonly precision: Scalars['Float']['output'];
    readonly recall: Scalars['Float']['output'];
    readonly samplesUsed: Scalars['Int']['output'];
    readonly status: Scalars['String']['output'];
    readonly success: Scalars['Boolean']['output'];
    readonly trainingTime: Scalars['Int']['output'];
};
export type Trend = 'DECREASING' | 'INCREASING' | 'STABLE';
export type TrustCategory = 'BEHAVIOR' | 'CONTENT' | 'TECHNICAL';
export type TrustEdge = {
    readonly __typename: 'TrustEdge';
    readonly category: TrustCategory;
    readonly from: Actor;
    readonly score: Scalars['Float']['output'];
    readonly to: Actor;
    readonly updatedAt: Scalars['Time']['output'];
};
export type TrustInput = {
    readonly category: TrustCategory;
    readonly score: Scalars['Float']['input'];
    readonly targetActorId: Scalars['ID']['input'];
};
export type UnfollowHashtagPayload = {
    readonly __typename: 'UnfollowHashtagPayload';
    readonly hashtag: Hashtag;
    readonly success: Scalars['Boolean']['output'];
};
export type UpdateEmojiInput = {
    readonly category?: InputMaybe<Scalars['String']['input']>;
    readonly visibleInPicker?: InputMaybe<Scalars['Boolean']['input']>;
};
export type UpdateHashtagNotificationsPayload = {
    readonly __typename: 'UpdateHashtagNotificationsPayload';
    readonly hashtag: Hashtag;
    readonly settings: HashtagNotificationSettings;
    readonly success: Scalars['Boolean']['output'];
};
export type UpdateListInput = {
    readonly exclusive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly repliesPolicy?: InputMaybe<RepliesPolicy>;
    readonly title?: InputMaybe<Scalars['String']['input']>;
};
export type UpdateMediaInput = {
    readonly description?: InputMaybe<Scalars['String']['input']>;
    readonly focus?: InputMaybe<FocusInput>;
};
export type UpdateProfileInput = {
    readonly avatar?: InputMaybe<Scalars['String']['input']>;
    readonly bio?: InputMaybe<Scalars['String']['input']>;
    readonly bot?: InputMaybe<Scalars['Boolean']['input']>;
    readonly discoverable?: InputMaybe<Scalars['Boolean']['input']>;
    readonly displayName?: InputMaybe<Scalars['String']['input']>;
    readonly fields?: InputMaybe<ReadonlyArray<ProfileFieldInput>>;
    readonly header?: InputMaybe<Scalars['String']['input']>;
    readonly language?: InputMaybe<Scalars['String']['input']>;
    readonly locked?: InputMaybe<Scalars['Boolean']['input']>;
    readonly noIndex?: InputMaybe<Scalars['Boolean']['input']>;
    readonly sensitive?: InputMaybe<Scalars['Boolean']['input']>;
};
export type UpdatePushSubscriptionInput = {
    readonly alerts: PushSubscriptionAlertsInput;
};
export type UpdateQuotePermissionsPayload = {
    readonly __typename: 'UpdateQuotePermissionsPayload';
    readonly affectedQuotes: Scalars['Int']['output'];
    readonly note: Object;
    readonly success: Scalars['Boolean']['output'];
};
export type UpdateRelationshipInput = {
    readonly languages?: InputMaybe<ReadonlyArray<Scalars['String']['input']>>;
    readonly note?: InputMaybe<Scalars['String']['input']>;
    readonly notify?: InputMaybe<Scalars['Boolean']['input']>;
    readonly showReblogs?: InputMaybe<Scalars['Boolean']['input']>;
};
export type UpdateScheduledStatusInput = {
    readonly scheduledAt: Scalars['Time']['input'];
};
export type UpdateUserPreferencesInput = {
    readonly autoplayGifs?: InputMaybe<Scalars['Boolean']['input']>;
    readonly defaultMediaSensitive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly defaultPostingVisibility?: InputMaybe<Visibility>;
    readonly expandMedia?: InputMaybe<ExpandMediaPreference>;
    readonly expandSpoilers?: InputMaybe<Scalars['Boolean']['input']>;
    readonly language?: InputMaybe<Scalars['String']['input']>;
    readonly personalizedSearchEnabled?: InputMaybe<Scalars['Boolean']['input']>;
    readonly preferredTimelineOrder?: InputMaybe<TimelineOrder>;
    readonly reblogFilters?: InputMaybe<ReadonlyArray<ReblogFilterInput>>;
    readonly searchSuggestionsEnabled?: InputMaybe<Scalars['Boolean']['input']>;
    readonly showFollowCounts?: InputMaybe<Scalars['Boolean']['input']>;
    readonly streaming?: InputMaybe<StreamingPreferencesInput>;
};
export type UploadMediaInput = {
    readonly description?: InputMaybe<Scalars['String']['input']>;
    readonly file: Scalars['Upload']['input'];
    readonly filename?: InputMaybe<Scalars['String']['input']>;
    readonly focus?: InputMaybe<FocusInput>;
    readonly mediaType?: InputMaybe<MediaCategory>;
    readonly sensitive?: InputMaybe<Scalars['Boolean']['input']>;
    readonly spoilerText?: InputMaybe<Scalars['String']['input']>;
};
export type UploadMediaPayload = {
    readonly __typename: 'UploadMediaPayload';
    readonly media: Media;
    readonly uploadId: Scalars['ID']['output'];
    readonly warnings?: Maybe<ReadonlyArray<Scalars['String']['output']>>;
};
export type UserPreferences = {
    readonly __typename: 'UserPreferences';
    readonly actorId: Scalars['ID']['output'];
    readonly discovery: DiscoveryPreferences;
    readonly notifications: NotificationPreferences;
    readonly posting: PostingPreferences;
    readonly privacy: PrivacyPreferences;
    readonly reading: ReadingPreferences;
    readonly reblogFilters: ReadonlyArray<ReblogFilter>;
    readonly streaming: StreamingPreferences;
};
export type Visibility = 'DIRECT' | 'FOLLOWERS' | 'PUBLIC' | 'UNLISTED';
export type Vouch = {
    readonly __typename: 'Vouch';
    readonly active: Scalars['Boolean']['output'];
    readonly confidence: Scalars['Float']['output'];
    readonly context: Scalars['String']['output'];
    readonly createdAt: Scalars['Time']['output'];
    readonly expiresAt: Scalars['Time']['output'];
    readonly from: Actor;
    readonly id: Scalars['ID']['output'];
    readonly revoked: Scalars['Boolean']['output'];
    readonly revokedAt?: Maybe<Scalars['Time']['output']>;
    readonly to: Actor;
    readonly voucherReputation: Scalars['Int']['output'];
};
export type WithdrawQuotePayload = {
    readonly __typename: 'WithdrawQuotePayload';
    readonly note: Object;
    readonly success: Scalars['Boolean']['output'];
    readonly withdrawnCount: Scalars['Int']['output'];
};
export type ActorByIdQueryVariables = Exact<{
    id?: InputMaybe<Scalars['ID']['input']>;
}>;
export type ActorByIdQuery = {
    readonly __typename: 'Query';
    readonly actor?: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    } | null | undefined;
};
export type ActorByUsernameQueryVariables = Exact<{
    username?: InputMaybe<Scalars['String']['input']>;
}>;
export type ActorByUsernameQuery = {
    readonly __typename: 'Query';
    readonly actor?: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    } | null | undefined;
};
export type RequestAiAnalysisMutationVariables = Exact<{
    objectId: Scalars['ID']['input'];
    objectType?: InputMaybe<Scalars['String']['input']>;
    force?: InputMaybe<Scalars['Boolean']['input']>;
}>;
export type RequestAiAnalysisMutation = {
    readonly __typename: 'Mutation';
    readonly requestAIAnalysis: {
        readonly __typename: 'AIAnalysisRequest';
        readonly message: string;
        readonly objectId: string;
        readonly estimatedTime: string;
    };
};
export type AiAnalysisQueryVariables = Exact<{
    objectId: Scalars['ID']['input'];
}>;
export type AiAnalysisQuery = {
    readonly __typename: 'Query';
    readonly aiAnalysis?: {
        readonly __typename: 'AIAnalysis';
        readonly id: string;
        readonly objectId: string;
        readonly objectType: string;
        readonly overallRisk: number;
        readonly moderationAction: ModerationAction;
        readonly confidence: number;
        readonly analyzedAt: string;
        readonly textAnalysis?: {
            readonly __typename: 'TextAnalysis';
            readonly sentiment: Sentiment;
            readonly toxicityScore: number;
            readonly toxicityLabels: ReadonlyArray<string>;
            readonly containsPII: boolean;
            readonly dominantLanguage: string;
            readonly keyPhrases: ReadonlyArray<string>;
            readonly sentimentScores: {
                readonly __typename: 'SentimentScores';
                readonly positive: number;
                readonly negative: number;
                readonly neutral: number;
                readonly mixed: number;
            };
            readonly entities: ReadonlyArray<{
                readonly __typename: 'Entity';
                readonly text: string;
                readonly type: string;
                readonly score: number;
            }>;
        } | null | undefined;
        readonly imageAnalysis?: {
            readonly __typename: 'ImageAnalysis';
            readonly isNSFW: boolean;
            readonly nsfwConfidence: number;
            readonly violenceScore: number;
            readonly weaponsDetected: boolean;
            readonly detectedText: ReadonlyArray<string>;
            readonly textToxicity: number;
            readonly deepfakeScore: number;
            readonly moderationLabels: ReadonlyArray<{
                readonly __typename: 'ModerationLabel';
                readonly name: string;
                readonly confidence: number;
                readonly parentName?: string | null | undefined;
            }>;
            readonly celebrityFaces: ReadonlyArray<{
                readonly __typename: 'Celebrity';
                readonly name: string;
                readonly confidence: number;
            }>;
        } | null | undefined;
        readonly aiDetection?: {
            readonly __typename: 'AIDetection';
            readonly aiGeneratedProbability: number;
            readonly generationModel?: string | null | undefined;
            readonly patternConsistency: number;
            readonly styleDeviation: number;
            readonly semanticCoherence: number;
            readonly suspiciousPatterns: ReadonlyArray<string>;
        } | null | undefined;
        readonly spamAnalysis?: {
            readonly __typename: 'SpamAnalysis';
            readonly spamScore: number;
            readonly postingVelocity: number;
            readonly repetitionScore: number;
            readonly linkDensity: number;
            readonly followerRatio: number;
            readonly interactionRate: number;
            readonly accountAgeDays: number;
            readonly spamIndicators: ReadonlyArray<{
                readonly __typename: 'SpamIndicator';
                readonly type: string;
                readonly description: string;
                readonly severity: number;
            }>;
        } | null | undefined;
    } | null | undefined;
};
export type AiStatsQueryVariables = Exact<{
    period: Period;
}>;
export type AiStatsQuery = {
    readonly __typename: 'Query';
    readonly aiStats: {
        readonly __typename: 'AIStats';
        readonly period: string;
        readonly totalAnalyses: number;
        readonly toxicContent: number;
        readonly spamDetected: number;
        readonly aiGenerated: number;
        readonly nsfwContent: number;
        readonly piiDetected: number;
        readonly toxicityRate: number;
        readonly spamRate: number;
        readonly aiContentRate: number;
        readonly nsfwRate: number;
        readonly moderationActions: {
            readonly __typename: 'ModerationActionCounts';
            readonly flag: number;
            readonly hide: number;
            readonly remove: number;
            readonly review: number;
            readonly shadowBan: number;
        };
    };
};
export type AiCapabilitiesQueryVariables = Exact<{
    [key: string]: never;
}>;
export type AiCapabilitiesQuery = {
    readonly __typename: 'Query';
    readonly aiCapabilities: {
        readonly __typename: 'AICapabilities';
        readonly moderationActions: ReadonlyArray<string>;
        readonly textAnalysis: {
            readonly __typename: 'TextAnalysisCapabilities';
            readonly sentimentAnalysis: boolean;
            readonly toxicityDetection: boolean;
            readonly spamDetection: boolean;
            readonly piiDetection: boolean;
            readonly entityExtraction: boolean;
            readonly languageDetection: boolean;
        };
        readonly imageAnalysis: {
            readonly __typename: 'ImageAnalysisCapabilities';
            readonly nsfwDetection: boolean;
            readonly violenceDetection: boolean;
            readonly textExtraction: boolean;
            readonly celebrityRecognition: boolean;
            readonly deepfakeDetection: boolean;
        };
        readonly aiDetection: {
            readonly __typename: 'AIDetectionCapabilities';
            readonly aiGeneratedContent: boolean;
            readonly patternAnalysis: boolean;
            readonly styleConsistency: boolean;
        };
        readonly costPerAnalysis: {
            readonly __typename: 'CostBreakdown';
            readonly period: Period;
            readonly totalCost: number;
            readonly dynamoDBCost: number;
            readonly s3StorageCost: number;
            readonly lambdaCost: number;
            readonly dataTransferCost: number;
            readonly breakdown: ReadonlyArray<{
                readonly __typename: 'CostItem';
                readonly operation: string;
                readonly count: number;
                readonly cost: number;
            }>;
        };
    };
};
export type ConversationsQueryVariables = Exact<{
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['Cursor']['input']>;
}>;
export type ConversationsQuery = {
    readonly __typename: 'Query';
    readonly conversations: ReadonlyArray<{
        readonly __typename: 'Conversation';
        readonly id: string;
        readonly unread: boolean;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
        readonly lastStatus?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
    }>;
};
export type ConversationQueryVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type ConversationQuery = {
    readonly __typename: 'Query';
    readonly conversation?: {
        readonly __typename: 'Conversation';
        readonly id: string;
        readonly unread: boolean;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
        readonly lastStatus?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
    } | null | undefined;
};
export type MarkConversationReadMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type MarkConversationReadMutation = {
    readonly __typename: 'Mutation';
    readonly markConversationAsRead: {
        readonly __typename: 'Conversation';
        readonly id: string;
        readonly unread: boolean;
        readonly updatedAt: string;
    };
};
export type DeleteConversationMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type DeleteConversationMutation = {
    readonly __typename: 'Mutation';
    readonly deleteConversation: boolean;
};
export type CostBreakdownQueryVariables = Exact<{
    period?: InputMaybe<Period>;
}>;
export type CostBreakdownQuery = {
    readonly __typename: 'Query';
    readonly costBreakdown: {
        readonly __typename: 'CostBreakdown';
        readonly period: Period;
        readonly totalCost: number;
        readonly dynamoDBCost: number;
        readonly s3StorageCost: number;
        readonly lambdaCost: number;
        readonly dataTransferCost: number;
        readonly breakdown: ReadonlyArray<{
            readonly __typename: 'CostItem';
            readonly operation: string;
            readonly count: number;
            readonly cost: number;
        }>;
    };
};
export type InstanceBudgetsQueryVariables = Exact<{
    [key: string]: never;
}>;
export type InstanceBudgetsQuery = {
    readonly __typename: 'Query';
    readonly instanceBudgets: ReadonlyArray<{
        readonly __typename: 'InstanceBudget';
        readonly domain: string;
        readonly monthlyBudgetUSD: number;
        readonly currentSpendUSD: number;
        readonly remainingBudgetUSD: number;
        readonly projectedOverspend?: number | null | undefined;
        readonly alertThreshold: number;
        readonly autoLimit: boolean;
        readonly period: string;
    }>;
};
export type SetInstanceBudgetMutationVariables = Exact<{
    domain: Scalars['String']['input'];
    monthlyUSD: Scalars['Float']['input'];
    autoLimit?: InputMaybe<Scalars['Boolean']['input']>;
}>;
export type SetInstanceBudgetMutation = {
    readonly __typename: 'Mutation';
    readonly setInstanceBudget: {
        readonly __typename: 'InstanceBudget';
        readonly domain: string;
        readonly monthlyBudgetUSD: number;
        readonly currentSpendUSD: number;
        readonly remainingBudgetUSD: number;
        readonly projectedOverspend?: number | null | undefined;
        readonly alertThreshold: number;
        readonly autoLimit: boolean;
        readonly period: string;
    };
};
export type OptimizeFederationCostsMutationVariables = Exact<{
    threshold: Scalars['Float']['input'];
}>;
export type OptimizeFederationCostsMutation = {
    readonly __typename: 'Mutation';
    readonly optimizeFederationCosts: {
        readonly __typename: 'CostOptimizationResult';
        readonly optimized: number;
        readonly savedMonthlyUSD: number;
        readonly actions: ReadonlyArray<{
            readonly __typename: 'OptimizationAction';
            readonly domain: string;
            readonly action: string;
            readonly savingsUSD: number;
            readonly impact: string;
        }>;
    };
};
export type FederationLimitsQueryVariables = Exact<{
    [key: string]: never;
}>;
export type FederationLimitsQuery = {
    readonly __typename: 'Query';
    readonly federationLimits: ReadonlyArray<{
        readonly __typename: 'FederationLimit';
        readonly domain: string;
        readonly ingressLimitMB: number;
        readonly egressLimitMB: number;
        readonly requestsPerMinute: number;
        readonly monthlyBudgetUSD?: number | null | undefined;
        readonly active: boolean;
        readonly createdAt: string;
        readonly updatedAt: string;
    }>;
};
export type SetFederationLimitMutationVariables = Exact<{
    domain: Scalars['String']['input'];
    limit: FederationLimitInput;
}>;
export type SetFederationLimitMutation = {
    readonly __typename: 'Mutation';
    readonly setFederationLimit: {
        readonly __typename: 'FederationLimit';
        readonly domain: string;
        readonly ingressLimitMB: number;
        readonly egressLimitMB: number;
        readonly requestsPerMinute: number;
        readonly monthlyBudgetUSD?: number | null | undefined;
        readonly active: boolean;
        readonly createdAt: string;
        readonly updatedAt: string;
    };
};
export type SyncThreadMutationVariables = Exact<{
    noteUrl: Scalars['String']['input'];
    depth?: InputMaybe<Scalars['Int']['input']>;
}>;
export type SyncThreadMutation = {
    readonly __typename: 'Mutation';
    readonly syncThread: {
        readonly __typename: 'SyncThreadPayload';
        readonly success: boolean;
        readonly syncedPosts: number;
        readonly errors?: ReadonlyArray<string> | null | undefined;
        readonly thread: {
            readonly __typename: 'ThreadContext';
            readonly replyCount: number;
            readonly participantCount: number;
            readonly lastActivity: string;
            readonly missingPosts: number;
            readonly syncStatus: SyncStatus;
            readonly rootNote: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly boostedObject?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly content: string;
                    readonly visibility: Visibility;
                    readonly sensitive: boolean;
                    readonly spoilerText?: string | null | undefined;
                    readonly createdAt: string;
                    readonly updatedAt: string;
                    readonly repliesCount: number;
                    readonly likesCount: number;
                    readonly sharesCount: number;
                    readonly estimatedCost: number;
                    readonly moderationScore?: number | null | undefined;
                    readonly quoteUrl?: string | null | undefined;
                    readonly quoteable: boolean;
                    readonly quotePermissions: QuotePermission;
                    readonly quoteCount: number;
                    readonly contentMap: ReadonlyArray<{
                        readonly __typename: 'ContentMap';
                        readonly language: string;
                        readonly content: string;
                    }>;
                    readonly attachments: ReadonlyArray<{
                        readonly __typename: 'Attachment';
                        readonly id: string;
                        readonly type: string;
                        readonly url: string;
                        readonly preview?: string | null | undefined;
                        readonly description?: string | null | undefined;
                        readonly blurhash?: string | null | undefined;
                        readonly width?: number | null | undefined;
                        readonly height?: number | null | undefined;
                        readonly duration?: number | null | undefined;
                    }>;
                    readonly tags: ReadonlyArray<{
                        readonly __typename: 'Tag';
                        readonly name: string;
                        readonly url: string;
                    }>;
                    readonly mentions: ReadonlyArray<{
                        readonly __typename: 'Mention';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly url: string;
                    }>;
                    readonly quoteContext?: {
                        readonly __typename: 'QuoteContext';
                        readonly quoteAllowed: boolean;
                        readonly quoteType: QuoteType;
                        readonly withdrawn: boolean;
                        readonly originalAuthor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly originalNote?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                        } | null | undefined;
                    } | null | undefined;
                    readonly communityNotes: ReadonlyArray<{
                        readonly __typename: 'CommunityNote';
                        readonly id: string;
                        readonly content: string;
                        readonly helpful: number;
                        readonly notHelpful: number;
                        readonly createdAt: string;
                        readonly author: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    }>;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly inReplyTo?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    } | null | undefined;
                } | null | undefined;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            };
        };
    };
};
export type SyncMissingRepliesMutationVariables = Exact<{
    noteId: Scalars['ID']['input'];
}>;
export type SyncMissingRepliesMutation = {
    readonly __typename: 'Mutation';
    readonly syncMissingReplies: {
        readonly __typename: 'SyncRepliesPayload';
        readonly success: boolean;
        readonly syncedReplies: number;
        readonly thread: {
            readonly __typename: 'ThreadContext';
            readonly replyCount: number;
            readonly participantCount: number;
            readonly lastActivity: string;
            readonly missingPosts: number;
            readonly syncStatus: SyncStatus;
            readonly rootNote: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly boostedObject?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly content: string;
                    readonly visibility: Visibility;
                    readonly sensitive: boolean;
                    readonly spoilerText?: string | null | undefined;
                    readonly createdAt: string;
                    readonly updatedAt: string;
                    readonly repliesCount: number;
                    readonly likesCount: number;
                    readonly sharesCount: number;
                    readonly estimatedCost: number;
                    readonly moderationScore?: number | null | undefined;
                    readonly quoteUrl?: string | null | undefined;
                    readonly quoteable: boolean;
                    readonly quotePermissions: QuotePermission;
                    readonly quoteCount: number;
                    readonly contentMap: ReadonlyArray<{
                        readonly __typename: 'ContentMap';
                        readonly language: string;
                        readonly content: string;
                    }>;
                    readonly attachments: ReadonlyArray<{
                        readonly __typename: 'Attachment';
                        readonly id: string;
                        readonly type: string;
                        readonly url: string;
                        readonly preview?: string | null | undefined;
                        readonly description?: string | null | undefined;
                        readonly blurhash?: string | null | undefined;
                        readonly width?: number | null | undefined;
                        readonly height?: number | null | undefined;
                        readonly duration?: number | null | undefined;
                    }>;
                    readonly tags: ReadonlyArray<{
                        readonly __typename: 'Tag';
                        readonly name: string;
                        readonly url: string;
                    }>;
                    readonly mentions: ReadonlyArray<{
                        readonly __typename: 'Mention';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly url: string;
                    }>;
                    readonly quoteContext?: {
                        readonly __typename: 'QuoteContext';
                        readonly quoteAllowed: boolean;
                        readonly quoteType: QuoteType;
                        readonly withdrawn: boolean;
                        readonly originalAuthor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly originalNote?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                        } | null | undefined;
                    } | null | undefined;
                    readonly communityNotes: ReadonlyArray<{
                        readonly __typename: 'CommunityNote';
                        readonly id: string;
                        readonly content: string;
                        readonly helpful: number;
                        readonly notHelpful: number;
                        readonly createdAt: string;
                        readonly author: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    }>;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly inReplyTo?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    } | null | undefined;
                } | null | undefined;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            };
        };
    };
};
export type ThreadContextQueryVariables = Exact<{
    noteId: Scalars['ID']['input'];
}>;
export type ThreadContextQuery = {
    readonly __typename: 'Query';
    readonly threadContext?: {
        readonly __typename: 'ThreadContext';
        readonly replyCount: number;
        readonly participantCount: number;
        readonly lastActivity: string;
        readonly missingPosts: number;
        readonly syncStatus: SyncStatus;
        readonly rootNote: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
    } | null | undefined;
};
export type SeveredRelationshipsQueryVariables = Exact<{
    instance?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['String']['input']>;
}>;
export type SeveredRelationshipsQuery = {
    readonly __typename: 'Query';
    readonly severedRelationships: {
        readonly __typename: 'SeveredRelationshipConnection';
        readonly totalCount: number;
        readonly edges: ReadonlyArray<{
            readonly __typename: 'SeveredRelationshipEdge';
            readonly cursor: string;
            readonly node: {
                readonly __typename: 'SeveredRelationship';
                readonly id: string;
                readonly localInstance: string;
                readonly remoteInstance: string;
                readonly reason: SeveranceReason;
                readonly affectedFollowers: number;
                readonly affectedFollowing: number;
                readonly timestamp: string;
                readonly reversible: boolean;
                readonly details?: {
                    readonly __typename: 'SeveranceDetails';
                    readonly description: string;
                    readonly metadata: ReadonlyArray<string>;
                    readonly adminNotes?: string | null | undefined;
                    readonly autoDetected: boolean;
                } | null | undefined;
            };
        }>;
        readonly pageInfo: {
            readonly __typename: 'PageInfo';
            readonly hasNextPage: boolean;
            readonly hasPreviousPage: boolean;
            readonly startCursor?: string | null | undefined;
            readonly endCursor?: string | null | undefined;
        };
    };
};
export type AcknowledgeSeveranceMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type AcknowledgeSeveranceMutation = {
    readonly __typename: 'Mutation';
    readonly acknowledgeSeverance: {
        readonly __typename: 'AcknowledgePayload';
        readonly success: boolean;
        readonly acknowledged: boolean;
        readonly severedRelationship: {
            readonly __typename: 'SeveredRelationship';
            readonly id: string;
            readonly localInstance: string;
            readonly remoteInstance: string;
        };
    };
};
export type AttemptReconnectionMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type AttemptReconnectionMutation = {
    readonly __typename: 'Mutation';
    readonly attemptReconnection: {
        readonly __typename: 'ReconnectionPayload';
        readonly success: boolean;
        readonly reconnected: number;
        readonly failed: number;
        readonly errors?: ReadonlyArray<string> | null | undefined;
        readonly severedRelationship: {
            readonly __typename: 'SeveredRelationship';
            readonly id: string;
            readonly localInstance: string;
            readonly remoteInstance: string;
        };
    };
};
export type FederationHealthQueryVariables = Exact<{
    threshold?: InputMaybe<Scalars['Float']['input']>;
}>;
export type FederationHealthQuery = {
    readonly __typename: 'Query';
    readonly federationHealth: ReadonlyArray<{
        readonly __typename: 'FederationManagementStatus';
        readonly domain: string;
        readonly status: FederationState;
        readonly reason?: string | null | undefined;
        readonly pausedUntil?: string | null | undefined;
    }>;
};
export type FederationStatusQueryVariables = Exact<{
    domain: Scalars['String']['input'];
}>;
export type FederationStatusQuery = {
    readonly __typename: 'Query';
    readonly federationStatus: {
        readonly __typename: 'FederationStatus';
        readonly domain: string;
        readonly reachable: boolean;
        readonly lastContact?: string | null | undefined;
        readonly sharedInbox?: string | null | undefined;
        readonly publicKey?: string | null | undefined;
        readonly software?: string | null | undefined;
        readonly version?: string | null | undefined;
    };
};
export type PauseFederationMutationVariables = Exact<{
    domain: Scalars['String']['input'];
    reason: Scalars['String']['input'];
    until?: InputMaybe<Scalars['Time']['input']>;
}>;
export type PauseFederationMutation = {
    readonly __typename: 'Mutation';
    readonly pauseFederation: {
        readonly __typename: 'FederationManagementStatus';
        readonly domain: string;
        readonly status: FederationState;
        readonly reason?: string | null | undefined;
        readonly pausedUntil?: string | null | undefined;
        readonly limits?: {
            readonly __typename: 'FederationLimit';
            readonly domain: string;
            readonly ingressLimitMB: number;
            readonly egressLimitMB: number;
            readonly requestsPerMinute: number;
            readonly monthlyBudgetUSD?: number | null | undefined;
            readonly active: boolean;
            readonly createdAt: string;
            readonly updatedAt: string;
        } | null | undefined;
    };
};
export type ResumeFederationMutationVariables = Exact<{
    domain: Scalars['String']['input'];
}>;
export type ResumeFederationMutation = {
    readonly __typename: 'Mutation';
    readonly resumeFederation: {
        readonly __typename: 'FederationManagementStatus';
        readonly domain: string;
        readonly status: FederationState;
        readonly reason?: string | null | undefined;
        readonly pausedUntil?: string | null | undefined;
        readonly limits?: {
            readonly __typename: 'FederationLimit';
            readonly domain: string;
            readonly ingressLimitMB: number;
            readonly egressLimitMB: number;
            readonly requestsPerMinute: number;
            readonly monthlyBudgetUSD?: number | null | undefined;
            readonly active: boolean;
            readonly createdAt: string;
            readonly updatedAt: string;
        } | null | undefined;
    };
};
export type FieldFieldsFragment = {
    readonly __typename: 'Field';
    readonly name: string;
    readonly value: string;
    readonly verifiedAt?: string | null | undefined;
};
export type ActorSummaryFragment = {
    readonly __typename: 'Actor';
    readonly id: string;
    readonly username: string;
    readonly domain?: string | null | undefined;
    readonly displayName?: string | null | undefined;
    readonly summary?: string | null | undefined;
    readonly avatar?: string | null | undefined;
    readonly header?: string | null | undefined;
    readonly followers: number;
    readonly following: number;
    readonly statusesCount: number;
    readonly bot: boolean;
    readonly locked: boolean;
    readonly updatedAt: string;
    readonly trustScore: number;
    readonly fields: ReadonlyArray<{
        readonly __typename: 'Field';
        readonly name: string;
        readonly value: string;
        readonly verifiedAt?: string | null | undefined;
    }>;
};
export type AttachmentFieldsFragment = {
    readonly __typename: 'Attachment';
    readonly id: string;
    readonly type: string;
    readonly url: string;
    readonly preview?: string | null | undefined;
    readonly description?: string | null | undefined;
    readonly blurhash?: string | null | undefined;
    readonly width?: number | null | undefined;
    readonly height?: number | null | undefined;
    readonly duration?: number | null | undefined;
};
export type TagFieldsFragment = {
    readonly __typename: 'Tag';
    readonly name: string;
    readonly url: string;
};
export type MentionFieldsFragment = {
    readonly __typename: 'Mention';
    readonly id: string;
    readonly username: string;
    readonly domain?: string | null | undefined;
    readonly url: string;
};
export type CommunityNoteFieldsFragment = {
    readonly __typename: 'CommunityNote';
    readonly id: string;
    readonly content: string;
    readonly helpful: number;
    readonly notHelpful: number;
    readonly createdAt: string;
    readonly author: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    };
};
export type QuoteContextFieldsFragment = {
    readonly __typename: 'QuoteContext';
    readonly quoteAllowed: boolean;
    readonly quoteType: QuoteType;
    readonly withdrawn: boolean;
    readonly originalAuthor: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    };
    readonly originalNote?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
    } | null | undefined;
};
export type ObjectContentFieldsFragment = {
    readonly __typename: 'Object';
    readonly id: string;
    readonly type: ObjectType;
    readonly content: string;
    readonly visibility: Visibility;
    readonly sensitive: boolean;
    readonly spoilerText?: string | null | undefined;
    readonly createdAt: string;
    readonly updatedAt: string;
    readonly repliesCount: number;
    readonly likesCount: number;
    readonly sharesCount: number;
    readonly estimatedCost: number;
    readonly moderationScore?: number | null | undefined;
    readonly quoteUrl?: string | null | undefined;
    readonly quoteable: boolean;
    readonly quotePermissions: QuotePermission;
    readonly quoteCount: number;
    readonly contentMap: ReadonlyArray<{
        readonly __typename: 'ContentMap';
        readonly language: string;
        readonly content: string;
    }>;
    readonly attachments: ReadonlyArray<{
        readonly __typename: 'Attachment';
        readonly id: string;
        readonly type: string;
        readonly url: string;
        readonly preview?: string | null | undefined;
        readonly description?: string | null | undefined;
        readonly blurhash?: string | null | undefined;
        readonly width?: number | null | undefined;
        readonly height?: number | null | undefined;
        readonly duration?: number | null | undefined;
    }>;
    readonly tags: ReadonlyArray<{
        readonly __typename: 'Tag';
        readonly name: string;
        readonly url: string;
    }>;
    readonly mentions: ReadonlyArray<{
        readonly __typename: 'Mention';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly url: string;
    }>;
    readonly quoteContext?: {
        readonly __typename: 'QuoteContext';
        readonly quoteAllowed: boolean;
        readonly quoteType: QuoteType;
        readonly withdrawn: boolean;
        readonly originalAuthor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly originalNote?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        } | null | undefined;
    } | null | undefined;
    readonly communityNotes: ReadonlyArray<{
        readonly __typename: 'CommunityNote';
        readonly id: string;
        readonly content: string;
        readonly helpful: number;
        readonly notHelpful: number;
        readonly createdAt: string;
        readonly author: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    }>;
    readonly actor: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    };
    readonly inReplyTo?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    } | null | undefined;
};
export type ObjectFieldsFragment = {
    readonly __typename: 'Object';
    readonly id: string;
    readonly type: ObjectType;
    readonly content: string;
    readonly visibility: Visibility;
    readonly sensitive: boolean;
    readonly spoilerText?: string | null | undefined;
    readonly createdAt: string;
    readonly updatedAt: string;
    readonly repliesCount: number;
    readonly likesCount: number;
    readonly sharesCount: number;
    readonly estimatedCost: number;
    readonly moderationScore?: number | null | undefined;
    readonly quoteUrl?: string | null | undefined;
    readonly quoteable: boolean;
    readonly quotePermissions: QuotePermission;
    readonly quoteCount: number;
    readonly boostedObject?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    } | null | undefined;
    readonly contentMap: ReadonlyArray<{
        readonly __typename: 'ContentMap';
        readonly language: string;
        readonly content: string;
    }>;
    readonly attachments: ReadonlyArray<{
        readonly __typename: 'Attachment';
        readonly id: string;
        readonly type: string;
        readonly url: string;
        readonly preview?: string | null | undefined;
        readonly description?: string | null | undefined;
        readonly blurhash?: string | null | undefined;
        readonly width?: number | null | undefined;
        readonly height?: number | null | undefined;
        readonly duration?: number | null | undefined;
    }>;
    readonly tags: ReadonlyArray<{
        readonly __typename: 'Tag';
        readonly name: string;
        readonly url: string;
    }>;
    readonly mentions: ReadonlyArray<{
        readonly __typename: 'Mention';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly url: string;
    }>;
    readonly quoteContext?: {
        readonly __typename: 'QuoteContext';
        readonly quoteAllowed: boolean;
        readonly quoteType: QuoteType;
        readonly withdrawn: boolean;
        readonly originalAuthor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly originalNote?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        } | null | undefined;
    } | null | undefined;
    readonly communityNotes: ReadonlyArray<{
        readonly __typename: 'CommunityNote';
        readonly id: string;
        readonly content: string;
        readonly helpful: number;
        readonly notHelpful: number;
        readonly createdAt: string;
        readonly author: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    }>;
    readonly actor: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    };
    readonly inReplyTo?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    } | null | undefined;
};
export type ActivityFieldsFragment = {
    readonly __typename: 'Activity';
    readonly id: string;
    readonly type: ActivityType;
    readonly published: string;
    readonly cost: number;
    readonly actor: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    };
    readonly object?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
    } | null | undefined;
    readonly target?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
    } | null | undefined;
};
export type CostUpdateFieldsFragment = {
    readonly __typename: 'CostUpdate';
    readonly operationCost: number;
    readonly dailyTotal: number;
    readonly monthlyProjection: number;
};
export type FollowHashtagMutationVariables = Exact<{
    hashtag: Scalars['String']['input'];
    notifyLevel?: InputMaybe<NotificationLevel>;
}>;
export type FollowHashtagMutation = {
    readonly __typename: 'Mutation';
    readonly followHashtag: {
        readonly __typename: 'HashtagFollowPayload';
        readonly success: boolean;
        readonly hashtag: {
            readonly __typename: 'Hashtag';
            readonly name: string;
            readonly url: string;
            readonly isFollowing: boolean;
            readonly followedAt?: string | null | undefined;
            readonly notificationSettings?: {
                readonly __typename: 'HashtagNotificationSettings';
                readonly level: NotificationLevel;
                readonly muted: boolean;
                readonly mutedUntil?: string | null | undefined;
            } | null | undefined;
        };
    };
};
export type UnfollowHashtagMutationVariables = Exact<{
    hashtag: Scalars['String']['input'];
}>;
export type UnfollowHashtagMutation = {
    readonly __typename: 'Mutation';
    readonly unfollowHashtag: {
        readonly __typename: 'UnfollowHashtagPayload';
        readonly success: boolean;
        readonly hashtag: {
            readonly __typename: 'Hashtag';
            readonly name: string;
            readonly url: string;
        };
    };
};
export type MuteHashtagMutationVariables = Exact<{
    hashtag: Scalars['String']['input'];
    until?: InputMaybe<Scalars['Time']['input']>;
}>;
export type MuteHashtagMutation = {
    readonly __typename: 'Mutation';
    readonly muteHashtag: {
        readonly __typename: 'MuteHashtagPayload';
        readonly success: boolean;
        readonly mutedUntil?: string | null | undefined;
        readonly hashtag: {
            readonly __typename: 'Hashtag';
            readonly name: string;
            readonly notificationSettings?: {
                readonly __typename: 'HashtagNotificationSettings';
                readonly muted: boolean;
                readonly mutedUntil?: string | null | undefined;
            } | null | undefined;
        };
    };
};
export type FollowedHashtagsQueryVariables = Exact<{
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['String']['input']>;
}>;
export type FollowedHashtagsQuery = {
    readonly __typename: 'Query';
    readonly followedHashtags: {
        readonly __typename: 'HashtagConnection';
        readonly totalCount: number;
        readonly edges: ReadonlyArray<{
            readonly __typename: 'HashtagEdge';
            readonly cursor: string;
            readonly node: {
                readonly __typename: 'Hashtag';
                readonly name: string;
                readonly url: string;
                readonly isFollowing: boolean;
                readonly followedAt?: string | null | undefined;
                readonly notificationSettings?: {
                    readonly __typename: 'HashtagNotificationSettings';
                    readonly level: NotificationLevel;
                    readonly muted: boolean;
                    readonly mutedUntil?: string | null | undefined;
                } | null | undefined;
            };
        }>;
        readonly pageInfo: {
            readonly __typename: 'PageInfo';
            readonly hasNextPage: boolean;
            readonly hasPreviousPage: boolean;
            readonly startCursor?: string | null | undefined;
            readonly endCursor?: string | null | undefined;
        };
    };
};
export type ListsQueryVariables = Exact<{
    [key: string]: never;
}>;
export type ListsQuery = {
    readonly __typename: 'Query';
    readonly lists: ReadonlyArray<{
        readonly __typename: 'List';
        readonly id: string;
        readonly title: string;
        readonly repliesPolicy: RepliesPolicy;
        readonly exclusive: boolean;
        readonly accountCount: number;
        readonly createdAt: string;
        readonly updatedAt: string;
    }>;
};
export type ListQueryVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type ListQuery = {
    readonly __typename: 'Query';
    readonly list?: {
        readonly __typename: 'List';
        readonly id: string;
        readonly title: string;
        readonly repliesPolicy: RepliesPolicy;
        readonly exclusive: boolean;
        readonly accountCount: number;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
    } | null | undefined;
};
export type ListAccountsQueryVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type ListAccountsQuery = {
    readonly __typename: 'Query';
    readonly listAccounts: ReadonlyArray<{
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    }>;
};
export type CreateListMutationVariables = Exact<{
    input: CreateListInput;
}>;
export type CreateListMutation = {
    readonly __typename: 'Mutation';
    readonly createList: {
        readonly __typename: 'List';
        readonly id: string;
        readonly title: string;
        readonly repliesPolicy: RepliesPolicy;
        readonly exclusive: boolean;
        readonly accountCount: number;
        readonly createdAt: string;
        readonly updatedAt: string;
    };
};
export type UpdateListMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    input: UpdateListInput;
}>;
export type UpdateListMutation = {
    readonly __typename: 'Mutation';
    readonly updateList: {
        readonly __typename: 'List';
        readonly id: string;
        readonly title: string;
        readonly repliesPolicy: RepliesPolicy;
        readonly exclusive: boolean;
        readonly accountCount: number;
        readonly createdAt: string;
        readonly updatedAt: string;
    };
};
export type DeleteListMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type DeleteListMutation = {
    readonly __typename: 'Mutation';
    readonly deleteList: boolean;
};
export type AddAccountsToListMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    accountIds: ReadonlyArray<Scalars['ID']['input']> | Scalars['ID']['input'];
}>;
export type AddAccountsToListMutation = {
    readonly __typename: 'Mutation';
    readonly addAccountsToList: {
        readonly __typename: 'List';
        readonly id: string;
        readonly accountCount: number;
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
    };
};
export type RemoveAccountsFromListMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    accountIds: ReadonlyArray<Scalars['ID']['input']> | Scalars['ID']['input'];
}>;
export type RemoveAccountsFromListMutation = {
    readonly __typename: 'Mutation';
    readonly removeAccountsFromList: {
        readonly __typename: 'List';
        readonly id: string;
        readonly accountCount: number;
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
    };
};
export type MediaQueryVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type MediaQuery = {
    readonly __typename: 'Query';
    readonly media?: {
        readonly __typename: 'Media';
        readonly id: string;
        readonly type: MediaType;
        readonly url: string;
        readonly previewUrl?: string | null | undefined;
        readonly description?: string | null | undefined;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly mediaCategory: MediaCategory;
        readonly blurhash?: string | null | undefined;
        readonly width?: number | null | undefined;
        readonly height?: number | null | undefined;
        readonly duration?: number | null | undefined;
        readonly size: number;
        readonly mimeType: string;
        readonly createdAt: string;
        readonly uploadedBy: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    } | null | undefined;
};
export type UpdateMediaMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    input: UpdateMediaInput;
}>;
export type UpdateMediaMutation = {
    readonly __typename: 'Mutation';
    readonly updateMedia: {
        readonly __typename: 'Media';
        readonly id: string;
        readonly description?: string | null | undefined;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly mediaCategory: MediaCategory;
        readonly blurhash?: string | null | undefined;
        readonly width?: number | null | undefined;
        readonly height?: number | null | undefined;
        readonly duration?: number | null | undefined;
        readonly url: string;
        readonly previewUrl?: string | null | undefined;
    };
};
export type UploadMediaMutationVariables = Exact<{
    input: UploadMediaInput;
}>;
export type UploadMediaMutation = {
    readonly __typename: 'Mutation';
    readonly uploadMedia: {
        readonly __typename: 'UploadMediaPayload';
        readonly uploadId: string;
        readonly warnings?: ReadonlyArray<string> | null | undefined;
        readonly media: {
            readonly __typename: 'Media';
            readonly id: string;
            readonly type: MediaType;
            readonly url: string;
            readonly previewUrl?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly mediaCategory: MediaCategory;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
            readonly size: number;
            readonly mimeType: string;
            readonly createdAt: string;
            readonly uploadedBy: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        };
    };
};
export type AddCommunityNoteMutationVariables = Exact<{
    input: CommunityNoteInput;
}>;
export type AddCommunityNoteMutation = {
    readonly __typename: 'Mutation';
    readonly addCommunityNote: {
        readonly __typename: 'CommunityNotePayload';
        readonly note: {
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        };
        readonly object: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        };
    };
};
export type VoteCommunityNoteMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    helpful: Scalars['Boolean']['input'];
}>;
export type VoteCommunityNoteMutation = {
    readonly __typename: 'Mutation';
    readonly voteCommunityNote: {
        readonly __typename: 'CommunityNote';
        readonly id: string;
        readonly content: string;
        readonly helpful: number;
        readonly notHelpful: number;
        readonly createdAt: string;
        readonly author: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    };
};
export type CommunityNotesByObjectQueryVariables = Exact<{
    objectId: Scalars['ID']['input'];
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['Cursor']['input']>;
}>;
export type CommunityNotesByObjectQuery = {
    readonly __typename: 'Query';
    readonly object?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    } | null | undefined;
};
export type FlagObjectMutationVariables = Exact<{
    input: FlagInput;
}>;
export type FlagObjectMutation = {
    readonly __typename: 'Mutation';
    readonly flagObject: {
        readonly __typename: 'FlagPayload';
        readonly moderationId: string;
        readonly queued: boolean;
    };
};
export type CreateModerationPatternMutationVariables = Exact<{
    input: ModerationPatternInput;
}>;
export type CreateModerationPatternMutation = {
    readonly __typename: 'Mutation';
    readonly createModerationPattern: {
        readonly __typename: 'ModerationPattern';
        readonly id: string;
        readonly pattern: string;
        readonly type: PatternType;
        readonly severity: ModerationSeverity;
        readonly matchCount: number;
        readonly falsePositiveRate: number;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly active: boolean;
        readonly createdBy: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    };
};
export type DeleteModerationPatternMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type DeleteModerationPatternMutation = {
    readonly __typename: 'Mutation';
    readonly deleteModerationPattern: boolean;
};
export type CreateNoteMutationVariables = Exact<{
    input: CreateNoteInput;
}>;
export type CreateNoteMutation = {
    readonly __typename: 'Mutation';
    readonly createNote: {
        readonly __typename: 'CreateNotePayload';
        readonly object: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
        readonly activity: {
            readonly __typename: 'Activity';
            readonly id: string;
            readonly type: ActivityType;
            readonly published: string;
            readonly cost: number;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly object?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
            readonly target?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        };
        readonly cost: {
            readonly __typename: 'CostUpdate';
            readonly operationCost: number;
            readonly dailyTotal: number;
            readonly monthlyProjection: number;
        };
    };
};
export type CreateQuoteNoteMutationVariables = Exact<{
    input: CreateQuoteNoteInput;
}>;
export type CreateQuoteNoteMutation = {
    readonly __typename: 'Mutation';
    readonly createQuoteNote: {
        readonly __typename: 'CreateNotePayload';
        readonly object: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
        readonly activity: {
            readonly __typename: 'Activity';
            readonly id: string;
            readonly type: ActivityType;
            readonly published: string;
            readonly cost: number;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly object?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
            readonly target?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        };
        readonly cost: {
            readonly __typename: 'CostUpdate';
            readonly operationCost: number;
            readonly dailyTotal: number;
            readonly monthlyProjection: number;
        };
    };
};
export type WithdrawFromQuotesMutationVariables = Exact<{
    noteId: Scalars['ID']['input'];
}>;
export type WithdrawFromQuotesMutation = {
    readonly __typename: 'Mutation';
    readonly withdrawFromQuotes: {
        readonly __typename: 'WithdrawQuotePayload';
        readonly success: boolean;
        readonly withdrawnCount: number;
        readonly note: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
    };
};
export type UpdateQuotePermissionsMutationVariables = Exact<{
    noteId: Scalars['ID']['input'];
    quoteable: Scalars['Boolean']['input'];
    permission: QuotePermission;
}>;
export type UpdateQuotePermissionsMutation = {
    readonly __typename: 'Mutation';
    readonly updateQuotePermissions: {
        readonly __typename: 'UpdateQuotePermissionsPayload';
        readonly success: boolean;
        readonly affectedQuotes: number;
        readonly note: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
    };
};
export type DeleteObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type DeleteObjectMutation = {
    readonly __typename: 'Mutation';
    readonly deleteObject: boolean;
};
export type LikeObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type LikeObjectMutation = {
    readonly __typename: 'Mutation';
    readonly likeObject: {
        readonly __typename: 'Activity';
        readonly id: string;
        readonly type: ActivityType;
        readonly published: string;
        readonly cost: number;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly object?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        } | null | undefined;
        readonly target?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        } | null | undefined;
    };
};
export type UnlikeObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnlikeObjectMutation = {
    readonly __typename: 'Mutation';
    readonly unlikeObject: boolean;
};
export type ShareObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type ShareObjectMutation = {
    readonly __typename: 'Mutation';
    readonly shareObject: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    };
};
export type UnshareObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnshareObjectMutation = {
    readonly __typename: 'Mutation';
    readonly unshareObject: boolean;
};
export type BookmarkObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type BookmarkObjectMutation = {
    readonly __typename: 'Mutation';
    readonly bookmarkObject: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    };
};
export type UnbookmarkObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnbookmarkObjectMutation = {
    readonly __typename: 'Mutation';
    readonly unbookmarkObject: boolean;
};
export type PinObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type PinObjectMutation = {
    readonly __typename: 'Mutation';
    readonly pinObject: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    };
};
export type UnpinObjectMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnpinObjectMutation = {
    readonly __typename: 'Mutation';
    readonly unpinObject: boolean;
};
export type ObjectWithQuotesQueryVariables = Exact<{
    id: Scalars['ID']['input'];
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['String']['input']>;
}>;
export type ObjectWithQuotesQuery = {
    readonly __typename: 'Query';
    readonly object?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly quotes: {
            readonly __typename: 'QuoteConnection';
            readonly totalCount: number;
            readonly edges: ReadonlyArray<{
                readonly __typename: 'QuoteEdge';
                readonly cursor: string;
                readonly node: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly content: string;
                    readonly visibility: Visibility;
                    readonly sensitive: boolean;
                    readonly spoilerText?: string | null | undefined;
                    readonly createdAt: string;
                    readonly updatedAt: string;
                    readonly repliesCount: number;
                    readonly likesCount: number;
                    readonly sharesCount: number;
                    readonly estimatedCost: number;
                    readonly moderationScore?: number | null | undefined;
                    readonly quoteUrl?: string | null | undefined;
                    readonly quoteable: boolean;
                    readonly quotePermissions: QuotePermission;
                    readonly quoteCount: number;
                    readonly boostedObject?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly content: string;
                        readonly visibility: Visibility;
                        readonly sensitive: boolean;
                        readonly spoilerText?: string | null | undefined;
                        readonly createdAt: string;
                        readonly updatedAt: string;
                        readonly repliesCount: number;
                        readonly likesCount: number;
                        readonly sharesCount: number;
                        readonly estimatedCost: number;
                        readonly moderationScore?: number | null | undefined;
                        readonly quoteUrl?: string | null | undefined;
                        readonly quoteable: boolean;
                        readonly quotePermissions: QuotePermission;
                        readonly quoteCount: number;
                        readonly contentMap: ReadonlyArray<{
                            readonly __typename: 'ContentMap';
                            readonly language: string;
                            readonly content: string;
                        }>;
                        readonly attachments: ReadonlyArray<{
                            readonly __typename: 'Attachment';
                            readonly id: string;
                            readonly type: string;
                            readonly url: string;
                            readonly preview?: string | null | undefined;
                            readonly description?: string | null | undefined;
                            readonly blurhash?: string | null | undefined;
                            readonly width?: number | null | undefined;
                            readonly height?: number | null | undefined;
                            readonly duration?: number | null | undefined;
                        }>;
                        readonly tags: ReadonlyArray<{
                            readonly __typename: 'Tag';
                            readonly name: string;
                            readonly url: string;
                        }>;
                        readonly mentions: ReadonlyArray<{
                            readonly __typename: 'Mention';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly url: string;
                        }>;
                        readonly quoteContext?: {
                            readonly __typename: 'QuoteContext';
                            readonly quoteAllowed: boolean;
                            readonly quoteType: QuoteType;
                            readonly withdrawn: boolean;
                            readonly originalAuthor: {
                                readonly __typename: 'Actor';
                                readonly id: string;
                                readonly username: string;
                                readonly domain?: string | null | undefined;
                                readonly displayName?: string | null | undefined;
                                readonly summary?: string | null | undefined;
                                readonly avatar?: string | null | undefined;
                                readonly header?: string | null | undefined;
                                readonly followers: number;
                                readonly following: number;
                                readonly statusesCount: number;
                                readonly bot: boolean;
                                readonly locked: boolean;
                                readonly updatedAt: string;
                                readonly trustScore: number;
                                readonly fields: ReadonlyArray<{
                                    readonly __typename: 'Field';
                                    readonly name: string;
                                    readonly value: string;
                                    readonly verifiedAt?: string | null | undefined;
                                }>;
                            };
                            readonly originalNote?: {
                                readonly __typename: 'Object';
                                readonly id: string;
                                readonly type: ObjectType;
                            } | null | undefined;
                        } | null | undefined;
                        readonly communityNotes: ReadonlyArray<{
                            readonly __typename: 'CommunityNote';
                            readonly id: string;
                            readonly content: string;
                            readonly helpful: number;
                            readonly notHelpful: number;
                            readonly createdAt: string;
                            readonly author: {
                                readonly __typename: 'Actor';
                                readonly id: string;
                                readonly username: string;
                                readonly domain?: string | null | undefined;
                                readonly displayName?: string | null | undefined;
                                readonly summary?: string | null | undefined;
                                readonly avatar?: string | null | undefined;
                                readonly header?: string | null | undefined;
                                readonly followers: number;
                                readonly following: number;
                                readonly statusesCount: number;
                                readonly bot: boolean;
                                readonly locked: boolean;
                                readonly updatedAt: string;
                                readonly trustScore: number;
                                readonly fields: ReadonlyArray<{
                                    readonly __typename: 'Field';
                                    readonly name: string;
                                    readonly value: string;
                                    readonly verifiedAt?: string | null | undefined;
                                }>;
                            };
                        }>;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly inReplyTo?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                            readonly actor: {
                                readonly __typename: 'Actor';
                                readonly id: string;
                                readonly username: string;
                                readonly domain?: string | null | undefined;
                                readonly displayName?: string | null | undefined;
                                readonly summary?: string | null | undefined;
                                readonly avatar?: string | null | undefined;
                                readonly header?: string | null | undefined;
                                readonly followers: number;
                                readonly following: number;
                                readonly statusesCount: number;
                                readonly bot: boolean;
                                readonly locked: boolean;
                                readonly updatedAt: string;
                                readonly trustScore: number;
                                readonly fields: ReadonlyArray<{
                                    readonly __typename: 'Field';
                                    readonly name: string;
                                    readonly value: string;
                                    readonly verifiedAt?: string | null | undefined;
                                }>;
                            };
                        } | null | undefined;
                    } | null | undefined;
                    readonly contentMap: ReadonlyArray<{
                        readonly __typename: 'ContentMap';
                        readonly language: string;
                        readonly content: string;
                    }>;
                    readonly attachments: ReadonlyArray<{
                        readonly __typename: 'Attachment';
                        readonly id: string;
                        readonly type: string;
                        readonly url: string;
                        readonly preview?: string | null | undefined;
                        readonly description?: string | null | undefined;
                        readonly blurhash?: string | null | undefined;
                        readonly width?: number | null | undefined;
                        readonly height?: number | null | undefined;
                        readonly duration?: number | null | undefined;
                    }>;
                    readonly tags: ReadonlyArray<{
                        readonly __typename: 'Tag';
                        readonly name: string;
                        readonly url: string;
                    }>;
                    readonly mentions: ReadonlyArray<{
                        readonly __typename: 'Mention';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly url: string;
                    }>;
                    readonly quoteContext?: {
                        readonly __typename: 'QuoteContext';
                        readonly quoteAllowed: boolean;
                        readonly quoteType: QuoteType;
                        readonly withdrawn: boolean;
                        readonly originalAuthor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly originalNote?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                        } | null | undefined;
                    } | null | undefined;
                    readonly communityNotes: ReadonlyArray<{
                        readonly __typename: 'CommunityNote';
                        readonly id: string;
                        readonly content: string;
                        readonly helpful: number;
                        readonly notHelpful: number;
                        readonly createdAt: string;
                        readonly author: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    }>;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly inReplyTo?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    } | null | undefined;
                };
            }>;
            readonly pageInfo: {
                readonly __typename: 'PageInfo';
                readonly hasNextPage: boolean;
                readonly hasPreviousPage: boolean;
                readonly startCursor?: string | null | undefined;
                readonly endCursor?: string | null | undefined;
            };
        };
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    } | null | undefined;
};
export type NotificationsQueryVariables = Exact<{
    types?: InputMaybe<ReadonlyArray<Scalars['String']['input']> | Scalars['String']['input']>;
    excludeTypes?: InputMaybe<ReadonlyArray<Scalars['String']['input']> | Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['Cursor']['input']>;
}>;
export type NotificationsQuery = {
    readonly __typename: 'Query';
    readonly notifications: {
        readonly __typename: 'NotificationConnection';
        readonly totalCount: number;
        readonly edges: ReadonlyArray<{
            readonly __typename: 'NotificationEdge';
            readonly cursor: string;
            readonly node: {
                readonly __typename: 'Notification';
                readonly id: string;
                readonly type: string;
                readonly read: boolean;
                readonly createdAt: string;
                readonly account: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly status?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly content: string;
                    readonly visibility: Visibility;
                    readonly sensitive: boolean;
                    readonly spoilerText?: string | null | undefined;
                    readonly createdAt: string;
                    readonly updatedAt: string;
                    readonly repliesCount: number;
                    readonly likesCount: number;
                    readonly sharesCount: number;
                    readonly estimatedCost: number;
                    readonly moderationScore?: number | null | undefined;
                    readonly quoteUrl?: string | null | undefined;
                    readonly quoteable: boolean;
                    readonly quotePermissions: QuotePermission;
                    readonly quoteCount: number;
                    readonly boostedObject?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly content: string;
                        readonly visibility: Visibility;
                        readonly sensitive: boolean;
                        readonly spoilerText?: string | null | undefined;
                        readonly createdAt: string;
                        readonly updatedAt: string;
                        readonly repliesCount: number;
                        readonly likesCount: number;
                        readonly sharesCount: number;
                        readonly estimatedCost: number;
                        readonly moderationScore?: number | null | undefined;
                        readonly quoteUrl?: string | null | undefined;
                        readonly quoteable: boolean;
                        readonly quotePermissions: QuotePermission;
                        readonly quoteCount: number;
                        readonly contentMap: ReadonlyArray<{
                            readonly __typename: 'ContentMap';
                            readonly language: string;
                            readonly content: string;
                        }>;
                        readonly attachments: ReadonlyArray<{
                            readonly __typename: 'Attachment';
                            readonly id: string;
                            readonly type: string;
                            readonly url: string;
                            readonly preview?: string | null | undefined;
                            readonly description?: string | null | undefined;
                            readonly blurhash?: string | null | undefined;
                            readonly width?: number | null | undefined;
                            readonly height?: number | null | undefined;
                            readonly duration?: number | null | undefined;
                        }>;
                        readonly tags: ReadonlyArray<{
                            readonly __typename: 'Tag';
                            readonly name: string;
                            readonly url: string;
                        }>;
                        readonly mentions: ReadonlyArray<{
                            readonly __typename: 'Mention';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly url: string;
                        }>;
                        readonly quoteContext?: {
                            readonly __typename: 'QuoteContext';
                            readonly quoteAllowed: boolean;
                            readonly quoteType: QuoteType;
                            readonly withdrawn: boolean;
                            readonly originalAuthor: {
                                readonly __typename: 'Actor';
                                readonly id: string;
                                readonly username: string;
                                readonly domain?: string | null | undefined;
                                readonly displayName?: string | null | undefined;
                                readonly summary?: string | null | undefined;
                                readonly avatar?: string | null | undefined;
                                readonly header?: string | null | undefined;
                                readonly followers: number;
                                readonly following: number;
                                readonly statusesCount: number;
                                readonly bot: boolean;
                                readonly locked: boolean;
                                readonly updatedAt: string;
                                readonly trustScore: number;
                                readonly fields: ReadonlyArray<{
                                    readonly __typename: 'Field';
                                    readonly name: string;
                                    readonly value: string;
                                    readonly verifiedAt?: string | null | undefined;
                                }>;
                            };
                            readonly originalNote?: {
                                readonly __typename: 'Object';
                                readonly id: string;
                                readonly type: ObjectType;
                            } | null | undefined;
                        } | null | undefined;
                        readonly communityNotes: ReadonlyArray<{
                            readonly __typename: 'CommunityNote';
                            readonly id: string;
                            readonly content: string;
                            readonly helpful: number;
                            readonly notHelpful: number;
                            readonly createdAt: string;
                            readonly author: {
                                readonly __typename: 'Actor';
                                readonly id: string;
                                readonly username: string;
                                readonly domain?: string | null | undefined;
                                readonly displayName?: string | null | undefined;
                                readonly summary?: string | null | undefined;
                                readonly avatar?: string | null | undefined;
                                readonly header?: string | null | undefined;
                                readonly followers: number;
                                readonly following: number;
                                readonly statusesCount: number;
                                readonly bot: boolean;
                                readonly locked: boolean;
                                readonly updatedAt: string;
                                readonly trustScore: number;
                                readonly fields: ReadonlyArray<{
                                    readonly __typename: 'Field';
                                    readonly name: string;
                                    readonly value: string;
                                    readonly verifiedAt?: string | null | undefined;
                                }>;
                            };
                        }>;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly inReplyTo?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                            readonly actor: {
                                readonly __typename: 'Actor';
                                readonly id: string;
                                readonly username: string;
                                readonly domain?: string | null | undefined;
                                readonly displayName?: string | null | undefined;
                                readonly summary?: string | null | undefined;
                                readonly avatar?: string | null | undefined;
                                readonly header?: string | null | undefined;
                                readonly followers: number;
                                readonly following: number;
                                readonly statusesCount: number;
                                readonly bot: boolean;
                                readonly locked: boolean;
                                readonly updatedAt: string;
                                readonly trustScore: number;
                                readonly fields: ReadonlyArray<{
                                    readonly __typename: 'Field';
                                    readonly name: string;
                                    readonly value: string;
                                    readonly verifiedAt?: string | null | undefined;
                                }>;
                            };
                        } | null | undefined;
                    } | null | undefined;
                    readonly contentMap: ReadonlyArray<{
                        readonly __typename: 'ContentMap';
                        readonly language: string;
                        readonly content: string;
                    }>;
                    readonly attachments: ReadonlyArray<{
                        readonly __typename: 'Attachment';
                        readonly id: string;
                        readonly type: string;
                        readonly url: string;
                        readonly preview?: string | null | undefined;
                        readonly description?: string | null | undefined;
                        readonly blurhash?: string | null | undefined;
                        readonly width?: number | null | undefined;
                        readonly height?: number | null | undefined;
                        readonly duration?: number | null | undefined;
                    }>;
                    readonly tags: ReadonlyArray<{
                        readonly __typename: 'Tag';
                        readonly name: string;
                        readonly url: string;
                    }>;
                    readonly mentions: ReadonlyArray<{
                        readonly __typename: 'Mention';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly url: string;
                    }>;
                    readonly quoteContext?: {
                        readonly __typename: 'QuoteContext';
                        readonly quoteAllowed: boolean;
                        readonly quoteType: QuoteType;
                        readonly withdrawn: boolean;
                        readonly originalAuthor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly originalNote?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                        } | null | undefined;
                    } | null | undefined;
                    readonly communityNotes: ReadonlyArray<{
                        readonly __typename: 'CommunityNote';
                        readonly id: string;
                        readonly content: string;
                        readonly helpful: number;
                        readonly notHelpful: number;
                        readonly createdAt: string;
                        readonly author: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    }>;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly inReplyTo?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    } | null | undefined;
                } | null | undefined;
            };
        }>;
        readonly pageInfo: {
            readonly __typename: 'PageInfo';
            readonly hasNextPage: boolean;
            readonly hasPreviousPage: boolean;
            readonly startCursor?: string | null | undefined;
            readonly endCursor?: string | null | undefined;
        };
    };
};
export type DismissNotificationMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type DismissNotificationMutation = {
    readonly __typename: 'Mutation';
    readonly dismissNotification: boolean;
};
export type ClearNotificationsMutationVariables = Exact<{
    [key: string]: never;
}>;
export type ClearNotificationsMutation = {
    readonly __typename: 'Mutation';
    readonly clearNotifications: boolean;
};
export type ObjectByIdQueryVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type ObjectByIdQuery = {
    readonly __typename: 'Query';
    readonly object?: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    } | null | undefined;
};
export type UserPreferencesQueryVariables = Exact<{
    [key: string]: never;
}>;
export type UserPreferencesQuery = {
    readonly __typename: 'Query';
    readonly userPreferences: {
        readonly __typename: 'UserPreferences';
        readonly actorId: string;
        readonly posting: {
            readonly __typename: 'PostingPreferences';
            readonly defaultVisibility: Visibility;
            readonly defaultSensitive: boolean;
            readonly defaultLanguage: string;
        };
        readonly reading: {
            readonly __typename: 'ReadingPreferences';
            readonly expandSpoilers: boolean;
            readonly expandMedia: ExpandMediaPreference;
            readonly autoplayGifs: boolean;
            readonly timelineOrder: TimelineOrder;
        };
        readonly discovery: {
            readonly __typename: 'DiscoveryPreferences';
            readonly showFollowCounts: boolean;
            readonly searchSuggestionsEnabled: boolean;
            readonly personalizedSearchEnabled: boolean;
        };
        readonly streaming: {
            readonly __typename: 'StreamingPreferences';
            readonly defaultQuality: StreamQuality;
            readonly autoQuality: boolean;
            readonly preloadNext: boolean;
            readonly dataSaver: boolean;
        };
        readonly notifications: {
            readonly __typename: 'NotificationPreferences';
            readonly email: boolean;
            readonly push: boolean;
            readonly inApp: boolean;
            readonly digest: DigestFrequency;
        };
        readonly privacy: {
            readonly __typename: 'PrivacyPreferences';
            readonly defaultVisibility: Visibility;
            readonly indexable: boolean;
            readonly showOnlineStatus: boolean;
        };
        readonly reblogFilters: ReadonlyArray<{
            readonly __typename: 'ReblogFilter';
            readonly key: string;
            readonly enabled: boolean;
        }>;
    };
};
export type UpdateUserPreferencesMutationVariables = Exact<{
    input: UpdateUserPreferencesInput;
}>;
export type UpdateUserPreferencesMutation = {
    readonly __typename: 'Mutation';
    readonly updateUserPreferences: {
        readonly __typename: 'UserPreferences';
        readonly actorId: string;
        readonly posting: {
            readonly __typename: 'PostingPreferences';
            readonly defaultVisibility: Visibility;
            readonly defaultSensitive: boolean;
            readonly defaultLanguage: string;
        };
        readonly reading: {
            readonly __typename: 'ReadingPreferences';
            readonly expandSpoilers: boolean;
            readonly expandMedia: ExpandMediaPreference;
            readonly autoplayGifs: boolean;
            readonly timelineOrder: TimelineOrder;
        };
        readonly discovery: {
            readonly __typename: 'DiscoveryPreferences';
            readonly showFollowCounts: boolean;
            readonly searchSuggestionsEnabled: boolean;
            readonly personalizedSearchEnabled: boolean;
        };
        readonly streaming: {
            readonly __typename: 'StreamingPreferences';
            readonly defaultQuality: StreamQuality;
            readonly autoQuality: boolean;
            readonly preloadNext: boolean;
            readonly dataSaver: boolean;
        };
        readonly notifications: {
            readonly __typename: 'NotificationPreferences';
            readonly email: boolean;
            readonly push: boolean;
            readonly inApp: boolean;
            readonly digest: DigestFrequency;
        };
        readonly privacy: {
            readonly __typename: 'PrivacyPreferences';
            readonly defaultVisibility: Visibility;
            readonly indexable: boolean;
            readonly showOnlineStatus: boolean;
        };
        readonly reblogFilters: ReadonlyArray<{
            readonly __typename: 'ReblogFilter';
            readonly key: string;
            readonly enabled: boolean;
        }>;
    };
};
export type UpdateStreamingPreferencesMutationVariables = Exact<{
    input: StreamingPreferencesInput;
}>;
export type UpdateStreamingPreferencesMutation = {
    readonly __typename: 'Mutation';
    readonly updateStreamingPreferences: {
        readonly __typename: 'UserPreferences';
        readonly actorId: string;
        readonly streaming: {
            readonly __typename: 'StreamingPreferences';
            readonly defaultQuality: StreamQuality;
            readonly autoQuality: boolean;
            readonly preloadNext: boolean;
            readonly dataSaver: boolean;
        };
    };
};
export type FollowersQueryVariables = Exact<{
    username: Scalars['String']['input'];
    limit?: InputMaybe<Scalars['Int']['input']>;
    cursor?: InputMaybe<Scalars['Cursor']['input']>;
}>;
export type FollowersQuery = {
    readonly __typename: 'Query';
    readonly followers: {
        readonly __typename: 'ActorListPage';
        readonly nextCursor?: string | null | undefined;
        readonly totalCount: number;
        readonly actors: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
    };
};
export type FollowingQueryVariables = Exact<{
    username: Scalars['String']['input'];
    limit?: InputMaybe<Scalars['Int']['input']>;
    cursor?: InputMaybe<Scalars['Cursor']['input']>;
}>;
export type FollowingQuery = {
    readonly __typename: 'Query';
    readonly following: {
        readonly __typename: 'ActorListPage';
        readonly nextCursor?: string | null | undefined;
        readonly totalCount: number;
        readonly actors: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
    };
};
export type UpdateProfileMutationVariables = Exact<{
    input: UpdateProfileInput;
}>;
export type UpdateProfileMutation = {
    readonly __typename: 'Mutation';
    readonly updateProfile: {
        readonly __typename: 'Actor';
        readonly id: string;
        readonly username: string;
        readonly domain?: string | null | undefined;
        readonly displayName?: string | null | undefined;
        readonly summary?: string | null | undefined;
        readonly avatar?: string | null | undefined;
        readonly header?: string | null | undefined;
        readonly followers: number;
        readonly following: number;
        readonly statusesCount: number;
        readonly bot: boolean;
        readonly locked: boolean;
        readonly updatedAt: string;
        readonly trustScore: number;
        readonly fields: ReadonlyArray<{
            readonly __typename: 'Field';
            readonly name: string;
            readonly value: string;
            readonly verifiedAt?: string | null | undefined;
        }>;
    };
};
export type PushSubscriptionQueryVariables = Exact<{
    [key: string]: never;
}>;
export type PushSubscriptionQuery = {
    readonly __typename: 'Query';
    readonly pushSubscription?: {
        readonly __typename: 'PushSubscription';
        readonly id: string;
        readonly endpoint: string;
        readonly policy: string;
        readonly serverKey?: string | null | undefined;
        readonly createdAt?: string | null | undefined;
        readonly updatedAt?: string | null | undefined;
        readonly keys: {
            readonly __typename: 'PushSubscriptionKeys';
            readonly auth: string;
            readonly p256dh: string;
        };
        readonly alerts: {
            readonly __typename: 'PushSubscriptionAlerts';
            readonly follow: boolean;
            readonly favourite: boolean;
            readonly reblog: boolean;
            readonly mention: boolean;
            readonly poll: boolean;
            readonly followRequest: boolean;
            readonly status: boolean;
            readonly update: boolean;
            readonly adminSignUp: boolean;
            readonly adminReport: boolean;
        };
    } | null | undefined;
};
export type RegisterPushSubscriptionMutationVariables = Exact<{
    input: RegisterPushSubscriptionInput;
}>;
export type RegisterPushSubscriptionMutation = {
    readonly __typename: 'Mutation';
    readonly registerPushSubscription: {
        readonly __typename: 'PushSubscription';
        readonly id: string;
        readonly endpoint: string;
        readonly policy: string;
        readonly serverKey?: string | null | undefined;
        readonly createdAt?: string | null | undefined;
        readonly updatedAt?: string | null | undefined;
        readonly keys: {
            readonly __typename: 'PushSubscriptionKeys';
            readonly auth: string;
            readonly p256dh: string;
        };
        readonly alerts: {
            readonly __typename: 'PushSubscriptionAlerts';
            readonly follow: boolean;
            readonly favourite: boolean;
            readonly reblog: boolean;
            readonly mention: boolean;
            readonly poll: boolean;
            readonly followRequest: boolean;
            readonly status: boolean;
            readonly update: boolean;
            readonly adminSignUp: boolean;
            readonly adminReport: boolean;
        };
    };
};
export type UpdatePushSubscriptionMutationVariables = Exact<{
    input: UpdatePushSubscriptionInput;
}>;
export type UpdatePushSubscriptionMutation = {
    readonly __typename: 'Mutation';
    readonly updatePushSubscription: {
        readonly __typename: 'PushSubscription';
        readonly id: string;
        readonly endpoint: string;
        readonly policy: string;
        readonly serverKey?: string | null | undefined;
        readonly createdAt?: string | null | undefined;
        readonly updatedAt?: string | null | undefined;
        readonly keys: {
            readonly __typename: 'PushSubscriptionKeys';
            readonly auth: string;
            readonly p256dh: string;
        };
        readonly alerts: {
            readonly __typename: 'PushSubscriptionAlerts';
            readonly follow: boolean;
            readonly favourite: boolean;
            readonly reblog: boolean;
            readonly mention: boolean;
            readonly poll: boolean;
            readonly followRequest: boolean;
            readonly status: boolean;
            readonly update: boolean;
            readonly adminSignUp: boolean;
            readonly adminReport: boolean;
        };
    };
};
export type DeletePushSubscriptionMutationVariables = Exact<{
    [key: string]: never;
}>;
export type DeletePushSubscriptionMutation = {
    readonly __typename: 'Mutation';
    readonly deletePushSubscription: boolean;
};
export type RelationshipQueryVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type RelationshipQuery = {
    readonly __typename: 'Query';
    readonly relationship?: {
        readonly __typename: 'Relationship';
        readonly id: string;
        readonly following: boolean;
        readonly followedBy: boolean;
        readonly blocking: boolean;
        readonly blockedBy: boolean;
        readonly muting: boolean;
        readonly mutingNotifications: boolean;
        readonly requested: boolean;
        readonly domainBlocking: boolean;
        readonly showingReblogs: boolean;
        readonly notifying: boolean;
        readonly languages?: ReadonlyArray<string> | null | undefined;
        readonly note?: string | null | undefined;
    } | null | undefined;
};
export type RelationshipsQueryVariables = Exact<{
    ids: ReadonlyArray<Scalars['ID']['input']> | Scalars['ID']['input'];
}>;
export type RelationshipsQuery = {
    readonly __typename: 'Query';
    readonly relationships: ReadonlyArray<{
        readonly __typename: 'Relationship';
        readonly id: string;
        readonly following: boolean;
        readonly followedBy: boolean;
        readonly blocking: boolean;
        readonly blockedBy: boolean;
        readonly muting: boolean;
        readonly mutingNotifications: boolean;
        readonly requested: boolean;
        readonly domainBlocking: boolean;
        readonly showingReblogs: boolean;
        readonly notifying: boolean;
        readonly languages?: ReadonlyArray<string> | null | undefined;
        readonly note?: string | null | undefined;
    }>;
};
export type FollowActorMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type FollowActorMutation = {
    readonly __typename: 'Mutation';
    readonly followActor: {
        readonly __typename: 'Activity';
        readonly id: string;
        readonly type: ActivityType;
        readonly published: string;
        readonly cost: number;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly object?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        } | null | undefined;
        readonly target?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
        } | null | undefined;
    };
};
export type UnfollowActorMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnfollowActorMutation = {
    readonly __typename: 'Mutation';
    readonly unfollowActor: boolean;
};
export type BlockActorMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type BlockActorMutation = {
    readonly __typename: 'Mutation';
    readonly blockActor: {
        readonly __typename: 'Relationship';
        readonly id: string;
        readonly following: boolean;
        readonly followedBy: boolean;
        readonly blocking: boolean;
        readonly blockedBy: boolean;
        readonly muting: boolean;
        readonly mutingNotifications: boolean;
        readonly requested: boolean;
        readonly domainBlocking: boolean;
        readonly showingReblogs: boolean;
        readonly notifying: boolean;
        readonly languages?: ReadonlyArray<string> | null | undefined;
        readonly note?: string | null | undefined;
    };
};
export type UnblockActorMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnblockActorMutation = {
    readonly __typename: 'Mutation';
    readonly unblockActor: boolean;
};
export type MuteActorMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    notifications?: InputMaybe<Scalars['Boolean']['input']>;
}>;
export type MuteActorMutation = {
    readonly __typename: 'Mutation';
    readonly muteActor: {
        readonly __typename: 'Relationship';
        readonly id: string;
        readonly following: boolean;
        readonly followedBy: boolean;
        readonly blocking: boolean;
        readonly blockedBy: boolean;
        readonly muting: boolean;
        readonly mutingNotifications: boolean;
        readonly requested: boolean;
        readonly domainBlocking: boolean;
        readonly showingReblogs: boolean;
        readonly notifying: boolean;
        readonly languages?: ReadonlyArray<string> | null | undefined;
        readonly note?: string | null | undefined;
    };
};
export type UnmuteActorMutationVariables = Exact<{
    id: Scalars['ID']['input'];
}>;
export type UnmuteActorMutation = {
    readonly __typename: 'Mutation';
    readonly unmuteActor: boolean;
};
export type UpdateRelationshipMutationVariables = Exact<{
    id: Scalars['ID']['input'];
    input: UpdateRelationshipInput;
}>;
export type UpdateRelationshipMutation = {
    readonly __typename: 'Mutation';
    readonly updateRelationship: {
        readonly __typename: 'Relationship';
        readonly id: string;
        readonly following: boolean;
        readonly followedBy: boolean;
        readonly blocking: boolean;
        readonly blockedBy: boolean;
        readonly muting: boolean;
        readonly mutingNotifications: boolean;
        readonly requested: boolean;
        readonly domainBlocking: boolean;
        readonly showingReblogs: boolean;
        readonly notifying: boolean;
        readonly languages?: ReadonlyArray<string> | null | undefined;
        readonly note?: string | null | undefined;
    };
};
export type SearchQueryVariables = Exact<{
    query: Scalars['String']['input'];
    type?: InputMaybe<Scalars['String']['input']>;
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['Cursor']['input']>;
}>;
export type SearchQuery = {
    readonly __typename: 'Query';
    readonly search: {
        readonly __typename: 'SearchResult';
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
        readonly statuses: ReadonlyArray<{
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        }>;
        readonly hashtags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
    };
};
export type TimelineUpdatesSubscriptionVariables = Exact<{
    type: TimelineType;
    listId?: InputMaybe<Scalars['ID']['input']>;
}>;
export type TimelineUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly timelineUpdates: {
        readonly __typename: 'Object';
        readonly id: string;
        readonly type: ObjectType;
        readonly content: string;
        readonly visibility: Visibility;
        readonly sensitive: boolean;
        readonly spoilerText?: string | null | undefined;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly repliesCount: number;
        readonly likesCount: number;
        readonly sharesCount: number;
        readonly estimatedCost: number;
        readonly moderationScore?: number | null | undefined;
        readonly quoteUrl?: string | null | undefined;
        readonly quoteable: boolean;
        readonly quotePermissions: QuotePermission;
        readonly quoteCount: number;
        readonly boostedObject?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly contentMap: ReadonlyArray<{
            readonly __typename: 'ContentMap';
            readonly language: string;
            readonly content: string;
        }>;
        readonly attachments: ReadonlyArray<{
            readonly __typename: 'Attachment';
            readonly id: string;
            readonly type: string;
            readonly url: string;
            readonly preview?: string | null | undefined;
            readonly description?: string | null | undefined;
            readonly blurhash?: string | null | undefined;
            readonly width?: number | null | undefined;
            readonly height?: number | null | undefined;
            readonly duration?: number | null | undefined;
        }>;
        readonly tags: ReadonlyArray<{
            readonly __typename: 'Tag';
            readonly name: string;
            readonly url: string;
        }>;
        readonly mentions: ReadonlyArray<{
            readonly __typename: 'Mention';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly url: string;
        }>;
        readonly quoteContext?: {
            readonly __typename: 'QuoteContext';
            readonly quoteAllowed: boolean;
            readonly quoteType: QuoteType;
            readonly withdrawn: boolean;
            readonly originalAuthor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly originalNote?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
            } | null | undefined;
        } | null | undefined;
        readonly communityNotes: ReadonlyArray<{
            readonly __typename: 'CommunityNote';
            readonly id: string;
            readonly content: string;
            readonly helpful: number;
            readonly notHelpful: number;
            readonly createdAt: string;
            readonly author: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        }>;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly inReplyTo?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
    };
};
export type NotificationStreamSubscriptionVariables = Exact<{
    types?: InputMaybe<ReadonlyArray<Scalars['String']['input']> | Scalars['String']['input']>;
}>;
export type NotificationStreamSubscription = {
    readonly __typename: 'Subscription';
    readonly notificationStream: {
        readonly __typename: 'Notification';
        readonly id: string;
        readonly type: string;
        readonly read: boolean;
        readonly createdAt: string;
        readonly account: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly status?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
    };
};
export type ConversationUpdatesSubscriptionVariables = Exact<{
    [key: string]: never;
}>;
export type ConversationUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly conversationUpdates: {
        readonly __typename: 'Conversation';
        readonly id: string;
        readonly unread: boolean;
        readonly createdAt: string;
        readonly updatedAt: string;
        readonly accounts: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
        readonly lastStatus?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
    };
};
export type ListUpdatesSubscriptionVariables = Exact<{
    listId: Scalars['ID']['input'];
}>;
export type ListUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly listUpdates: {
        readonly __typename: 'ListUpdate';
        readonly type: string;
        readonly timestamp: string;
        readonly list: {
            readonly __typename: 'List';
            readonly id: string;
            readonly title: string;
            readonly repliesPolicy: RepliesPolicy;
            readonly exclusive: boolean;
            readonly accountCount: number;
        };
        readonly account?: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        } | null | undefined;
    };
};
export type QuoteActivitySubscriptionVariables = Exact<{
    noteId: Scalars['ID']['input'];
}>;
export type QuoteActivitySubscription = {
    readonly __typename: 'Subscription';
    readonly quoteActivity: {
        readonly __typename: 'QuoteActivityUpdate';
        readonly type: string;
        readonly timestamp: string;
        readonly quote?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly quoter?: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        } | null | undefined;
    };
};
export type HashtagActivitySubscriptionVariables = Exact<{
    hashtags: ReadonlyArray<Scalars['String']['input']> | Scalars['String']['input'];
}>;
export type HashtagActivitySubscription = {
    readonly __typename: 'Subscription';
    readonly hashtagActivity: {
        readonly __typename: 'HashtagActivityUpdate';
        readonly hashtag: string;
        readonly timestamp: string;
        readonly post: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
        readonly author: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    };
};
export type ActivityStreamSubscriptionVariables = Exact<{
    types?: InputMaybe<ReadonlyArray<ActivityType> | ActivityType>;
}>;
export type ActivityStreamSubscription = {
    readonly __typename: 'Subscription';
    readonly activityStream: {
        readonly __typename: 'Activity';
        readonly id: string;
        readonly type: ActivityType;
        readonly published: string;
        readonly cost: number;
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly object?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
        readonly target?: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        } | null | undefined;
    };
};
export type RelationshipUpdatesSubscriptionVariables = Exact<{
    actorId?: InputMaybe<Scalars['ID']['input']>;
}>;
export type RelationshipUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly relationshipUpdates: {
        readonly __typename: 'RelationshipUpdate';
        readonly type: string;
        readonly timestamp: string;
        readonly relationship: {
            readonly __typename: 'Relationship';
            readonly id: string;
            readonly following: boolean;
            readonly followedBy: boolean;
            readonly blocking: boolean;
            readonly blockedBy: boolean;
            readonly muting: boolean;
            readonly mutingNotifications: boolean;
            readonly requested: boolean;
            readonly domainBlocking: boolean;
            readonly showingReblogs: boolean;
            readonly notifying: boolean;
            readonly languages?: ReadonlyArray<string> | null | undefined;
            readonly note?: string | null | undefined;
        };
        readonly actor: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    };
};
export type CostUpdatesSubscriptionVariables = Exact<{
    threshold?: InputMaybe<Scalars['Int']['input']>;
}>;
export type CostUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly costUpdates: {
        readonly __typename: 'CostUpdate';
        readonly operationCost: number;
        readonly dailyTotal: number;
        readonly monthlyProjection: number;
    };
};
export type ModerationEventsSubscriptionVariables = Exact<{
    actorId?: InputMaybe<Scalars['ID']['input']>;
}>;
export type ModerationEventsSubscription = {
    readonly __typename: 'Subscription';
    readonly moderationEvents: {
        readonly __typename: 'ModerationDecision';
        readonly id: string;
        readonly decision: string;
        readonly confidence: number;
        readonly evidence: ReadonlyArray<string>;
        readonly timestamp: string;
        readonly object: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
        readonly reviewers: ReadonlyArray<{
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        }>;
    };
};
export type TrustUpdatesSubscriptionVariables = Exact<{
    actorId: Scalars['ID']['input'];
}>;
export type TrustUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly trustUpdates: {
        readonly __typename: 'TrustEdge';
        readonly category: TrustCategory;
        readonly score: number;
        readonly updatedAt: string;
        readonly from: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly to: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    };
};
export type AiAnalysisUpdatesSubscriptionVariables = Exact<{
    objectId?: InputMaybe<Scalars['ID']['input']>;
}>;
export type AiAnalysisUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly aiAnalysisUpdates: {
        readonly __typename: 'AIAnalysis';
        readonly id: string;
        readonly objectId: string;
        readonly objectType: string;
        readonly overallRisk: number;
        readonly moderationAction: ModerationAction;
        readonly confidence: number;
        readonly analyzedAt: string;
        readonly textAnalysis?: {
            readonly __typename: 'TextAnalysis';
            readonly sentiment: Sentiment;
            readonly toxicityScore: number;
            readonly toxicityLabels: ReadonlyArray<string>;
            readonly containsPII: boolean;
            readonly dominantLanguage: string;
            readonly keyPhrases: ReadonlyArray<string>;
            readonly sentimentScores: {
                readonly __typename: 'SentimentScores';
                readonly positive: number;
                readonly negative: number;
                readonly neutral: number;
                readonly mixed: number;
            };
            readonly entities: ReadonlyArray<{
                readonly __typename: 'Entity';
                readonly type: string;
                readonly text: string;
                readonly score: number;
            }>;
        } | null | undefined;
        readonly imageAnalysis?: {
            readonly __typename: 'ImageAnalysis';
            readonly isNSFW: boolean;
            readonly nsfwConfidence: number;
            readonly violenceScore: number;
            readonly weaponsDetected: boolean;
            readonly detectedText: ReadonlyArray<string>;
            readonly textToxicity: number;
            readonly deepfakeScore: number;
            readonly moderationLabels: ReadonlyArray<{
                readonly __typename: 'ModerationLabel';
                readonly name: string;
                readonly confidence: number;
                readonly parentName?: string | null | undefined;
            }>;
            readonly celebrityFaces: ReadonlyArray<{
                readonly __typename: 'Celebrity';
                readonly name: string;
                readonly confidence: number;
                readonly urls: ReadonlyArray<string>;
            }>;
        } | null | undefined;
        readonly aiDetection?: {
            readonly __typename: 'AIDetection';
            readonly aiGeneratedProbability: number;
            readonly generationModel?: string | null | undefined;
            readonly patternConsistency: number;
            readonly styleDeviation: number;
            readonly semanticCoherence: number;
            readonly suspiciousPatterns: ReadonlyArray<string>;
        } | null | undefined;
        readonly spamAnalysis?: {
            readonly __typename: 'SpamAnalysis';
            readonly spamScore: number;
            readonly postingVelocity: number;
            readonly repetitionScore: number;
            readonly linkDensity: number;
            readonly followerRatio: number;
            readonly interactionRate: number;
            readonly accountAgeDays: number;
            readonly spamIndicators: ReadonlyArray<{
                readonly __typename: 'SpamIndicator';
                readonly type: string;
                readonly description: string;
                readonly severity: number;
            }>;
        } | null | undefined;
    };
};
export type MetricsUpdatesSubscriptionVariables = Exact<{
    categories?: InputMaybe<ReadonlyArray<Scalars['String']['input']> | Scalars['String']['input']>;
    services?: InputMaybe<ReadonlyArray<Scalars['String']['input']> | Scalars['String']['input']>;
    threshold?: InputMaybe<Scalars['Float']['input']>;
}>;
export type MetricsUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly metricsUpdates: {
        readonly __typename: 'MetricsUpdate';
        readonly metricId: string;
        readonly serviceName: string;
        readonly metricType: string;
        readonly subscriptionCategory: string;
        readonly aggregationLevel: string;
        readonly timestamp: string;
        readonly count: number;
        readonly sum: number;
        readonly min: number;
        readonly max: number;
        readonly average: number;
        readonly p50?: number | null | undefined;
        readonly p95?: number | null | undefined;
        readonly p99?: number | null | undefined;
        readonly unit?: string | null | undefined;
        readonly userCostMicrocents?: number | null | undefined;
        readonly totalCostMicrocents?: number | null | undefined;
        readonly userId?: string | null | undefined;
        readonly tenantId?: string | null | undefined;
        readonly instanceDomain?: string | null | undefined;
        readonly dimensions: ReadonlyArray<{
            readonly __typename: 'MetricsDimension';
            readonly key: string;
            readonly value: string;
        }>;
    };
};
export type ModerationAlertsSubscriptionVariables = Exact<{
    severity?: InputMaybe<ModerationSeverity>;
}>;
export type ModerationAlertsSubscription = {
    readonly __typename: 'Subscription';
    readonly moderationAlerts: {
        readonly __typename: 'ModerationAlert';
        readonly id: string;
        readonly severity: ModerationSeverity;
        readonly matchedText: string;
        readonly confidence: number;
        readonly suggestedAction: ModerationAction;
        readonly timestamp: string;
        readonly handled: boolean;
        readonly pattern?: {
            readonly __typename: 'ModerationPattern';
            readonly id: string;
            readonly pattern: string;
            readonly type: PatternType;
            readonly severity: ModerationSeverity;
            readonly matchCount: number;
            readonly falsePositiveRate: number;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly active: boolean;
            readonly createdBy: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
        } | null | undefined;
        readonly content: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
    };
};
export type CostAlertsSubscriptionVariables = Exact<{
    thresholdUSD: Scalars['Float']['input'];
}>;
export type CostAlertsSubscription = {
    readonly __typename: 'Subscription';
    readonly costAlerts: {
        readonly __typename: 'CostAlert';
        readonly id: string;
        readonly type: string;
        readonly amount: number;
        readonly threshold: number;
        readonly domain?: string | null | undefined;
        readonly message: string;
        readonly timestamp: string;
    };
};
export type BudgetAlertsSubscriptionVariables = Exact<{
    domain?: InputMaybe<Scalars['String']['input']>;
}>;
export type BudgetAlertsSubscription = {
    readonly __typename: 'Subscription';
    readonly budgetAlerts: {
        readonly __typename: 'BudgetAlert';
        readonly id: string;
        readonly domain: string;
        readonly budgetUSD: number;
        readonly spentUSD: number;
        readonly percentUsed: number;
        readonly projectedOverspend?: number | null | undefined;
        readonly alertLevel: AlertLevel;
        readonly timestamp: string;
    };
};
export type FederationHealthUpdatesSubscriptionVariables = Exact<{
    domain?: InputMaybe<Scalars['String']['input']>;
}>;
export type FederationHealthUpdatesSubscription = {
    readonly __typename: 'Subscription';
    readonly federationHealthUpdates: {
        readonly __typename: 'FederationHealthUpdate';
        readonly domain: string;
        readonly previousStatus: InstanceHealthStatus;
        readonly currentStatus: InstanceHealthStatus;
        readonly timestamp: string;
        readonly issues: ReadonlyArray<{
            readonly __typename: 'HealthIssue';
            readonly type: string;
            readonly severity: IssueSeverity;
            readonly description: string;
            readonly detectedAt: string;
            readonly impact: string;
        }>;
    };
};
export type ModerationQueueUpdateSubscriptionVariables = Exact<{
    priority?: InputMaybe<Priority>;
}>;
export type ModerationQueueUpdateSubscription = {
    readonly __typename: 'Subscription';
    readonly moderationQueueUpdate: {
        readonly __typename: 'ModerationItem';
        readonly id: string;
        readonly reportCount: number;
        readonly severity: ModerationSeverity;
        readonly priority: Priority;
        readonly deadline: string;
        readonly content: {
            readonly __typename: 'Object';
            readonly id: string;
            readonly type: ObjectType;
            readonly content: string;
            readonly visibility: Visibility;
            readonly sensitive: boolean;
            readonly spoilerText?: string | null | undefined;
            readonly createdAt: string;
            readonly updatedAt: string;
            readonly repliesCount: number;
            readonly likesCount: number;
            readonly sharesCount: number;
            readonly estimatedCost: number;
            readonly moderationScore?: number | null | undefined;
            readonly quoteUrl?: string | null | undefined;
            readonly quoteable: boolean;
            readonly quotePermissions: QuotePermission;
            readonly quoteCount: number;
            readonly boostedObject?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            } | null | undefined;
            readonly contentMap: ReadonlyArray<{
                readonly __typename: 'ContentMap';
                readonly language: string;
                readonly content: string;
            }>;
            readonly attachments: ReadonlyArray<{
                readonly __typename: 'Attachment';
                readonly id: string;
                readonly type: string;
                readonly url: string;
                readonly preview?: string | null | undefined;
                readonly description?: string | null | undefined;
                readonly blurhash?: string | null | undefined;
                readonly width?: number | null | undefined;
                readonly height?: number | null | undefined;
                readonly duration?: number | null | undefined;
            }>;
            readonly tags: ReadonlyArray<{
                readonly __typename: 'Tag';
                readonly name: string;
                readonly url: string;
            }>;
            readonly mentions: ReadonlyArray<{
                readonly __typename: 'Mention';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly url: string;
            }>;
            readonly quoteContext?: {
                readonly __typename: 'QuoteContext';
                readonly quoteAllowed: boolean;
                readonly quoteType: QuoteType;
                readonly withdrawn: boolean;
                readonly originalAuthor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly originalNote?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                } | null | undefined;
            } | null | undefined;
            readonly communityNotes: ReadonlyArray<{
                readonly __typename: 'CommunityNote';
                readonly id: string;
                readonly content: string;
                readonly helpful: number;
                readonly notHelpful: number;
                readonly createdAt: string;
                readonly author: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            }>;
            readonly actor: {
                readonly __typename: 'Actor';
                readonly id: string;
                readonly username: string;
                readonly domain?: string | null | undefined;
                readonly displayName?: string | null | undefined;
                readonly summary?: string | null | undefined;
                readonly avatar?: string | null | undefined;
                readonly header?: string | null | undefined;
                readonly followers: number;
                readonly following: number;
                readonly statusesCount: number;
                readonly bot: boolean;
                readonly locked: boolean;
                readonly updatedAt: string;
                readonly trustScore: number;
                readonly fields: ReadonlyArray<{
                    readonly __typename: 'Field';
                    readonly name: string;
                    readonly value: string;
                    readonly verifiedAt?: string | null | undefined;
                }>;
            };
            readonly inReplyTo?: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
            } | null | undefined;
        };
        readonly assignedTo?: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        } | null | undefined;
    };
};
export type ThreatIntelligenceSubscriptionVariables = Exact<{
    [key: string]: never;
}>;
export type ThreatIntelligenceSubscription = {
    readonly __typename: 'Subscription';
    readonly threatIntelligence: {
        readonly __typename: 'ThreatAlert';
        readonly id: string;
        readonly type: string;
        readonly severity: ModerationSeverity;
        readonly source: string;
        readonly description: string;
        readonly affectedInstances: ReadonlyArray<string>;
        readonly mitigationSteps: ReadonlyArray<string>;
        readonly timestamp: string;
    };
};
export type PerformanceAlertSubscriptionVariables = Exact<{
    severity: AlertSeverity;
}>;
export type PerformanceAlertSubscription = {
    readonly __typename: 'Subscription';
    readonly performanceAlert: {
        readonly __typename: 'PerformanceAlert';
        readonly id: string;
        readonly service: ServiceCategory;
        readonly metric: string;
        readonly threshold: number;
        readonly actualValue: number;
        readonly severity: AlertSeverity;
        readonly timestamp: string;
    };
};
export type InfrastructureEventSubscriptionVariables = Exact<{
    [key: string]: never;
}>;
export type InfrastructureEventSubscription = {
    readonly __typename: 'Subscription';
    readonly infrastructureEvent: {
        readonly __typename: 'InfrastructureEvent';
        readonly id: string;
        readonly type: InfrastructureEventType;
        readonly service: string;
        readonly description: string;
        readonly impact: string;
        readonly timestamp: string;
    };
};
export type TimelineQueryVariables = Exact<{
    type: TimelineType;
    first?: InputMaybe<Scalars['Int']['input']>;
    after?: InputMaybe<Scalars['Cursor']['input']>;
    hashtag?: InputMaybe<Scalars['String']['input']>;
    listId?: InputMaybe<Scalars['ID']['input']>;
    actorId?: InputMaybe<Scalars['ID']['input']>;
    mediaOnly?: InputMaybe<Scalars['Boolean']['input']>;
}>;
export type TimelineQuery = {
    readonly __typename: 'Query';
    readonly timeline: {
        readonly __typename: 'ObjectConnection';
        readonly totalCount: number;
        readonly edges: ReadonlyArray<{
            readonly __typename: 'ObjectEdge';
            readonly cursor: string;
            readonly node: {
                readonly __typename: 'Object';
                readonly id: string;
                readonly type: ObjectType;
                readonly content: string;
                readonly visibility: Visibility;
                readonly sensitive: boolean;
                readonly spoilerText?: string | null | undefined;
                readonly createdAt: string;
                readonly updatedAt: string;
                readonly repliesCount: number;
                readonly likesCount: number;
                readonly sharesCount: number;
                readonly estimatedCost: number;
                readonly moderationScore?: number | null | undefined;
                readonly quoteUrl?: string | null | undefined;
                readonly quoteable: boolean;
                readonly quotePermissions: QuotePermission;
                readonly quoteCount: number;
                readonly boostedObject?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly content: string;
                    readonly visibility: Visibility;
                    readonly sensitive: boolean;
                    readonly spoilerText?: string | null | undefined;
                    readonly createdAt: string;
                    readonly updatedAt: string;
                    readonly repliesCount: number;
                    readonly likesCount: number;
                    readonly sharesCount: number;
                    readonly estimatedCost: number;
                    readonly moderationScore?: number | null | undefined;
                    readonly quoteUrl?: string | null | undefined;
                    readonly quoteable: boolean;
                    readonly quotePermissions: QuotePermission;
                    readonly quoteCount: number;
                    readonly contentMap: ReadonlyArray<{
                        readonly __typename: 'ContentMap';
                        readonly language: string;
                        readonly content: string;
                    }>;
                    readonly attachments: ReadonlyArray<{
                        readonly __typename: 'Attachment';
                        readonly id: string;
                        readonly type: string;
                        readonly url: string;
                        readonly preview?: string | null | undefined;
                        readonly description?: string | null | undefined;
                        readonly blurhash?: string | null | undefined;
                        readonly width?: number | null | undefined;
                        readonly height?: number | null | undefined;
                        readonly duration?: number | null | undefined;
                    }>;
                    readonly tags: ReadonlyArray<{
                        readonly __typename: 'Tag';
                        readonly name: string;
                        readonly url: string;
                    }>;
                    readonly mentions: ReadonlyArray<{
                        readonly __typename: 'Mention';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly url: string;
                    }>;
                    readonly quoteContext?: {
                        readonly __typename: 'QuoteContext';
                        readonly quoteAllowed: boolean;
                        readonly quoteType: QuoteType;
                        readonly withdrawn: boolean;
                        readonly originalAuthor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                        readonly originalNote?: {
                            readonly __typename: 'Object';
                            readonly id: string;
                            readonly type: ObjectType;
                        } | null | undefined;
                    } | null | undefined;
                    readonly communityNotes: ReadonlyArray<{
                        readonly __typename: 'CommunityNote';
                        readonly id: string;
                        readonly content: string;
                        readonly helpful: number;
                        readonly notHelpful: number;
                        readonly createdAt: string;
                        readonly author: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    }>;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly inReplyTo?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                        readonly actor: {
                            readonly __typename: 'Actor';
                            readonly id: string;
                            readonly username: string;
                            readonly domain?: string | null | undefined;
                            readonly displayName?: string | null | undefined;
                            readonly summary?: string | null | undefined;
                            readonly avatar?: string | null | undefined;
                            readonly header?: string | null | undefined;
                            readonly followers: number;
                            readonly following: number;
                            readonly statusesCount: number;
                            readonly bot: boolean;
                            readonly locked: boolean;
                            readonly updatedAt: string;
                            readonly trustScore: number;
                            readonly fields: ReadonlyArray<{
                                readonly __typename: 'Field';
                                readonly name: string;
                                readonly value: string;
                                readonly verifiedAt?: string | null | undefined;
                            }>;
                        };
                    } | null | undefined;
                } | null | undefined;
                readonly contentMap: ReadonlyArray<{
                    readonly __typename: 'ContentMap';
                    readonly language: string;
                    readonly content: string;
                }>;
                readonly attachments: ReadonlyArray<{
                    readonly __typename: 'Attachment';
                    readonly id: string;
                    readonly type: string;
                    readonly url: string;
                    readonly preview?: string | null | undefined;
                    readonly description?: string | null | undefined;
                    readonly blurhash?: string | null | undefined;
                    readonly width?: number | null | undefined;
                    readonly height?: number | null | undefined;
                    readonly duration?: number | null | undefined;
                }>;
                readonly tags: ReadonlyArray<{
                    readonly __typename: 'Tag';
                    readonly name: string;
                    readonly url: string;
                }>;
                readonly mentions: ReadonlyArray<{
                    readonly __typename: 'Mention';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly url: string;
                }>;
                readonly quoteContext?: {
                    readonly __typename: 'QuoteContext';
                    readonly quoteAllowed: boolean;
                    readonly quoteType: QuoteType;
                    readonly withdrawn: boolean;
                    readonly originalAuthor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                    readonly originalNote?: {
                        readonly __typename: 'Object';
                        readonly id: string;
                        readonly type: ObjectType;
                    } | null | undefined;
                } | null | undefined;
                readonly communityNotes: ReadonlyArray<{
                    readonly __typename: 'CommunityNote';
                    readonly id: string;
                    readonly content: string;
                    readonly helpful: number;
                    readonly notHelpful: number;
                    readonly createdAt: string;
                    readonly author: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                }>;
                readonly actor: {
                    readonly __typename: 'Actor';
                    readonly id: string;
                    readonly username: string;
                    readonly domain?: string | null | undefined;
                    readonly displayName?: string | null | undefined;
                    readonly summary?: string | null | undefined;
                    readonly avatar?: string | null | undefined;
                    readonly header?: string | null | undefined;
                    readonly followers: number;
                    readonly following: number;
                    readonly statusesCount: number;
                    readonly bot: boolean;
                    readonly locked: boolean;
                    readonly updatedAt: string;
                    readonly trustScore: number;
                    readonly fields: ReadonlyArray<{
                        readonly __typename: 'Field';
                        readonly name: string;
                        readonly value: string;
                        readonly verifiedAt?: string | null | undefined;
                    }>;
                };
                readonly inReplyTo?: {
                    readonly __typename: 'Object';
                    readonly id: string;
                    readonly type: ObjectType;
                    readonly actor: {
                        readonly __typename: 'Actor';
                        readonly id: string;
                        readonly username: string;
                        readonly domain?: string | null | undefined;
                        readonly displayName?: string | null | undefined;
                        readonly summary?: string | null | undefined;
                        readonly avatar?: string | null | undefined;
                        readonly header?: string | null | undefined;
                        readonly followers: number;
                        readonly following: number;
                        readonly statusesCount: number;
                        readonly bot: boolean;
                        readonly locked: boolean;
                        readonly updatedAt: string;
                        readonly trustScore: number;
                        readonly fields: ReadonlyArray<{
                            readonly __typename: 'Field';
                            readonly name: string;
                            readonly value: string;
                            readonly verifiedAt?: string | null | undefined;
                        }>;
                    };
                } | null | undefined;
            };
        }>;
        readonly pageInfo: {
            readonly __typename: 'PageInfo';
            readonly hasNextPage: boolean;
            readonly hasPreviousPage: boolean;
            readonly startCursor?: string | null | undefined;
            readonly endCursor?: string | null | undefined;
        };
    };
};
export type TrustGraphQueryVariables = Exact<{
    actorId: Scalars['ID']['input'];
    category?: InputMaybe<TrustCategory>;
}>;
export type TrustGraphQuery = {
    readonly __typename: 'Query';
    readonly trustGraph: ReadonlyArray<{
        readonly __typename: 'TrustEdge';
        readonly category: TrustCategory;
        readonly score: number;
        readonly updatedAt: string;
        readonly from: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
        readonly to: {
            readonly __typename: 'Actor';
            readonly id: string;
            readonly username: string;
            readonly domain?: string | null | undefined;
            readonly displayName?: string | null | undefined;
            readonly summary?: string | null | undefined;
            readonly avatar?: string | null | undefined;
            readonly header?: string | null | undefined;
            readonly followers: number;
            readonly following: number;
            readonly statusesCount: number;
            readonly bot: boolean;
            readonly locked: boolean;
            readonly updatedAt: string;
            readonly trustScore: number;
            readonly fields: ReadonlyArray<{
                readonly __typename: 'Field';
                readonly name: string;
                readonly value: string;
                readonly verifiedAt?: string | null | undefined;
            }>;
        };
    }>;
};
export declare const AttachmentFieldsFragmentDoc: DocumentNode<AttachmentFieldsFragment, unknown>;
export declare const TagFieldsFragmentDoc: DocumentNode<TagFieldsFragment, unknown>;
export declare const MentionFieldsFragmentDoc: DocumentNode<MentionFieldsFragment, unknown>;
export declare const FieldFieldsFragmentDoc: DocumentNode<FieldFieldsFragment, unknown>;
export declare const ActorSummaryFragmentDoc: DocumentNode<ActorSummaryFragment, unknown>;
export declare const QuoteContextFieldsFragmentDoc: DocumentNode<QuoteContextFieldsFragment, unknown>;
export declare const CommunityNoteFieldsFragmentDoc: DocumentNode<CommunityNoteFieldsFragment, unknown>;
export declare const ObjectContentFieldsFragmentDoc: DocumentNode<ObjectContentFieldsFragment, unknown>;
export declare const ObjectFieldsFragmentDoc: DocumentNode<ObjectFieldsFragment, unknown>;
export declare const ActivityFieldsFragmentDoc: DocumentNode<ActivityFieldsFragment, unknown>;
export declare const CostUpdateFieldsFragmentDoc: DocumentNode<CostUpdateFieldsFragment, unknown>;
export declare const ActorByIdDocument: DocumentNode<ActorByIdQuery, ActorByIdQueryVariables>;
export declare const ActorByUsernameDocument: DocumentNode<ActorByUsernameQuery, ActorByUsernameQueryVariables>;
export declare const RequestAiAnalysisDocument: DocumentNode<RequestAiAnalysisMutation, RequestAiAnalysisMutationVariables>;
export declare const AiAnalysisDocument: DocumentNode<AiAnalysisQuery, AiAnalysisQueryVariables>;
export declare const AiStatsDocument: DocumentNode<AiStatsQuery, AiStatsQueryVariables>;
export declare const AiCapabilitiesDocument: DocumentNode<AiCapabilitiesQuery, AiCapabilitiesQueryVariables>;
export declare const ConversationsDocument: DocumentNode<ConversationsQuery, ConversationsQueryVariables>;
export declare const ConversationDocument: DocumentNode<ConversationQuery, ConversationQueryVariables>;
export declare const MarkConversationReadDocument: DocumentNode<MarkConversationReadMutation, MarkConversationReadMutationVariables>;
export declare const DeleteConversationDocument: DocumentNode<DeleteConversationMutation, DeleteConversationMutationVariables>;
export declare const CostBreakdownDocument: DocumentNode<CostBreakdownQuery, CostBreakdownQueryVariables>;
export declare const InstanceBudgetsDocument: DocumentNode<InstanceBudgetsQuery, InstanceBudgetsQueryVariables>;
export declare const SetInstanceBudgetDocument: DocumentNode<SetInstanceBudgetMutation, SetInstanceBudgetMutationVariables>;
export declare const OptimizeFederationCostsDocument: DocumentNode<OptimizeFederationCostsMutation, OptimizeFederationCostsMutationVariables>;
export declare const FederationLimitsDocument: DocumentNode<FederationLimitsQuery, FederationLimitsQueryVariables>;
export declare const SetFederationLimitDocument: DocumentNode<SetFederationLimitMutation, SetFederationLimitMutationVariables>;
export declare const SyncThreadDocument: DocumentNode<SyncThreadMutation, SyncThreadMutationVariables>;
export declare const SyncMissingRepliesDocument: DocumentNode<SyncMissingRepliesMutation, SyncMissingRepliesMutationVariables>;
export declare const ThreadContextDocument: DocumentNode<ThreadContextQuery, ThreadContextQueryVariables>;
export declare const SeveredRelationshipsDocument: DocumentNode<SeveredRelationshipsQuery, SeveredRelationshipsQueryVariables>;
export declare const AcknowledgeSeveranceDocument: DocumentNode<AcknowledgeSeveranceMutation, AcknowledgeSeveranceMutationVariables>;
export declare const AttemptReconnectionDocument: DocumentNode<AttemptReconnectionMutation, AttemptReconnectionMutationVariables>;
export declare const FederationHealthDocument: DocumentNode<FederationHealthQuery, FederationHealthQueryVariables>;
export declare const FederationStatusDocument: DocumentNode<FederationStatusQuery, FederationStatusQueryVariables>;
export declare const PauseFederationDocument: DocumentNode<PauseFederationMutation, PauseFederationMutationVariables>;
export declare const ResumeFederationDocument: DocumentNode<ResumeFederationMutation, ResumeFederationMutationVariables>;
export declare const FollowHashtagDocument: DocumentNode<FollowHashtagMutation, FollowHashtagMutationVariables>;
export declare const UnfollowHashtagDocument: DocumentNode<UnfollowHashtagMutation, UnfollowHashtagMutationVariables>;
export declare const MuteHashtagDocument: DocumentNode<MuteHashtagMutation, MuteHashtagMutationVariables>;
export declare const FollowedHashtagsDocument: DocumentNode<FollowedHashtagsQuery, FollowedHashtagsQueryVariables>;
export declare const ListsDocument: DocumentNode<ListsQuery, ListsQueryVariables>;
export declare const ListDocument: DocumentNode<ListQuery, ListQueryVariables>;
export declare const ListAccountsDocument: DocumentNode<ListAccountsQuery, ListAccountsQueryVariables>;
export declare const CreateListDocument: DocumentNode<CreateListMutation, CreateListMutationVariables>;
export declare const UpdateListDocument: DocumentNode<UpdateListMutation, UpdateListMutationVariables>;
export declare const DeleteListDocument: DocumentNode<DeleteListMutation, DeleteListMutationVariables>;
export declare const AddAccountsToListDocument: DocumentNode<AddAccountsToListMutation, AddAccountsToListMutationVariables>;
export declare const RemoveAccountsFromListDocument: DocumentNode<RemoveAccountsFromListMutation, RemoveAccountsFromListMutationVariables>;
export declare const MediaDocument: DocumentNode<MediaQuery, MediaQueryVariables>;
export declare const UpdateMediaDocument: DocumentNode<UpdateMediaMutation, UpdateMediaMutationVariables>;
export declare const UploadMediaDocument: DocumentNode<UploadMediaMutation, UploadMediaMutationVariables>;
export declare const AddCommunityNoteDocument: DocumentNode<AddCommunityNoteMutation, AddCommunityNoteMutationVariables>;
export declare const VoteCommunityNoteDocument: DocumentNode<VoteCommunityNoteMutation, VoteCommunityNoteMutationVariables>;
export declare const CommunityNotesByObjectDocument: DocumentNode<CommunityNotesByObjectQuery, CommunityNotesByObjectQueryVariables>;
export declare const FlagObjectDocument: DocumentNode<FlagObjectMutation, FlagObjectMutationVariables>;
export declare const CreateModerationPatternDocument: DocumentNode<CreateModerationPatternMutation, CreateModerationPatternMutationVariables>;
export declare const DeleteModerationPatternDocument: DocumentNode<DeleteModerationPatternMutation, DeleteModerationPatternMutationVariables>;
export declare const CreateNoteDocument: DocumentNode<CreateNoteMutation, CreateNoteMutationVariables>;
export declare const CreateQuoteNoteDocument: DocumentNode<CreateQuoteNoteMutation, CreateQuoteNoteMutationVariables>;
export declare const WithdrawFromQuotesDocument: DocumentNode<WithdrawFromQuotesMutation, WithdrawFromQuotesMutationVariables>;
export declare const UpdateQuotePermissionsDocument: DocumentNode<UpdateQuotePermissionsMutation, UpdateQuotePermissionsMutationVariables>;
export declare const DeleteObjectDocument: DocumentNode<DeleteObjectMutation, DeleteObjectMutationVariables>;
export declare const LikeObjectDocument: DocumentNode<LikeObjectMutation, LikeObjectMutationVariables>;
export declare const UnlikeObjectDocument: DocumentNode<UnlikeObjectMutation, UnlikeObjectMutationVariables>;
export declare const ShareObjectDocument: DocumentNode<ShareObjectMutation, ShareObjectMutationVariables>;
export declare const UnshareObjectDocument: DocumentNode<UnshareObjectMutation, UnshareObjectMutationVariables>;
export declare const BookmarkObjectDocument: DocumentNode<BookmarkObjectMutation, BookmarkObjectMutationVariables>;
export declare const UnbookmarkObjectDocument: DocumentNode<UnbookmarkObjectMutation, UnbookmarkObjectMutationVariables>;
export declare const PinObjectDocument: DocumentNode<PinObjectMutation, PinObjectMutationVariables>;
export declare const UnpinObjectDocument: DocumentNode<UnpinObjectMutation, UnpinObjectMutationVariables>;
export declare const ObjectWithQuotesDocument: DocumentNode<ObjectWithQuotesQuery, ObjectWithQuotesQueryVariables>;
export declare const NotificationsDocument: DocumentNode<NotificationsQuery, NotificationsQueryVariables>;
export declare const DismissNotificationDocument: DocumentNode<DismissNotificationMutation, DismissNotificationMutationVariables>;
export declare const ClearNotificationsDocument: DocumentNode<ClearNotificationsMutation, ClearNotificationsMutationVariables>;
export declare const ObjectByIdDocument: DocumentNode<ObjectByIdQuery, ObjectByIdQueryVariables>;
export declare const UserPreferencesDocument: DocumentNode<UserPreferencesQuery, UserPreferencesQueryVariables>;
export declare const UpdateUserPreferencesDocument: DocumentNode<UpdateUserPreferencesMutation, UpdateUserPreferencesMutationVariables>;
export declare const UpdateStreamingPreferencesDocument: DocumentNode<UpdateStreamingPreferencesMutation, UpdateStreamingPreferencesMutationVariables>;
export declare const FollowersDocument: DocumentNode<FollowersQuery, FollowersQueryVariables>;
export declare const FollowingDocument: DocumentNode<FollowingQuery, FollowingQueryVariables>;
export declare const UpdateProfileDocument: DocumentNode<UpdateProfileMutation, UpdateProfileMutationVariables>;
export declare const PushSubscriptionDocument: DocumentNode<PushSubscriptionQuery, PushSubscriptionQueryVariables>;
export declare const RegisterPushSubscriptionDocument: DocumentNode<RegisterPushSubscriptionMutation, RegisterPushSubscriptionMutationVariables>;
export declare const UpdatePushSubscriptionDocument: DocumentNode<UpdatePushSubscriptionMutation, UpdatePushSubscriptionMutationVariables>;
export declare const DeletePushSubscriptionDocument: DocumentNode<DeletePushSubscriptionMutation, DeletePushSubscriptionMutationVariables>;
export declare const RelationshipDocument: DocumentNode<RelationshipQuery, RelationshipQueryVariables>;
export declare const RelationshipsDocument: DocumentNode<RelationshipsQuery, RelationshipsQueryVariables>;
export declare const FollowActorDocument: DocumentNode<FollowActorMutation, FollowActorMutationVariables>;
export declare const UnfollowActorDocument: DocumentNode<UnfollowActorMutation, UnfollowActorMutationVariables>;
export declare const BlockActorDocument: DocumentNode<BlockActorMutation, BlockActorMutationVariables>;
export declare const UnblockActorDocument: DocumentNode<UnblockActorMutation, UnblockActorMutationVariables>;
export declare const MuteActorDocument: DocumentNode<MuteActorMutation, MuteActorMutationVariables>;
export declare const UnmuteActorDocument: DocumentNode<UnmuteActorMutation, UnmuteActorMutationVariables>;
export declare const UpdateRelationshipDocument: DocumentNode<UpdateRelationshipMutation, UpdateRelationshipMutationVariables>;
export declare const SearchDocument: DocumentNode<SearchQuery, SearchQueryVariables>;
export declare const TimelineUpdatesDocument: DocumentNode<TimelineUpdatesSubscription, TimelineUpdatesSubscriptionVariables>;
export declare const NotificationStreamDocument: DocumentNode<NotificationStreamSubscription, NotificationStreamSubscriptionVariables>;
export declare const ConversationUpdatesDocument: DocumentNode<ConversationUpdatesSubscription, ConversationUpdatesSubscriptionVariables>;
export declare const ListUpdatesDocument: DocumentNode<ListUpdatesSubscription, ListUpdatesSubscriptionVariables>;
export declare const QuoteActivityDocument: DocumentNode<QuoteActivitySubscription, QuoteActivitySubscriptionVariables>;
export declare const HashtagActivityDocument: DocumentNode<HashtagActivitySubscription, HashtagActivitySubscriptionVariables>;
export declare const ActivityStreamDocument: DocumentNode<ActivityStreamSubscription, ActivityStreamSubscriptionVariables>;
export declare const RelationshipUpdatesDocument: DocumentNode<RelationshipUpdatesSubscription, RelationshipUpdatesSubscriptionVariables>;
export declare const CostUpdatesDocument: DocumentNode<CostUpdatesSubscription, CostUpdatesSubscriptionVariables>;
export declare const ModerationEventsDocument: DocumentNode<ModerationEventsSubscription, ModerationEventsSubscriptionVariables>;
export declare const TrustUpdatesDocument: DocumentNode<TrustUpdatesSubscription, TrustUpdatesSubscriptionVariables>;
export declare const AiAnalysisUpdatesDocument: DocumentNode<AiAnalysisUpdatesSubscription, AiAnalysisUpdatesSubscriptionVariables>;
export declare const MetricsUpdatesDocument: DocumentNode<MetricsUpdatesSubscription, MetricsUpdatesSubscriptionVariables>;
export declare const ModerationAlertsDocument: DocumentNode<ModerationAlertsSubscription, ModerationAlertsSubscriptionVariables>;
export declare const CostAlertsDocument: DocumentNode<CostAlertsSubscription, CostAlertsSubscriptionVariables>;
export declare const BudgetAlertsDocument: DocumentNode<BudgetAlertsSubscription, BudgetAlertsSubscriptionVariables>;
export declare const FederationHealthUpdatesDocument: DocumentNode<FederationHealthUpdatesSubscription, FederationHealthUpdatesSubscriptionVariables>;
export declare const ModerationQueueUpdateDocument: DocumentNode<ModerationQueueUpdateSubscription, ModerationQueueUpdateSubscriptionVariables>;
export declare const ThreatIntelligenceDocument: DocumentNode<ThreatIntelligenceSubscription, ThreatIntelligenceSubscriptionVariables>;
export declare const PerformanceAlertDocument: DocumentNode<PerformanceAlertSubscription, PerformanceAlertSubscriptionVariables>;
export declare const InfrastructureEventDocument: DocumentNode<InfrastructureEventSubscription, InfrastructureEventSubscriptionVariables>;
export declare const TimelineDocument: DocumentNode<TimelineQuery, TimelineQueryVariables>;
export declare const TrustGraphDocument: DocumentNode<TrustGraphQuery, TrustGraphQueryVariables>;
//# sourceMappingURL=types.d.ts.map