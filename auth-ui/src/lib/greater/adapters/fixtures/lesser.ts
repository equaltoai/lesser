/**
 * Lesser GraphQL API test fixtures
 * Realistic test data for the fictional Lesser GraphQL API
 */

import type {
	LesserAccountFragment,
	LesserPostFragment,
	LesserNotificationFragment,
	LesserObjectFragment,
	LesserMediaFragment,
	LesserPollFragment,
	LesserStreamingUpdate,
	LesserRelationshipFragment,
	LesserGraphQLResponse,
	LesserTimelineConnection,
	LesserAIAnalysisFragment,
} from '../mappers/lesser/types.js';

const requireFixture = <T>(items: readonly T[], index: number): T => {
	const item = items[index];
	if (!item) {
		throw new Error(`Fixture item not found at index ${index}`);
	}
	return item;
};

export const lesserAIAnalysisFixture: LesserAIAnalysisFragment = {
	id: 'ai-analysis-1',
	objectId: 'post-with-ai-analysis',
	objectType: 'POST',
	overallRisk: 0.75,
	moderationAction: 'FLAG',
	confidence: 0.9,
	analyzedAt: '2023-10-26T10:00:00Z',
	textAnalysis: {
		sentiment: 'NEGATIVE',
		sentimentScores: {
			positive: 0.1,
			negative: 0.8,
			neutral: 0.1,
			mixed: 0.0,
		},
		toxicityScore: 0.8,
		toxicityLabels: ['hate_speech'],
		containsPII: false,
		dominantLanguage: 'en',
		entities: [{ text: 'bad words', type: 'PROFANITY', confidence: 0.9 }],
		keyPhrases: ['bad words'],
	},
	imageAnalysis: {
		moderationLabels: [{ name: 'Graphic Violence', confidence: 0.9, parentName: 'Violence' }],
		isNSFW: true,
		nsfwConfidence: 0.9,
		violenceScore: 0.8,
		weaponsDetected: true,
		detectedText: ['violence'],
		textToxicity: 0.7,
		celebrityFaces: [],
		deepfakeScore: 0.1,
	},
	aiDetection: {
		aiGeneratedProbability: 0.1,
		generationModel: 'Lesser-Gen-1',
		patternConsistency: 0.9,
		styleDeviation: 0.2,
		semanticCoherence: 0.8,
		suspiciousPatterns: [],
	},
	spamAnalysis: {
		spamScore: 0.9,
		spamIndicators: [
			{ type: 'LINK_SPAM', description: 'Contains suspicious links', severity: 0.9 },
		],
		postingVelocity: 10,
		repetitionScore: 0.8,
		linkDensity: 0.7,
		followerRatio: 0.1,
		interactionRate: 0.01,
		accountAgeDays: 1,
	},
};

// Sample Lesser accounts
export const lesserAccountFixtures: LesserAccountFragment[] = [
	{
		id: 'acc_8f3a9b2c1d4e5f6g',
		handle: 'bob@lesser.network',
		localHandle: 'bob',
		displayName: 'Bob Builder',
		bio: 'Building the future, one block at a time 🏗️\nArchitect & Developer\nhttps://bobbuilder.dev',
		avatarUrl: 'https://cdn.lesser.network/avatars/8f3a9b2c1d4e5f6g/avatar.webp',
		bannerUrl: 'https://cdn.lesser.network/banners/8f3a9b2c1d4e5f6g/banner.webp',
		joinedAt: '2023-03-15T08:30:00.000Z',
		isVerified: true,
		isBot: false,
		isLocked: false,
		followerCount: 2456,
		followingCount: 789,
		postCount: 1234,
		profileFields: [
			{
				label: 'Website',
				content:
					'<a href="https://bobbuilder.dev" target="_blank" rel="noopener">bobbuilder.dev</a>',
				verifiedAt: '2023-04-01T10:00:00.000Z',
			},
			{
				label: 'Pronouns',
				content: 'he/him',
			},
			{
				label: 'Location',
				content: 'Portland, OR',
			},
		],
		customEmojis: [
			{
				code: 'hammer',
				imageUrl: 'https://cdn.lesser.network/emojis/hammer.gif',
				staticUrl: 'https://cdn.lesser.network/emojis/hammer.png',
				category: 'tools',
				isVisible: true,
			},
		],
	},
	{
		id: 'acc_9g4h5i6j7k8l9m0n',
		handle: 'newsbot@feeds.lesser.network',
		localHandle: 'newsbot',
		displayName: 'Tech News Bot',
		bio: 'Automated tech news updates 🤖\nLatest from the world of technology\nManaged by @admin',
		avatarUrl: 'https://cdn.feeds.lesser.network/bots/newsbot/avatar.png',
		bannerUrl: 'https://cdn.feeds.lesser.network/bots/newsbot/banner.png',
		joinedAt: '2023-01-10T12:00:00.000Z',
		isVerified: false,
		isBot: true,
		isLocked: false,
		followerCount: 15678,
		followingCount: 5,
		postCount: 8901,
		profileFields: [
			{
				label: 'Source',
				content: 'Aggregated from multiple tech news sources',
			},
			{
				label: 'Update Frequency',
				content: 'Every 15 minutes',
			},
		],
		customEmojis: [
			{
				code: 'robot_face',
				imageUrl: 'https://cdn.feeds.lesser.network/emojis/robot.png',
				staticUrl: 'https://cdn.feeds.lesser.network/emojis/robot.png',
				isVisible: true,
			},
		],
	},
];

const getAccount = (index: number): LesserAccountFragment =>
	requireFixture(lesserAccountFixtures, index);

// Sample Lesser media attachments
export const lesserMediaFixtures: LesserMediaFragment[] = [
	{
		id: 'media_a1b2c3d4e5f6g7h8',
		mediaType: 'IMAGE',
		url: 'https://cdn.lesser.network/media/a1b2c3d4e5f6g7h8/original.jpg',
		thumbnailUrl: 'https://cdn.lesser.network/media/a1b2c3d4e5f6g7h8/thumb.jpg',
		altText: 'A modern office space with natural lighting, showing developers working on laptops',
		blurhash: 'LKO2:N%2Tw=w]~RBVZRi};RPxuwH',
		metadata: {
			dimensions: {
				width: 2048,
				height: 1365,
				aspectRatio: 1.5,
			},
			fileSize: 2048576,
			format: 'JPEG',
		},
	},
	{
		id: 'media_b2c3d4e5f6g7h8i9',
		mediaType: 'VIDEO',
		url: 'https://cdn.lesser.network/media/b2c3d4e5f6g7h8i9/original.mp4',
		thumbnailUrl: 'https://cdn.lesser.network/media/b2c3d4e5f6g7h8i9/thumb.jpg',
		altText: 'Screen recording showing the new dashboard interface in action',
		metadata: {
			dimensions: {
				width: 1920,
				height: 1080,
				aspectRatio: 1.777777777777778,
			},
			duration: 45.2,
			frameRate: 30,
			bitrate: 8000000,
			fileSize: 45398016,
			format: 'MP4',
		},
	},
	{
		id: 'media_c3d4e5f6g7h8i9j0',
		mediaType: 'GIF',
		url: 'https://cdn.lesser.network/media/c3d4e5f6g7h8i9j0/original.gif',
		thumbnailUrl: 'https://cdn.lesser.network/media/c3d4e5f6g7h8i9j0/thumb.jpg',
		altText: 'Animated GIF showing a progress bar loading animation',
		metadata: {
			dimensions: {
				width: 400,
				height: 300,
				aspectRatio: 1.333333333333333,
			},
			duration: 2.5,
			frameRate: 24,
			fileSize: 1048576,
			format: 'GIF',
		},
	},
];

const getMedia = (index: number): LesserMediaFragment => requireFixture(lesserMediaFixtures, index);

// Sample Lesser polls
export const lesserPollFixtures: LesserPollFragment[] = [
	{
		id: 'poll_x1y2z3a4b5c6d7e8',
		question: "What's your favorite programming language for web development?",
		options: [
			{
				index: 0,
				text: 'JavaScript/TypeScript',
				voteCount: 142,
			},
			{
				index: 1,
				text: 'Python',
				voteCount: 68,
			},
			{
				index: 2,
				text: 'Rust',
				voteCount: 25,
			},
			{
				index: 3,
				text: 'Go',
				voteCount: 31,
			},
		],
		expiresAt: '2023-12-20T18:00:00.000Z',
		isExpired: false,
		allowsMultiple: false,
		totalVotes: 266,
		participantCount: 266,
		userVote: {
			choices: [0],
		},
	},
	{
		id: 'poll_y2z3a4b5c6d7e8f9',
		question: 'Which features should we prioritize next? (Select all that apply)',
		options: [
			{
				index: 0,
				text: 'Dark mode theme',
				voteCount: 89,
			},
			{
				index: 1,
				text: 'Mobile app',
				voteCount: 156,
			},
			{
				index: 2,
				text: 'Advanced search',
				voteCount: 73,
			},
			{
				index: 3,
				text: 'Video calls',
				voteCount: 45,
			},
		],
		expiresAt: '2023-12-25T12:00:00.000Z',
		isExpired: false,
		allowsMultiple: true,
		totalVotes: 363,
		participantCount: 198,
	},
];

// Sample Lesser posts
export const lesserPostFixtures: LesserPostFragment[] = [
	{
		id: 'post_p1q2r3s4t5u6v7w8',
		publishedAt: '2023-12-15T14:30:00.000Z',
		content:
			'Just released a major update to our design system! 🎨 \n\nNew components include:\n• Advanced data tables\n• Improved form validation\n• Better accessibility features\n\nCheck it out and let us know what you think! #DesignSystems #WebDev',
		visibility: 'PUBLIC',
		isSensitive: false,
		language: 'en',
		author: getAccount(0),
		attachments: [getMedia(0), getMedia(1)],
		mentions: [],
		hashtags: [
			{
				name: 'DesignSystems',
				url: 'https://lesser.network/tags/designsystems',
				trendingData: [
					{
						timestamp: '2023-12-15T00:00:00.000Z',
						usage: 34,
						accounts: 28,
					},
					{
						timestamp: '2023-12-14T00:00:00.000Z',
						usage: 19,
						accounts: 15,
					},
				],
			},
			{
				name: 'WebDev',
				url: 'https://lesser.network/tags/webdev',
			},
		],
		emojis: [
			{
				code: 'art',
				imageUrl: 'https://cdn.lesser.network/emojis/art.png',
				staticUrl: 'https://cdn.lesser.network/emojis/art.png',
				isVisible: true,
			},
		],
		interactionCounts: {
			replies: 12,
			shares: 8,
			favorites: 34,
		},
		userInteractions: {
			isFavorited: true,
			isShared: false,
			isBookmarked: true,
		},
		isPinned: true,
		poll: lesserPollFixtures[0],
	},
	{
		id: 'post_q2r3s4t5u6v7w8x9',
		publishedAt: '2023-12-15T16:45:00.000Z',
		content:
			'🤖 TECH NEWS UPDATE:\n\nMajor framework releases this week:\n- React 19 beta with improved concurrent features\n- Vue 3.4 with better TypeScript support\n- Svelte 5 introducing runes for reactive state\n\nStay updated with the latest! #TechNews #WebFrameworks',
		visibility: 'PUBLIC',
		isSensitive: false,
		language: 'en',
		author: getAccount(1),
		attachments: [],
		mentions: [],
		hashtags: [
			{
				name: 'TechNews',
				url: 'https://lesser.network/tags/technews',
			},
			{
				name: 'WebFrameworks',
				url: 'https://lesser.network/tags/webframeworks',
			},
		],
		emojis: [
			{
				code: 'robot_face',
				imageUrl: 'https://cdn.feeds.lesser.network/emojis/robot.png',
				staticUrl: 'https://cdn.feeds.lesser.network/emojis/robot.png',
				isVisible: true,
			},
		],
		interactionCounts: {
			replies: 23,
			shares: 67,
			favorites: 89,
		},
		userInteractions: {
			isFavorited: false,
			isShared: true,
			isBookmarked: false,
		},
		isPinned: false,
	},
	{
		id: 'post_r3s4t5u6v7w8x9y0',
		publishedAt: '2023-12-15T17:20:00.000Z',
		content:
			'Thanks for all the feedback on the design system update! 🙏\n\nReplying to the most common questions:\n\nQ: Will there be a Figma plugin?\nA: Yes! Coming next month.\n\nQ: What about Vue components?\nA: Already in development, stay tuned!',
		visibility: 'PUBLIC',
		isSensitive: false,
		language: 'en',
		author: getAccount(0),
		attachments: [],
		mentions: [],
		hashtags: [],
		emojis: [],
		replyTo: {
			id: 'post_p1q2r3s4t5u6v7w8',
			authorId: 'acc_8f3a9b2c1d4e5f6g',
		},
		interactionCounts: {
			replies: 5,
			shares: 2,
			favorites: 18,
		},
		userInteractions: {
			isFavorited: false,
			isShared: false,
			isBookmarked: false,
		},
		isPinned: false,
		lastEditedAt: '2023-12-15T17:25:00.000Z',
	},
	{
		id: 'post_s4t5u6v7w8x9y0z1',
		publishedAt: '2023-12-15T19:15:00.000Z',
		content: 'Sharing an interesting thread about accessibility in modern web apps...',
		contentWarning: 'Discussion about web accessibility challenges',
		visibility: 'FOLLOWERS',
		isSensitive: true,
		language: 'en',
		author: getAccount(0),
		attachments: [],
		mentions: [
			{
				id: 'acc_t5u6v7w8x9y0z1a2',
				username: 'a11y_expert',
				domain: 'accessibility.network',
				url: 'https://accessibility.network/@a11y_expert',
			},
		],
		hashtags: [
			{
				name: 'a11y',
				url: 'https://lesser.network/tags/a11y',
			},
			{
				name: 'WebAccessibility',
				url: 'https://lesser.network/tags/webaccessibility',
			},
		],
		emojis: [],
		interactionCounts: {
			replies: 7,
			shares: 3,
			favorites: 12,
		},
		userInteractions: {
			isFavorited: false,
			isShared: false,
			isBookmarked: true,
		},
		isPinned: false,
	},
	{
		id: 'post-with-ai-analysis',
		publishedAt: '2023-12-15T19:15:00.000Z',
		content: 'This is a post with AI analysis.',
		contentWarning: 'AI analysis',
		visibility: 'PUBLIC',
		isSensitive: true,
		language: 'en',
		author: getAccount(0),
		attachments: [],
		mentions: [],
		hashtags: [],
		emojis: [],
		interactionCounts: {
			replies: 7,
			shares: 3,
			favorites: 12,
		},
		userInteractions: {
			isFavorited: false,
			isShared: false,
			isBookmarked: true,
		},
		isPinned: false,
		aiAnalysis: lesserAIAnalysisFixture,
	},
];

const getPost = (index: number): LesserPostFragment => requireFixture(lesserPostFixtures, index);

const createNotificationStatus = (
	id: string,
	actor: LesserAccountFragment,
	overrides: Partial<LesserObjectFragment> = {}
): LesserObjectFragment => {
	const base: LesserObjectFragment = {
		id,
		type: 'POST',
		actor,
		content: 'Lesser notification content',
		inReplyTo: undefined,
		visibility: 'PUBLIC',
		sensitive: false,
		spoilerText: undefined,
		attachments: [],
		tags: [],
		mentions: [],
		createdAt: '2023-12-15T00:00:00.000Z',
		updatedAt: '2023-12-15T00:00:00.000Z',
		repliesCount: 0,
		likesCount: 0,
		sharesCount: 0,
		estimatedCost: 0,
		moderationScore: undefined,
		communityNotes: [],
		quoteUrl: undefined,
		quoteable: true,
		quotePermissions: 'EVERYONE',
		quoteContext: undefined,
		quoteCount: 0,
		aiAnalysis: undefined,
	};

	return {
		...base,
		...overrides,
	};
};

const favoriteNotificationStatus = createNotificationStatus('status_favorite_post', getAccount(0), {
	content: 'Thanks for the support on the new moderation console!',
	createdAt: '2023-12-15T14:45:00.000Z',
	updatedAt: '2023-12-15T14:45:00.000Z',
	likesCount: 24,
	sharesCount: 6,
	repliesCount: 3,
	estimatedCost: 5,
});

const shareNotificationStatus = createNotificationStatus('status_share_post', getAccount(0), {
	content: 'Sharing our transport manager walkthrough.',
	createdAt: '2023-12-15T15:05:00.000Z',
	updatedAt: '2023-12-15T15:05:00.000Z',
	likesCount: 18,
	sharesCount: 9,
	estimatedCost: 7,
});

const mentionNotificationStatus = createNotificationStatus('status_mention_post', getAccount(0), {
	content: '@bob your fallback guide is trending!',
	createdAt: '2023-12-15T17:10:00.000Z',
	updatedAt: '2023-12-15T17:10:00.000Z',
	repliesCount: 4,
	estimatedCost: 4,
});

const pollEndedNotificationStatus = createNotificationStatus('status_poll_post', getAccount(0), {
	content: 'Poll results: SSE fallback wins by a landslide.',
	createdAt: '2023-12-16T17:30:00.000Z',
	updatedAt: '2023-12-16T17:30:00.000Z',
	sharesCount: 5,
	estimatedCost: 3,
});

// Sample Lesser notifications
export const lesserNotificationFixtures: LesserNotificationFragment[] = [
	{
		id: 'notif_n1o2p3q4r5s6t7u8',
		notificationType: 'FAVORITE',
		createdAt: '2023-12-15T15:30:00.000Z',
		triggerAccount: getAccount(1),
		status: favoriteNotificationStatus,
		isRead: false,
	},
	{
		id: 'notif_o2p3q4r5s6t7u8v9',
		notificationType: 'SHARE',
		createdAt: '2023-12-15T16:15:00.000Z',
		triggerAccount: getAccount(1),
		status: shareNotificationStatus,
		isRead: false,
	},
	{
		id: 'notif_p3q4r5s6t7u8v9w0',
		notificationType: 'MENTION',
		createdAt: '2023-12-15T17:45:00.000Z',
		triggerAccount: getAccount(1),
		status: mentionNotificationStatus,
		isRead: true,
	},
	{
		id: 'notif_q4r5s6t7u8v9w0x1',
		notificationType: 'FOLLOW',
		createdAt: '2023-12-15T10:20:00.000Z',
		triggerAccount: getAccount(1),
		isRead: true,
	},
	{
		id: 'notif_r5s6t7u8v9w0x1y2',
		notificationType: 'POLL_ENDED',
		createdAt: '2023-12-16T18:00:00.000Z',
		triggerAccount: getAccount(0),
		status: pollEndedNotificationStatus,
		isRead: false,
	},
];

const getNotification = (index: number): LesserNotificationFragment =>
	requireFixture(lesserNotificationFixtures, index);

// Sample Lesser relationships
export const lesserRelationshipFixtures: LesserRelationshipFragment[] = [
	{
		target: { id: 'acc_8f3a9b2c1d4e5f6g' },
		isFollowing: true,
		isFollowedBy: true,
		hasPendingRequest: false,
		isBlocking: false,
		isBlockedBy: false,
		isMuting: false,
		isMutingNotifications: false,
		isDomainBlocked: false,
		isEndorsed: true,
		personalNote: 'Great developer with excellent design skills',
	},
	{
		target: { id: 'acc_9g4h5i6j7k8l9m0n' },
		isFollowing: true,
		isFollowedBy: false,
		hasPendingRequest: false,
		isBlocking: false,
		isBlockedBy: false,
		isMuting: false,
		isMutingNotifications: false,
		isDomainBlocked: false,
		isEndorsed: false,
	},
];

// Sample Lesser streaming updates
export const lesserStreamingUpdateFixtures: LesserStreamingUpdate[] = [
	{
		__typename: 'StreamingUpdate',
		eventType: 'POST_CREATED',
		timestamp: '2023-12-15T14:30:00.000Z',
		data: {
			__typename: 'PostStreamingData',
			post: getPost(0),
			timeline: 'home',
		},
	},
	{
		__typename: 'StreamingUpdate',
		eventType: 'NOTIFICATION_CREATED',
		timestamp: '2023-12-15T15:30:00.000Z',
		data: {
			__typename: 'NotificationStreamingData',
			notification: getNotification(0),
		},
	},
	{
		__typename: 'StreamingUpdate',
		eventType: 'POST_UPDATED',
		timestamp: '2023-12-15T17:25:00.000Z',
		data: {
			__typename: 'PostStreamingData',
			post: {
				...getPost(2),
				lastEditedAt: '2023-12-15T17:25:00.000Z',
			},
		},
	},
	{
		__typename: 'StreamingUpdate',
		eventType: 'POST_DELETED',
		timestamp: '2023-12-15T20:00:00.000Z',
		data: {
			__typename: 'DeleteStreamingData',
			deletedId: 'post_deleted_example_123',
			deletedType: 'POST',
		},
	},
	{
		__typename: 'StreamingUpdate',
		eventType: 'ACCOUNT_UPDATED',
		timestamp: '2023-12-15T18:10:00.000Z',
		data: {
			__typename: 'AccountStreamingData',
			account: {
				...getAccount(0),
				followerCount: getAccount(0).followerCount + 1,
			},
		},
	},
];

type TimelineResponse = LesserGraphQLResponse<{ timeline: LesserTimelineConnection | null } | null>;

// Sample Lesser GraphQL responses
export const lesserGraphQLResponseFixtures: TimelineResponse[] = [
	// Successful timeline query
	{
		data: {
			timeline: {
				edges: lesserPostFixtures.map((post, index) => ({
					node: post,
					cursor: `cursor_${index}`,
				})),
				pageInfo: {
					hasNextPage: true,
					hasPreviousPage: false,
					startCursor: 'cursor_0',
					endCursor: 'cursor_3',
				},
				totalCount: 42,
			},
		},
	},
	// Error response
	{
		data: null,
		errors: [
			{
				message: 'Authentication required',
				locations: [{ line: 2, column: 3 }],
				path: ['timeline'],
				extensions: {
					code: 'UNAUTHENTICATED',
				},
			},
		],
	},
	// Partial data with errors
	{
		data: {
			timeline: null,
		},
		errors: [
			{
				message: 'Timeline access denied',
				path: ['timeline'],
				extensions: {
					code: 'FORBIDDEN',
				},
			},
		],
	},
];

// Sample Lesser timeline connection
export const lesserTimelineConnectionFixture: LesserTimelineConnection = {
	edges: lesserPostFixtures.map((post, index) => ({
		node: post,
		cursor: `timeline_cursor_${index}`,
	})),
	pageInfo: {
		hasNextPage: true,
		hasPreviousPage: false,
		startCursor: 'timeline_cursor_0',
		endCursor: 'timeline_cursor_3',
	},
	totalCount: 156,
};

// Error test cases
export const lesserErrorFixtures = {
	invalidAccount: {
		id: '',
		handle: null,
		// Missing required fields
	},
	invalidPost: {
		id: 'post_123',
		content: 'Valid content',
		publishedAt: 'invalid-date',
		// Missing author field
	},
	malformedGraphQLResponse: {
		// Missing data and errors fields
		extensions: {},
	},
	invalidStreamingUpdate: {
		__typename: 'StreamingUpdate',
		eventType: null,
		// Missing timestamp and data
	},
};

// Batch test data
export const lesserBatchFixtures = {
	accounts: lesserAccountFixtures,
	posts: lesserPostFixtures,
	notifications: lesserNotificationFixtures,
	streamingUpdates: lesserStreamingUpdateFixtures,
	mixed: [
		lesserAccountFixtures[0],
		{ invalid: 'data' },
		lesserAccountFixtures[1],
		undefined,
		lesserAccountFixtures[0], // Duplicate
	],
};
