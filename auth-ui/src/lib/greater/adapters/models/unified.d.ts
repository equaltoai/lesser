/**
 * Unified data models for Fediverse concepts
 * These models provide a consistent interface across different API sources
 */
export interface SourceMetadata {
	/** Original API source */
	source: 'mastodon' | 'lesser' | 'unknown';
	/** Source-specific API version */
	apiVersion: string;
	/** Timestamp when this data was last updated */
	lastUpdated: number;
	/** Original raw payload for debugging */
	rawPayload?: unknown;
}
export interface UnifiedAccount {
	/** Unique identifier (may be different format per platform) */
	id: string;
	/** Username without domain */
	username: string;
	/** Full username with domain (e.g., user@domain.com) */
	acct: string;
	/** Display name */
	displayName: string;
	/** User bio/note in HTML */
	note: string;
	/** Avatar image URL */
	avatar: string;
	/** Header/banner image URL */
	header: string;
	/** Account creation date */
	createdAt: string;
	/** Follower count */
	followersCount: number;
	/** Following count */
	followingCount: number;
	/** Post count */
	statusesCount: number;
	/** Whether account is locked/requires approval */
	locked: boolean;
	/** Whether account is verified */
	verified: boolean;
	/** Whether this is a bot account */
	bot: boolean;
	/** Account fields/metadata */
	fields: AccountField[];
	/** Relationship to current user (if applicable) */
	relationship?: AccountRelationship;
	/** Source metadata */
	metadata: SourceMetadata;
	/** Trust score (0-100) - Lesser only */
	trustScore?: number;
	/** Detailed reputation information - Lesser only */
	reputation?: {
		actorId: string;
		instance: string;
		totalScore: number;
		trustScore: number;
		activityScore: number;
		moderationScore: number;
		communityScore: number;
		calculatedAt: string;
		version: string;
		evidence: {
			totalPosts: number;
			totalFollowers: number;
			accountAge: number;
			vouchCount: number;
			trustingActors: number;
			averageTrustScore: number;
		};
		signature?: string;
	};
	/** Vouches received - Lesser only */
	vouches?: Array<{
		id: string;
		fromId: string;
		toId: string;
		confidence: number;
		context: string;
		voucherReputation: number;
		createdAt: string;
		expiresAt: string;
		active: boolean;
		revoked: boolean;
		revokedAt?: string;
	}>;
}
export interface AccountField {
	/** Field name */
	name: string;
	/** Field value (HTML) */
	value: string;
	/** Verification timestamp if verified */
	verifiedAt?: string;
}
export interface AccountRelationship {
	/** Whether current user follows this account */
	following: boolean;
	/** Whether current user is followed by this account */
	followedBy: boolean;
	/** Whether current user has requested to follow */
	requested: boolean;
	/** Whether this account is blocked */
	blocking: boolean;
	/** Whether this account is muted */
	muting: boolean;
	/** Whether this account's boosts are muted */
	mutingNotifications: boolean;
	/** Whether this account is domain blocked */
	domainBlocking: boolean;
	/** Whether this account is endorsed */
	endorsed: boolean;
	/** Private note about this account */
	note?: string;
}
export interface UnifiedStatus {
	/** Unique identifier */
	id: string;
	/** Creation timestamp */
	createdAt: string;
	/** Status content in HTML */
	content: string;
	/** Content warning/spoiler text */
	spoilerText?: string;
	/** Visibility level */
	visibility: 'public' | 'unlisted' | 'private' | 'direct';
	/** Whether content is sensitive */
	sensitive: boolean;
	/** Language code */
	language?: string;
	/** Account that posted this status */
	account: UnifiedAccount;
	/** Media attachments */
	mediaAttachments: MediaAttachment[];
	/** Mentions in this status */
	mentions: Mention[];
	/** Hashtags in this status */
	tags: Tag[];
	/** Custom emojis used */
	emojis: CustomEmoji[];
	/** Reply information */
	inReplyTo?: {
		id: string;
		accountId: string;
		account?: UnifiedAccount;
	};
	/** Boost/reblog information */
	reblog?: UnifiedStatus;
	/** Engagement counts */
	repliesCount: number;
	reblogsCount: number;
	favouritesCount: number;
	/** Current user's interaction state */
	favourited: boolean;
	reblogged: boolean;
	bookmarked: boolean;
	/** Pin status */
	pinned: boolean;
	/** Edit history */
	editedAt?: string;
	/** Poll attached to this status */
	poll?: Poll;
	/** Source metadata */
	metadata: SourceMetadata;
	/** Whether this status has been deleted (tombstoned) */
	isDeleted?: boolean;
	/** Timestamp when the status was deleted */
	deletedAt?: string;
	/** Original object type before deletion (if provided by backend) */
	formerType?: string;
	/** Estimated cost in microcents - Lesser only */
	estimatedCost?: number;
	/** Moderation score (0-1, higher = more problematic) - Lesser only */
	moderationScore?: number;
	/** Community notes attached - Lesser only */
	communityNotes?: Array<{
		id: string;
		authorId: string;
		authorUsername: string;
		authorDisplayName: string;
		content: string;
		helpful: number;
		notHelpful: number;
		createdAt: string;
	}>;
	/** Quote post URL - Lesser only */
	quoteUrl?: string;
	/** Whether this can be quoted - Lesser only */
	quoteable?: boolean;
	/** Quote permission level - Lesser only */
	quotePermissions?: 'EVERYONE' | 'FOLLOWERS' | 'NONE';
	/** Quote context - Lesser only */
	quoteContext?: {
		originalAuthorId: string;
		originalNoteId?: string;
		quoteAllowed: boolean;
		quoteType: 'FULL' | 'PARTIAL' | 'COMMENTARY' | 'REACTION';
		withdrawn: boolean;
	};
	/** Number of times quoted - Lesser only */
	quoteCount?: number;
	/** AI analysis results - Lesser only */
	aiAnalysis?: AIAnalysis;
}
export interface AIAnalysis {
	id: string;
	objectId: string;
	objectType: string;
	overallRisk: number;
	moderationAction: 'NONE' | 'FLAG' | 'HIDE' | 'REMOVE' | 'SHADOW_BAN' | 'REVIEW';
	confidence: number;
	analyzedAt: string;
	textAnalysis?: TextAnalysis;
	imageAnalysis?: ImageAnalysis;
	aiDetection?: AIDetection;
	spamAnalysis?: SpamAnalysis;
}
export interface TextAnalysis {
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
export interface ImageAnalysis {
	moderationLabels: Array<{
		name: string;
		confidence: number;
		parentName?: string;
	}>;
	isNSFW: boolean;
	nsfwConfidence: number;
	violenceScore: number;
	weaponsDetected: boolean;
	detectedText: string[];
	textToxicity: number;
	celebrityFaces: Array<{
		name: string;
		confidence: number;
		urls: string[];
	}>;
	deepfakeScore: number;
}
export interface AIDetection {
	aiGeneratedProbability: number;
	generationModel?: string;
	patternConsistency: number;
	styleDeviation: number;
	semanticCoherence: number;
	suspiciousPatterns: string[];
}
export interface SpamAnalysis {
	spamScore: number;
	spamIndicators: Array<{
		type: string;
		description: string;
		severity: number;
	}>;
	postingVelocity: number;
	repetitionScore: number;
	linkDensity: number;
	followerRatio: number;
	interactionRate: number;
	accountAgeDays: number;
}
export interface MediaAttachment {
	/** Unique identifier */
	id: string;
	/** Media type */
	type: 'image' | 'video' | 'audio' | 'gifv' | 'unknown';
	/** Original media URL */
	url: string;
	/** Preview/thumbnail URL */
	previewUrl?: string;
	/** Remote URL if this is from another instance */
	remoteUrl?: string;
	/** Alt text description */
	description?: string;
	/** Whether the media is marked as sensitive */
	sensitive?: boolean;
	/** Spoiler/content warning text */
	spoilerText?: string | null;
	/** Server-reported media category */
	mediaCategory?: 'IMAGE' | 'VIDEO' | 'AUDIO' | 'GIFV' | 'DOCUMENT' | 'UNKNOWN';
	/** MIME type reported by the backend */
	mimeType?: string;
	/** Blurhash for placeholder */
	blurhash?: string;
	/** Media dimensions and metadata */
	meta?: MediaMeta;
}
export interface MediaMeta {
	/** Original media properties */
	original?: {
		width: number;
		height: number;
		size?: string;
		aspect?: number;
		duration?: number;
		fps?: number;
		bitrate?: number;
	};
	/** Small/preview properties */
	small?: {
		width: number;
		height: number;
		size?: string;
		aspect?: number;
	};
}
export interface Mention {
	/** Mentioned account ID */
	id: string;
	/** Username */
	username: string;
	/** Full username with domain */
	acct: string;
	/** Profile URL */
	url: string;
}
export interface Tag {
	/** Tag name (without #) */
	name: string;
	/** Tag URL */
	url: string;
	/** Usage statistics */
	history?: TagHistory[];
}
export interface TagHistory {
	/** Day timestamp */
	day: string;
	/** Number of uses */
	uses: string;
	/** Number of accounts using */
	accounts: string;
}
export interface CustomEmoji {
	/** Emoji shortcode */
	shortcode: string;
	/** Static image URL */
	staticUrl: string;
	/** Animated image URL */
	url: string;
	/** Whether this emoji is visible in picker */
	visibleInPicker: boolean;
	/** Emoji category */
	category?: string;
}
export interface Poll {
	/** Poll ID */
	id: string;
	/** Poll expiration date */
	expiresAt?: string;
	/** Whether poll has expired */
	expired: boolean;
	/** Whether multiple choices allowed */
	multiple: boolean;
	/** Total votes cast */
	votesCount: number;
	/** Number of voters */
	votersCount?: number;
	/** Poll options */
	options: PollOption[];
	/** Whether current user has voted */
	voted?: boolean;
	/** Current user's choices (if voted) */
	ownVotes?: number[];
}
export interface PollOption {
	/** Option title */
	title: string;
	/** Number of votes for this option */
	votesCount?: number;
}
export interface UnifiedNotification {
	/** Unique identifier */
	id: string;
	/** Notification type */
	type:
		| 'mention'
		| 'status'
		| 'reblog'
		| 'follow'
		| 'follow_request'
		| 'favourite'
		| 'poll'
		| 'update'
		| 'admin.sign_up'
		| 'admin.report'
		| 'quote'
		| 'community_note'
		| 'trust_update'
		| 'cost_alert'
		| 'moderation_action';
	/** Creation timestamp */
	createdAt: string;
	/** Account that triggered this notification */
	account: UnifiedAccount;
	/** Associated status (if applicable) */
	status?: UnifiedStatus;
	/** Associated report (for admin notifications) */
	report?: AdminReport;
	/** Whether notification has been read */
	read?: boolean;
	/** Source metadata */
	metadata: SourceMetadata;
	/** Quote status (for quote notifications) */
	quoteStatus?: UnifiedStatus;
	/** Community note details (for community_note notifications) */
	communityNote?: {
		id: string;
		authorId: string;
		authorUsername: string;
		authorDisplayName: string;
		content: string;
		helpful: number;
		notHelpful: number;
		createdAt: string;
	};
	/** Trust update details (for trust_update notifications) */
	trustUpdate?: {
		newScore: number;
		previousScore?: number;
		reason?: string;
	};
	/** Cost alert details (for cost_alert notifications) */
	costAlert?: {
		amount: number;
		threshold: number;
		message: string;
	};
	/** Moderation action details (for moderation_action notifications) */
	moderationAction?: {
		action: string;
		reason: string;
		statusId?: string;
	};
}
export interface AdminReport {
	/** Report ID */
	id: string;
	/** Reported account */
	targetAccount: UnifiedAccount;
	/** Reporting account */
	account: UnifiedAccount;
	/** Report reason */
	comment: string;
	/** Whether action was taken */
	actionTaken: boolean;
	/** Creation timestamp */
	createdAt: string;
}
export interface StreamingUpdate {
	/** Update type */
	type:
		| 'status'
		| 'delete'
		| 'notification'
		| 'filters_changed'
		| 'conversation'
		| 'announcement'
		| 'timelineUpdates'
		| 'notificationStream'
		| 'conversationUpdates'
		| 'listUpdates'
		| 'activityStream'
		| 'relationshipUpdates'
		| 'quoteActivity'
		| 'hashtagActivity'
		| 'trustUpdates'
		| 'moderationEvents'
		| 'moderationAlerts'
		| 'moderationQueueUpdate'
		| 'threatIntelligence'
		| 'aiAnalysisUpdates'
		| 'costUpdates'
		| 'costAlerts'
		| 'budgetAlerts'
		| 'metricsUpdates'
		| 'performanceAlert'
		| 'federationHealthUpdates'
		| 'infrastructureEvent';
	/** Event data */
	payload: unknown;
	/** Stream this came from */
	stream: string;
	/** Event timestamp */
	timestamp: number;
	/** Source metadata */
	metadata: SourceMetadata;
}
export interface StreamingDelete {
	/** ID of deleted item */
	id: string;
	/** Type of deleted item */
	itemType: 'status' | 'account' | 'notification';
	/** Deletion timestamp */
	timestamp: number;
}
export interface StreamingEdit {
	/** ID of edited item */
	id: string;
	/** Updated item data */
	data: UnifiedStatus | UnifiedAccount | UnifiedNotification;
	/** Edit timestamp */
	timestamp: number;
	/** Edit reason/type */
	editType: 'content' | 'metadata' | 'visibility';
}
export interface MappingError extends Error {
	/** Error type */
	type: 'validation' | 'transformation' | 'missing_field' | 'unknown_format';
	/** Original payload that caused error */
	payload?: unknown;
	/** Field path where error occurred */
	fieldPath?: string;
	/** Source API information */
	source?: {
		api: string;
		version: string;
		endpoint?: string;
	};
}
export interface MapperResult<T> {
	/** Whether mapping was successful */
	success: boolean;
	/** Mapped data (if successful) */
	data?: T;
	/** Error information (if failed) */
	error?: MappingError;
	/** Warnings during mapping */
	warnings?: string[];
	/** Performance metrics */
	metrics?: {
		mappingTimeMs: number;
		fieldsProcessed: number;
		fallbacksUsed: number;
	};
}
export interface BatchMapperResult<T> {
	/** Successfully mapped items */
	successful: T[];
	/** Failed mappings with errors */
	failed: {
		payload: unknown;
		error: MappingError;
	}[];
	/** Total items processed */
	totalProcessed: number;
	/** Processing time in milliseconds */
	processingTimeMs: number;
}
export interface ValidationRule<T = unknown> {
	/** Rule name for debugging */
	name: string;
	/** Validation function */
	validate: (value: T) => boolean;
	/** Error message if validation fails */
	message: string;
	/** Whether this rule is required vs optional */
	required?: boolean;
}
export interface FieldValidator {
	/** Field path in dot notation */
	path: string;
	/** Validation rules for this field */
	rules: ValidationRule<unknown>[];
	/** Transformer function */
	transform?: (value: unknown) => unknown;
	/** Default value if field is missing */
	defaultValue?: unknown;
}
//# sourceMappingURL=unified.d.ts.map
