/**
 * Mastodon REST API v1 payload types
 * Based on official Mastodon API documentation
 * https://docs.joinmastodon.org/api/
 */
export interface MastodonAccount {
	id: string;
	username: string;
	acct: string;
	display_name: string;
	locked: boolean;
	bot: boolean;
	discoverable?: boolean;
	group: boolean;
	created_at: string;
	note: string;
	url: string;
	avatar: string;
	avatar_static: string;
	header: string;
	header_static: string;
	followers_count: number;
	following_count: number;
	statuses_count: number;
	last_status_at?: string;
	verified?: boolean;
	fields?: MastodonField[];
	emojis?: MastodonEmoji[];
	moved?: MastodonAccount;
	suspended?: boolean;
	limited?: boolean;
}
export interface MastodonField {
	name: string;
	value: string;
	verified_at?: string;
}
export interface MastodonEmoji {
	shortcode: string;
	url: string;
	static_url: string;
	visible_in_picker: boolean;
	category?: string;
}
export interface MastodonStatus {
	id: string;
	created_at: string;
	in_reply_to_id?: string;
	in_reply_to_account_id?: string;
	sensitive: boolean;
	spoiler_text: string;
	visibility: 'public' | 'unlisted' | 'private' | 'direct';
	language?: string;
	uri: string;
	url?: string;
	replies_count: number;
	reblogs_count: number;
	favourites_count: number;
	edited_at?: string;
	favourited?: boolean;
	reblogged?: boolean;
	muted?: boolean;
	bookmarked?: boolean;
	pinned?: boolean;
	content: string;
	reblog?: MastodonStatus;
	account: MastodonAccount;
	media_attachments: MastodonMediaAttachment[];
	mentions: MastodonMention[];
	tags: MastodonTag[];
	emojis: MastodonEmoji[];
	card?: MastodonPreviewCard;
	poll?: MastodonPoll;
	application?: MastodonApplication;
	filtered?: MastodonFilterResult[];
}
export interface MastodonMediaAttachment {
	id: string;
	type: 'image' | 'video' | 'gifv' | 'audio' | 'unknown';
	url: string;
	preview_url: string;
	remote_url?: string;
	meta?: MastodonMediaMeta;
	description?: string;
	blurhash?: string;
}
export interface MastodonMediaMeta {
	original?: {
		width?: number;
		height?: number;
		size?: string;
		aspect?: number;
		frame_rate?: string;
		duration?: number;
		bitrate?: number;
	};
	small?: {
		width?: number;
		height?: number;
		size?: string;
		aspect?: number;
	};
	focus?: {
		x: number;
		y: number;
	};
}
export interface MastodonMention {
	id: string;
	username: string;
	url: string;
	acct: string;
}
export interface MastodonTag {
	name: string;
	url: string;
	history?: MastodonTagHistory[];
}
export interface MastodonTagHistory {
	day: string;
	uses: string;
	accounts: string;
}
export interface MastodonPreviewCard {
	url: string;
	title: string;
	description: string;
	type: 'link' | 'photo' | 'video' | 'rich';
	author_name?: string;
	author_url?: string;
	provider_name?: string;
	provider_url?: string;
	html?: string;
	width?: number;
	height?: number;
	image?: string;
	embed_url?: string;
	blurhash?: string;
}
export interface MastodonPoll {
	id: string;
	expires_at?: string;
	expired: boolean;
	multiple: boolean;
	votes_count: number;
	voters_count?: number;
	voted?: boolean;
	own_votes?: number[];
	options: MastodonPollOption[];
	emojis: MastodonEmoji[];
}
export interface MastodonPollOption {
	title: string;
	votes_count?: number;
}
export interface MastodonApplication {
	name: string;
	website?: string;
	vapid_key?: string;
	client_id?: string;
	client_secret?: string;
}
export interface MastodonFilterResult {
	filter: MastodonFilter;
	keyword_matches?: string[];
	status_matches?: string[];
}
export interface MastodonFilter {
	id: string;
	title: string;
	context: ('home' | 'notifications' | 'public' | 'thread' | 'account')[];
	expires_at?: string;
	filter_action: 'warn' | 'hide';
	keywords: MastodonFilterKeyword[];
	statuses: MastodonFilterStatus[];
}
export interface MastodonFilterKeyword {
	id: string;
	keyword: string;
	whole_word: boolean;
}
export interface MastodonFilterStatus {
	id: string;
	status_id: string;
}
export interface MastodonNotification {
	id: string;
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
		| 'admin.report';
	created_at: string;
	account: MastodonAccount;
	status?: MastodonStatus;
	report?: MastodonReport;
}
export interface MastodonReport {
	id: string;
	action_taken: boolean;
	action_taken_at?: string;
	category: 'spam' | 'violation' | 'other';
	comment: string;
	forwarded: boolean;
	created_at: string;
	status_ids?: string[];
	rule_ids?: string[];
	target_account: MastodonAccount;
}
export interface MastodonRelationship {
	id: string;
	following: boolean;
	showing_reblogs: boolean;
	notifying: boolean;
	followed_by: boolean;
	blocking: boolean;
	blocked_by: boolean;
	muting: boolean;
	muting_notifications: boolean;
	requested: boolean;
	domain_blocking: boolean;
	endorsed: boolean;
	note: string;
}
export interface MastodonStreamingEvent {
	stream: string[];
	event: 'update' | 'delete' | 'notification' | 'filters_changed' | 'conversation' | 'announcement';
	payload: string;
}
export interface MastodonContext {
	ancestors: MastodonStatus[];
	descendants: MastodonStatus[];
}
export interface MastodonSearchResults {
	accounts: MastodonAccount[];
	statuses: MastodonStatus[];
	hashtags: MastodonTag[];
}
export interface MastodonInstance {
	uri: string;
	title: string;
	short_description: string;
	description: string;
	email: string;
	version: string;
	urls: {
		streaming_api: string;
	};
	stats: {
		user_count: number;
		status_count: number;
		domain_count: number;
	};
	thumbnail?: string;
	languages: string[];
	registrations: boolean;
	approval_required: boolean;
	invites_enabled: boolean;
	configuration: {
		statuses: {
			max_characters: number;
			max_media_attachments: number;
			characters_reserved_per_url: number;
		};
		media_attachments: {
			supported_mime_types: string[];
			image_size_limit: number;
			image_matrix_limit: number;
			video_size_limit: number;
			video_frame_rate_limit: number;
			video_matrix_limit: number;
		};
		polls: {
			max_options: number;
			max_characters_per_option: number;
			min_expiration: number;
			max_expiration: number;
		};
	};
	contact_account?: MastodonAccount;
	rules: MastodonRule[];
}
export interface MastodonRule {
	id: string;
	text: string;
}
export interface MastodonError {
	error: string;
	error_description?: string;
}
export interface MastodonPaginationParams {
	max_id?: string;
	since_id?: string;
	min_id?: string;
	limit?: number;
}
export interface MastodonPaginatedResponse<T> {
	data: T[];
	pagination?: {
		next?: string;
		prev?: string;
	};
}
export declare const MASTODON_VISIBILITY_VALUES: readonly [
	'public',
	'unlisted',
	'private',
	'direct',
];
export declare const MASTODON_MEDIA_TYPES: readonly ['image', 'video', 'gifv', 'audio', 'unknown'];
export declare const MASTODON_NOTIFICATION_TYPES: readonly [
	'mention',
	'status',
	'reblog',
	'follow',
	'follow_request',
	'favourite',
	'poll',
	'update',
	'admin.sign_up',
	'admin.report',
];
export declare function isMastodonAccount(obj: unknown): obj is MastodonAccount;
export declare function isMastodonStatus(obj: unknown): obj is MastodonStatus;
export declare function isMastodonNotification(obj: unknown): obj is MastodonNotification;
export declare function isMastodonStreamingEvent(obj: unknown): obj is MastodonStreamingEvent;
//# sourceMappingURL=types.d.ts.map
