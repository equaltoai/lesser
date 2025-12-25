/**
 * Optimistic Update Policies for GraphQL Mutations
 *
 * Provides instant UI feedback before server response
 */
import type { ApolloCache } from '@apollo/client';
type StatusPayload = {
	__typename?: string;
	id: string;
} & Record<string, unknown>;
type PollOption = {
	votesCount: number;
} & Record<string, unknown>;
/**
 * Optimistic response for favouriting a status
 */
export declare function optimisticFavourite(
	statusId: string,
	currentState: boolean
): {
	__typename: string;
	id: string;
	favourited: boolean;
	favouritesCount: number;
};
/**
 * Optimistic response for reblogging a status
 */
export declare function optimisticReblog(
	statusId: string,
	currentState: boolean
): {
	__typename: string;
	id: string;
	reblogged: boolean;
	reblogsCount: number;
};
/**
 * Optimistic response for bookmarking a status
 */
export declare function optimisticBookmark(
	statusId: string,
	currentState: boolean
): {
	__typename: string;
	id: string;
	bookmarked: boolean;
};
/**
 * Optimistic response for following an account
 */
export declare function optimisticFollow(
	accountId: string,
	currentState: boolean
): {
	__typename: string;
	id: string;
	following: boolean;
	requested: boolean;
};
/**
 * Optimistic response for blocking an account
 */
export declare function optimisticBlock(
	accountId: string,
	currentState: boolean
): {
	__typename: string;
	id: string;
	blocking: boolean;
	following: boolean;
	followedBy: boolean;
};
/**
 * Optimistic response for muting an account
 */
export declare function optimisticMute(
	accountId: string,
	currentState: boolean,
	notifications?: boolean
): {
	__typename: string;
	id: string;
	muting: boolean;
	mutingNotifications: boolean;
};
/**
 * Update cache after favouriting a status
 */
export declare function updateCacheAfterFavourite(
	cache: ApolloCache,
	statusId: string,
	favourited: boolean
): void;
/**
 * Update cache after reblogging a status
 */
export declare function updateCacheAfterReblog(
	cache: ApolloCache,
	statusId: string,
	reblogged: boolean
): void;
/**
 * Update cache after bookmarking a status
 */
export declare function updateCacheAfterBookmark(
	cache: ApolloCache,
	statusId: string,
	bookmarked: boolean
): void;
/**
 * Update cache after following an account
 */
export declare function updateCacheAfterFollow(
	cache: ApolloCache,
	accountId: string,
	following: boolean,
	locked: boolean
): void;
/**
 * Update cache after blocking an account
 */
export declare function updateCacheAfterBlock(
	cache: ApolloCache,
	accountId: string,
	blocking: boolean
): void;
/**
 * Update cache after muting an account
 */
export declare function updateCacheAfterMute(
	cache: ApolloCache,
	accountId: string,
	muting: boolean,
	notifications?: boolean
): void;
/**
 * Remove status from cache (after delete)
 */
export declare function removeStatusFromCache(cache: ApolloCache, statusId: string): void;
/**
 * Add status to cache (after create)
 */
export declare function addStatusToCache(
	cache: ApolloCache,
	status: StatusPayload,
	timelineField?: string
): void;
/**
 * Update status in cache (after edit)
 */
export declare function updateStatusInCache(cache: ApolloCache, status: StatusPayload): void;
/**
 * Optimistic response for deleting a status
 */
export declare function optimisticDeleteStatus(statusId: string): {
	deleteStatus: {
		__typename: string;
		id: string;
	};
};
/**
 * Optimistic response for voting in a poll
 */
export declare function optimisticVotePoll(
	pollId: string,
	choices: number[],
	options: PollOption[]
): {
	votePoll: {
		__typename: 'Poll';
		id: string;
		voted: boolean;
		ownVotes: number[];
		votesCount: number;
		options: PollOption[];
	};
};
export {};
//# sourceMappingURL=optimistic.d.ts.map
