/**
 * Lesser GraphQL API payload mappers
 * Transform Lesser GraphQL API responses to unified models
 */

import type {
	LesserAccountFragment,
	LesserPostFragment,
	LesserNotificationFragment,
	LesserMediaFragment,
	LesserAttachmentFragment,
	LesserMentionFragment,
	LesserHashtagFragment,
	LesserEmojiFragment,
	LesserPollFragment,
	LesserRelationshipFragment,
	LesserStreamingUpdate,
	LesserDeleteStreamingData,
	LesserPostStreamingData,
	LesserNotificationStreamingData,
	LesserAccountStreamingData,
	LesserStreamingData,
	LesserObjectFragment,
	LesserGraphQLResponse,
	LesserGraphQLError,
	LesserTimelineConnection,
	LesserAIAnalysisFragment,
} from './types.js';

import type {
	UnifiedAccount,
	UnifiedStatus,
	UnifiedNotification,
	MediaAttachment,
	Mention,
	Tag,
	CustomEmoji,
	Poll,
	AccountRelationship,
	StreamingUpdate,
	StreamingDelete,
	StreamingEdit,
	MapperResult,
	BatchMapperResult,
	MappingError,
	SourceMetadata,
	AIAnalysis,
	AdminReport,
} from '../../models/unified.js';

/**
 * Create source metadata for Lesser GraphQL payloads
 */
function createLesserMetadata(rawPayload?: unknown): SourceMetadata {
	return {
		source: 'lesser',
		apiVersion: 'graphql-v1',
		lastUpdated: Date.now(),
		rawPayload,
	};
}

/**
 * Create a mapping error with GraphQL context
 */
function createMappingError(
	type: MappingError['type'],
	message: string,
	payload?: unknown,
	fieldPath?: string,
	graphqlErrors?: LesserGraphQLError[]
): MappingError {
	const error = new Error(message) as MappingError;
	error.type = type;
	error.payload = payload;
	error.fieldPath = fieldPath;
	error.source = {
		api: 'lesser',
		version: 'graphql-v1',
	};

	// Add GraphQL error context if available
	if (graphqlErrors && graphqlErrors.length > 0) {
		error.message += ` GraphQL errors: ${graphqlErrors.map((e) => e.message).join(', ')}`;
	}

	return error;
}

/**
 * Safely extract string field with fallback
 */
function safeString(value: unknown, fallback = ''): string {
	return typeof value === 'string' ? value : fallback;
}

/**
 * Safely extract number field with fallback
 */
function safeNumber(value: unknown, fallback = 0): number {
	return typeof value === 'number' ? value : fallback;
}

/**
 * Safely extract boolean field with fallback
 */
function safeBoolean(value: unknown, fallback = false): boolean {
	return typeof value === 'boolean' ? value : fallback;
}

/**
 * Map Lesser GraphQL visibility to unified visibility
 */
function mapLesserVisibility(visibility: string): 'public' | 'unlisted' | 'private' | 'direct' {
	switch (visibility) {
		case 'PUBLIC':
			return 'public';
		case 'UNLISTED':
			return 'unlisted';
		case 'FOLLOWERS':
			return 'private';
		case 'DIRECT':
			return 'direct';
		default:
			return 'public';
	}
}

/**
 * Map Lesser media type to unified media type
 */
function mapLesserMediaType(mediaType: string): 'image' | 'video' | 'audio' | 'gifv' | 'unknown' {
	switch (mediaType) {
		case 'IMAGE':
			return 'image';
		case 'VIDEO':
			return 'video';
		case 'AUDIO':
			return 'audio';
		case 'GIF':
		case 'GIFV':
			return 'gifv';
		case 'DOCUMENT':
			return 'unknown';
		default:
			return 'unknown';
	}
}

/**
 * Map Lesser notification type to unified notification type
 */
function mapLesserNotificationType(type: string): UnifiedNotification['type'] {
	switch (type) {
		case 'MENTION':
			return 'mention';
		case 'FOLLOW':
			return 'follow';
		case 'FOLLOW_REQUEST':
			return 'follow_request';
		case 'SHARE':
			return 'reblog';
		case 'FAVORITE':
			return 'favourite';
		case 'POST':
			return 'status';
		case 'POLL_ENDED':
			return 'poll';
		case 'STATUS_UPDATE':
			return 'update';
		case 'ADMIN_SIGNUP':
			return 'admin.sign_up';
		case 'ADMIN_REPORT':
			return 'admin.report';
		// Lesser-specific notification types
		case 'QUOTE':
			return 'quote';
		case 'COMMUNITY_NOTE':
			return 'community_note';
		case 'TRUST_UPDATE':
			return 'trust_update';
		case 'COST_ALERT':
			return 'cost_alert';
		case 'MODERATION_ACTION':
			return 'moderation_action';
		default:
			return 'mention';
	}
}

/**
 * Map Lesser account to unified account model
 */
export function mapLesserAccount(
	account: LesserAccountFragment,
	relationship?: LesserRelationshipFragment
): MapperResult<UnifiedAccount> {
	const startTime = performance.now();

	try {
		if (!account || typeof account.id !== 'string') {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid account: missing or invalid id', account),
			};
		}

		const unified: UnifiedAccount = {
			id: account.id,
			username: safeString(account.localHandle || account.handle.split('@')[0]),
			acct: safeString(account.handle),
			displayName: safeString(account.displayName),
			note: safeString(account.bio),
			avatar: safeString(account.avatarUrl),
			header: safeString(account.bannerUrl),
			createdAt: safeString(account.joinedAt),
			followersCount: safeNumber(account.followerCount),
			followingCount: safeNumber(account.followingCount),
			statusesCount: safeNumber(account.postCount),
			locked: safeBoolean(account.isLocked),
			verified: safeBoolean(account.isVerified),
			bot: safeBoolean(account.isBot),
			fields: (account.profileFields || []).map((field) => ({
				name: safeString(field.label),
				value: safeString(field.content),
				verifiedAt: field.verifiedAt || undefined,
			})),
			relationship: relationship ? mapLesserRelationship(relationship) : undefined,
			metadata: createLesserMetadata(account),

			// Map Lesser-specific fields
			trustScore: account.trustScore !== undefined ? safeNumber(account.trustScore) : undefined,
			reputation: account.reputation
				? {
						actorId: safeString(account.reputation.actorId),
						instance: safeString(account.reputation.instance),
						totalScore: safeNumber(account.reputation.totalScore),
						trustScore: safeNumber(account.reputation.trustScore),
						activityScore: safeNumber(account.reputation.activityScore),
						moderationScore: safeNumber(account.reputation.moderationScore),
						communityScore: safeNumber(account.reputation.communityScore),
						calculatedAt: safeString(account.reputation.calculatedAt),
						version: safeString(account.reputation.version),
						evidence: {
							totalPosts: safeNumber(account.reputation.evidence?.totalPosts),
							totalFollowers: safeNumber(account.reputation.evidence?.totalFollowers),
							accountAge: safeNumber(account.reputation.evidence?.accountAge),
							vouchCount: safeNumber(account.reputation.evidence?.vouchCount),
							trustingActors: safeNumber(account.reputation.evidence?.trustingActors),
							averageTrustScore: safeNumber(account.reputation.evidence?.averageTrustScore),
						},
						signature: account.reputation.signature || undefined,
					}
				: undefined,
			vouches: account.vouches
				? account.vouches.map((vouch) => ({
						id: safeString(vouch.id),
						fromId: safeString(vouch.from?.id || ''),
						toId: safeString(vouch.to?.id || ''),
						confidence: safeNumber(vouch.confidence),
						context: safeString(vouch.context),
						voucherReputation: safeNumber(vouch.voucherReputation),
						createdAt: safeString(vouch.createdAt),
						expiresAt: safeString(vouch.expiresAt),
						active: safeBoolean(vouch.active),
						revoked: safeBoolean(vouch.revoked),
						revokedAt: vouch.revokedAt || undefined,
					}))
				: undefined,
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(account).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map account: ${error instanceof Error ? error.message : 'Unknown error'}`,
				account
			),
		};
	}
}

/**
 * Map Lesser relationship to unified relationship model
 */
function mapLesserRelationship(relationship: LesserRelationshipFragment): AccountRelationship {
	return {
		following: safeBoolean(relationship.isFollowing),
		followedBy: safeBoolean(relationship.isFollowedBy),
		requested: safeBoolean(relationship.hasPendingRequest),
		blocking: safeBoolean(relationship.isBlocking),
		muting: safeBoolean(relationship.isMuting),
		mutingNotifications: safeBoolean(relationship.isMutingNotifications),
		domainBlocking: safeBoolean(relationship.isDomainBlocked),
		endorsed: safeBoolean(relationship.isEndorsed),
		note: relationship.personalNote || undefined,
	};
}

function normalizeAttachmentType(type: string): MediaAttachment['type'] {
	return mapLesserMediaType(type?.toUpperCase() ?? 'UNKNOWN');
}

function mapLesserObjectAttachment(attachment: LesserAttachmentFragment): MediaAttachment {
	return {
		id: safeString(attachment.id),
		type: normalizeAttachmentType(attachment.type),
		url: safeString(attachment.url),
		previewUrl: attachment.preview || undefined,
		remoteUrl: undefined,
		description: attachment.description || undefined,
		sensitive: attachment.sensitive,
		spoilerText: attachment.spoilerText ?? undefined,
		mediaCategory: attachment.mediaCategory,
		mimeType: attachment.mimeType,
		blurhash: attachment.blurhash || undefined,
		meta:
			attachment.width && attachment.height
				? {
						original: {
							width: safeNumber(attachment.width),
							height: safeNumber(attachment.height),
							aspect: attachment.height ? attachment.width / attachment.height : 0,
						},
					}
				: undefined,
	};
}

function isPostStreamingData(data: LesserStreamingData): data is LesserPostStreamingData {
	return data.__typename === 'PostStreamingData';
}

function isNotificationStreamingData(
	data: LesserStreamingData
): data is LesserNotificationStreamingData {
	return data.__typename === 'NotificationStreamingData';
}

function isAccountStreamingData(data: LesserStreamingData): data is LesserAccountStreamingData {
	return data.__typename === 'AccountStreamingData';
}

function isDeleteStreamingData(data: LesserStreamingData): data is LesserDeleteStreamingData {
	return data.__typename === 'DeleteStreamingData';
}

function mapLesserAIAnalysis(aiAnalysis: LesserAIAnalysisFragment): AIAnalysis {
	return {
		id: safeString(aiAnalysis.id),
		objectId: safeString(aiAnalysis.objectId),
		objectType: safeString(aiAnalysis.objectType),
		overallRisk: safeNumber(aiAnalysis.overallRisk),
		moderationAction: aiAnalysis.moderationAction,
		confidence: safeNumber(aiAnalysis.confidence),
		analyzedAt: safeString(aiAnalysis.analyzedAt),
		textAnalysis: aiAnalysis.textAnalysis
			? {
					sentiment: aiAnalysis.textAnalysis.sentiment,
					sentimentScores: {
						positive: safeNumber(aiAnalysis.textAnalysis.sentimentScores.positive),
						negative: safeNumber(aiAnalysis.textAnalysis.sentimentScores.negative),
						neutral: safeNumber(aiAnalysis.textAnalysis.sentimentScores.neutral),
						mixed: safeNumber(aiAnalysis.textAnalysis.sentimentScores.mixed),
					},
					toxicityScore: safeNumber(aiAnalysis.textAnalysis.toxicityScore),
					toxicityLabels: aiAnalysis.textAnalysis.toxicityLabels || [],
					containsPII: safeBoolean(aiAnalysis.textAnalysis.containsPII),
					dominantLanguage: safeString(aiAnalysis.textAnalysis.dominantLanguage),
					entities: (aiAnalysis.textAnalysis.entities || []).map((entity) => ({
						text: safeString(entity.text),
						type: safeString(entity.type),
						confidence: safeNumber(entity.confidence),
					})),
					keyPhrases: aiAnalysis.textAnalysis.keyPhrases || [],
				}
			: undefined,
		imageAnalysis: aiAnalysis.imageAnalysis
			? {
					moderationLabels: (aiAnalysis.imageAnalysis.moderationLabels || []).map((label) => ({
						name: safeString(label.name),
						confidence: safeNumber(label.confidence),
						parentName: safeString(label.parentName),
					})),
					isNSFW: safeBoolean(aiAnalysis.imageAnalysis.isNSFW),
					nsfwConfidence: safeNumber(aiAnalysis.imageAnalysis.nsfwConfidence),
					violenceScore: safeNumber(aiAnalysis.imageAnalysis.violenceScore),
					weaponsDetected: safeBoolean(aiAnalysis.imageAnalysis.weaponsDetected),
					detectedText: aiAnalysis.imageAnalysis.detectedText || [],
					textToxicity: safeNumber(aiAnalysis.imageAnalysis.textToxicity),
					celebrityFaces: (aiAnalysis.imageAnalysis.celebrityFaces || []).map((celeb) => ({
						name: safeString(celeb.name),
						confidence: safeNumber(celeb.confidence),
						urls: celeb.urls || [],
					})),
					deepfakeScore: safeNumber(aiAnalysis.imageAnalysis.deepfakeScore),
				}
			: undefined,
		aiDetection: aiAnalysis.aiDetection
			? {
					aiGeneratedProbability: safeNumber(aiAnalysis.aiDetection.aiGeneratedProbability),
					generationModel: safeString(aiAnalysis.aiDetection.generationModel),
					patternConsistency: safeNumber(aiAnalysis.aiDetection.patternConsistency),
					styleDeviation: safeNumber(aiAnalysis.aiDetection.styleDeviation),
					semanticCoherence: safeNumber(aiAnalysis.aiDetection.semanticCoherence),
					suspiciousPatterns: aiAnalysis.aiDetection.suspiciousPatterns || [],
				}
			: undefined,
		spamAnalysis: aiAnalysis.spamAnalysis
			? {
					spamScore: safeNumber(aiAnalysis.spamAnalysis.spamScore),
					spamIndicators: (aiAnalysis.spamAnalysis.spamIndicators || []).map((indicator) => ({
						type: safeString(indicator.type),
						description: safeString(indicator.description),
						severity: safeNumber(indicator.severity),
					})),
					postingVelocity: safeNumber(aiAnalysis.spamAnalysis.postingVelocity),
					repetitionScore: safeNumber(aiAnalysis.spamAnalysis.repetitionScore),
					linkDensity: safeNumber(aiAnalysis.spamAnalysis.linkDensity),
					followerRatio: safeNumber(aiAnalysis.spamAnalysis.followerRatio),
					interactionRate: safeNumber(aiAnalysis.spamAnalysis.interactionRate),
					accountAgeDays: safeNumber(aiAnalysis.spamAnalysis.accountAgeDays),
				}
			: undefined,
	};
}

/**
 * Map a basic Lesser Object to UnifiedStatus
 * Used for notifications and other contexts where we have a basic Object, not a full LesserPostFragment
 */
export function mapLesserObject(obj: LesserObjectFragment): MapperResult<UnifiedStatus> {
	const startTime = performance.now();

	try {
		if (!obj || typeof obj.id !== 'string') {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid object: missing or invalid id', obj),
			};
		}

		if (!obj.actor) {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid object: missing actor', obj),
			};
		}

		const accountResult = mapLesserAccount(obj.actor);
		if (!accountResult.success) {
			return {
				success: false,
				error: createMappingError('transformation', 'Failed to map object actor', obj, 'actor'),
			};
		}
		if (!accountResult.data) {
			return {
				success: false,
				error: createMappingError(
					'transformation',
					'Mapped object actor missing data',
					obj,
					'actor'
				),
			};
		}

		const communityNotes = obj.communityNotes
			? obj.communityNotes.map((note) => ({
					id: safeString(note.id),
					authorId: safeString(note.author?.id || ''),
					authorUsername: safeString(note.author?.handle || ''),
					authorDisplayName: safeString(note.author?.displayName || ''),
					content: safeString(note.content),
					helpful: safeNumber(note.helpful),
					notHelpful: safeNumber(note.notHelpful),
					createdAt: safeString(note.createdAt),
				}))
			: undefined;

		const replyTo = obj.inReplyTo;
		const replyAccountResult =
			replyTo?.actor !== undefined ? mapLesserAccount(replyTo.actor) : undefined;
		const replyAccount =
			replyAccountResult && replyAccountResult.success ? replyAccountResult.data : undefined;

		const reblogResult = obj.shareOf
			? mapLesserObject(obj.shareOf)
			: obj.boostedObject
				? mapLesserObject(obj.boostedObject)
				: undefined;
		const reblogStatus = reblogResult && reblogResult.success ? reblogResult.data : undefined;

		const unified: UnifiedStatus = {
			id: obj.id,
			createdAt: safeString(obj.createdAt),
			content: safeString(obj.content),
			spoilerText: obj.spoilerText || undefined,
			sensitive: safeBoolean(obj.sensitive),
			visibility: mapLesserVisibility(obj.visibility),
			account: accountResult.data,
			mediaAttachments: obj.attachments?.map(mapLesserObjectAttachment) || [],
			mentions: (obj.mentions || []).map(mapLesserMention),
			tags:
				obj.tags?.map((tag) => ({
					name: safeString(tag.name),
					url: safeString(tag.url),
				})) || [],
			emojis: [],
			inReplyTo: replyTo
				? {
						id: safeString(replyTo.id),
						accountId: safeString(replyTo.authorId ?? replyAccount?.id ?? ''),
						account: replyAccount,
					}
				: undefined,
			reblog: reblogStatus,
			repliesCount: safeNumber(obj.repliesCount),
			reblogsCount: safeNumber(obj.sharesCount),
			favouritesCount: safeNumber(obj.likesCount),
			favourited: false,
			reblogged: false,
			bookmarked: false,
			pinned: false,
			metadata: createLesserMetadata(obj),

			// Lesser-specific fields
			estimatedCost: obj.estimatedCost !== undefined ? safeNumber(obj.estimatedCost) : undefined,
			moderationScore:
				obj.moderationScore !== undefined ? safeNumber(obj.moderationScore) : undefined,
			communityNotes,
			quoteUrl: obj.quoteUrl || undefined,
			quoteable: obj.quoteable !== undefined ? safeBoolean(obj.quoteable) : undefined,
			quotePermissions: obj.quotePermissions || undefined,
			quoteContext: obj.quoteContext
				? {
						originalAuthorId: safeString(obj.quoteContext.originalAuthor?.id || ''),
						originalNoteId: obj.quoteContext.originalNote?.id || undefined,
						quoteAllowed: safeBoolean(obj.quoteContext.quoteAllowed),
						quoteType: obj.quoteContext.quoteType || 'FULL',
						withdrawn: safeBoolean(obj.quoteContext.withdrawn),
					}
				: undefined,
			quoteCount: obj.quoteCount !== undefined ? safeNumber(obj.quoteCount) : undefined,
			aiAnalysis: obj.aiAnalysis ? mapLesserAIAnalysis(obj.aiAnalysis) : undefined,
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(obj).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		const endTime = performance.now();
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map Lesser object: ${error instanceof Error ? error.message : 'Unknown error'}`,
				obj
			),
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: 0,
				fallbacksUsed: 0,
			},
		};
	}
}

/**
 * Map Lesser post to unified status model
 */
export function mapLesserPost(post: LesserPostFragment): MapperResult<UnifiedStatus> {
	const startTime = performance.now();

	try {
		if (!post || typeof post.id !== 'string') {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid post: missing or invalid id', post),
			};
		}

		if (!post.author) {
			return {
				success: false,
				error: createMappingError('validation', 'Invalid post: missing author', post),
			};
		}

		const accountResult = mapLesserAccount(post.author);
		if (!accountResult.success || !accountResult.data) {
			return {
				success: false,
				error:
					accountResult.error ??
					createMappingError('transformation', 'Failed to map post author', post, 'author'),
			};
		}

		const account = accountResult.data;
		const replyTo = post.inReplyTo ?? post.replyTo;
		const replyAccountResult =
			replyTo?.actor !== undefined ? mapLesserAccount(replyTo.actor) : undefined;
		const replyAccount =
			replyAccountResult && replyAccountResult.success ? replyAccountResult.data : undefined;
		const reblogSource = post.shareOf ?? post.boostedObject;
		const reblogResult = reblogSource ? mapLesserPost(reblogSource) : undefined;
		const reblogStatus = reblogResult && reblogResult.success ? reblogResult.data : undefined;

		const unified: UnifiedStatus = {
			id: post.id,
			createdAt: safeString(post.publishedAt),
			content: safeString(post.content),
			spoilerText: post.contentWarning || undefined,
			visibility: mapLesserVisibility(post.visibility || 'PUBLIC'),
			sensitive: safeBoolean(post.isSensitive),
			language: post.language || undefined,
			account,
			mediaAttachments: (post.attachments || []).map(mapLesserMediaAttachment),
			mentions: (post.mentions || []).map(mapLesserMention),
			tags: (post.hashtags || []).map(mapLesserHashtag),
			emojis: (post.emojis || []).map(mapLesserEmoji),
			inReplyTo: replyTo
				? {
						id: replyTo.id,
						accountId: safeString(replyTo.authorId ?? replyAccount?.id ?? ''),
						account: replyAccount,
					}
				: undefined,
			reblog: reblogStatus,
			repliesCount: safeNumber(post.interactionCounts?.replies),
			reblogsCount: safeNumber(post.interactionCounts?.shares),
			favouritesCount: safeNumber(post.interactionCounts?.favorites),
			favourited: safeBoolean(post.userInteractions?.isFavorited),
			reblogged: safeBoolean(post.userInteractions?.isShared),
			bookmarked: safeBoolean(post.userInteractions?.isBookmarked),
			pinned: safeBoolean(post.isPinned),
			editedAt: post.lastEditedAt || undefined,
			poll: post.poll ? mapLesserPoll(post.poll) : undefined,
			metadata: createLesserMetadata(post),

			// Map Lesser-specific fields
			estimatedCost: post.estimatedCost !== undefined ? safeNumber(post.estimatedCost) : undefined,
			moderationScore:
				post.moderationScore !== undefined ? safeNumber(post.moderationScore) : undefined,
			communityNotes: post.communityNotes
				? post.communityNotes.map((note) => ({
						id: safeString(note.id),
						authorId: safeString(note.author?.id || ''),
						authorUsername: safeString(note.author?.handle || ''),
						authorDisplayName: safeString(note.author?.displayName || ''),
						content: safeString(note.content),
						helpful: safeNumber(note.helpful),
						notHelpful: safeNumber(note.notHelpful),
						createdAt: safeString(note.createdAt),
					}))
				: undefined,
			quoteUrl: post.quoteUrl || undefined,
			quoteable: post.quoteable !== undefined ? safeBoolean(post.quoteable) : undefined,
			quotePermissions: post.quotePermissions || undefined,
			quoteContext: post.quoteContext
				? {
						originalAuthorId: safeString(post.quoteContext.originalAuthor?.id || ''),
						originalNoteId: post.quoteContext.originalNote?.id || undefined,
						quoteAllowed: safeBoolean(post.quoteContext.quoteAllowed),
						quoteType: post.quoteContext.quoteType || 'FULL',
						withdrawn: safeBoolean(post.quoteContext.withdrawn),
					}
				: undefined,
			quoteCount: post.quoteCount !== undefined ? safeNumber(post.quoteCount) : undefined,
			aiAnalysis: post.aiAnalysis ? mapLesserAIAnalysis(post.aiAnalysis) : undefined,
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(post).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map post: ${error instanceof Error ? error.message : 'Unknown error'}`,
				post
			),
		};
	}
}

/**
 * Map Lesser media attachment to unified model
 */
function mapLesserMediaAttachment(attachment: LesserMediaFragment): MediaAttachment {
	return {
		id: safeString(attachment.id),
		type: mapLesserMediaType(attachment.mediaType || 'UNKNOWN'),
		url: safeString(attachment.url),
		previewUrl: attachment.thumbnailUrl || undefined,
		remoteUrl: attachment.remoteUrl || undefined,
		description: attachment.altText || undefined,
		sensitive: attachment.sensitive,
		spoilerText: attachment.spoilerText || undefined,
		mediaCategory: attachment.mediaCategory,
		mimeType: attachment.mimeType,
		blurhash: attachment.blurhash || undefined,
		meta: attachment.metadata
			? {
					original: attachment.metadata.dimensions
						? {
								width: safeNumber(attachment.metadata.dimensions.width),
								height: safeNumber(attachment.metadata.dimensions.height),
								aspect: safeNumber(attachment.metadata.dimensions.aspectRatio),
								duration: attachment.metadata.duration || undefined,
								fps: attachment.metadata.frameRate || undefined,
								bitrate: attachment.metadata.bitrate || undefined,
							}
						: undefined,
				}
			: undefined,
	};
}

/**
 * Map Lesser mention to unified model
 */
function mapLesserMention(mention: LesserMentionFragment): Mention {
	return {
		id: safeString(mention.id),
		username: safeString(mention.username),
		acct: safeString(mention.domain ? `${mention.username}@${mention.domain}` : mention.username),
		url: safeString(mention.url),
	};
}

/**
 * Map Lesser hashtag to unified model
 */
function mapLesserHashtag(hashtag: LesserHashtagFragment): Tag {
	return {
		name: safeString(hashtag.name),
		url: safeString(hashtag.url),
		history: hashtag.trendingData?.map((data) => ({
			day: safeString(data.timestamp),
			uses: safeString(data.usage),
			accounts: safeString(data.accounts),
		})),
	};
}

/**
 * Map Lesser emoji to unified model
 */
function mapLesserEmoji(emoji: LesserEmojiFragment): CustomEmoji {
	return {
		shortcode: safeString(emoji.code),
		staticUrl: safeString(emoji.staticUrl),
		url: safeString(emoji.imageUrl),
		visibleInPicker: safeBoolean(emoji.isVisible),
		category: emoji.category || undefined,
	};
}

/**
 * Map Lesser poll to unified model
 */
function mapLesserPoll(poll: LesserPollFragment): Poll {
	return {
		id: safeString(poll.id),
		expiresAt: poll.expiresAt || undefined,
		expired: safeBoolean(poll.isExpired),
		multiple: safeBoolean(poll.allowsMultiple),
		votesCount: safeNumber(poll.totalVotes),
		votersCount:
			poll.participantCount !== undefined ? safeNumber(poll.participantCount) : undefined,
		options: (poll.options || []).map((option) => ({
			title: safeString(option.text),
			votesCount: option.voteCount !== undefined ? safeNumber(option.voteCount) : undefined,
		})),
		voted: poll.userVote !== undefined,
		ownVotes: poll.userVote?.choices || undefined,
	};
}

/**
 * Map Lesser notification to unified notification model
 */
export function mapLesserNotification(
	notification: LesserNotificationFragment
): MapperResult<UnifiedNotification> {
	const startTime = performance.now();

	try {
		if (!notification || typeof notification.id !== 'string') {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid notification: missing or invalid id',
					notification
				),
			};
		}

		if (!notification.triggerAccount) {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid notification: missing trigger account',
					notification
				),
			};
		}

		const accountResult = mapLesserAccount(notification.triggerAccount);
		if (!accountResult.success || !accountResult.data) {
			return {
				success: false,
				error:
					accountResult.error ??
					createMappingError(
						'transformation',
						'Failed to map notification account',
						notification,
						'triggerAccount'
					),
			};
		}

		const account = accountResult.data;
		const statusResult = notification.status ? mapLesserObject(notification.status) : undefined;
		const status = statusResult && statusResult.success ? statusResult.data : undefined;

		let report: AdminReport | undefined;
		if (notification.adminReport) {
			const reportedAccountResult = mapLesserAccount(notification.adminReport.reportedAccount);
			const reporterAccountResult = mapLesserAccount(notification.adminReport.reporterAccount);
			if (
				reportedAccountResult.success &&
				reportedAccountResult.data &&
				reporterAccountResult.success &&
				reporterAccountResult.data
			) {
				report = {
					id: safeString(notification.adminReport.id),
					targetAccount: reportedAccountResult.data,
					account: reporterAccountResult.data,
					comment: safeString(notification.adminReport.reason),
					actionTaken: safeBoolean(notification.adminReport.isActionTaken),
					createdAt: safeString(notification.adminReport.submittedAt),
				};
			}
		}

		const unified: UnifiedNotification = {
			id: notification.id,
			type: mapLesserNotificationType(notification.notificationType),
			createdAt: safeString(notification.createdAt),
			account,
			status,
			report,
			read: notification.isRead,
			metadata: createLesserMetadata(notification),

			// Derive Lesser-specific notification payloads from status/account fields
			// Since Lesser schema doesn't expose separate payload fields, we infer them
			quoteStatus: notification.notificationType === 'QUOTE' ? status : undefined,
			communityNote:
				notification.notificationType === 'COMMUNITY_NOTE' &&
				notification.status?.communityNotes?.[0]
					? {
							id: safeString(notification.status.communityNotes[0].id),
							authorId: safeString(notification.status.communityNotes[0].author?.id || ''),
							authorUsername: safeString(
								notification.status.communityNotes[0].author?.handle || ''
							),
							authorDisplayName: safeString(
								notification.status.communityNotes[0].author?.displayName || ''
							),
							content: safeString(notification.status.communityNotes[0].content),
							helpful: safeNumber(notification.status.communityNotes[0].helpful),
							notHelpful: safeNumber(notification.status.communityNotes[0].notHelpful),
							createdAt: safeString(notification.status.communityNotes[0].createdAt),
						}
					: undefined,
			trustUpdate:
				notification.notificationType === 'TRUST_UPDATE' &&
				notification.triggerAccount.trustScore !== undefined
					? {
							newScore: safeNumber(notification.triggerAccount.trustScore),
							previousScore: undefined, // Would need to track previous value
							reason: undefined,
						}
					: undefined,
			costAlert:
				notification.notificationType === 'COST_ALERT'
					? {
							amount: notification.status?.estimatedCost
								? safeNumber(notification.status.estimatedCost)
								: 0,
							threshold: 1000000, // Default threshold, would come from user settings
							message: 'Cost threshold exceeded',
						}
					: undefined,
			moderationAction:
				notification.notificationType === 'MODERATION_ACTION' && notification.status?.aiAnalysis
					? {
							action: safeString(notification.status.aiAnalysis.moderationAction),
							reason: 'AI moderation action',
							statusId: notification.status.id,
						}
					: undefined,
		};

		const endTime = performance.now();
		return {
			success: true,
			data: unified,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(notification).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map notification: ${error instanceof Error ? error.message : 'Unknown error'}`,
				notification
			),
		};
	}
}

/**
 * Map Lesser streaming update to unified streaming models
 */
export function mapLesserStreamingUpdate(
	update: LesserStreamingUpdate
): MapperResult<StreamingUpdate | StreamingDelete | StreamingEdit> {
	const startTime = performance.now();

	try {
		if (!update || typeof update.eventType !== 'string') {
			return {
				success: false,
				error: createMappingError(
					'validation',
					'Invalid streaming update: missing or invalid event type',
					update
				),
			};
		}

		const timestamp = new Date(update.timestamp).getTime();
		const metadata = createLesserMetadata(update);
		const data = update.data;

		if (update.eventType === 'POST_DELETED' && isDeleteStreamingData(data)) {
			const deleteUpdate: StreamingDelete = {
				id: safeString(data.deletedId),
				itemType:
					data.deletedType === 'POST'
						? 'status'
						: data.deletedType === 'ACCOUNT'
							? 'account'
							: 'notification',
				timestamp,
			};

			const endTime = performance.now();
			return {
				success: true,
				data: deleteUpdate,
				metrics: {
					mappingTimeMs: endTime - startTime,
					fieldsProcessed: 3,
					fallbacksUsed: 0,
				},
			};
		}

		if (
			(update.eventType === 'POST_CREATED' || update.eventType === 'POST_UPDATED') &&
			isPostStreamingData(data)
		) {
			const postResult = mapLesserPost(data.post);
			if (!postResult.success || !postResult.data) {
				return {
					success: false,
					error:
						postResult.error ??
						createMappingError('transformation', 'Failed to map streaming post payload', data.post),
				};
			}

			if (update.eventType === 'POST_UPDATED') {
				const editUpdate: StreamingEdit = {
					id: postResult.data.id,
					data: postResult.data,
					timestamp,
					editType: 'content',
				};

				const endTime = performance.now();
				return {
					success: true,
					data: editUpdate,
					metrics: {
						mappingTimeMs: endTime - startTime,
						fieldsProcessed: Object.keys(data.post).length,
						fallbacksUsed: 0,
					},
				};
			}

			const streamingUpdate: StreamingUpdate = {
				type: 'status',
				payload: postResult.data,
				stream: data.timeline || 'lesser-stream',
				timestamp,
				metadata,
			};

			const endTime = performance.now();
			return {
				success: true,
				data: streamingUpdate,
				metrics: {
					mappingTimeMs: endTime - startTime,
					fieldsProcessed: Object.keys(data.post).length,
					fallbacksUsed: 0,
				},
			};
		}

		if (update.eventType === 'NOTIFICATION_CREATED' && isNotificationStreamingData(data)) {
			const notificationResult = mapLesserNotification(data.notification);
			if (!notificationResult.success || !notificationResult.data) {
				return {
					success: false,
					error:
						notificationResult.error ??
						createMappingError(
							'transformation',
							'Failed to map streaming notification payload',
							data.notification
						),
				};
			}

			const streamingUpdate: StreamingUpdate = {
				type: 'notification',
				payload: notificationResult.data,
				stream: 'lesser-stream',
				timestamp,
				metadata,
			};

			const endTime = performance.now();
			return {
				success: true,
				data: streamingUpdate,
				metrics: {
					mappingTimeMs: endTime - startTime,
					fieldsProcessed: Object.keys(data.notification).length,
					fallbacksUsed: 0,
				},
			};
		}

		if (update.eventType === 'ACCOUNT_UPDATED' && isAccountStreamingData(data)) {
			const accountResult = mapLesserAccount(data.account);
			if (accountResult.success && accountResult.data) {
				const streamingUpdate: StreamingUpdate = {
					type: 'status',
					payload: accountResult.data,
					stream: 'lesser-stream',
					timestamp,
					metadata,
				};

				const endTime = performance.now();
				return {
					success: true,
					data: streamingUpdate,
					metrics: {
						mappingTimeMs: endTime - startTime,
						fieldsProcessed: Object.keys(data.account).length,
						fallbacksUsed: 0,
					},
				};
			}
		}

		const streamingUpdate: StreamingUpdate = {
			type: 'status',
			payload: data,
			stream: 'lesser-stream',
			timestamp,
			metadata,
		};

		const endTime = performance.now();
		return {
			success: true,
			data: streamingUpdate,
			metrics: {
				mappingTimeMs: endTime - startTime,
				fieldsProcessed: Object.keys(update).length,
				fallbacksUsed: 0,
			},
		};
	} catch (error) {
		return {
			success: false,
			error: createMappingError(
				'transformation',
				`Failed to map streaming update: ${error instanceof Error ? error.message : 'Unknown error'}`,
				update
			),
		};
	}
}

/**
 * Handle GraphQL response wrapper and extract data
 */
export function handleLesserGraphQLResponse<T>(
	response: LesserGraphQLResponse<T>
): MapperResult<T> {
	if (response.errors && response.errors.length > 0) {
		return {
			success: false,
			error: createMappingError(
				'validation',
				'GraphQL response contains errors',
				response,
				undefined,
				response.errors
			),
		};
	}

	if (!response.data) {
		return {
			success: false,
			error: createMappingError('validation', 'GraphQL response missing data', response),
		};
	}

	return {
		success: true,
		data: response.data,
	};
}

/**
 * Map Lesser timeline connection to posts
 */
export function mapLesserTimelineConnection(
	connection: LesserTimelineConnection
): BatchMapperResult<UnifiedStatus> {
	const startTime = performance.now();
	const successful: UnifiedStatus[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	if (!connection.edges) {
		return {
			successful,
			failed,
			totalProcessed: 0,
			processingTimeMs: 0,
		};
	}

	for (const edge of connection.edges) {
		if (edge.node) {
			const result = mapLesserPost(edge.node);
			if (result.success && result.data) {
				successful.push(result.data);
			} else if (result.error) {
				failed.push({ payload: edge.node, error: result.error });
			}
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: connection.edges.length,
		processingTimeMs: endTime - startTime,
	};
}

/**
 * Batch map multiple Lesser accounts
 */
export function batchMapLesserAccounts(
	accounts: LesserAccountFragment[],
	relationships?: Map<string, LesserRelationshipFragment>
): BatchMapperResult<UnifiedAccount> {
	const startTime = performance.now();
	const successful: UnifiedAccount[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	for (const account of accounts) {
		const relationship = relationships?.get(account.id);
		const result = mapLesserAccount(account, relationship);

		if (result.success && result.data) {
			successful.push(result.data);
		} else if (result.error) {
			failed.push({ payload: account, error: result.error });
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: accounts.length,
		processingTimeMs: endTime - startTime,
	};
}

/**
 * Batch map multiple Lesser posts
 */
export function batchMapLesserPosts(posts: LesserPostFragment[]): BatchMapperResult<UnifiedStatus> {
	const startTime = performance.now();
	const successful: UnifiedStatus[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	for (const post of posts) {
		const result = mapLesserPost(post);

		if (result.success && result.data) {
			successful.push(result.data);
		} else if (result.error) {
			failed.push({ payload: post, error: result.error });
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: posts.length,
		processingTimeMs: endTime - startTime,
	};
}

/**
 * Batch map multiple Lesser notifications
 */
export function batchMapLesserNotifications(
	notifications: LesserNotificationFragment[]
): BatchMapperResult<UnifiedNotification> {
	const startTime = performance.now();
	const successful: UnifiedNotification[] = [];
	const failed: { payload: unknown; error: MappingError }[] = [];

	for (const notification of notifications) {
		const result = mapLesserNotification(notification);

		if (result.success && result.data) {
			successful.push(result.data);
		} else if (result.error) {
			failed.push({ payload: notification, error: result.error });
		}
	}

	const endTime = performance.now();
	return {
		successful,
		failed,
		totalProcessed: notifications.length,
		processingTimeMs: endTime - startTime,
	};
}
