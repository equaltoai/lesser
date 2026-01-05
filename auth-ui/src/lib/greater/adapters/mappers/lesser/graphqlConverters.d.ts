import type { LesserAccountFragment, LesserObjectFragment } from './types.js';
export declare function convertGraphQLActorToLesserAccount(
	actor: unknown
): LesserAccountFragment | null;
export declare function convertGraphQLObjectToLesser(
	object: unknown,
	depth?: number
): LesserObjectFragment | null;
export interface ActorListPage {
	actors: LesserAccountFragment[];
	nextCursor?: string;
	totalCount: number;
}
export declare function convertGraphQLActorListPage(data: unknown): ActorListPage | null;
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
	reblogFilters: Array<{
		key: string;
		enabled: boolean;
	}>;
}
export declare function convertGraphQLUserPreferences(data: unknown): UserPreferences | null;
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
export declare function convertGraphQLPushSubscription(data: unknown): PushSubscription | null;
//# sourceMappingURL=graphqlConverters.d.ts.map
