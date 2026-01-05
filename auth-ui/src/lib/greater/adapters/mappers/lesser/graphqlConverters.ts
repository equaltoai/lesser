import type {
	LesserAccountFragment,
	LesserAttachmentFragment,
	LesserCommunityNoteFragment,
	LesserMentionFragment,
	LesserObjectFragment,
	LesserQuoteContextFragment,
	LesserTagFragment,
} from './types.js';

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === 'object' && value !== null;

const isString = (value: unknown): value is string => typeof value === 'string';
const isNumber = (value: unknown): value is number => typeof value === 'number';
const isBoolean = (value: unknown): value is boolean => typeof value === 'boolean';

const QUOTE_PERMISSIONS = new Set<LesserObjectFragment['quotePermissions']>([
	'EVERYONE',
	'FOLLOWERS',
	'NONE',
]);

const QUOTE_TYPES = new Set<LesserQuoteContextFragment['quoteType']>([
	'FULL',
	'PARTIAL',
	'COMMENTARY',
	'REACTION',
]);

const buildHandle = (username: string, domain?: string | null): string =>
	domain && domain.length > 0 ? `${username}@${domain}` : username;

const toNumber = (value: unknown, fallback = 0): number => (isNumber(value) ? value : fallback);

const toBoolean = (value: unknown, fallback = false): boolean =>
	isBoolean(value) ? value : fallback;

const toString = (value: unknown, fallback = ''): string => (isString(value) ? value : fallback);

const toISODate = (value: unknown): string => {
	if (isString(value)) {
		return value;
	}
	if (isNumber(value) && Number.isFinite(value)) {
		return new Date(value).toISOString();
	}
	if (value instanceof Date && Number.isFinite(value.getTime())) {
		return value.toISOString();
	}
	return new Date(0).toISOString();
};

const resolveTimestamp = (...candidates: unknown[]): string | undefined => {
	for (const candidate of candidates) {
		if (isString(candidate) && candidate.trim().length > 0) {
			return candidate;
		}

		if (candidate instanceof Date && Number.isFinite(candidate.getTime())) {
			return candidate.toISOString();
		}

		if (isNumber(candidate) && Number.isFinite(candidate)) {
			return new Date(candidate).toISOString();
		}
	}

	return undefined;
};

const convertProfileFields = (value: unknown): LesserAccountFragment['profileFields'] => {
	if (!Array.isArray(value)) {
		return [];
	}

	return value
		.map((field): LesserAccountFragment['profileFields'][number] | null => {
			if (!isRecord(field)) {
				return null;
			}
			const label = toString(field['label'] ?? field['name']);
			const content = toString(field['content'] ?? field['value']);
			const verifiedAt = isString(field['verifiedAt']) ? field['verifiedAt'] : undefined;
			if (!label && !content) {
				return null;
			}
			return { label, content, verifiedAt };
		})
		.filter((field): field is LesserAccountFragment['profileFields'][number] => field !== null);
};

const convertCustomEmojis = (value: unknown): LesserAccountFragment['customEmojis'] => {
	if (!Array.isArray(value)) {
		return [];
	}

	return value
		.map((emoji): LesserAccountFragment['customEmojis'][number] | null => {
			if (!isRecord(emoji)) {
				return null;
			}
			const code = toString(emoji['code'] ?? emoji['shortcode']);
			const imageUrl = toString(emoji['imageUrl'] ?? emoji['url']);
			const staticUrl = toString(emoji['staticUrl'] ?? emoji['static_url']);
			if (!code || !imageUrl || !staticUrl) {
				return null;
			}
			return {
				code,
				imageUrl,
				staticUrl,
				category: isString(emoji['category']) ? emoji['category'] : undefined,
				isVisible: typeof emoji['isVisible'] === 'boolean' ? emoji['isVisible'] : true,
			};
		})
		.filter((emoji): emoji is LesserAccountFragment['customEmojis'][number] => emoji !== null);
};

const toMaybeNumber = (value: unknown): number | undefined => (isNumber(value) ? value : undefined);

function deriveActorId(actor: Record<string, unknown>): string | null {
	const idValue = actor['id'];
	if (isString(idValue)) {
		return idValue;
	}

	const handle = actor['handle'];
	if (isString(handle)) {
		return handle;
	}

	const username = actor['username'];
	if (isString(username)) {
		const domain = isString(actor['domain']) ? actor['domain'] : undefined;
		return buildHandle(username, domain);
	}

	return null;
}

export function convertGraphQLActorToLesserAccount(actor: unknown): LesserAccountFragment | null {
	if (!isRecord(actor)) {
		return null;
	}

	const idValue = deriveActorId(actor);
	if (!idValue) {
		return null;
	}

	// Support already-normalized Lesser account payloads to keep compatibility with existing tests/mocks.
	const handle = actor['handle'];
	const localHandle = actor['localHandle'];
	if (isString(handle) && isString(localHandle)) {
		return {
			id: idValue,
			handle,
			localHandle,
			displayName: toString(actor['displayName'], localHandle),
			bio: toString(actor['bio'] ?? actor['summary']),
			avatarUrl: toString(actor['avatarUrl'] ?? actor['avatar']),
			bannerUrl: toString(actor['bannerUrl'] ?? actor['header']),
			joinedAt:
				resolveTimestamp(actor['joinedAt'], actor['createdAt'], actor['updatedAt']) ??
				toISODate(actor['createdAt']),
			isVerified: toBoolean(actor['isVerified'] ?? actor['verified']),
			isBot: toBoolean(actor['isBot'] ?? actor['bot']),
			isLocked: toBoolean(actor['isLocked'] ?? actor['locked']),
			followerCount: toNumber(actor['followerCount'] ?? actor['followers']),
			followingCount: toNumber(actor['followingCount'] ?? actor['following']),
			postCount: toNumber(actor['postCount'] ?? actor['statusesCount']),
			profileFields: convertProfileFields(actor['profileFields'] ?? actor['fields']),
			customEmojis: convertCustomEmojis(actor['customEmojis']),
			trustScore: toMaybeNumber(actor['trustScore']),
			reputation: isRecord(actor['reputation'])
				? (actor['reputation'] as unknown as LesserAccountFragment['reputation'])
				: undefined,
			vouches: Array.isArray(actor['vouches'])
				? (actor['vouches'] as unknown as LesserAccountFragment['vouches'])
				: undefined,
		};
	}

	const username = actor['username'];
	if (!isString(username)) {
		return null;
	}

	const domain = isString(actor['domain']) ? actor['domain'] : undefined;
	const displayName = isString(actor['displayName']) ? actor['displayName'] : username;
	const summary = toString(actor['summary']);
	const avatar = toString(actor['avatar']);
	const header = toString(actor['header']);
	const joinedAt =
		resolveTimestamp(actor['createdAt'], actor['updatedAt']) ?? toISODate(actor['createdAt']);

	const profileFields = convertProfileFields(actor['fields']);

	return {
		id: idValue,
		handle: buildHandle(username, domain),
		localHandle: username,
		displayName,
		bio: summary,
		avatarUrl: avatar,
		bannerUrl: header,
		joinedAt,
		isVerified: toBoolean(actor['verified']),
		isBot: toBoolean(actor['bot']),
		isLocked: toBoolean(actor['locked']),
		followerCount: toNumber(actor['followers']),
		followingCount: toNumber(actor['following']),
		postCount: toNumber(actor['statusesCount']),
		profileFields,
		customEmojis: [],
		trustScore: toMaybeNumber(actor['trustScore']),
		reputation: undefined,
		vouches: undefined,
	};
}

function convertGraphQLAttachment(attachment: unknown): LesserAttachmentFragment | null {
	if (!isRecord(attachment)) {
		return null;
	}

	const id = attachment['id'];
	const type = attachment['type'];
	const url = attachment['url'];
	if (!isString(id) || !isString(type) || !isString(url)) {
		return null;
	}

	const previewValue = attachment['preview'] ?? attachment['previewUrl'];

	return {
		id,
		type,
		url,
		preview: isString(previewValue) ? previewValue : undefined,
		description: isString(attachment['description']) ? attachment['description'] : undefined,
		sensitive: typeof attachment['sensitive'] === 'boolean' ? attachment['sensitive'] : undefined,
		spoilerText: isString(attachment['spoilerText']) ? attachment['spoilerText'] : undefined,
		mediaCategory: isString(attachment['mediaCategory'])
			? (attachment['mediaCategory'] as LesserAttachmentFragment['mediaCategory'])
			: undefined,
		mimeType: isString(attachment['mimeType']) ? attachment['mimeType'] : undefined,
		blurhash: isString(attachment['blurhash']) ? attachment['blurhash'] : undefined,
		width: isNumber(attachment['width']) ? attachment['width'] : undefined,
		height: isNumber(attachment['height']) ? attachment['height'] : undefined,
		duration: isNumber(attachment['duration']) ? attachment['duration'] : undefined,
	};
}

function convertGraphQLMention(mention: unknown): LesserMentionFragment | null {
	if (!isRecord(mention)) {
		return null;
	}

	const id = mention['id'];
	const username = mention['username'];
	const url = mention['url'];
	if (!isString(id) || !isString(username) || !isString(url)) {
		return null;
	}

	return {
		id,
		username,
		domain: isString(mention['domain']) ? mention['domain'] : undefined,
		url,
	};
}

function convertGraphQLTag(tag: unknown): LesserTagFragment | null {
	if (!isRecord(tag)) {
		return null;
	}

	const name = tag['name'];
	const url = tag['url'];
	if (!isString(name) || !isString(url)) {
		return null;
	}

	return { name, url };
}

function convertGraphQLCommunityNote(note: unknown): LesserCommunityNoteFragment | null {
	if (!isRecord(note)) {
		return null;
	}

	const id = note['id'];
	const content = note['content'];
	const helpful = note['helpful'];
	const notHelpful = note['notHelpful'];
	const createdAt = note['createdAt'];
	if (
		!isString(id) ||
		!isString(content) ||
		!isNumber(helpful) ||
		!isNumber(notHelpful) ||
		!isString(createdAt)
	) {
		return null;
	}

	const author = convertGraphQLActorToLesserAccount(note['author']);

	return {
		id,
		content,
		helpful,
		notHelpful,
		createdAt,
		author: author ?? undefined,
	};
}

function convertGraphQLQuoteContext(context: unknown): LesserQuoteContextFragment | undefined {
	if (!isRecord(context)) {
		return undefined;
	}

	const originalAuthor = convertGraphQLActorToLesserAccount(context['originalAuthor']);
	const originalNote = context['originalNote'];
	const quoteAllowed = toBoolean(context['quoteAllowed']);
	const rawQuoteType = context['quoteType'];
	const quoteType =
		isString(rawQuoteType) &&
		QUOTE_TYPES.has(rawQuoteType as LesserQuoteContextFragment['quoteType'])
			? (rawQuoteType as LesserQuoteContextFragment['quoteType'])
			: 'FULL';

	return {
		originalAuthor: originalAuthor ?? undefined,
		originalNote:
			isRecord(originalNote) && isString(originalNote['id'])
				? { id: originalNote['id'] }
				: undefined,
		quoteAllowed,
		quoteType,
		withdrawn: toBoolean(context['withdrawn']),
	};
}

function normalizeVisibility(value: unknown): LesserObjectFragment['visibility'] {
	const visibility = isString(value) ? value : 'PUBLIC';
	switch (visibility) {
		case 'PUBLIC':
		case 'UNLISTED':
		case 'PRIVATE':
		case 'DIRECT':
			return visibility;
		case 'FOLLOWERS':
			return 'PRIVATE';
		default:
			return 'PUBLIC';
	}
}

function normalizeQuotePermissions(value: unknown): LesserObjectFragment['quotePermissions'] {
	if (isString(value)) {
		const upper = value.toUpperCase() as LesserObjectFragment['quotePermissions'];
		if (QUOTE_PERMISSIONS.has(upper)) {
			return upper;
		}
	}
	return 'EVERYONE';
}

function convertGraphQLReplyRef(value: unknown): LesserObjectFragment['inReplyTo'] {
	if (isRecord(value) && isString(value['id'])) {
		const actor = convertGraphQLActorToLesserAccount(value['actor']) ?? undefined;
		const authorId = isString(value['authorId']) ? value['authorId'] : undefined;
		return { id: value['id'], actor, authorId };
	}

	if (isString(value)) {
		return { id: value };
	}

	return undefined;
}

export function convertGraphQLObjectToLesser(
	object: unknown,
	depth = 0
): LesserObjectFragment | null {
	if (!isRecord(object)) {
		return null;
	}

	const id = object['id'];
	const type = object['type'];
	const content = object['content'];
	const visibility = object['visibility'];
	const sensitive = object['sensitive'];
	const actorValue = object['actor'];
	if (!isString(id) || !isString(type) || !isString(content)) {
		return null;
	}

	const createdAt =
		resolveTimestamp(object['createdAt'], object['published'], object['updatedAt']) ??
		resolveTimestamp(object['lastActivity']);
	const updatedAt =
		resolveTimestamp(object['updatedAt'], object['createdAt'], object['published']) ?? createdAt;

	if (!createdAt || !updatedAt) {
		return null;
	}

	const actor = convertGraphQLActorToLesserAccount(actorValue);
	if (!actor) {
		return null;
	}

	const attachmentsValue = object['attachments'];
	const attachments: LesserAttachmentFragment[] = Array.isArray(attachmentsValue)
		? attachmentsValue
				.map(convertGraphQLAttachment)
				.filter((attachment): attachment is LesserAttachmentFragment => attachment !== null)
		: [];

	const tagsValue = object['tags'];
	const tags: LesserTagFragment[] = Array.isArray(tagsValue)
		? tagsValue.map(convertGraphQLTag).filter((tag): tag is LesserTagFragment => tag !== null)
		: [];

	const mentionsValue = object['mentions'];
	const mentions: LesserMentionFragment[] = Array.isArray(mentionsValue)
		? mentionsValue
				.map(convertGraphQLMention)
				.filter((mention): mention is LesserMentionFragment => mention !== null)
		: [];

	const communityNotesValue = object['communityNotes'];
	const communityNotes: LesserCommunityNoteFragment[] = Array.isArray(communityNotesValue)
		? communityNotesValue
				.map(convertGraphQLCommunityNote)
				.filter((note): note is LesserCommunityNoteFragment => note !== null)
		: [];

	const inReplyTo = convertGraphQLReplyRef(object['inReplyTo']);
	const quoteContext = convertGraphQLQuoteContext(object['quoteContext']);
	const boostedValue = object['boostedObject'] ?? object['shareOf'];
	const boosted =
		// Limit recursion depth to avoid runaway object trees
		depth < 1 && isRecord(boostedValue)
			? convertGraphQLObjectToLesser(boostedValue, depth + 1)
			: null;

	return {
		id,
		type,
		actor,
		content,
		inReplyTo: inReplyTo ?? undefined,
		visibility: normalizeVisibility(visibility),
		sensitive: toBoolean(sensitive),
		spoilerText: isString(object['spoilerText']) ? object['spoilerText'] : undefined,
		attachments,
		tags,
		mentions,
		createdAt,
		updatedAt,
		shareOf: boosted ?? undefined,
		boostedObject: boosted ?? undefined,
		repliesCount: toNumber(object['repliesCount']),
		likesCount: toNumber(object['likesCount']),
		sharesCount: toNumber(object['sharesCount']),
		estimatedCost: toNumber(object['estimatedCost']),
		moderationScore: isNumber(object['moderationScore']) ? object['moderationScore'] : undefined,
		communityNotes,
		quoteUrl: isString(object['quoteUrl']) ? object['quoteUrl'] : undefined,
		quoteable: toBoolean(object['quoteable']),
		quotePermissions: normalizeQuotePermissions(object['quotePermissions']),
		quoteContext,
		quoteCount: isNumber(object['quoteCount']) ? object['quoteCount'] : undefined,
		aiAnalysis: undefined,
	};
}

// New converters for followers/following, preferences, and push
export interface ActorListPage {
	actors: LesserAccountFragment[];
	nextCursor?: string;
	totalCount: number;
}

export function convertGraphQLActorListPage(data: unknown): ActorListPage | null {
	if (!isRecord(data)) {
		return null;
	}

	const actorsValue = data['actors'];
	const actors: LesserAccountFragment[] = Array.isArray(actorsValue)
		? actorsValue
				.map(convertGraphQLActorToLesserAccount)
				.filter((actor): actor is LesserAccountFragment => actor !== null)
		: [];

	return {
		actors,
		nextCursor: isString(data['nextCursor']) ? data['nextCursor'] : undefined,
		totalCount: toNumber(data['totalCount']),
	};
}

export interface UserPreferences {
	actorId: string;
	posting: {
		defaultVisibility: 'PUBLIC' | 'UNLISTED' | 'PRIVATE' | 'DIRECT';
		defaultSensitive: boolean;
		defaultLanguage: string;
	};
	reading: {
		expandSpoilers: boolean;
		expandMedia: 'DEFAULT' | 'SHOW_ALL' | 'HIDE_ALL';
		autoplayGifs: boolean;
		timelineOrder: 'NEWEST' | 'OLDEST';
	};
	discovery: {
		showFollowCounts: boolean;
		searchSuggestionsEnabled: boolean;
		personalizedSearchEnabled: boolean;
	};
	streaming: {
		defaultQuality: 'AUTO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'ULTRA';
		autoQuality: boolean;
		preloadNext: boolean;
		dataSaver: boolean;
	};
	notifications: {
		email: boolean;
		push: boolean;
		inApp: boolean;
		digest: 'NEVER' | 'DAILY' | 'WEEKLY' | 'MONTHLY';
	};
	privacy: {
		defaultVisibility: 'PUBLIC' | 'UNLISTED' | 'PRIVATE' | 'DIRECT';
		indexable: boolean;
		showOnlineStatus: boolean;
	};
	reblogFilters: Array<{ key: string; enabled: boolean }>;
}

export function convertGraphQLUserPreferences(data: unknown): UserPreferences | null {
	if (!isRecord(data)) {
		return null;
	}

	const actorId = data['actorId'];
	if (!isString(actorId)) {
		return null;
	}

	const posting = isRecord(data['posting']) ? data['posting'] : {};
	const reading = isRecord(data['reading']) ? data['reading'] : {};
	const discovery = isRecord(data['discovery']) ? data['discovery'] : {};
	const streaming = isRecord(data['streaming']) ? data['streaming'] : {};
	const notifications = isRecord(data['notifications']) ? data['notifications'] : {};
	const privacy = isRecord(data['privacy']) ? data['privacy'] : {};
	const reblogFiltersValue = data['reblogFilters'];

	return {
		actorId,
		posting: {
			defaultVisibility: toString(
				posting['defaultVisibility'],
				'PUBLIC'
			) as UserPreferences['posting']['defaultVisibility'],
			defaultSensitive: toBoolean(posting['defaultSensitive']),
			defaultLanguage: toString(posting['defaultLanguage'], 'en'),
		},
		reading: {
			expandSpoilers: toBoolean(reading['expandSpoilers']),
			expandMedia: toString(
				reading['expandMedia'],
				'DEFAULT'
			) as UserPreferences['reading']['expandMedia'],
			autoplayGifs: toBoolean(reading['autoplayGifs']),
			timelineOrder: toString(
				reading['timelineOrder'],
				'NEWEST'
			) as UserPreferences['reading']['timelineOrder'],
		},
		discovery: {
			showFollowCounts: toBoolean(discovery['showFollowCounts'], true),
			searchSuggestionsEnabled: toBoolean(discovery['searchSuggestionsEnabled'], true),
			personalizedSearchEnabled: toBoolean(discovery['personalizedSearchEnabled'], true),
		},
		streaming: {
			defaultQuality: toString(
				streaming['defaultQuality'],
				'AUTO'
			) as UserPreferences['streaming']['defaultQuality'],
			autoQuality: toBoolean(streaming['autoQuality'], true),
			preloadNext: toBoolean(streaming['preloadNext']),
			dataSaver: toBoolean(streaming['dataSaver']),
		},
		notifications: {
			email: toBoolean(notifications['email']),
			push: toBoolean(notifications['push'], true),
			inApp: toBoolean(notifications['inApp'], true),
			digest: toString(
				notifications['digest'],
				'NEVER'
			) as UserPreferences['notifications']['digest'],
		},
		privacy: {
			defaultVisibility: toString(
				privacy['defaultVisibility'],
				'PUBLIC'
			) as UserPreferences['privacy']['defaultVisibility'],
			indexable: toBoolean(privacy['indexable'], true),
			showOnlineStatus: toBoolean(privacy['showOnlineStatus']),
		},
		reblogFilters: Array.isArray(reblogFiltersValue)
			? reblogFiltersValue
					.map((filter): UserPreferences['reblogFilters'][number] | null => {
						if (!isRecord(filter)) {
							return null;
						}
						return {
							key: toString(filter['key']),
							enabled: toBoolean(filter['enabled'], true),
						};
					})
					.filter((filter): filter is UserPreferences['reblogFilters'][number] => filter !== null)
			: [],
	};
}

export interface PushSubscription {
	id: string;
	endpoint: string;
	keys: {
		auth: string;
		p256dh: string;
	};
	alerts: {
		follow: boolean;
		favourite: boolean;
		reblog: boolean;
		mention: boolean;
		poll: boolean;
		followRequest: boolean;
		status: boolean;
		update: boolean;
		adminSignUp: boolean;
		adminReport: boolean;
	};
	policy: string;
	serverKey?: string;
	createdAt?: string;
	updatedAt?: string;
}

export function convertGraphQLPushSubscription(data: unknown): PushSubscription | null {
	if (!isRecord(data)) {
		return null;
	}

	const id = data['id'];
	const endpoint = data['endpoint'];
	const keys = data['keys'];
	const alerts = data['alerts'];

	if (!isString(id) || !isString(endpoint) || !isRecord(keys) || !isRecord(alerts)) {
		return null;
	}

	return {
		id,
		endpoint,
		keys: {
			auth: toString(keys['auth']),
			p256dh: toString(keys['p256dh']),
		},
		alerts: {
			follow: toBoolean(alerts['follow'], true),
			favourite: toBoolean(alerts['favourite'], true),
			reblog: toBoolean(alerts['reblog'], true),
			mention: toBoolean(alerts['mention'], true),
			poll: toBoolean(alerts['poll'], true),
			followRequest: toBoolean(alerts['followRequest'], true),
			status: toBoolean(alerts['status'], true),
			update: toBoolean(alerts['update'], true),
			adminSignUp: toBoolean(alerts['adminSignUp']),
			adminReport: toBoolean(alerts['adminReport']),
		},
		policy: toString(data['policy'], 'all'),
		serverKey: isString(data['serverKey']) ? data['serverKey'] : undefined,
		createdAt: isString(data['createdAt']) ? data['createdAt'] : undefined,
		updatedAt: isString(data['updatedAt']) ? data['updatedAt'] : undefined,
	};
}
