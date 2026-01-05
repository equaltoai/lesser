/**
 * Mastodon API test fixtures
 * Realistic test data based on Mastodon API documentation
 */

import type {
	MastodonAccount,
	MastodonStatus,
	MastodonNotification,
	MastodonMediaAttachment,
	MastodonPoll,
	MastodonStreamingEvent,
	MastodonRelationship,
} from '../mappers/mastodon/types.js';

const requireFixture = <T>(items: readonly T[], index: number): T => {
	const item = items[index];
	if (!item) {
		throw new Error(`Missing Mastodon fixture at index ${index}`);
	}
	return item;
};

// Sample Mastodon accounts
export const mastodonAccountFixtures: MastodonAccount[] = [
	{
		id: '109319135653080826',
		username: 'alice',
		acct: 'alice@mastodon.social',
		display_name: 'Alice Cooper',
		locked: false,
		bot: false,
		discoverable: true,
		group: false,
		created_at: '2022-11-15T08:30:00.000Z',
		note: '<p>Friendly neighborhood developer 🚀<br />Building cool things with code.<br /><a href="https://github.com/alice" target="_blank" rel="nofollow noopener noreferrer">GitHub</a></p>',
		url: 'https://mastodon.social/@alice',
		avatar:
			'https://files.mastodon.social/accounts/avatars/109/319/135/653/080/826/original/abc123.jpg',
		avatar_static:
			'https://files.mastodon.social/accounts/avatars/109/319/135/653/080/826/static/abc123.jpg',
		header:
			'https://files.mastodon.social/accounts/headers/109/319/135/653/080/826/original/def456.jpg',
		header_static:
			'https://files.mastodon.social/accounts/headers/109/319/135/653/080/826/static/def456.jpg',
		followers_count: 1234,
		following_count: 567,
		statuses_count: 890,
		last_status_at: '2023-12-15T10:30:00.000Z',
		verified: true,
		fields: [
			{
				name: 'Website',
				value:
					'<a href="https://alice.dev" target="_blank" rel="nofollow noopener noreferrer">alice.dev</a>',
				verified_at: '2023-01-15T12:00:00.000Z',
			},
			{
				name: 'Location',
				value: 'San Francisco, CA',
			},
		],
		emojis: [
			{
				shortcode: 'rocket',
				url: 'https://files.mastodon.social/custom_emojis/images/000/001/234/original/rocket.png',
				static_url:
					'https://files.mastodon.social/custom_emojis/images/000/001/234/static/rocket.png',
				visible_in_picker: true,
				category: 'activities',
			},
		],
	},
	{
		id: '109320456789012345',
		username: 'bot_example',
		acct: 'bot_example@example.org',
		display_name: 'Example Bot',
		locked: false,
		bot: true,
		discoverable: false,
		group: false,
		created_at: '2022-11-16T14:20:00.000Z',
		note: "<p>I'm a helpful bot! 🤖<br />Automated posts about tech news.</p>",
		url: 'https://example.org/@bot_example',
		avatar: 'https://example.org/avatars/bot.png',
		avatar_static: 'https://example.org/avatars/bot.png',
		header: 'https://example.org/headers/bot-header.png',
		header_static: 'https://example.org/headers/bot-header.png',
		followers_count: 89,
		following_count: 12,
		statuses_count: 456,
		last_status_at: '2023-12-15T09:15:00.000Z',
		verified: false,
		fields: [],
		emojis: [],
	},
];

const getAccount = (index: number): MastodonAccount =>
	requireFixture(mastodonAccountFixtures, index);

// Sample Mastodon media attachments
export const mastodonMediaAttachments: MastodonMediaAttachment[] = [
	{
		id: '109321000111222333',
		type: 'image',
		url: 'https://files.mastodon.social/media_attachments/files/109/321/000/111/222/333/original/sunset.jpg',
		preview_url:
			'https://files.mastodon.social/media_attachments/files/109/321/000/111/222/333/small/sunset.jpg',
		description: 'Beautiful sunset over the mountains with orange and purple hues',
		blurhash: 'LKO2:N%2Tw=w]~RBVZRi};RPxuwH',
		meta: {
			original: {
				width: 1920,
				height: 1080,
				size: '1920x1080',
				aspect: 1.7777777777777777,
			},
			small: {
				width: 640,
				height: 360,
				size: '640x360',
				aspect: 1.7777777777777777,
			},
		},
	},
	{
		id: '109321000111222334',
		type: 'video',
		url: 'https://files.mastodon.social/media_attachments/files/109/321/000/111/222/334/original/timelapse.mp4',
		preview_url:
			'https://files.mastodon.social/media_attachments/files/109/321/000/111/222/334/small/timelapse.jpg',
		description: 'Time-lapse video of clouds moving across the sky',
		meta: {
			original: {
				width: 1280,
				height: 720,
				frame_rate: '30/1',
				duration: 10.5,
				bitrate: 2500000,
			},
			small: {
				width: 400,
				height: 225,
				size: '400x225',
				aspect: 1.7777777777777777,
			},
		},
	},
];

// Sample Mastodon polls
export const mastodonPollFixtures: MastodonPoll[] = [
	{
		id: '109321111222333444',
		expires_at: '2023-12-16T10:30:00.000Z',
		expired: false,
		multiple: false,
		votes_count: 42,
		voters_count: 42,
		voted: true,
		own_votes: [1],
		options: [
			{
				title: 'Coffee ☕',
				votes_count: 18,
			},
			{
				title: 'Tea 🍵',
				votes_count: 24,
			},
		],
		emojis: [],
	},
	{
		id: '109321111222333445',
		expires_at: '2023-12-20T15:00:00.000Z',
		expired: false,
		multiple: true,
		votes_count: 156,
		voters_count: 89,
		voted: false,
		options: [
			{
				title: 'JavaScript',
				votes_count: 67,
			},
			{
				title: 'TypeScript',
				votes_count: 45,
			},
			{
				title: 'Python',
				votes_count: 34,
			},
			{
				title: 'Rust',
				votes_count: 10,
			},
		],
		emojis: [],
	},
];

// Sample Mastodon statuses
export const mastodonStatusFixtures: MastodonStatus[] = [
	{
		id: '109321123456789012',
		created_at: '2023-12-15T10:30:00.000Z',
		in_reply_to_id: undefined,
		in_reply_to_account_id: undefined,
		sensitive: false,
		spoiler_text: '',
		visibility: 'public',
		language: 'en',
		uri: 'https://mastodon.social/users/alice/statuses/109321123456789012',
		url: 'https://mastodon.social/@alice/109321123456789012',
		replies_count: 3,
		reblogs_count: 7,
		favourites_count: 14,
		favourited: true,
		reblogged: false,
		muted: false,
		bookmarked: true,
		pinned: false,
		content:
			'<p>Just shipped a new feature! 🚀 The new timeline view is much more responsive now. <a href="https://mastodon.social/tags/webdev" class="mention hashtag" rel="tag">#<span>webdev</span></a> <a href="https://mastodon.social/tags/svelte" class="mention hashtag" rel="tag">#<span>svelte</span></a></p>',
		account: getAccount(0),
		media_attachments: mastodonMediaAttachments[0] ? [mastodonMediaAttachments[0]] : [],
		mentions: [],
		tags: [
			{
				name: 'webdev',
				url: 'https://mastodon.social/tags/webdev',
			},
			{
				name: 'svelte',
				url: 'https://mastodon.social/tags/svelte',
				history: [
					{
						day: '1702598400', // 2023-12-15
						uses: '23',
						accounts: '15',
					},
					{
						day: '1702512000', // 2023-12-14
						uses: '18',
						accounts: '12',
					},
				],
			},
		],
		emojis: [
			{
				shortcode: 'rocket',
				url: 'https://files.mastodon.social/custom_emojis/images/000/001/234/original/rocket.png',
				static_url:
					'https://files.mastodon.social/custom_emojis/images/000/001/234/static/rocket.png',
				visible_in_picker: true,
				category: 'activities',
			},
		],
		poll: mastodonPollFixtures[0],
	},
	{
		id: '109321123456789013',
		created_at: '2023-12-15T14:45:00.000Z',
		in_reply_to_id: '109321123456789012',
		in_reply_to_account_id: '109319135653080826',
		sensitive: false,
		spoiler_text: '',
		visibility: 'public',
		language: 'en',
		uri: 'https://example.org/users/bot_example/statuses/109321123456789013',
		url: 'https://example.org/@bot_example/109321123456789013',
		replies_count: 0,
		reblogs_count: 2,
		favourites_count: 5,
		favourited: false,
		reblogged: false,
		muted: false,
		bookmarked: false,
		pinned: false,
		content:
			'<p><span class="h-card"><a href="https://mastodon.social/@alice" class="u-url mention">@<span>alice</span></a></span> Congratulations on the new feature! The performance improvements are really noticeable. 🎉</p>',
		account: getAccount(1),
		media_attachments: [],
		mentions: [
			{
				id: '109319135653080826',
				username: 'alice',
				url: 'https://mastodon.social/@alice',
				acct: 'alice@mastodon.social',
			},
		],
		tags: [],
		emojis: [],
	},
	{
		id: '109321123456789014',
		created_at: '2023-12-15T16:20:00.000Z',
		in_reply_to_id: undefined,
		in_reply_to_account_id: undefined,
		sensitive: true,
		spoiler_text: 'Political discussion',
		visibility: 'unlisted',
		language: 'en',
		uri: 'https://mastodon.social/users/alice/statuses/109321123456789014',
		url: 'https://mastodon.social/@alice/109321123456789014',
		replies_count: 8,
		reblogs_count: 0,
		favourites_count: 3,
		edited_at: '2023-12-15T16:25:00.000Z',
		favourited: false,
		reblogged: false,
		muted: false,
		bookmarked: false,
		pinned: false,
		content:
			'<p>Some thoughts on the recent policy changes and their impact on the tech industry...</p>',
		account: getAccount(0),
		media_attachments: [],
		mentions: [],
		tags: [],
		emojis: [],
	},
];

// Sample Mastodon notifications
export const mastodonNotificationFixtures: MastodonNotification[] = [
	{
		id: '109321234567890123',
		type: 'favourite',
		created_at: '2023-12-15T11:15:00.000Z',
		account: getAccount(1),
		status: mastodonStatusFixtures[0],
	},
	{
		id: '109321234567890124',
		type: 'reblog',
		created_at: '2023-12-15T12:30:00.000Z',
		account: getAccount(1),
		status: mastodonStatusFixtures[0],
	},
	{
		id: '109321234567890125',
		type: 'mention',
		created_at: '2023-12-15T14:45:00.000Z',
		account: getAccount(1),
		status: mastodonStatusFixtures[1],
	},
	{
		id: '109321234567890126',
		type: 'follow',
		created_at: '2023-12-15T09:00:00.000Z',
		account: getAccount(1),
	},
	{
		id: '109321234567890127',
		type: 'poll',
		created_at: '2023-12-16T10:30:00.000Z',
		account: getAccount(0),
		status: mastodonStatusFixtures[0],
	},
];

// Sample Mastodon relationships
export const mastodonRelationshipFixtures: MastodonRelationship[] = [
	{
		id: '109319135653080826',
		following: true,
		showing_reblogs: true,
		notifying: false,
		followed_by: true,
		blocking: false,
		blocked_by: false,
		muting: false,
		muting_notifications: false,
		requested: false,
		domain_blocking: false,
		endorsed: true,
		note: 'Great developer and helpful community member',
	},
	{
		id: '109320456789012345',
		following: false,
		showing_reblogs: false,
		notifying: false,
		followed_by: false,
		blocking: false,
		blocked_by: false,
		muting: false,
		muting_notifications: false,
		requested: false,
		domain_blocking: false,
		endorsed: false,
		note: '',
	},
];

// Sample Mastodon streaming events
export const mastodonStreamingEventFixtures: MastodonStreamingEvent[] = [
	{
		stream: ['user'],
		event: 'update',
		payload: JSON.stringify(mastodonStatusFixtures[0]),
	},
	{
		stream: ['user'],
		event: 'notification',
		payload: JSON.stringify(mastodonNotificationFixtures[0]),
	},
	{
		stream: ['public'],
		event: 'update',
		payload: JSON.stringify(mastodonStatusFixtures[1]),
	},
	{
		stream: ['user'],
		event: 'delete',
		payload: '109321123456789012',
	},
	{
		stream: ['hashtag'],
		event: 'update',
		payload: JSON.stringify({
			...mastodonStatusFixtures[0],
			id: '109321123456789015',
			content: '<p>Updated post content after edit</p>',
			edited_at: '2023-12-15T11:00:00.000Z',
		}),
	},
];

// Error test cases
export const mastodonErrorFixtures = {
	invalidAccount: {
		id: null,
		username: '',
		// Missing required fields
	},
	invalidStatus: {
		id: '123',
		content: 'Valid content',
		// Missing account field
	},
	malformedJson: {
		stream: ['user'],
		event: 'update',
		payload: 'invalid json {{',
	},
	missingFields: {
		id: '123',
		type: 'favourite',
		created_at: '2023-12-15T11:15:00.000Z',
		// Missing account field
	},
};

// Batch test data
export const mastodonBatchFixtures = {
	accounts: mastodonAccountFixtures,
	statuses: mastodonStatusFixtures,
	notifications: mastodonNotificationFixtures,
	mixed: [
		mastodonAccountFixtures[0],
		{ invalid: 'data' },
		mastodonAccountFixtures[1],
		null,
		mastodonAccountFixtures[0], // Duplicate
	],
};
