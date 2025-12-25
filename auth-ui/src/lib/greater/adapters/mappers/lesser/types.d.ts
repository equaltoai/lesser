/**
 * Lesser GraphQL API payload types
 * Fictional GraphQL-based Fediverse API similar to Mastodon functionality
 */
export interface LesserGraphQLResponse<T = unknown> {
	data?: T;
	errors?: LesserGraphQLError[];
	extensions?: Record<string, unknown>;
}
export interface LesserGraphQLError {
	message: string;
	locations?: Array<{
		line: number;
		column: number;
	}>;
	path?: Array<string | number>;
	extensions?: Record<string, unknown>;
}
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
	from?: {
		id: string;
	};
	to?: {
		id: string;
	};
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
	originalNote?: {
		id: string;
	};
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
	repliesCount: number;
	likesCount: number;
	sharesCount: number;
	estimatedCost: number;
	moderationScore?: number;
	communityNotes: LesserCommunityNoteFragment[];
	quoteUrl?: string;
	quoteable: boolean;
	quotePermissions: 'EVERYONE' | 'FOLLOWERS' | 'NONE';
	quoteContext?: LesserQuoteContextFragment;
	quoteCount?: number;
	aiAnalysis?: LesserAIAnalysisFragment;
}
export interface LesserAttachmentFragment {
	id: string;
	type: string;
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
	status?: LesserObjectFragment;
	adminReport?: LesserAdminReportFragment;
	isRead?: boolean;
}
export interface LesserAdminReportFragment {
	id: string;
	reportedAccount: LesserAccountFragment;
	reporterAccount: LesserAccountFragment;
	reason: string;
	isActionTaken: boolean;
	submittedAt: string;
	category: 'SPAM' | 'HARASSMENT' | 'HATE_SPEECH' | 'OTHER';
}
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
export interface LesserSearchResults {
	accounts: LesserAccountFragment[];
	posts: LesserPostFragment[];
	hashtags: LesserHashtagFragment[];
}
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
export interface LesserTimelineSubscription {
	timelineUpdated: LesserStreamingUpdate;
}
export interface LesserNotificationSubscription {
	notificationReceived: LesserStreamingUpdate;
}
export declare function isLesserGraphQLResponse<T>(obj: unknown): obj is LesserGraphQLResponse<T>;
export declare function isLesserAccountFragment(obj: unknown): obj is LesserAccountFragment;
export declare function isLesserPostFragment(obj: unknown): obj is LesserPostFragment;
export declare function isLesserNotificationFragment(
	obj: unknown
): obj is LesserNotificationFragment;
export declare function isLesserStreamingUpdate(obj: unknown): obj is LesserStreamingUpdate;
export declare const LESSER_ACCOUNT_FRAGMENT =
	'\n  fragment LesserAccount on Account {\n    id\n    handle\n    localHandle\n    displayName\n    bio\n    avatarUrl\n    bannerUrl\n    joinedAt\n    isVerified\n    isBot\n    isLocked\n    followerCount\n    followingCount\n    postCount\n    profileFields {\n      label\n      content\n      verifiedAt\n    }\n    customEmojis {\n      code\n      imageUrl\n      staticUrl\n      category\n      isVisible\n    }\n  }\n';
export declare const LESSER_POST_FRAGMENT =
	'\n  fragment LesserPost on Post {\n    id\n    publishedAt\n    content\n    contentWarning\n    visibility\n    isSensitive\n    language\n    author {\n      ...LesserAccount\n    }\n    attachments {\n      id\n      mediaType\n      url\n      thumbnailUrl\n      remoteUrl\n      altText\n      blurhash\n      metadata {\n        dimensions {\n          width\n          height\n          aspectRatio\n        }\n        duration\n        frameRate\n        bitrate\n      }\n    }\n    mentions {\n      account {\n        id\n        handle\n        displayName\n        profileUrl\n      }\n    }\n    hashtags {\n      name\n      url\n    }\n    emojis {\n      code\n      imageUrl\n      staticUrl\n    }\n    replyTo {\n      id\n      authorId\n    }\n    shareOf {\n      ...LesserPost\n    }\n    interactionCounts {\n      replies\n      shares\n      favorites\n    }\n    userInteractions {\n      isFavorited\n      isShared\n      isBookmarked\n    }\n    isPinned\n    lastEditedAt\n    poll {\n      id\n      question\n      options {\n        index\n        text\n        voteCount\n      }\n      expiresAt\n      isExpired\n      allowsMultiple\n      totalVotes\n      participantCount\n      userVote {\n        choices\n      }\n    }\n  }\n';
export declare const LESSER_NOTIFICATION_FRAGMENT =
	'\n  fragment LesserNotification on Notification {\n    id\n    notificationType\n    createdAt\n    triggerAccount {\n      ...LesserAccount\n    }\n    targetPost {\n      ...LesserPost\n    }\n    isRead\n  }\n';
export declare const LESSER_VISIBILITY_VALUES: readonly [
	'PUBLIC',
	'UNLISTED',
	'FOLLOWERS',
	'DIRECT',
];
export declare const LESSER_MEDIA_TYPES: readonly ['IMAGE', 'VIDEO', 'AUDIO', 'GIF', 'UNKNOWN'];
export declare const LESSER_NOTIFICATION_TYPES: readonly [
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
];
//# sourceMappingURL=types.d.ts.map
