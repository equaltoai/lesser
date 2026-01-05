/**
 * Lesser GraphQL API payload types
 * Fictional GraphQL-based Fediverse API similar to Mastodon functionality
 */

// Base GraphQL response wrapper
export interface LesserGraphQLResponse<T = unknown> {
	data?: T;
	errors?: LesserGraphQLError[];
	extensions?: Record<string, unknown>;
}

export interface LesserGraphQLError {
	message: string;
	locations?: Array<{ line: number; column: number }>;
	path?: Array<string | number>;
	extensions?: Record<string, unknown>;
}

// Fragment definitions for reusable GraphQL pieces
export interface LesserAccountFragment {
	id: string;
	handle: string;
	localHandle: string;
	displayName: string;
	bio: string;
	avatarUrl: string;
	bannerUrl: string;
	joinedAt: string;
	isVerified: boolean;
	isBot: boolean;
	isLocked: boolean;
	followerCount: number;
	followingCount: number;
	postCount: number;
	profileFields: LesserProfileField[];
	customEmojis: LesserEmojiFragment[];

	// Lesser-specific fields
	trustScore?: number;
	reputation?: LesserReputationFragment;
	vouches?: LesserVouchFragment[];
}

export interface LesserReputationFragment {
	actorId: string;
	instance: string;
	totalScore: number;
	trustScore: number;
	activityScore: number;
	moderationScore: number;
	communityScore: number;
	calculatedAt: string;
	version: string;
	evidence?: {
		totalPosts: number;
		totalFollowers: number;
		accountAge: number;
		vouchCount: number;
		trustingActors: number;
		averageTrustScore: number;
	};
	signature?: string;
}

export interface LesserVouchFragment {
	id: string;
	from?: { id: string };
	to?: { id: string };
	confidence: number;
	context: string;
	voucherReputation: number;
	createdAt: string;
	expiresAt: string;
	active: boolean;
	revoked: boolean;
	revokedAt?: string;
}

export interface LesserProfileField {
	label: string;
	content: string;
	verifiedAt?: string;
}

export interface LesserEmojiFragment {
	code: string;
	imageUrl: string;
	staticUrl: string;
	category?: string;
	isVisible: boolean;
}

export interface LesserReplyRef {
	id: string;
	actor?: LesserAccountFragment;
	authorId?: string;
}

// Post/Status types
export interface LesserPostFragment {
	id: string;
	publishedAt: string;
	content: string;
	contentWarning?: string;
	visibility: 'PUBLIC' | 'UNLISTED' | 'FOLLOWERS' | 'DIRECT';
	isSensitive: boolean;
	language?: string;
	author: LesserAccountFragment;
	attachments: LesserMediaFragment[];
	mentions: LesserMentionFragment[];
	hashtags: LesserHashtagFragment[];
	emojis: LesserEmojiFragment[];
	replyTo?: LesserReplyRef;
	inReplyTo?: LesserReplyRef;
	shareOf?: LesserPostFragment;
	boostedObject?: LesserPostFragment;
	interactionCounts: LesserInteractionCounts;
	userInteractions: LesserUserInteractions;
	isPinned: boolean;
	lastEditedAt?: string;
	poll?: LesserPollFragment;

	// Lesser-specific fields
	estimatedCost?: number;
	moderationScore?: number;
	communityNotes?: LesserCommunityNoteFragment[];
	quoteUrl?: string;
	quoteable?: boolean;
	quotePermissions?: 'EVERYONE' | 'FOLLOWERS' | 'NONE';
	quoteContext?: LesserQuoteContextFragment;
	quoteCount?: number;
	aiAnalysis?: LesserAIAnalysisFragment;
}

export interface LesserCommunityNoteFragment {
	id: string;
	author?: LesserAccountFragment;
	content: string;
	helpful: number;
	notHelpful: number;
	createdAt: string;
}

export interface LesserQuoteContextFragment {
	originalAuthor?: LesserAccountFragment;
	originalNote?: { id: string };
	quoteAllowed: boolean;
	quoteType: 'FULL' | 'PARTIAL' | 'COMMENTARY' | 'REACTION';
	withdrawn: boolean;
}

export interface LesserAIAnalysisFragment {
	id: string;
	objectId: string;
	objectType: string;
	textAnalysis?: LesserTextAnalysisFragment;
	imageAnalysis?: LesserImageAnalysisFragment;
	aiDetection?: LesserAIDetectionFragment;
	spamAnalysis?: LesserSpamAnalysisFragment;
	overallRisk: number;
	moderationAction: 'NONE' | 'FLAG' | 'HIDE' | 'REMOVE' | 'SHADOW_BAN' | 'REVIEW';
	confidence: number;
	analyzedAt: string;
}

export interface LesserTextAnalysisFragment {
	sentiment: 'POSITIVE' | 'NEGATIVE' | 'NEUTRAL' | 'MIXED';
	sentimentScores: {
		positive: number;
		negative: number;
		neutral: number;
		mixed: number;
	};
	toxicityScore: number;
	toxicityLabels: string[];
	containsPII: boolean;
	dominantLanguage: string;
	entities: Array<{
		text: string;
		type: string;
		confidence: number;
	}>;
	keyPhrases: string[];
}

export interface LesserModerationLabelFragment {
	name: string;
	confidence: number;
	parentName?: string;
}

export interface LesserCelebrityFragment {
	name: string;
	confidence: number;
	urls: string[];
}

export interface LesserImageAnalysisFragment {
	moderationLabels: LesserModerationLabelFragment[];
	isNSFW: boolean;
	nsfwConfidence: number;
	violenceScore: number;
	weaponsDetected: boolean;
	detectedText: string[];
	textToxicity: number;
	celebrityFaces: LesserCelebrityFragment[];
	deepfakeScore: number;
}

export interface LesserAIDetectionFragment {
	aiGeneratedProbability: number;
	generationModel?: string;
	patternConsistency: number;
	styleDeviation: number;
	semanticCoherence: number;
	suspiciousPatterns: string[];
}

export interface LesserSpamIndicatorFragment {
	type: string;
	description: string;
	severity: number;
}

export interface LesserSpamAnalysisFragment {
	spamScore: number;
	spamIndicators: LesserSpamIndicatorFragment[];
	postingVelocity: number;
	repetitionScore: number;
	linkDensity: number;
	followerRatio: number;
	interactionRate: number;
	accountAgeDays: number;
}

export interface LesserInteractionCounts {
	replies: number;
	shares: number;
	favorites: number;
}

export interface LesserUserInteractions {
	isFavorited: boolean;
	isShared: boolean;
	isBookmarked: boolean;
}

// Media attachment types
export interface LesserMediaFragment {
	id: string;
	mediaType: 'IMAGE' | 'VIDEO' | 'AUDIO' | 'GIF' | 'UNKNOWN';
	url: string;
	thumbnailUrl?: string;
	remoteUrl?: string;
	altText?: string;
	sensitive?: boolean;
	spoilerText?: string | null;
	mediaCategory?: 'IMAGE' | 'VIDEO' | 'AUDIO' | 'GIFV' | 'DOCUMENT' | 'UNKNOWN';
	mimeType?: string;
	blurhash?: string;
	metadata?: LesserMediaMetadata;
}

export interface LesserMediaMetadata {
	dimensions?: {
		width: number;
		height: number;
		aspectRatio: number;
	};
	duration?: number;
	frameRate?: number;
	bitrate?: number;
	fileSize?: number;
	format?: string;
}

// Hashtag types
export interface LesserHashtagFragment {
	name: string;
	url: string;
	trendingData?: LesserTrendingData[];
}

export interface LesserTrendingData {
	timestamp: string;
	usage: number;
	accounts: number;
}

// Poll types
export interface LesserPollFragment {
	id: string;
	question: string;
	options: LesserPollOption[];
	expiresAt?: string;
	isExpired: boolean;
	allowsMultiple: boolean;
	totalVotes: number;
	participantCount?: number;
	userVote?: {
		choices: number[];
	};
}

export interface LesserPollOption {
	index: number;
	text: string;
	voteCount?: number;
}

// Basic Object fragment (for notifications, etc.)
export interface LesserObjectFragment {
	id: string;
	type: string;
	actor: LesserAccountFragment;
	content: string;
	inReplyTo?: LesserReplyRef;
	visibility: 'PUBLIC' | 'UNLISTED' | 'PRIVATE' | 'DIRECT';
	sensitive: boolean;
	spoilerText?: string;
	attachments: LesserAttachmentFragment[];
	tags: LesserTagFragment[];
	mentions: LesserMentionFragment[];
	createdAt: string;
	updatedAt: string;
	shareOf?: LesserObjectFragment;
	boostedObject?: LesserObjectFragment;

	// Engagement metrics
	repliesCount: number;
	likesCount: number;
	sharesCount: number;

	// Lesser enhancements
	estimatedCost: number;
	moderationScore?: number;
	communityNotes: LesserCommunityNoteFragment[];

	// Quote Posts extensions
	quoteUrl?: string;
	quoteable: boolean;
	quotePermissions: 'EVERYONE' | 'FOLLOWERS' | 'NONE';
	quoteContext?: LesserQuoteContextFragment;
	quoteCount?: number;

	// AI analysis
	aiAnalysis?: LesserAIAnalysisFragment;
}

// Basic Attachment fragment (matches GraphQL Object.attachments)
export interface LesserAttachmentFragment {
	id: string;
	type: string; // String, not enum
	url: string;
	preview?: string;
	description?: string;
	sensitive?: boolean;
	spoilerText?: string | null;
	mediaCategory?: 'IMAGE' | 'VIDEO' | 'AUDIO' | 'GIFV' | 'DOCUMENT' | 'UNKNOWN';
	mimeType?: string;
	blurhash?: string;
	width?: number;
	height?: number;
	duration?: number;
}

// Basic Tag fragment (matches GraphQL Object.tags)
export interface LesserTagFragment {
	name: string;
	url: string;
}

export interface LesserMentionFragment {
	id: string;
	username: string;
	domain?: string;
	url: string;
}

// Notification types
export interface LesserNotificationFragment {
	id: string;
	notificationType:
		| 'MENTION'
		| 'FOLLOW'
		| 'FOLLOW_REQUEST'
		| 'SHARE'
		| 'FAVORITE'
		| 'POST'
		| 'POLL_ENDED'
		| 'STATUS_UPDATE'
		| 'ADMIN_SIGNUP'
		| 'ADMIN_REPORT'
		| 'QUOTE'
		| 'COMMUNITY_NOTE'
		| 'TRUST_UPDATE'
		| 'COST_ALERT'
		| 'MODERATION_ACTION';
	createdAt: string;
	triggerAccount: LesserAccountFragment;
	status?: LesserObjectFragment; // Changed to LesserObjectFragment (basic Object, not full LesserPostFragment)
	adminReport?: LesserAdminReportFragment;
	isRead?: boolean;

	// Note: Lesser-specific notification payloads are derived from status/triggerAccount Lesser fields
	// - quote: status.quoteContext indicates a quote notification
	// - community_note: status.communityNotes contains the note
	// - trust_update: derived from triggerAccount.trustScore changes
	// - cost_alert/moderation_action: inferred from notification type
}

// Admin report types
export interface LesserAdminReportFragment {
	id: string;
	reportedAccount: LesserAccountFragment;
	reporterAccount: LesserAccountFragment;
	reason: string;
	isActionTaken: boolean;
	submittedAt: string;
	category: 'SPAM' | 'HARASSMENT' | 'HATE_SPEECH' | 'OTHER';
}

// Relationship types
export interface LesserRelationshipFragment {
	target: {
		id: string;
	};
	isFollowing: boolean;
	isFollowedBy: boolean;
	hasPendingRequest: boolean;
	isBlocking: boolean;
	isBlockedBy: boolean;
	isMuting: boolean;
	isMutingNotifications: boolean;
	isDomainBlocked: boolean;
	isEndorsed: boolean;
	personalNote?: string;
}

// Search result types
export interface LesserSearchResults {
	accounts: LesserAccountFragment[];
	posts: LesserPostFragment[];
	hashtags: LesserHashtagFragment[];
}

// Timeline types
export interface LesserTimelineConnection {
	edges: LesserTimelineEdge[];
	pageInfo: LesserPageInfo;
	totalCount?: number;
}

export interface LesserTimelineEdge {
	node: LesserPostFragment;
	cursor: string;
}

export interface LesserPageInfo {
	hasNextPage: boolean;
	hasPreviousPage: boolean;
	startCursor?: string;
	endCursor?: string;
}

// Streaming/subscription types
export interface LesserStreamingUpdate {
	__typename: string;
	eventType:
		| 'POST_CREATED'
		| 'POST_UPDATED'
		| 'POST_DELETED'
		| 'NOTIFICATION_CREATED'
		| 'ACCOUNT_UPDATED';
	timestamp: string;
	data: LesserStreamingData;
}

export type LesserStreamingData =
	| LesserPostStreamingData
	| LesserNotificationStreamingData
	| LesserAccountStreamingData
	| LesserDeleteStreamingData;

export interface LesserPostStreamingData {
	__typename: 'PostStreamingData';
	post: LesserPostFragment;
	timeline?: string;
}

export interface LesserNotificationStreamingData {
	__typename: 'NotificationStreamingData';
	notification: LesserNotificationFragment;
}

export interface LesserAccountStreamingData {
	__typename: 'AccountStreamingData';
	account: LesserAccountFragment;
}

export interface LesserDeleteStreamingData {
	__typename: 'DeleteStreamingData';
	deletedId: string;
	deletedType: 'POST' | 'ACCOUNT' | 'NOTIFICATION';
}

// Query and mutation types
export interface LesserGetTimelineQuery {
	timeline: LesserTimelineConnection;
}

export interface LesserGetTimelineVariables {
	timelineType: 'HOME' | 'PUBLIC' | 'LOCAL' | 'HASHTAG' | 'LIST';
	first?: number;
	after?: string;
	before?: string;
	hashtag?: string;
	listId?: string;
}

export interface LesserGetNotificationsQuery {
	notifications: {
		edges: Array<{
			node: LesserNotificationFragment;
			cursor: string;
		}>;
		pageInfo: LesserPageInfo;
	};
}

export interface LesserGetNotificationsVariables {
	first?: number;
	after?: string;
	types?: string[];
	excludeTypes?: string[];
}

export interface LesserCreatePostMutation {
	createPost: {
		success: boolean;
		post?: LesserPostFragment;
		errors?: string[];
	};
}

export interface LesserCreatePostVariables {
	content: string;
	visibility: 'PUBLIC' | 'UNLISTED' | 'FOLLOWERS' | 'DIRECT';
	contentWarning?: string;
	isSensitive?: boolean;
	replyToId?: string;
	mediaIds?: string[];
	poll?: {
		question: string;
		options: string[];
		allowsMultiple: boolean;
		expiresIn?: number;
	};
}

// Instance/server info types
export interface LesserInstanceInfo {
	domain: string;
	title: string;
	description: string;
	version: string;
	adminContact: string;
	isRegistrationOpen: boolean;
	requiresApproval: boolean;
	supportedLanguages: string[];
	limits: {
		maxPostLength: number;
		maxMediaAttachments: number;
		maxPollOptions: number;
		maxPollOptionLength: number;
		mediaUploadSizeLimit: number;
	};
	statistics: {
		userCount: number;
		postCount: number;
		federatedInstances: number;
	};
}

// Error handling specific to GraphQL
export interface LesserValidationError {
	field: string;
	code: string;
	message: string;
}

export interface LesserMutationError {
	type: 'VALIDATION' | 'AUTHORIZATION' | 'RATE_LIMIT' | 'SERVER_ERROR';
	message: string;
	details?: LesserValidationError[];
}

// Subscription types for real-time updates
export interface LesserTimelineSubscription {
	timelineUpdated: LesserStreamingUpdate;
}

export interface LesserNotificationSubscription {
	notificationReceived: LesserStreamingUpdate;
}

// Type guards and validation helpers
const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === 'object' && value !== null;

export function isLesserGraphQLResponse<T>(obj: unknown): obj is LesserGraphQLResponse<T> {
	if (!isRecord(obj)) {
		return false;
	}

	const errors = obj['errors'];
	return 'data' in obj || ('errors' in obj && Array.isArray(errors));
}

export function isLesserAccountFragment(obj: unknown): obj is LesserAccountFragment {
	if (!isRecord(obj)) {
		return false;
	}

	const id = obj['id'];
	const handle = obj['handle'];
	return typeof id === 'string' && typeof handle === 'string';
}

export function isLesserPostFragment(obj: unknown): obj is LesserPostFragment {
	if (!isRecord(obj)) {
		return false;
	}

	const id = obj['id'];
	const content = obj['content'];
	const author = obj['author'];
	return typeof id === 'string' && typeof content === 'string' && isRecord(author);
}

export function isLesserNotificationFragment(obj: unknown): obj is LesserNotificationFragment {
	if (!isRecord(obj)) {
		return false;
	}

	const id = obj['id'];
	const notificationType = obj['notificationType'];
	const triggerAccount = obj['triggerAccount'];

	return typeof id === 'string' && typeof notificationType === 'string' && isRecord(triggerAccount);
}

export function isLesserStreamingUpdate(obj: unknown): obj is LesserStreamingUpdate {
	if (!isRecord(obj)) {
		return false;
	}

	const typename = obj['__typename'];
	const eventType = obj['eventType'];
	const data = obj['data'];

	return typeof typename === 'string' && typeof eventType === 'string' && isRecord(data);
}

// GraphQL fragment strings (for actual GraphQL usage)
export const LESSER_ACCOUNT_FRAGMENT = `
  fragment LesserAccount on Account {
    id
    handle
    localHandle
    displayName
    bio
    avatarUrl
    bannerUrl
    joinedAt
    isVerified
    isBot
    isLocked
    followerCount
    followingCount
    postCount
    profileFields {
      label
      content
      verifiedAt
    }
    customEmojis {
      code
      imageUrl
      staticUrl
      category
      isVisible
    }
  }
`;

export const LESSER_POST_FRAGMENT = `
  fragment LesserPost on Post {
    id
    publishedAt
    content
    contentWarning
    visibility
    isSensitive
    language
    author {
      ...LesserAccount
    }
    attachments {
      id
      mediaType
      url
      thumbnailUrl
      remoteUrl
      altText
      blurhash
      metadata {
        dimensions {
          width
          height
          aspectRatio
        }
        duration
        frameRate
        bitrate
      }
    }
    mentions {
      account {
        id
        handle
        displayName
        profileUrl
      }
    }
    hashtags {
      name
      url
    }
    emojis {
      code
      imageUrl
      staticUrl
    }
    replyTo {
      id
      authorId
    }
    shareOf {
      ...LesserPost
    }
    interactionCounts {
      replies
      shares
      favorites
    }
    userInteractions {
      isFavorited
      isShared
      isBookmarked
    }
    isPinned
    lastEditedAt
    poll {
      id
      question
      options {
        index
        text
        voteCount
      }
      expiresAt
      isExpired
      allowsMultiple
      totalVotes
      participantCount
      userVote {
        choices
      }
    }
  }
`;

export const LESSER_NOTIFICATION_FRAGMENT = `
  fragment LesserNotification on Notification {
    id
    notificationType
    createdAt
    triggerAccount {
      ...LesserAccount
    }
    targetPost {
      ...LesserPost
    }
    isRead
  }
`;

// Constants for validation
export const LESSER_VISIBILITY_VALUES = ['PUBLIC', 'UNLISTED', 'FOLLOWERS', 'DIRECT'] as const;
export const LESSER_MEDIA_TYPES = ['IMAGE', 'VIDEO', 'AUDIO', 'GIF', 'UNKNOWN'] as const;
export const LESSER_NOTIFICATION_TYPES = [
	'MENTION',
	'FOLLOW',
	'FOLLOW_REQUEST',
	'SHARE',
	'FAVORITE',
	'POST',
	'POLL_ENDED',
	'STATUS_UPDATE',
	'ADMIN_SIGNUP',
	'ADMIN_REPORT',
] as const;
