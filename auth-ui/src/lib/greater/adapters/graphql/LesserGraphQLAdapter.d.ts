/**
 * Lesser GraphQL Adapter aligned with the current Lesser schema.
 *
 * Provides typed accessors and convenience helpers around the generated
 * GraphQL operations. Consumers should migrate towards the generic timeline
 * and object accessors rather than the legacy Mastodon-style wrappers.
 */
import { Observable, type FetchResult } from '@apollo/client';
import { type GraphQLClientConfig } from './client.js';
import type {
	TimelineQueryVariables,
	TimelineType,
	NotificationsQueryVariables,
	SearchQueryVariables,
	CreateNoteMutationVariables,
	CreateQuoteNoteMutationVariables,
	CreateListMutationVariables,
	UpdateListMutationVariables,
	ConversationsQueryVariables,
	UpdateRelationshipMutationVariables,
	TimelineUpdatesSubscription,
	TimelineUpdatesSubscriptionVariables,
	NotificationStreamSubscription,
	NotificationStreamSubscriptionVariables,
	ConversationUpdatesSubscription,
	ListUpdatesSubscription,
	ListUpdatesSubscriptionVariables,
	QuoteActivitySubscription,
	QuoteActivitySubscriptionVariables,
	HashtagActivitySubscription,
	HashtagActivitySubscriptionVariables,
	ActivityStreamSubscription,
	ActivityStreamSubscriptionVariables,
	RelationshipUpdatesSubscription,
	RelationshipUpdatesSubscriptionVariables,
	CostUpdatesSubscription,
	CostUpdatesSubscriptionVariables,
	ModerationEventsSubscription,
	ModerationEventsSubscriptionVariables,
	TrustUpdatesSubscription,
	TrustUpdatesSubscriptionVariables,
	AiAnalysisUpdatesSubscription,
	AiAnalysisUpdatesSubscriptionVariables,
	MetricsUpdatesSubscription,
	MetricsUpdatesSubscriptionVariables,
	ModerationAlertsSubscription,
	ModerationAlertsSubscriptionVariables,
	CostAlertsSubscription,
	CostAlertsSubscriptionVariables,
	BudgetAlertsSubscription,
	BudgetAlertsSubscriptionVariables,
	FederationHealthUpdatesSubscription,
	FederationHealthUpdatesSubscriptionVariables,
	ModerationQueueUpdateSubscription,
	ModerationQueueUpdateSubscriptionVariables,
	ThreatIntelligenceSubscription,
	PerformanceAlertSubscription,
	PerformanceAlertSubscriptionVariables,
	InfrastructureEventSubscription,
	RelationshipQuery,
	ModerationPatternInput,
	HashtagNotificationSettingsInput,
	NotificationLevel,
	UploadMediaInput,
	UploadMediaMutation,
} from './generated/types.js';
export type LesserGraphQLAdapterConfig = GraphQLClientConfig;
export type TimelineVariables = TimelineQueryVariables;
export type SearchVariables = SearchQueryVariables;
export type CreateNoteVariables = CreateNoteMutationVariables;
export declare class LesserGraphQLAdapter {
	private readonly client;
	private readonly httpEndpoint;
	private readonly baseHeaders;
	private authToken;
	constructor(config: LesserGraphQLAdapterConfig);
	updateToken(token: string | null): void;
	close(): void;
	private query;
	private mutate;
	private static hasMissingTargetIdError;
	private buildUploadMediaFormData;
	fetchTimeline(variables: TimelineQueryVariables): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	fetchHomeTimeline(
		pagination?: Partial<Pick<TimelineQueryVariables, 'first' | 'after'>>
	): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	fetchPublicTimeline(
		pagination?: Partial<Pick<TimelineQueryVariables, 'first' | 'after'>>,
		scope?: Extract<TimelineType, 'PUBLIC' | 'LOCAL'>
	): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	fetchDirectTimeline(
		pagination?: Partial<Pick<TimelineQueryVariables, 'first' | 'after'>>
	): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	fetchHashtagTimeline(
		hashtag: string,
		pagination?: Partial<Pick<TimelineQueryVariables, 'first' | 'after'>>
	): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	fetchListTimeline(
		listId: string,
		pagination?: Partial<Pick<TimelineQueryVariables, 'first' | 'after'>>
	): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	fetchActorTimeline(
		actorId: string,
		pagination?: Partial<Pick<TimelineQueryVariables, 'first' | 'after' | 'mediaOnly'>>
	): Promise<{
		readonly __typename: 'ObjectConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'ObjectEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	getObject(id: string): Promise<
		| {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
		  }
		| null
		| undefined
	>;
	getActorById(id: string): Promise<
		| {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
		  }
		| null
		| undefined
	>;
	getActorByUsername(username: string): Promise<
		| {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
		  }
		| null
		| undefined
	>;
	search(variables: SearchQueryVariables): Promise<{
		readonly __typename: 'SearchResult';
		readonly accounts: ReadonlyArray<{
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		}>;
		readonly statuses: ReadonlyArray<{
			readonly __typename: 'Object';
			readonly id: string;
			readonly type: import('./index.js').ObjectType;
			readonly content: string;
			readonly visibility: import('./index.js').Visibility;
			readonly sensitive: boolean;
			readonly spoilerText?: string | null | undefined;
			readonly createdAt: string;
			readonly updatedAt: string;
			readonly repliesCount: number;
			readonly likesCount: number;
			readonly sharesCount: number;
			readonly estimatedCost: number;
			readonly moderationScore?: number | null | undefined;
			readonly quoteUrl?: string | null | undefined;
			readonly quoteable: boolean;
			readonly quotePermissions: import('./index.js').QuotePermission;
			readonly quoteCount: number;
			readonly boostedObject?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly content: string;
						readonly visibility: import('./index.js').Visibility;
						readonly sensitive: boolean;
						readonly spoilerText?: string | null | undefined;
						readonly createdAt: string;
						readonly updatedAt: string;
						readonly repliesCount: number;
						readonly likesCount: number;
						readonly sharesCount: number;
						readonly estimatedCost: number;
						readonly moderationScore?: number | null | undefined;
						readonly quoteUrl?: string | null | undefined;
						readonly quoteable: boolean;
						readonly quotePermissions: import('./index.js').QuotePermission;
						readonly quoteCount: number;
						readonly contentMap: ReadonlyArray<{
							readonly __typename: 'ContentMap';
							readonly language: string;
							readonly content: string;
						}>;
						readonly attachments: ReadonlyArray<{
							readonly __typename: 'Attachment';
							readonly id: string;
							readonly type: string;
							readonly url: string;
							readonly preview?: string | null | undefined;
							readonly description?: string | null | undefined;
							readonly blurhash?: string | null | undefined;
							readonly width?: number | null | undefined;
							readonly height?: number | null | undefined;
							readonly duration?: number | null | undefined;
						}>;
						readonly tags: ReadonlyArray<{
							readonly __typename: 'Tag';
							readonly name: string;
							readonly url: string;
						}>;
						readonly mentions: ReadonlyArray<{
							readonly __typename: 'Mention';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly url: string;
						}>;
						readonly quoteContext?:
							| {
									readonly __typename: 'QuoteContext';
									readonly quoteAllowed: boolean;
									readonly quoteType: import('./index.js').QuoteType;
									readonly withdrawn: boolean;
									readonly originalAuthor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly originalNote?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly communityNotes: ReadonlyArray<{
							readonly __typename: 'CommunityNote';
							readonly id: string;
							readonly content: string;
							readonly helpful: number;
							readonly notHelpful: number;
							readonly createdAt: string;
							readonly author: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
						}>;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly inReplyTo?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly contentMap: ReadonlyArray<{
				readonly __typename: 'ContentMap';
				readonly language: string;
				readonly content: string;
			}>;
			readonly attachments: ReadonlyArray<{
				readonly __typename: 'Attachment';
				readonly id: string;
				readonly type: string;
				readonly url: string;
				readonly preview?: string | null | undefined;
				readonly description?: string | null | undefined;
				readonly blurhash?: string | null | undefined;
				readonly width?: number | null | undefined;
				readonly height?: number | null | undefined;
				readonly duration?: number | null | undefined;
			}>;
			readonly tags: ReadonlyArray<{
				readonly __typename: 'Tag';
				readonly name: string;
				readonly url: string;
			}>;
			readonly mentions: ReadonlyArray<{
				readonly __typename: 'Mention';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly url: string;
			}>;
			readonly quoteContext?:
				| {
						readonly __typename: 'QuoteContext';
						readonly quoteAllowed: boolean;
						readonly quoteType: import('./index.js').QuoteType;
						readonly withdrawn: boolean;
						readonly originalAuthor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly originalNote?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly communityNotes: ReadonlyArray<{
				readonly __typename: 'CommunityNote';
				readonly id: string;
				readonly content: string;
				readonly helpful: number;
				readonly notHelpful: number;
				readonly createdAt: string;
				readonly author: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
			}>;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly inReplyTo?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
				  }
				| null
				| undefined;
		}>;
		readonly hashtags: ReadonlyArray<{
			readonly __typename: 'Tag';
			readonly name: string;
			readonly url: string;
		}>;
	}>;
	fetchNotifications(variables: NotificationsQueryVariables): Promise<{
		readonly __typename: 'NotificationConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'NotificationEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Notification';
				readonly id: string;
				readonly type: string;
				readonly read: boolean;
				readonly createdAt: string;
				readonly account: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly status?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly boostedObject?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly content: string;
										readonly visibility: import('./index.js').Visibility;
										readonly sensitive: boolean;
										readonly spoilerText?: string | null | undefined;
										readonly createdAt: string;
										readonly updatedAt: string;
										readonly repliesCount: number;
										readonly likesCount: number;
										readonly sharesCount: number;
										readonly estimatedCost: number;
										readonly moderationScore?: number | null | undefined;
										readonly quoteUrl?: string | null | undefined;
										readonly quoteable: boolean;
										readonly quotePermissions: import('./index.js').QuotePermission;
										readonly quoteCount: number;
										readonly contentMap: ReadonlyArray<{
											readonly __typename: 'ContentMap';
											readonly language: string;
											readonly content: string;
										}>;
										readonly attachments: ReadonlyArray<{
											readonly __typename: 'Attachment';
											readonly id: string;
											readonly type: string;
											readonly url: string;
											readonly preview?: string | null | undefined;
											readonly description?: string | null | undefined;
											readonly blurhash?: string | null | undefined;
											readonly width?: number | null | undefined;
											readonly height?: number | null | undefined;
											readonly duration?: number | null | undefined;
										}>;
										readonly tags: ReadonlyArray<{
											readonly __typename: 'Tag';
											readonly name: string;
											readonly url: string;
										}>;
										readonly mentions: ReadonlyArray<{
											readonly __typename: 'Mention';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly url: string;
										}>;
										readonly quoteContext?:
											| {
													readonly __typename: 'QuoteContext';
													readonly quoteAllowed: boolean;
													readonly quoteType: import('./index.js').QuoteType;
													readonly withdrawn: boolean;
													readonly originalAuthor: {
														readonly __typename: 'Actor';
														readonly id: string;
														readonly username: string;
														readonly domain?: string | null | undefined;
														readonly displayName?: string | null | undefined;
														readonly summary?: string | null | undefined;
														readonly avatar?: string | null | undefined;
														readonly header?: string | null | undefined;
														readonly followers: number;
														readonly following: number;
														readonly statusesCount: number;
														readonly bot: boolean;
														readonly locked: boolean;
														readonly updatedAt: string;
														readonly trustScore: number;
														readonly fields: ReadonlyArray<{
															readonly __typename: 'Field';
															readonly name: string;
															readonly value: string;
															readonly verifiedAt?: string | null | undefined;
														}>;
													};
													readonly originalNote?:
														| {
																readonly __typename: 'Object';
																readonly id: string;
																readonly type: import('./index.js').ObjectType;
														  }
														| null
														| undefined;
											  }
											| null
											| undefined;
										readonly communityNotes: ReadonlyArray<{
											readonly __typename: 'CommunityNote';
											readonly id: string;
											readonly content: string;
											readonly helpful: number;
											readonly notHelpful: number;
											readonly createdAt: string;
											readonly author: {
												readonly __typename: 'Actor';
												readonly id: string;
												readonly username: string;
												readonly domain?: string | null | undefined;
												readonly displayName?: string | null | undefined;
												readonly summary?: string | null | undefined;
												readonly avatar?: string | null | undefined;
												readonly header?: string | null | undefined;
												readonly followers: number;
												readonly following: number;
												readonly statusesCount: number;
												readonly bot: boolean;
												readonly locked: boolean;
												readonly updatedAt: string;
												readonly trustScore: number;
												readonly fields: ReadonlyArray<{
													readonly __typename: 'Field';
													readonly name: string;
													readonly value: string;
													readonly verifiedAt?: string | null | undefined;
												}>;
											};
										}>;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly inReplyTo?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
													readonly actor: {
														readonly __typename: 'Actor';
														readonly id: string;
														readonly username: string;
														readonly domain?: string | null | undefined;
														readonly displayName?: string | null | undefined;
														readonly summary?: string | null | undefined;
														readonly avatar?: string | null | undefined;
														readonly header?: string | null | undefined;
														readonly followers: number;
														readonly following: number;
														readonly statusesCount: number;
														readonly bot: boolean;
														readonly locked: boolean;
														readonly updatedAt: string;
														readonly trustScore: number;
														readonly fields: ReadonlyArray<{
															readonly __typename: 'Field';
															readonly name: string;
															readonly value: string;
															readonly verifiedAt?: string | null | undefined;
														}>;
													};
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	dismissNotification(id: string): Promise<boolean>;
	clearNotifications(): Promise<boolean>;
	getConversations(variables: ConversationsQueryVariables): Promise<
		readonly {
			readonly __typename: 'Conversation';
			readonly id: string;
			readonly unread: boolean;
			readonly createdAt: string;
			readonly updatedAt: string;
			readonly accounts: ReadonlyArray<{
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			}>;
			readonly lastStatus?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly content: string;
						readonly visibility: import('./index.js').Visibility;
						readonly sensitive: boolean;
						readonly spoilerText?: string | null | undefined;
						readonly createdAt: string;
						readonly updatedAt: string;
						readonly repliesCount: number;
						readonly likesCount: number;
						readonly sharesCount: number;
						readonly estimatedCost: number;
						readonly moderationScore?: number | null | undefined;
						readonly quoteUrl?: string | null | undefined;
						readonly quoteable: boolean;
						readonly quotePermissions: import('./index.js').QuotePermission;
						readonly quoteCount: number;
						readonly boostedObject?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly content: string;
									readonly visibility: import('./index.js').Visibility;
									readonly sensitive: boolean;
									readonly spoilerText?: string | null | undefined;
									readonly createdAt: string;
									readonly updatedAt: string;
									readonly repliesCount: number;
									readonly likesCount: number;
									readonly sharesCount: number;
									readonly estimatedCost: number;
									readonly moderationScore?: number | null | undefined;
									readonly quoteUrl?: string | null | undefined;
									readonly quoteable: boolean;
									readonly quotePermissions: import('./index.js').QuotePermission;
									readonly quoteCount: number;
									readonly contentMap: ReadonlyArray<{
										readonly __typename: 'ContentMap';
										readonly language: string;
										readonly content: string;
									}>;
									readonly attachments: ReadonlyArray<{
										readonly __typename: 'Attachment';
										readonly id: string;
										readonly type: string;
										readonly url: string;
										readonly preview?: string | null | undefined;
										readonly description?: string | null | undefined;
										readonly blurhash?: string | null | undefined;
										readonly width?: number | null | undefined;
										readonly height?: number | null | undefined;
										readonly duration?: number | null | undefined;
									}>;
									readonly tags: ReadonlyArray<{
										readonly __typename: 'Tag';
										readonly name: string;
										readonly url: string;
									}>;
									readonly mentions: ReadonlyArray<{
										readonly __typename: 'Mention';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly url: string;
									}>;
									readonly quoteContext?:
										| {
												readonly __typename: 'QuoteContext';
												readonly quoteAllowed: boolean;
												readonly quoteType: import('./index.js').QuoteType;
												readonly withdrawn: boolean;
												readonly originalAuthor: {
													readonly __typename: 'Actor';
													readonly id: string;
													readonly username: string;
													readonly domain?: string | null | undefined;
													readonly displayName?: string | null | undefined;
													readonly summary?: string | null | undefined;
													readonly avatar?: string | null | undefined;
													readonly header?: string | null | undefined;
													readonly followers: number;
													readonly following: number;
													readonly statusesCount: number;
													readonly bot: boolean;
													readonly locked: boolean;
													readonly updatedAt: string;
													readonly trustScore: number;
													readonly fields: ReadonlyArray<{
														readonly __typename: 'Field';
														readonly name: string;
														readonly value: string;
														readonly verifiedAt?: string | null | undefined;
													}>;
												};
												readonly originalNote?:
													| {
															readonly __typename: 'Object';
															readonly id: string;
															readonly type: import('./index.js').ObjectType;
													  }
													| null
													| undefined;
										  }
										| null
										| undefined;
									readonly communityNotes: ReadonlyArray<{
										readonly __typename: 'CommunityNote';
										readonly id: string;
										readonly content: string;
										readonly helpful: number;
										readonly notHelpful: number;
										readonly createdAt: string;
										readonly author: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
									}>;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly inReplyTo?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
												readonly actor: {
													readonly __typename: 'Actor';
													readonly id: string;
													readonly username: string;
													readonly domain?: string | null | undefined;
													readonly displayName?: string | null | undefined;
													readonly summary?: string | null | undefined;
													readonly avatar?: string | null | undefined;
													readonly header?: string | null | undefined;
													readonly followers: number;
													readonly following: number;
													readonly statusesCount: number;
													readonly bot: boolean;
													readonly locked: boolean;
													readonly updatedAt: string;
													readonly trustScore: number;
													readonly fields: ReadonlyArray<{
														readonly __typename: 'Field';
														readonly name: string;
														readonly value: string;
														readonly verifiedAt?: string | null | undefined;
													}>;
												};
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly contentMap: ReadonlyArray<{
							readonly __typename: 'ContentMap';
							readonly language: string;
							readonly content: string;
						}>;
						readonly attachments: ReadonlyArray<{
							readonly __typename: 'Attachment';
							readonly id: string;
							readonly type: string;
							readonly url: string;
							readonly preview?: string | null | undefined;
							readonly description?: string | null | undefined;
							readonly blurhash?: string | null | undefined;
							readonly width?: number | null | undefined;
							readonly height?: number | null | undefined;
							readonly duration?: number | null | undefined;
						}>;
						readonly tags: ReadonlyArray<{
							readonly __typename: 'Tag';
							readonly name: string;
							readonly url: string;
						}>;
						readonly mentions: ReadonlyArray<{
							readonly __typename: 'Mention';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly url: string;
						}>;
						readonly quoteContext?:
							| {
									readonly __typename: 'QuoteContext';
									readonly quoteAllowed: boolean;
									readonly quoteType: import('./index.js').QuoteType;
									readonly withdrawn: boolean;
									readonly originalAuthor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly originalNote?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly communityNotes: ReadonlyArray<{
							readonly __typename: 'CommunityNote';
							readonly id: string;
							readonly content: string;
							readonly helpful: number;
							readonly notHelpful: number;
							readonly createdAt: string;
							readonly author: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
						}>;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly inReplyTo?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
		}[]
	>;
	getConversation(id: string): Promise<
		| {
				readonly __typename: 'Conversation';
				readonly id: string;
				readonly unread: boolean;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly accounts: ReadonlyArray<{
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				}>;
				readonly lastStatus?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly boostedObject?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly content: string;
										readonly visibility: import('./index.js').Visibility;
										readonly sensitive: boolean;
										readonly spoilerText?: string | null | undefined;
										readonly createdAt: string;
										readonly updatedAt: string;
										readonly repliesCount: number;
										readonly likesCount: number;
										readonly sharesCount: number;
										readonly estimatedCost: number;
										readonly moderationScore?: number | null | undefined;
										readonly quoteUrl?: string | null | undefined;
										readonly quoteable: boolean;
										readonly quotePermissions: import('./index.js').QuotePermission;
										readonly quoteCount: number;
										readonly contentMap: ReadonlyArray<{
											readonly __typename: 'ContentMap';
											readonly language: string;
											readonly content: string;
										}>;
										readonly attachments: ReadonlyArray<{
											readonly __typename: 'Attachment';
											readonly id: string;
											readonly type: string;
											readonly url: string;
											readonly preview?: string | null | undefined;
											readonly description?: string | null | undefined;
											readonly blurhash?: string | null | undefined;
											readonly width?: number | null | undefined;
											readonly height?: number | null | undefined;
											readonly duration?: number | null | undefined;
										}>;
										readonly tags: ReadonlyArray<{
											readonly __typename: 'Tag';
											readonly name: string;
											readonly url: string;
										}>;
										readonly mentions: ReadonlyArray<{
											readonly __typename: 'Mention';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly url: string;
										}>;
										readonly quoteContext?:
											| {
													readonly __typename: 'QuoteContext';
													readonly quoteAllowed: boolean;
													readonly quoteType: import('./index.js').QuoteType;
													readonly withdrawn: boolean;
													readonly originalAuthor: {
														readonly __typename: 'Actor';
														readonly id: string;
														readonly username: string;
														readonly domain?: string | null | undefined;
														readonly displayName?: string | null | undefined;
														readonly summary?: string | null | undefined;
														readonly avatar?: string | null | undefined;
														readonly header?: string | null | undefined;
														readonly followers: number;
														readonly following: number;
														readonly statusesCount: number;
														readonly bot: boolean;
														readonly locked: boolean;
														readonly updatedAt: string;
														readonly trustScore: number;
														readonly fields: ReadonlyArray<{
															readonly __typename: 'Field';
															readonly name: string;
															readonly value: string;
															readonly verifiedAt?: string | null | undefined;
														}>;
													};
													readonly originalNote?:
														| {
																readonly __typename: 'Object';
																readonly id: string;
																readonly type: import('./index.js').ObjectType;
														  }
														| null
														| undefined;
											  }
											| null
											| undefined;
										readonly communityNotes: ReadonlyArray<{
											readonly __typename: 'CommunityNote';
											readonly id: string;
											readonly content: string;
											readonly helpful: number;
											readonly notHelpful: number;
											readonly createdAt: string;
											readonly author: {
												readonly __typename: 'Actor';
												readonly id: string;
												readonly username: string;
												readonly domain?: string | null | undefined;
												readonly displayName?: string | null | undefined;
												readonly summary?: string | null | undefined;
												readonly avatar?: string | null | undefined;
												readonly header?: string | null | undefined;
												readonly followers: number;
												readonly following: number;
												readonly statusesCount: number;
												readonly bot: boolean;
												readonly locked: boolean;
												readonly updatedAt: string;
												readonly trustScore: number;
												readonly fields: ReadonlyArray<{
													readonly __typename: 'Field';
													readonly name: string;
													readonly value: string;
													readonly verifiedAt?: string | null | undefined;
												}>;
											};
										}>;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly inReplyTo?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
													readonly actor: {
														readonly __typename: 'Actor';
														readonly id: string;
														readonly username: string;
														readonly domain?: string | null | undefined;
														readonly displayName?: string | null | undefined;
														readonly summary?: string | null | undefined;
														readonly avatar?: string | null | undefined;
														readonly header?: string | null | undefined;
														readonly followers: number;
														readonly following: number;
														readonly statusesCount: number;
														readonly bot: boolean;
														readonly locked: boolean;
														readonly updatedAt: string;
														readonly trustScore: number;
														readonly fields: ReadonlyArray<{
															readonly __typename: 'Field';
															readonly name: string;
															readonly value: string;
															readonly verifiedAt?: string | null | undefined;
														}>;
													};
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
		  }
		| null
		| undefined
	>;
	markConversationAsRead(id: string): Promise<{
		readonly __typename: 'Conversation';
		readonly id: string;
		readonly unread: boolean;
		readonly updatedAt: string;
	}>;
	deleteConversation(id: string): Promise<boolean>;
	getLists(): Promise<
		readonly {
			readonly __typename: 'List';
			readonly id: string;
			readonly title: string;
			readonly repliesPolicy: import('./index.js').RepliesPolicy;
			readonly exclusive: boolean;
			readonly accountCount: number;
			readonly createdAt: string;
			readonly updatedAt: string;
		}[]
	>;
	getList(id: string): Promise<
		| {
				readonly __typename: 'List';
				readonly id: string;
				readonly title: string;
				readonly repliesPolicy: import('./index.js').RepliesPolicy;
				readonly exclusive: boolean;
				readonly accountCount: number;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly accounts: ReadonlyArray<{
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				}>;
		  }
		| null
		| undefined
	>;
	getListAccounts(id: string): Promise<
		readonly {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		}[]
	>;
	createList(input: CreateListMutationVariables['input']): Promise<{
		readonly __typename: 'List';
		readonly id: string;
		readonly title: string;
		readonly repliesPolicy: import('./index.js').RepliesPolicy;
		readonly exclusive: boolean;
		readonly accountCount: number;
		readonly createdAt: string;
		readonly updatedAt: string;
	}>;
	updateList(
		id: string,
		input: UpdateListMutationVariables['input']
	): Promise<{
		readonly __typename: 'List';
		readonly id: string;
		readonly title: string;
		readonly repliesPolicy: import('./index.js').RepliesPolicy;
		readonly exclusive: boolean;
		readonly accountCount: number;
		readonly createdAt: string;
		readonly updatedAt: string;
	}>;
	deleteList(id: string): Promise<boolean>;
	addAccountsToList(
		id: string,
		accountIds: string[]
	): Promise<{
		readonly __typename: 'List';
		readonly id: string;
		readonly accountCount: number;
		readonly accounts: ReadonlyArray<{
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		}>;
	}>;
	removeAccountsFromList(
		id: string,
		accountIds: string[]
	): Promise<{
		readonly __typename: 'List';
		readonly id: string;
		readonly accountCount: number;
		readonly accounts: ReadonlyArray<{
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		}>;
	}>;
	uploadMedia(input: UploadMediaInput): Promise<UploadMediaMutation['uploadMedia']>;
	createNote(input: CreateNoteMutationVariables['input']): Promise<{
		readonly __typename: 'CreateNotePayload';
		readonly object: {
			readonly __typename: 'Object';
			readonly id: string;
			readonly type: import('./index.js').ObjectType;
			readonly content: string;
			readonly visibility: import('./index.js').Visibility;
			readonly sensitive: boolean;
			readonly spoilerText?: string | null | undefined;
			readonly createdAt: string;
			readonly updatedAt: string;
			readonly repliesCount: number;
			readonly likesCount: number;
			readonly sharesCount: number;
			readonly estimatedCost: number;
			readonly moderationScore?: number | null | undefined;
			readonly quoteUrl?: string | null | undefined;
			readonly quoteable: boolean;
			readonly quotePermissions: import('./index.js').QuotePermission;
			readonly quoteCount: number;
			readonly boostedObject?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly content: string;
						readonly visibility: import('./index.js').Visibility;
						readonly sensitive: boolean;
						readonly spoilerText?: string | null | undefined;
						readonly createdAt: string;
						readonly updatedAt: string;
						readonly repliesCount: number;
						readonly likesCount: number;
						readonly sharesCount: number;
						readonly estimatedCost: number;
						readonly moderationScore?: number | null | undefined;
						readonly quoteUrl?: string | null | undefined;
						readonly quoteable: boolean;
						readonly quotePermissions: import('./index.js').QuotePermission;
						readonly quoteCount: number;
						readonly contentMap: ReadonlyArray<{
							readonly __typename: 'ContentMap';
							readonly language: string;
							readonly content: string;
						}>;
						readonly attachments: ReadonlyArray<{
							readonly __typename: 'Attachment';
							readonly id: string;
							readonly type: string;
							readonly url: string;
							readonly preview?: string | null | undefined;
							readonly description?: string | null | undefined;
							readonly blurhash?: string | null | undefined;
							readonly width?: number | null | undefined;
							readonly height?: number | null | undefined;
							readonly duration?: number | null | undefined;
						}>;
						readonly tags: ReadonlyArray<{
							readonly __typename: 'Tag';
							readonly name: string;
							readonly url: string;
						}>;
						readonly mentions: ReadonlyArray<{
							readonly __typename: 'Mention';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly url: string;
						}>;
						readonly quoteContext?:
							| {
									readonly __typename: 'QuoteContext';
									readonly quoteAllowed: boolean;
									readonly quoteType: import('./index.js').QuoteType;
									readonly withdrawn: boolean;
									readonly originalAuthor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly originalNote?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly communityNotes: ReadonlyArray<{
							readonly __typename: 'CommunityNote';
							readonly id: string;
							readonly content: string;
							readonly helpful: number;
							readonly notHelpful: number;
							readonly createdAt: string;
							readonly author: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
						}>;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly inReplyTo?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly contentMap: ReadonlyArray<{
				readonly __typename: 'ContentMap';
				readonly language: string;
				readonly content: string;
			}>;
			readonly attachments: ReadonlyArray<{
				readonly __typename: 'Attachment';
				readonly id: string;
				readonly type: string;
				readonly url: string;
				readonly preview?: string | null | undefined;
				readonly description?: string | null | undefined;
				readonly blurhash?: string | null | undefined;
				readonly width?: number | null | undefined;
				readonly height?: number | null | undefined;
				readonly duration?: number | null | undefined;
			}>;
			readonly tags: ReadonlyArray<{
				readonly __typename: 'Tag';
				readonly name: string;
				readonly url: string;
			}>;
			readonly mentions: ReadonlyArray<{
				readonly __typename: 'Mention';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly url: string;
			}>;
			readonly quoteContext?:
				| {
						readonly __typename: 'QuoteContext';
						readonly quoteAllowed: boolean;
						readonly quoteType: import('./index.js').QuoteType;
						readonly withdrawn: boolean;
						readonly originalAuthor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly originalNote?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly communityNotes: ReadonlyArray<{
				readonly __typename: 'CommunityNote';
				readonly id: string;
				readonly content: string;
				readonly helpful: number;
				readonly notHelpful: number;
				readonly createdAt: string;
				readonly author: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
			}>;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly inReplyTo?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
				  }
				| null
				| undefined;
		};
		readonly activity: {
			readonly __typename: 'Activity';
			readonly id: string;
			readonly type: import('./index.js').ActivityType;
			readonly published: string;
			readonly cost: number;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly object?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
				  }
				| null
				| undefined;
			readonly target?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
				  }
				| null
				| undefined;
		};
		readonly cost: {
			readonly __typename: 'CostUpdate';
			readonly operationCost: number;
			readonly dailyTotal: number;
			readonly monthlyProjection: number;
		};
	}>;
	createQuoteNote(input: CreateQuoteNoteMutationVariables['input']): Promise<{
		readonly __typename: 'CreateNotePayload';
		readonly object: {
			readonly __typename: 'Object';
			readonly id: string;
			readonly type: import('./index.js').ObjectType;
			readonly content: string;
			readonly visibility: import('./index.js').Visibility;
			readonly sensitive: boolean;
			readonly spoilerText?: string | null | undefined;
			readonly createdAt: string;
			readonly updatedAt: string;
			readonly repliesCount: number;
			readonly likesCount: number;
			readonly sharesCount: number;
			readonly estimatedCost: number;
			readonly moderationScore?: number | null | undefined;
			readonly quoteUrl?: string | null | undefined;
			readonly quoteable: boolean;
			readonly quotePermissions: import('./index.js').QuotePermission;
			readonly quoteCount: number;
			readonly boostedObject?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly content: string;
						readonly visibility: import('./index.js').Visibility;
						readonly sensitive: boolean;
						readonly spoilerText?: string | null | undefined;
						readonly createdAt: string;
						readonly updatedAt: string;
						readonly repliesCount: number;
						readonly likesCount: number;
						readonly sharesCount: number;
						readonly estimatedCost: number;
						readonly moderationScore?: number | null | undefined;
						readonly quoteUrl?: string | null | undefined;
						readonly quoteable: boolean;
						readonly quotePermissions: import('./index.js').QuotePermission;
						readonly quoteCount: number;
						readonly contentMap: ReadonlyArray<{
							readonly __typename: 'ContentMap';
							readonly language: string;
							readonly content: string;
						}>;
						readonly attachments: ReadonlyArray<{
							readonly __typename: 'Attachment';
							readonly id: string;
							readonly type: string;
							readonly url: string;
							readonly preview?: string | null | undefined;
							readonly description?: string | null | undefined;
							readonly blurhash?: string | null | undefined;
							readonly width?: number | null | undefined;
							readonly height?: number | null | undefined;
							readonly duration?: number | null | undefined;
						}>;
						readonly tags: ReadonlyArray<{
							readonly __typename: 'Tag';
							readonly name: string;
							readonly url: string;
						}>;
						readonly mentions: ReadonlyArray<{
							readonly __typename: 'Mention';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly url: string;
						}>;
						readonly quoteContext?:
							| {
									readonly __typename: 'QuoteContext';
									readonly quoteAllowed: boolean;
									readonly quoteType: import('./index.js').QuoteType;
									readonly withdrawn: boolean;
									readonly originalAuthor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly originalNote?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly communityNotes: ReadonlyArray<{
							readonly __typename: 'CommunityNote';
							readonly id: string;
							readonly content: string;
							readonly helpful: number;
							readonly notHelpful: number;
							readonly createdAt: string;
							readonly author: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
						}>;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly inReplyTo?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly contentMap: ReadonlyArray<{
				readonly __typename: 'ContentMap';
				readonly language: string;
				readonly content: string;
			}>;
			readonly attachments: ReadonlyArray<{
				readonly __typename: 'Attachment';
				readonly id: string;
				readonly type: string;
				readonly url: string;
				readonly preview?: string | null | undefined;
				readonly description?: string | null | undefined;
				readonly blurhash?: string | null | undefined;
				readonly width?: number | null | undefined;
				readonly height?: number | null | undefined;
				readonly duration?: number | null | undefined;
			}>;
			readonly tags: ReadonlyArray<{
				readonly __typename: 'Tag';
				readonly name: string;
				readonly url: string;
			}>;
			readonly mentions: ReadonlyArray<{
				readonly __typename: 'Mention';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly url: string;
			}>;
			readonly quoteContext?:
				| {
						readonly __typename: 'QuoteContext';
						readonly quoteAllowed: boolean;
						readonly quoteType: import('./index.js').QuoteType;
						readonly withdrawn: boolean;
						readonly originalAuthor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly originalNote?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly communityNotes: ReadonlyArray<{
				readonly __typename: 'CommunityNote';
				readonly id: string;
				readonly content: string;
				readonly helpful: number;
				readonly notHelpful: number;
				readonly createdAt: string;
				readonly author: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
			}>;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly inReplyTo?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
				  }
				| null
				| undefined;
		};
		readonly activity: {
			readonly __typename: 'Activity';
			readonly id: string;
			readonly type: import('./index.js').ActivityType;
			readonly published: string;
			readonly cost: number;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly object?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
				  }
				| null
				| undefined;
			readonly target?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
				  }
				| null
				| undefined;
		};
		readonly cost: {
			readonly __typename: 'CostUpdate';
			readonly operationCost: number;
			readonly dailyTotal: number;
			readonly monthlyProjection: number;
		};
	}>;
	getObjectWithQuotes(
		id: string,
		first?: number,
		after?: string
	): Promise<
		| {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly quotes: {
					readonly __typename: 'QuoteConnection';
					readonly totalCount: number;
					readonly edges: ReadonlyArray<{
						readonly __typename: 'QuoteEdge';
						readonly cursor: string;
						readonly node: {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly boostedObject?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly content: string;
										readonly visibility: import('./index.js').Visibility;
										readonly sensitive: boolean;
										readonly spoilerText?: string | null | undefined;
										readonly createdAt: string;
										readonly updatedAt: string;
										readonly repliesCount: number;
										readonly likesCount: number;
										readonly sharesCount: number;
										readonly estimatedCost: number;
										readonly moderationScore?: number | null | undefined;
										readonly quoteUrl?: string | null | undefined;
										readonly quoteable: boolean;
										readonly quotePermissions: import('./index.js').QuotePermission;
										readonly quoteCount: number;
										readonly contentMap: ReadonlyArray<{
											readonly __typename: 'ContentMap';
											readonly language: string;
											readonly content: string;
										}>;
										readonly attachments: ReadonlyArray<{
											readonly __typename: 'Attachment';
											readonly id: string;
											readonly type: string;
											readonly url: string;
											readonly preview?: string | null | undefined;
											readonly description?: string | null | undefined;
											readonly blurhash?: string | null | undefined;
											readonly width?: number | null | undefined;
											readonly height?: number | null | undefined;
											readonly duration?: number | null | undefined;
										}>;
										readonly tags: ReadonlyArray<{
											readonly __typename: 'Tag';
											readonly name: string;
											readonly url: string;
										}>;
										readonly mentions: ReadonlyArray<{
											readonly __typename: 'Mention';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly url: string;
										}>;
										readonly quoteContext?:
											| {
													readonly __typename: 'QuoteContext';
													readonly quoteAllowed: boolean;
													readonly quoteType: import('./index.js').QuoteType;
													readonly withdrawn: boolean;
													readonly originalAuthor: {
														readonly __typename: 'Actor';
														readonly id: string;
														readonly username: string;
														readonly domain?: string | null | undefined;
														readonly displayName?: string | null | undefined;
														readonly summary?: string | null | undefined;
														readonly avatar?: string | null | undefined;
														readonly header?: string | null | undefined;
														readonly followers: number;
														readonly following: number;
														readonly statusesCount: number;
														readonly bot: boolean;
														readonly locked: boolean;
														readonly updatedAt: string;
														readonly trustScore: number;
														readonly fields: ReadonlyArray<{
															readonly __typename: 'Field';
															readonly name: string;
															readonly value: string;
															readonly verifiedAt?: string | null | undefined;
														}>;
													};
													readonly originalNote?:
														| {
																readonly __typename: 'Object';
																readonly id: string;
																readonly type: import('./index.js').ObjectType;
														  }
														| null
														| undefined;
											  }
											| null
											| undefined;
										readonly communityNotes: ReadonlyArray<{
											readonly __typename: 'CommunityNote';
											readonly id: string;
											readonly content: string;
											readonly helpful: number;
											readonly notHelpful: number;
											readonly createdAt: string;
											readonly author: {
												readonly __typename: 'Actor';
												readonly id: string;
												readonly username: string;
												readonly domain?: string | null | undefined;
												readonly displayName?: string | null | undefined;
												readonly summary?: string | null | undefined;
												readonly avatar?: string | null | undefined;
												readonly header?: string | null | undefined;
												readonly followers: number;
												readonly following: number;
												readonly statusesCount: number;
												readonly bot: boolean;
												readonly locked: boolean;
												readonly updatedAt: string;
												readonly trustScore: number;
												readonly fields: ReadonlyArray<{
													readonly __typename: 'Field';
													readonly name: string;
													readonly value: string;
													readonly verifiedAt?: string | null | undefined;
												}>;
											};
										}>;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly inReplyTo?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
													readonly actor: {
														readonly __typename: 'Actor';
														readonly id: string;
														readonly username: string;
														readonly domain?: string | null | undefined;
														readonly displayName?: string | null | undefined;
														readonly summary?: string | null | undefined;
														readonly avatar?: string | null | undefined;
														readonly header?: string | null | undefined;
														readonly followers: number;
														readonly following: number;
														readonly statusesCount: number;
														readonly bot: boolean;
														readonly locked: boolean;
														readonly updatedAt: string;
														readonly trustScore: number;
														readonly fields: ReadonlyArray<{
															readonly __typename: 'Field';
															readonly name: string;
															readonly value: string;
															readonly verifiedAt?: string | null | undefined;
														}>;
													};
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
						};
					}>;
					readonly pageInfo: {
						readonly __typename: 'PageInfo';
						readonly hasNextPage: boolean;
						readonly hasPreviousPage: boolean;
						readonly startCursor?: string | null | undefined;
						readonly endCursor?: string | null | undefined;
					};
				};
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
		  }
		| null
		| undefined
	>;
	withdrawFromQuotes(noteId: string): Promise<{
		readonly __typename: 'WithdrawQuotePayload';
		readonly success: boolean;
		readonly withdrawnCount: number;
		readonly note: {
			readonly __typename: 'Object';
			readonly id: string;
			readonly type: import('./index.js').ObjectType;
			readonly content: string;
			readonly visibility: import('./index.js').Visibility;
			readonly sensitive: boolean;
			readonly spoilerText?: string | null | undefined;
			readonly createdAt: string;
			readonly updatedAt: string;
			readonly repliesCount: number;
			readonly likesCount: number;
			readonly sharesCount: number;
			readonly estimatedCost: number;
			readonly moderationScore?: number | null | undefined;
			readonly quoteUrl?: string | null | undefined;
			readonly quoteable: boolean;
			readonly quotePermissions: import('./index.js').QuotePermission;
			readonly quoteCount: number;
			readonly boostedObject?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly content: string;
						readonly visibility: import('./index.js').Visibility;
						readonly sensitive: boolean;
						readonly spoilerText?: string | null | undefined;
						readonly createdAt: string;
						readonly updatedAt: string;
						readonly repliesCount: number;
						readonly likesCount: number;
						readonly sharesCount: number;
						readonly estimatedCost: number;
						readonly moderationScore?: number | null | undefined;
						readonly quoteUrl?: string | null | undefined;
						readonly quoteable: boolean;
						readonly quotePermissions: import('./index.js').QuotePermission;
						readonly quoteCount: number;
						readonly contentMap: ReadonlyArray<{
							readonly __typename: 'ContentMap';
							readonly language: string;
							readonly content: string;
						}>;
						readonly attachments: ReadonlyArray<{
							readonly __typename: 'Attachment';
							readonly id: string;
							readonly type: string;
							readonly url: string;
							readonly preview?: string | null | undefined;
							readonly description?: string | null | undefined;
							readonly blurhash?: string | null | undefined;
							readonly width?: number | null | undefined;
							readonly height?: number | null | undefined;
							readonly duration?: number | null | undefined;
						}>;
						readonly tags: ReadonlyArray<{
							readonly __typename: 'Tag';
							readonly name: string;
							readonly url: string;
						}>;
						readonly mentions: ReadonlyArray<{
							readonly __typename: 'Mention';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly url: string;
						}>;
						readonly quoteContext?:
							| {
									readonly __typename: 'QuoteContext';
									readonly quoteAllowed: boolean;
									readonly quoteType: import('./index.js').QuoteType;
									readonly withdrawn: boolean;
									readonly originalAuthor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly originalNote?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly communityNotes: ReadonlyArray<{
							readonly __typename: 'CommunityNote';
							readonly id: string;
							readonly content: string;
							readonly helpful: number;
							readonly notHelpful: number;
							readonly createdAt: string;
							readonly author: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
						}>;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly inReplyTo?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly contentMap: ReadonlyArray<{
				readonly __typename: 'ContentMap';
				readonly language: string;
				readonly content: string;
			}>;
			readonly attachments: ReadonlyArray<{
				readonly __typename: 'Attachment';
				readonly id: string;
				readonly type: string;
				readonly url: string;
				readonly preview?: string | null | undefined;
				readonly description?: string | null | undefined;
				readonly blurhash?: string | null | undefined;
				readonly width?: number | null | undefined;
				readonly height?: number | null | undefined;
				readonly duration?: number | null | undefined;
			}>;
			readonly tags: ReadonlyArray<{
				readonly __typename: 'Tag';
				readonly name: string;
				readonly url: string;
			}>;
			readonly mentions: ReadonlyArray<{
				readonly __typename: 'Mention';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly url: string;
			}>;
			readonly quoteContext?:
				| {
						readonly __typename: 'QuoteContext';
						readonly quoteAllowed: boolean;
						readonly quoteType: import('./index.js').QuoteType;
						readonly withdrawn: boolean;
						readonly originalAuthor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly originalNote?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly communityNotes: ReadonlyArray<{
				readonly __typename: 'CommunityNote';
				readonly id: string;
				readonly content: string;
				readonly helpful: number;
				readonly notHelpful: number;
				readonly createdAt: string;
				readonly author: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
			}>;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly inReplyTo?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
				  }
				| null
				| undefined;
		};
	}>;
	updateQuotePermissions(
		noteId: string,
		quoteable: boolean,
		permission: 'EVERYONE' | 'FOLLOWERS' | 'NONE'
	): Promise<{
		readonly __typename: 'UpdateQuotePermissionsPayload';
		readonly success: boolean;
		readonly affectedQuotes: number;
		readonly note: {
			readonly __typename: 'Object';
			readonly id: string;
			readonly type: import('./index.js').ObjectType;
			readonly content: string;
			readonly visibility: import('./index.js').Visibility;
			readonly sensitive: boolean;
			readonly spoilerText?: string | null | undefined;
			readonly createdAt: string;
			readonly updatedAt: string;
			readonly repliesCount: number;
			readonly likesCount: number;
			readonly sharesCount: number;
			readonly estimatedCost: number;
			readonly moderationScore?: number | null | undefined;
			readonly quoteUrl?: string | null | undefined;
			readonly quoteable: boolean;
			readonly quotePermissions: import('./index.js').QuotePermission;
			readonly quoteCount: number;
			readonly boostedObject?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly content: string;
						readonly visibility: import('./index.js').Visibility;
						readonly sensitive: boolean;
						readonly spoilerText?: string | null | undefined;
						readonly createdAt: string;
						readonly updatedAt: string;
						readonly repliesCount: number;
						readonly likesCount: number;
						readonly sharesCount: number;
						readonly estimatedCost: number;
						readonly moderationScore?: number | null | undefined;
						readonly quoteUrl?: string | null | undefined;
						readonly quoteable: boolean;
						readonly quotePermissions: import('./index.js').QuotePermission;
						readonly quoteCount: number;
						readonly contentMap: ReadonlyArray<{
							readonly __typename: 'ContentMap';
							readonly language: string;
							readonly content: string;
						}>;
						readonly attachments: ReadonlyArray<{
							readonly __typename: 'Attachment';
							readonly id: string;
							readonly type: string;
							readonly url: string;
							readonly preview?: string | null | undefined;
							readonly description?: string | null | undefined;
							readonly blurhash?: string | null | undefined;
							readonly width?: number | null | undefined;
							readonly height?: number | null | undefined;
							readonly duration?: number | null | undefined;
						}>;
						readonly tags: ReadonlyArray<{
							readonly __typename: 'Tag';
							readonly name: string;
							readonly url: string;
						}>;
						readonly mentions: ReadonlyArray<{
							readonly __typename: 'Mention';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly url: string;
						}>;
						readonly quoteContext?:
							| {
									readonly __typename: 'QuoteContext';
									readonly quoteAllowed: boolean;
									readonly quoteType: import('./index.js').QuoteType;
									readonly withdrawn: boolean;
									readonly originalAuthor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
									readonly originalNote?:
										| {
												readonly __typename: 'Object';
												readonly id: string;
												readonly type: import('./index.js').ObjectType;
										  }
										| null
										| undefined;
							  }
							| null
							| undefined;
						readonly communityNotes: ReadonlyArray<{
							readonly __typename: 'CommunityNote';
							readonly id: string;
							readonly content: string;
							readonly helpful: number;
							readonly notHelpful: number;
							readonly createdAt: string;
							readonly author: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
						}>;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly inReplyTo?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
									readonly actor: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly contentMap: ReadonlyArray<{
				readonly __typename: 'ContentMap';
				readonly language: string;
				readonly content: string;
			}>;
			readonly attachments: ReadonlyArray<{
				readonly __typename: 'Attachment';
				readonly id: string;
				readonly type: string;
				readonly url: string;
				readonly preview?: string | null | undefined;
				readonly description?: string | null | undefined;
				readonly blurhash?: string | null | undefined;
				readonly width?: number | null | undefined;
				readonly height?: number | null | undefined;
				readonly duration?: number | null | undefined;
			}>;
			readonly tags: ReadonlyArray<{
				readonly __typename: 'Tag';
				readonly name: string;
				readonly url: string;
			}>;
			readonly mentions: ReadonlyArray<{
				readonly __typename: 'Mention';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly url: string;
			}>;
			readonly quoteContext?:
				| {
						readonly __typename: 'QuoteContext';
						readonly quoteAllowed: boolean;
						readonly quoteType: import('./index.js').QuoteType;
						readonly withdrawn: boolean;
						readonly originalAuthor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
						readonly originalNote?:
							| {
									readonly __typename: 'Object';
									readonly id: string;
									readonly type: import('./index.js').ObjectType;
							  }
							| null
							| undefined;
				  }
				| null
				| undefined;
			readonly communityNotes: ReadonlyArray<{
				readonly __typename: 'CommunityNote';
				readonly id: string;
				readonly content: string;
				readonly helpful: number;
				readonly notHelpful: number;
				readonly createdAt: string;
				readonly author: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
			}>;
			readonly actor: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly inReplyTo?:
				| {
						readonly __typename: 'Object';
						readonly id: string;
						readonly type: import('./index.js').ObjectType;
						readonly actor: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
				  }
				| null
				| undefined;
		};
	}>;
	deleteObject(id: string): Promise<boolean>;
	likeObject(id: string): Promise<{
		readonly __typename: 'Activity';
		readonly id: string;
		readonly type: import('./index.js').ActivityType;
		readonly published: string;
		readonly cost: number;
		readonly actor: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
		readonly object?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
			  }
			| null
			| undefined;
		readonly target?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
			  }
			| null
			| undefined;
	}>;
	unlikeObject(id: string): Promise<boolean>;
	shareObject(id: string): Promise<{
		readonly __typename: 'Object';
		readonly id: string;
		readonly type: import('./index.js').ObjectType;
		readonly content: string;
		readonly visibility: import('./index.js').Visibility;
		readonly sensitive: boolean;
		readonly spoilerText?: string | null | undefined;
		readonly createdAt: string;
		readonly updatedAt: string;
		readonly repliesCount: number;
		readonly likesCount: number;
		readonly sharesCount: number;
		readonly estimatedCost: number;
		readonly moderationScore?: number | null | undefined;
		readonly quoteUrl?: string | null | undefined;
		readonly quoteable: boolean;
		readonly quotePermissions: import('./index.js').QuotePermission;
		readonly quoteCount: number;
		readonly boostedObject?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly content: string;
					readonly visibility: import('./index.js').Visibility;
					readonly sensitive: boolean;
					readonly spoilerText?: string | null | undefined;
					readonly createdAt: string;
					readonly updatedAt: string;
					readonly repliesCount: number;
					readonly likesCount: number;
					readonly sharesCount: number;
					readonly estimatedCost: number;
					readonly moderationScore?: number | null | undefined;
					readonly quoteUrl?: string | null | undefined;
					readonly quoteable: boolean;
					readonly quotePermissions: import('./index.js').QuotePermission;
					readonly quoteCount: number;
					readonly contentMap: ReadonlyArray<{
						readonly __typename: 'ContentMap';
						readonly language: string;
						readonly content: string;
					}>;
					readonly attachments: ReadonlyArray<{
						readonly __typename: 'Attachment';
						readonly id: string;
						readonly type: string;
						readonly url: string;
						readonly preview?: string | null | undefined;
						readonly description?: string | null | undefined;
						readonly blurhash?: string | null | undefined;
						readonly width?: number | null | undefined;
						readonly height?: number | null | undefined;
						readonly duration?: number | null | undefined;
					}>;
					readonly tags: ReadonlyArray<{
						readonly __typename: 'Tag';
						readonly name: string;
						readonly url: string;
					}>;
					readonly mentions: ReadonlyArray<{
						readonly __typename: 'Mention';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly url: string;
					}>;
					readonly quoteContext?:
						| {
								readonly __typename: 'QuoteContext';
								readonly quoteAllowed: boolean;
								readonly quoteType: import('./index.js').QuoteType;
								readonly withdrawn: boolean;
								readonly originalAuthor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
								readonly originalNote?:
									| {
											readonly __typename: 'Object';
											readonly id: string;
											readonly type: import('./index.js').ObjectType;
									  }
									| null
									| undefined;
						  }
						| null
						| undefined;
					readonly communityNotes: ReadonlyArray<{
						readonly __typename: 'CommunityNote';
						readonly id: string;
						readonly content: string;
						readonly helpful: number;
						readonly notHelpful: number;
						readonly createdAt: string;
						readonly author: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
					}>;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly inReplyTo?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
								readonly actor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
						  }
						| null
						| undefined;
			  }
			| null
			| undefined;
		readonly contentMap: ReadonlyArray<{
			readonly __typename: 'ContentMap';
			readonly language: string;
			readonly content: string;
		}>;
		readonly attachments: ReadonlyArray<{
			readonly __typename: 'Attachment';
			readonly id: string;
			readonly type: string;
			readonly url: string;
			readonly preview?: string | null | undefined;
			readonly description?: string | null | undefined;
			readonly blurhash?: string | null | undefined;
			readonly width?: number | null | undefined;
			readonly height?: number | null | undefined;
			readonly duration?: number | null | undefined;
		}>;
		readonly tags: ReadonlyArray<{
			readonly __typename: 'Tag';
			readonly name: string;
			readonly url: string;
		}>;
		readonly mentions: ReadonlyArray<{
			readonly __typename: 'Mention';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly url: string;
		}>;
		readonly quoteContext?:
			| {
					readonly __typename: 'QuoteContext';
					readonly quoteAllowed: boolean;
					readonly quoteType: import('./index.js').QuoteType;
					readonly withdrawn: boolean;
					readonly originalAuthor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly originalNote?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
						  }
						| null
						| undefined;
			  }
			| null
			| undefined;
		readonly communityNotes: ReadonlyArray<{
			readonly __typename: 'CommunityNote';
			readonly id: string;
			readonly content: string;
			readonly helpful: number;
			readonly notHelpful: number;
			readonly createdAt: string;
			readonly author: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
		}>;
		readonly actor: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
		readonly inReplyTo?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
			  }
			| null
			| undefined;
	}>;
	unshareObject(id: string): Promise<boolean>;
	bookmarkObject(id: string): Promise<{
		readonly __typename: 'Object';
		readonly id: string;
		readonly type: import('./index.js').ObjectType;
		readonly content: string;
		readonly visibility: import('./index.js').Visibility;
		readonly sensitive: boolean;
		readonly spoilerText?: string | null | undefined;
		readonly createdAt: string;
		readonly updatedAt: string;
		readonly repliesCount: number;
		readonly likesCount: number;
		readonly sharesCount: number;
		readonly estimatedCost: number;
		readonly moderationScore?: number | null | undefined;
		readonly quoteUrl?: string | null | undefined;
		readonly quoteable: boolean;
		readonly quotePermissions: import('./index.js').QuotePermission;
		readonly quoteCount: number;
		readonly boostedObject?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly content: string;
					readonly visibility: import('./index.js').Visibility;
					readonly sensitive: boolean;
					readonly spoilerText?: string | null | undefined;
					readonly createdAt: string;
					readonly updatedAt: string;
					readonly repliesCount: number;
					readonly likesCount: number;
					readonly sharesCount: number;
					readonly estimatedCost: number;
					readonly moderationScore?: number | null | undefined;
					readonly quoteUrl?: string | null | undefined;
					readonly quoteable: boolean;
					readonly quotePermissions: import('./index.js').QuotePermission;
					readonly quoteCount: number;
					readonly contentMap: ReadonlyArray<{
						readonly __typename: 'ContentMap';
						readonly language: string;
						readonly content: string;
					}>;
					readonly attachments: ReadonlyArray<{
						readonly __typename: 'Attachment';
						readonly id: string;
						readonly type: string;
						readonly url: string;
						readonly preview?: string | null | undefined;
						readonly description?: string | null | undefined;
						readonly blurhash?: string | null | undefined;
						readonly width?: number | null | undefined;
						readonly height?: number | null | undefined;
						readonly duration?: number | null | undefined;
					}>;
					readonly tags: ReadonlyArray<{
						readonly __typename: 'Tag';
						readonly name: string;
						readonly url: string;
					}>;
					readonly mentions: ReadonlyArray<{
						readonly __typename: 'Mention';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly url: string;
					}>;
					readonly quoteContext?:
						| {
								readonly __typename: 'QuoteContext';
								readonly quoteAllowed: boolean;
								readonly quoteType: import('./index.js').QuoteType;
								readonly withdrawn: boolean;
								readonly originalAuthor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
								readonly originalNote?:
									| {
											readonly __typename: 'Object';
											readonly id: string;
											readonly type: import('./index.js').ObjectType;
									  }
									| null
									| undefined;
						  }
						| null
						| undefined;
					readonly communityNotes: ReadonlyArray<{
						readonly __typename: 'CommunityNote';
						readonly id: string;
						readonly content: string;
						readonly helpful: number;
						readonly notHelpful: number;
						readonly createdAt: string;
						readonly author: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
					}>;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly inReplyTo?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
								readonly actor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
						  }
						| null
						| undefined;
			  }
			| null
			| undefined;
		readonly contentMap: ReadonlyArray<{
			readonly __typename: 'ContentMap';
			readonly language: string;
			readonly content: string;
		}>;
		readonly attachments: ReadonlyArray<{
			readonly __typename: 'Attachment';
			readonly id: string;
			readonly type: string;
			readonly url: string;
			readonly preview?: string | null | undefined;
			readonly description?: string | null | undefined;
			readonly blurhash?: string | null | undefined;
			readonly width?: number | null | undefined;
			readonly height?: number | null | undefined;
			readonly duration?: number | null | undefined;
		}>;
		readonly tags: ReadonlyArray<{
			readonly __typename: 'Tag';
			readonly name: string;
			readonly url: string;
		}>;
		readonly mentions: ReadonlyArray<{
			readonly __typename: 'Mention';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly url: string;
		}>;
		readonly quoteContext?:
			| {
					readonly __typename: 'QuoteContext';
					readonly quoteAllowed: boolean;
					readonly quoteType: import('./index.js').QuoteType;
					readonly withdrawn: boolean;
					readonly originalAuthor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly originalNote?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
						  }
						| null
						| undefined;
			  }
			| null
			| undefined;
		readonly communityNotes: ReadonlyArray<{
			readonly __typename: 'CommunityNote';
			readonly id: string;
			readonly content: string;
			readonly helpful: number;
			readonly notHelpful: number;
			readonly createdAt: string;
			readonly author: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
		}>;
		readonly actor: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
		readonly inReplyTo?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
			  }
			| null
			| undefined;
	}>;
	unbookmarkObject(id: string): Promise<boolean>;
	pinObject(id: string): Promise<{
		readonly __typename: 'Object';
		readonly id: string;
		readonly type: import('./index.js').ObjectType;
		readonly content: string;
		readonly visibility: import('./index.js').Visibility;
		readonly sensitive: boolean;
		readonly spoilerText?: string | null | undefined;
		readonly createdAt: string;
		readonly updatedAt: string;
		readonly repliesCount: number;
		readonly likesCount: number;
		readonly sharesCount: number;
		readonly estimatedCost: number;
		readonly moderationScore?: number | null | undefined;
		readonly quoteUrl?: string | null | undefined;
		readonly quoteable: boolean;
		readonly quotePermissions: import('./index.js').QuotePermission;
		readonly quoteCount: number;
		readonly boostedObject?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly content: string;
					readonly visibility: import('./index.js').Visibility;
					readonly sensitive: boolean;
					readonly spoilerText?: string | null | undefined;
					readonly createdAt: string;
					readonly updatedAt: string;
					readonly repliesCount: number;
					readonly likesCount: number;
					readonly sharesCount: number;
					readonly estimatedCost: number;
					readonly moderationScore?: number | null | undefined;
					readonly quoteUrl?: string | null | undefined;
					readonly quoteable: boolean;
					readonly quotePermissions: import('./index.js').QuotePermission;
					readonly quoteCount: number;
					readonly contentMap: ReadonlyArray<{
						readonly __typename: 'ContentMap';
						readonly language: string;
						readonly content: string;
					}>;
					readonly attachments: ReadonlyArray<{
						readonly __typename: 'Attachment';
						readonly id: string;
						readonly type: string;
						readonly url: string;
						readonly preview?: string | null | undefined;
						readonly description?: string | null | undefined;
						readonly blurhash?: string | null | undefined;
						readonly width?: number | null | undefined;
						readonly height?: number | null | undefined;
						readonly duration?: number | null | undefined;
					}>;
					readonly tags: ReadonlyArray<{
						readonly __typename: 'Tag';
						readonly name: string;
						readonly url: string;
					}>;
					readonly mentions: ReadonlyArray<{
						readonly __typename: 'Mention';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly url: string;
					}>;
					readonly quoteContext?:
						| {
								readonly __typename: 'QuoteContext';
								readonly quoteAllowed: boolean;
								readonly quoteType: import('./index.js').QuoteType;
								readonly withdrawn: boolean;
								readonly originalAuthor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
								readonly originalNote?:
									| {
											readonly __typename: 'Object';
											readonly id: string;
											readonly type: import('./index.js').ObjectType;
									  }
									| null
									| undefined;
						  }
						| null
						| undefined;
					readonly communityNotes: ReadonlyArray<{
						readonly __typename: 'CommunityNote';
						readonly id: string;
						readonly content: string;
						readonly helpful: number;
						readonly notHelpful: number;
						readonly createdAt: string;
						readonly author: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
					}>;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly inReplyTo?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
								readonly actor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
						  }
						| null
						| undefined;
			  }
			| null
			| undefined;
		readonly contentMap: ReadonlyArray<{
			readonly __typename: 'ContentMap';
			readonly language: string;
			readonly content: string;
		}>;
		readonly attachments: ReadonlyArray<{
			readonly __typename: 'Attachment';
			readonly id: string;
			readonly type: string;
			readonly url: string;
			readonly preview?: string | null | undefined;
			readonly description?: string | null | undefined;
			readonly blurhash?: string | null | undefined;
			readonly width?: number | null | undefined;
			readonly height?: number | null | undefined;
			readonly duration?: number | null | undefined;
		}>;
		readonly tags: ReadonlyArray<{
			readonly __typename: 'Tag';
			readonly name: string;
			readonly url: string;
		}>;
		readonly mentions: ReadonlyArray<{
			readonly __typename: 'Mention';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly url: string;
		}>;
		readonly quoteContext?:
			| {
					readonly __typename: 'QuoteContext';
					readonly quoteAllowed: boolean;
					readonly quoteType: import('./index.js').QuoteType;
					readonly withdrawn: boolean;
					readonly originalAuthor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly originalNote?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
						  }
						| null
						| undefined;
			  }
			| null
			| undefined;
		readonly communityNotes: ReadonlyArray<{
			readonly __typename: 'CommunityNote';
			readonly id: string;
			readonly content: string;
			readonly helpful: number;
			readonly notHelpful: number;
			readonly createdAt: string;
			readonly author: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
		}>;
		readonly actor: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
		readonly inReplyTo?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
			  }
			| null
			| undefined;
	}>;
	unpinObject(id: string): Promise<boolean>;
	getRelationship(id: string): Promise<RelationshipQuery['relationship']>;
	getRelationships(ids: string[]): Promise<
		readonly {
			readonly __typename: 'Relationship';
			readonly id: string;
			readonly following: boolean;
			readonly followedBy: boolean;
			readonly blocking: boolean;
			readonly blockedBy: boolean;
			readonly muting: boolean;
			readonly mutingNotifications: boolean;
			readonly requested: boolean;
			readonly domainBlocking: boolean;
			readonly showingReblogs: boolean;
			readonly notifying: boolean;
			readonly languages?: ReadonlyArray<string> | null | undefined;
			readonly note?: string | null | undefined;
		}[]
	>;
	followActor(id: string): Promise<{
		readonly __typename: 'Activity';
		readonly id: string;
		readonly type: import('./index.js').ActivityType;
		readonly published: string;
		readonly cost: number;
		readonly actor: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
		readonly object?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
			  }
			| null
			| undefined;
		readonly target?:
			| {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
			  }
			| null
			| undefined;
	}>;
	unfollowActor(id: string): Promise<boolean>;
	blockActor(id: string): Promise<{
		readonly __typename: 'Relationship';
		readonly id: string;
		readonly following: boolean;
		readonly followedBy: boolean;
		readonly blocking: boolean;
		readonly blockedBy: boolean;
		readonly muting: boolean;
		readonly mutingNotifications: boolean;
		readonly requested: boolean;
		readonly domainBlocking: boolean;
		readonly showingReblogs: boolean;
		readonly notifying: boolean;
		readonly languages?: ReadonlyArray<string> | null | undefined;
		readonly note?: string | null | undefined;
	}>;
	unblockActor(id: string): Promise<boolean>;
	muteActor(
		id: string,
		notifications?: boolean
	): Promise<{
		readonly __typename: 'Relationship';
		readonly id: string;
		readonly following: boolean;
		readonly followedBy: boolean;
		readonly blocking: boolean;
		readonly blockedBy: boolean;
		readonly muting: boolean;
		readonly mutingNotifications: boolean;
		readonly requested: boolean;
		readonly domainBlocking: boolean;
		readonly showingReblogs: boolean;
		readonly notifying: boolean;
		readonly languages?: ReadonlyArray<string> | null | undefined;
		readonly note?: string | null | undefined;
	}>;
	unmuteActor(id: string): Promise<boolean>;
	updateRelationship(
		id: string,
		input: UpdateRelationshipMutationVariables['input']
	): Promise<{
		readonly __typename: 'Relationship';
		readonly id: string;
		readonly following: boolean;
		readonly followedBy: boolean;
		readonly blocking: boolean;
		readonly blockedBy: boolean;
		readonly muting: boolean;
		readonly mutingNotifications: boolean;
		readonly requested: boolean;
		readonly domainBlocking: boolean;
		readonly showingReblogs: boolean;
		readonly notifying: boolean;
		readonly languages?: ReadonlyArray<string> | null | undefined;
		readonly note?: string | null | undefined;
	}>;
	getFollowers(
		username: string,
		limit?: number,
		cursor?: string
	): Promise<{
		readonly __typename: 'ActorListPage';
		readonly nextCursor?: string | null | undefined;
		readonly totalCount: number;
		readonly actors: ReadonlyArray<{
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		}>;
	}>;
	getFollowing(
		username: string,
		limit?: number,
		cursor?: string
	): Promise<{
		readonly __typename: 'ActorListPage';
		readonly nextCursor?: string | null | undefined;
		readonly totalCount: number;
		readonly actors: ReadonlyArray<{
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		}>;
	}>;
	updateProfile(input: {
		displayName?: string;
		bio?: string;
		avatar?: string;
		header?: string;
		locked?: boolean;
		bot?: boolean;
		discoverable?: boolean;
		noIndex?: boolean;
		sensitive?: boolean;
		language?: string;
		fields?: Array<{
			name: string;
			value: string;
			verifiedAt?: string;
		}>;
	}): Promise<{
		readonly __typename: 'Actor';
		readonly id: string;
		readonly username: string;
		readonly domain?: string | null | undefined;
		readonly displayName?: string | null | undefined;
		readonly summary?: string | null | undefined;
		readonly avatar?: string | null | undefined;
		readonly header?: string | null | undefined;
		readonly followers: number;
		readonly following: number;
		readonly statusesCount: number;
		readonly bot: boolean;
		readonly locked: boolean;
		readonly updatedAt: string;
		readonly trustScore: number;
		readonly fields: ReadonlyArray<{
			readonly __typename: 'Field';
			readonly name: string;
			readonly value: string;
			readonly verifiedAt?: string | null | undefined;
		}>;
	}>;
	getUserPreferences(): Promise<{
		readonly __typename: 'UserPreferences';
		readonly actorId: string;
		readonly posting: {
			readonly __typename: 'PostingPreferences';
			readonly defaultVisibility: import('./index.js').Visibility;
			readonly defaultSensitive: boolean;
			readonly defaultLanguage: string;
		};
		readonly reading: {
			readonly __typename: 'ReadingPreferences';
			readonly expandSpoilers: boolean;
			readonly expandMedia: import('./index.js').ExpandMediaPreference;
			readonly autoplayGifs: boolean;
			readonly timelineOrder: import('./index.js').TimelineOrder;
		};
		readonly discovery: {
			readonly __typename: 'DiscoveryPreferences';
			readonly showFollowCounts: boolean;
			readonly searchSuggestionsEnabled: boolean;
			readonly personalizedSearchEnabled: boolean;
		};
		readonly streaming: {
			readonly __typename: 'StreamingPreferences';
			readonly defaultQuality: import('./index.js').StreamQuality;
			readonly autoQuality: boolean;
			readonly preloadNext: boolean;
			readonly dataSaver: boolean;
		};
		readonly notifications: {
			readonly __typename: 'NotificationPreferences';
			readonly email: boolean;
			readonly push: boolean;
			readonly inApp: boolean;
			readonly digest: import('./index.js').DigestFrequency;
		};
		readonly privacy: {
			readonly __typename: 'PrivacyPreferences';
			readonly defaultVisibility: import('./index.js').Visibility;
			readonly indexable: boolean;
			readonly showOnlineStatus: boolean;
		};
		readonly reblogFilters: ReadonlyArray<{
			readonly __typename: 'ReblogFilter';
			readonly key: string;
			readonly enabled: boolean;
		}>;
	}>;
	updateUserPreferences(input: {
		language?: string;
		defaultPostingVisibility?: 'PUBLIC' | 'UNLISTED' | 'FOLLOWERS' | 'DIRECT';
		defaultMediaSensitive?: boolean;
		expandSpoilers?: boolean;
		expandMedia?: 'DEFAULT' | 'SHOW_ALL' | 'HIDE_ALL';
		autoplayGifs?: boolean;
		showFollowCounts?: boolean;
		preferredTimelineOrder?: 'NEWEST' | 'OLDEST';
		searchSuggestionsEnabled?: boolean;
		personalizedSearchEnabled?: boolean;
		reblogFilters?: Array<{
			key: string;
			enabled: boolean;
		}>;
		streaming?: {
			defaultQuality?: 'AUTO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'ULTRA';
			autoQuality?: boolean;
			preloadNext?: boolean;
			dataSaver?: boolean;
		};
	}): Promise<{
		readonly __typename: 'UserPreferences';
		readonly actorId: string;
		readonly posting: {
			readonly __typename: 'PostingPreferences';
			readonly defaultVisibility: import('./index.js').Visibility;
			readonly defaultSensitive: boolean;
			readonly defaultLanguage: string;
		};
		readonly reading: {
			readonly __typename: 'ReadingPreferences';
			readonly expandSpoilers: boolean;
			readonly expandMedia: import('./index.js').ExpandMediaPreference;
			readonly autoplayGifs: boolean;
			readonly timelineOrder: import('./index.js').TimelineOrder;
		};
		readonly discovery: {
			readonly __typename: 'DiscoveryPreferences';
			readonly showFollowCounts: boolean;
			readonly searchSuggestionsEnabled: boolean;
			readonly personalizedSearchEnabled: boolean;
		};
		readonly streaming: {
			readonly __typename: 'StreamingPreferences';
			readonly defaultQuality: import('./index.js').StreamQuality;
			readonly autoQuality: boolean;
			readonly preloadNext: boolean;
			readonly dataSaver: boolean;
		};
		readonly notifications: {
			readonly __typename: 'NotificationPreferences';
			readonly email: boolean;
			readonly push: boolean;
			readonly inApp: boolean;
			readonly digest: import('./index.js').DigestFrequency;
		};
		readonly privacy: {
			readonly __typename: 'PrivacyPreferences';
			readonly defaultVisibility: import('./index.js').Visibility;
			readonly indexable: boolean;
			readonly showOnlineStatus: boolean;
		};
		readonly reblogFilters: ReadonlyArray<{
			readonly __typename: 'ReblogFilter';
			readonly key: string;
			readonly enabled: boolean;
		}>;
	}>;
	updateStreamingPreferences(input: {
		defaultQuality?: 'AUTO' | 'LOW' | 'MEDIUM' | 'HIGH' | 'ULTRA';
		autoQuality?: boolean;
		preloadNext?: boolean;
		dataSaver?: boolean;
	}): Promise<{
		readonly __typename: 'UserPreferences';
		readonly actorId: string;
		readonly streaming: {
			readonly __typename: 'StreamingPreferences';
			readonly defaultQuality: import('./index.js').StreamQuality;
			readonly autoQuality: boolean;
			readonly preloadNext: boolean;
			readonly dataSaver: boolean;
		};
	}>;
	getPushSubscription(): Promise<
		| {
				readonly __typename: 'PushSubscription';
				readonly id: string;
				readonly endpoint: string;
				readonly policy: string;
				readonly serverKey?: string | null | undefined;
				readonly createdAt?: string | null | undefined;
				readonly updatedAt?: string | null | undefined;
				readonly keys: {
					readonly __typename: 'PushSubscriptionKeys';
					readonly auth: string;
					readonly p256dh: string;
				};
				readonly alerts: {
					readonly __typename: 'PushSubscriptionAlerts';
					readonly follow: boolean;
					readonly favourite: boolean;
					readonly reblog: boolean;
					readonly mention: boolean;
					readonly poll: boolean;
					readonly followRequest: boolean;
					readonly status: boolean;
					readonly update: boolean;
					readonly adminSignUp: boolean;
					readonly adminReport: boolean;
				};
		  }
		| null
		| undefined
	>;
	registerPushSubscription(input: {
		endpoint: string;
		keys: {
			auth: string;
			p256dh: string;
		};
		alerts: {
			follow?: boolean;
			favourite?: boolean;
			reblog?: boolean;
			mention?: boolean;
			poll?: boolean;
			followRequest?: boolean;
			status?: boolean;
			update?: boolean;
			adminSignUp?: boolean;
			adminReport?: boolean;
		};
	}): Promise<{
		readonly __typename: 'PushSubscription';
		readonly id: string;
		readonly endpoint: string;
		readonly policy: string;
		readonly serverKey?: string | null | undefined;
		readonly createdAt?: string | null | undefined;
		readonly updatedAt?: string | null | undefined;
		readonly keys: {
			readonly __typename: 'PushSubscriptionKeys';
			readonly auth: string;
			readonly p256dh: string;
		};
		readonly alerts: {
			readonly __typename: 'PushSubscriptionAlerts';
			readonly follow: boolean;
			readonly favourite: boolean;
			readonly reblog: boolean;
			readonly mention: boolean;
			readonly poll: boolean;
			readonly followRequest: boolean;
			readonly status: boolean;
			readonly update: boolean;
			readonly adminSignUp: boolean;
			readonly adminReport: boolean;
		};
	}>;
	updatePushSubscription(input: {
		alerts: {
			follow?: boolean;
			favourite?: boolean;
			reblog?: boolean;
			mention?: boolean;
			poll?: boolean;
			followRequest?: boolean;
			status?: boolean;
			update?: boolean;
			adminSignUp?: boolean;
			adminReport?: boolean;
		};
	}): Promise<{
		readonly __typename: 'PushSubscription';
		readonly id: string;
		readonly endpoint: string;
		readonly policy: string;
		readonly serverKey?: string | null | undefined;
		readonly createdAt?: string | null | undefined;
		readonly updatedAt?: string | null | undefined;
		readonly keys: {
			readonly __typename: 'PushSubscriptionKeys';
			readonly auth: string;
			readonly p256dh: string;
		};
		readonly alerts: {
			readonly __typename: 'PushSubscriptionAlerts';
			readonly follow: boolean;
			readonly favourite: boolean;
			readonly reblog: boolean;
			readonly mention: boolean;
			readonly poll: boolean;
			readonly followRequest: boolean;
			readonly status: boolean;
			readonly update: boolean;
			readonly adminSignUp: boolean;
			readonly adminReport: boolean;
		};
	}>;
	deletePushSubscription(): Promise<boolean>;
	addCommunityNote(input: { objectId: string; content: string }): Promise<{
		readonly __typename: 'CommunityNotePayload';
		readonly note: {
			readonly __typename: 'CommunityNote';
			readonly id: string;
			readonly content: string;
			readonly helpful: number;
			readonly notHelpful: number;
			readonly createdAt: string;
			readonly author: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
		};
		readonly object: {
			readonly __typename: 'Object';
			readonly id: string;
			readonly type: import('./index.js').ObjectType;
		};
	}>;
	voteCommunityNote(
		id: string,
		helpful: boolean
	): Promise<{
		readonly __typename: 'CommunityNote';
		readonly id: string;
		readonly content: string;
		readonly helpful: number;
		readonly notHelpful: number;
		readonly createdAt: string;
		readonly author: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
	}>;
	getCommunityNotesByObject(
		objectId: string,
		first?: number,
		after?: string
	): Promise<
		| {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
		  }
		| null
		| undefined
	>;
	flagObject(input: { objectId: string; reason: string; evidence?: string[] }): Promise<{
		readonly __typename: 'FlagPayload';
		readonly moderationId: string;
		readonly queued: boolean;
	}>;
	createModerationPattern(input: ModerationPatternInput): Promise<{
		readonly __typename: 'ModerationPattern';
		readonly id: string;
		readonly pattern: string;
		readonly type: import('./index.js').PatternType;
		readonly severity: import('./index.js').ModerationSeverity;
		readonly matchCount: number;
		readonly falsePositiveRate: number;
		readonly createdAt: string;
		readonly updatedAt: string;
		readonly active: boolean;
		readonly createdBy: {
			readonly __typename: 'Actor';
			readonly id: string;
			readonly username: string;
			readonly domain?: string | null | undefined;
			readonly displayName?: string | null | undefined;
			readonly summary?: string | null | undefined;
			readonly avatar?: string | null | undefined;
			readonly header?: string | null | undefined;
			readonly followers: number;
			readonly following: number;
			readonly statusesCount: number;
			readonly bot: boolean;
			readonly locked: boolean;
			readonly updatedAt: string;
			readonly trustScore: number;
			readonly fields: ReadonlyArray<{
				readonly __typename: 'Field';
				readonly name: string;
				readonly value: string;
				readonly verifiedAt?: string | null | undefined;
			}>;
		};
	}>;
	deleteModerationPattern(id: string): Promise<boolean>;
	requestAIAnalysis(
		objectId: string,
		objectType?: string,
		force?: boolean
	): Promise<{
		readonly __typename: 'AIAnalysisRequest';
		readonly message: string;
		readonly objectId: string;
		readonly estimatedTime: string;
	}>;
	getAIAnalysis(objectId: string): Promise<
		| {
				readonly __typename: 'AIAnalysis';
				readonly id: string;
				readonly objectId: string;
				readonly objectType: string;
				readonly overallRisk: number;
				readonly moderationAction: import('./index.js').ModerationAction;
				readonly confidence: number;
				readonly analyzedAt: string;
				readonly textAnalysis?:
					| {
							readonly __typename: 'TextAnalysis';
							readonly sentiment: import('./index.js').Sentiment;
							readonly toxicityScore: number;
							readonly toxicityLabels: ReadonlyArray<string>;
							readonly containsPII: boolean;
							readonly dominantLanguage: string;
							readonly keyPhrases: ReadonlyArray<string>;
							readonly sentimentScores: {
								readonly __typename: 'SentimentScores';
								readonly positive: number;
								readonly negative: number;
								readonly neutral: number;
								readonly mixed: number;
							};
							readonly entities: ReadonlyArray<{
								readonly __typename: 'Entity';
								readonly text: string;
								readonly type: string;
								readonly score: number;
							}>;
					  }
					| null
					| undefined;
				readonly imageAnalysis?:
					| {
							readonly __typename: 'ImageAnalysis';
							readonly isNSFW: boolean;
							readonly nsfwConfidence: number;
							readonly violenceScore: number;
							readonly weaponsDetected: boolean;
							readonly detectedText: ReadonlyArray<string>;
							readonly textToxicity: number;
							readonly deepfakeScore: number;
							readonly moderationLabels: ReadonlyArray<{
								readonly __typename: 'ModerationLabel';
								readonly name: string;
								readonly confidence: number;
								readonly parentName?: string | null | undefined;
							}>;
							readonly celebrityFaces: ReadonlyArray<{
								readonly __typename: 'Celebrity';
								readonly name: string;
								readonly confidence: number;
							}>;
					  }
					| null
					| undefined;
				readonly aiDetection?:
					| {
							readonly __typename: 'AIDetection';
							readonly aiGeneratedProbability: number;
							readonly generationModel?: string | null | undefined;
							readonly patternConsistency: number;
							readonly styleDeviation: number;
							readonly semanticCoherence: number;
							readonly suspiciousPatterns: ReadonlyArray<string>;
					  }
					| null
					| undefined;
				readonly spamAnalysis?:
					| {
							readonly __typename: 'SpamAnalysis';
							readonly spamScore: number;
							readonly postingVelocity: number;
							readonly repetitionScore: number;
							readonly linkDensity: number;
							readonly followerRatio: number;
							readonly interactionRate: number;
							readonly accountAgeDays: number;
							readonly spamIndicators: ReadonlyArray<{
								readonly __typename: 'SpamIndicator';
								readonly type: string;
								readonly description: string;
								readonly severity: number;
							}>;
					  }
					| null
					| undefined;
		  }
		| null
		| undefined
	>;
	getAIStats(period: 'HOUR' | 'DAY' | 'WEEK' | 'MONTH' | 'YEAR'): Promise<{
		readonly __typename: 'AIStats';
		readonly period: string;
		readonly totalAnalyses: number;
		readonly toxicContent: number;
		readonly spamDetected: number;
		readonly aiGenerated: number;
		readonly nsfwContent: number;
		readonly piiDetected: number;
		readonly toxicityRate: number;
		readonly spamRate: number;
		readonly aiContentRate: number;
		readonly nsfwRate: number;
		readonly moderationActions: {
			readonly __typename: 'ModerationActionCounts';
			readonly flag: number;
			readonly hide: number;
			readonly remove: number;
			readonly review: number;
			readonly shadowBan: number;
		};
	}>;
	getAICapabilities(): Promise<{
		readonly __typename: 'AICapabilities';
		readonly moderationActions: ReadonlyArray<string>;
		readonly textAnalysis: {
			readonly __typename: 'TextAnalysisCapabilities';
			readonly sentimentAnalysis: boolean;
			readonly toxicityDetection: boolean;
			readonly spamDetection: boolean;
			readonly piiDetection: boolean;
			readonly entityExtraction: boolean;
			readonly languageDetection: boolean;
		};
		readonly imageAnalysis: {
			readonly __typename: 'ImageAnalysisCapabilities';
			readonly nsfwDetection: boolean;
			readonly violenceDetection: boolean;
			readonly textExtraction: boolean;
			readonly celebrityRecognition: boolean;
			readonly deepfakeDetection: boolean;
		};
		readonly aiDetection: {
			readonly __typename: 'AIDetectionCapabilities';
			readonly aiGeneratedContent: boolean;
			readonly patternAnalysis: boolean;
			readonly styleConsistency: boolean;
		};
		readonly costPerAnalysis: {
			readonly __typename: 'CostBreakdown';
			readonly period: import('./index.js').Period;
			readonly totalCost: number;
			readonly dynamoDBCost: number;
			readonly s3StorageCost: number;
			readonly lambdaCost: number;
			readonly dataTransferCost: number;
			readonly breakdown: ReadonlyArray<{
				readonly __typename: 'CostItem';
				readonly operation: string;
				readonly count: number;
				readonly cost: number;
			}>;
		};
	}>;
	getTrustGraph(
		actorId: string,
		category?: 'CONTENT' | 'BEHAVIOR' | 'TECHNICAL'
	): Promise<
		readonly {
			readonly __typename: 'TrustEdge';
			readonly category: import('./index.js').TrustCategory;
			readonly score: number;
			readonly updatedAt: string;
			readonly from: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
			readonly to: {
				readonly __typename: 'Actor';
				readonly id: string;
				readonly username: string;
				readonly domain?: string | null | undefined;
				readonly displayName?: string | null | undefined;
				readonly summary?: string | null | undefined;
				readonly avatar?: string | null | undefined;
				readonly header?: string | null | undefined;
				readonly followers: number;
				readonly following: number;
				readonly statusesCount: number;
				readonly bot: boolean;
				readonly locked: boolean;
				readonly updatedAt: string;
				readonly trustScore: number;
				readonly fields: ReadonlyArray<{
					readonly __typename: 'Field';
					readonly name: string;
					readonly value: string;
					readonly verifiedAt?: string | null | undefined;
				}>;
			};
		}[]
	>;
	getCostBreakdown(period?: 'HOUR' | 'DAY' | 'WEEK' | 'MONTH' | 'YEAR'): Promise<{
		readonly __typename: 'CostBreakdown';
		readonly period: import('./index.js').Period;
		readonly totalCost: number;
		readonly dynamoDBCost: number;
		readonly s3StorageCost: number;
		readonly lambdaCost: number;
		readonly dataTransferCost: number;
		readonly breakdown: ReadonlyArray<{
			readonly __typename: 'CostItem';
			readonly operation: string;
			readonly count: number;
			readonly cost: number;
		}>;
	}>;
	getInstanceBudgets(): Promise<
		readonly {
			readonly __typename: 'InstanceBudget';
			readonly domain: string;
			readonly monthlyBudgetUSD: number;
			readonly currentSpendUSD: number;
			readonly remainingBudgetUSD: number;
			readonly projectedOverspend?: number | null | undefined;
			readonly alertThreshold: number;
			readonly autoLimit: boolean;
			readonly period: string;
		}[]
	>;
	setInstanceBudget(
		domain: string,
		monthlyUSD: number,
		autoLimit?: boolean
	): Promise<{
		readonly __typename: 'InstanceBudget';
		readonly domain: string;
		readonly monthlyBudgetUSD: number;
		readonly currentSpendUSD: number;
		readonly remainingBudgetUSD: number;
		readonly projectedOverspend?: number | null | undefined;
		readonly alertThreshold: number;
		readonly autoLimit: boolean;
		readonly period: string;
	}>;
	optimizeFederationCosts(threshold: number): Promise<{
		readonly __typename: 'CostOptimizationResult';
		readonly optimized: number;
		readonly savedMonthlyUSD: number;
		readonly actions: ReadonlyArray<{
			readonly __typename: 'OptimizationAction';
			readonly domain: string;
			readonly action: string;
			readonly savingsUSD: number;
			readonly impact: string;
		}>;
	}>;
	getFederationLimits(): Promise<
		readonly {
			readonly __typename: 'FederationLimit';
			readonly domain: string;
			readonly ingressLimitMB: number;
			readonly egressLimitMB: number;
			readonly requestsPerMinute: number;
			readonly monthlyBudgetUSD?: number | null | undefined;
			readonly active: boolean;
			readonly createdAt: string;
			readonly updatedAt: string;
		}[]
	>;
	setFederationLimit(
		domain: string,
		limit: Record<string, unknown>
	): Promise<{
		readonly __typename: 'FederationLimit';
		readonly domain: string;
		readonly ingressLimitMB: number;
		readonly egressLimitMB: number;
		readonly requestsPerMinute: number;
		readonly monthlyBudgetUSD?: number | null | undefined;
		readonly active: boolean;
		readonly createdAt: string;
		readonly updatedAt: string;
	}>;
	syncThread(
		noteUrl: string,
		depth?: number
	): Promise<{
		readonly __typename: 'SyncThreadPayload';
		readonly success: boolean;
		readonly syncedPosts: number;
		readonly errors?: ReadonlyArray<string> | null | undefined;
		readonly thread: {
			readonly __typename: 'ThreadContext';
			readonly replyCount: number;
			readonly participantCount: number;
			readonly lastActivity: string;
			readonly missingPosts: number;
			readonly syncStatus: import('./index.js').SyncStatus;
			readonly rootNote: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		};
	}>;
	syncMissingReplies(noteId: string): Promise<{
		readonly __typename: 'SyncRepliesPayload';
		readonly success: boolean;
		readonly syncedReplies: number;
		readonly thread: {
			readonly __typename: 'ThreadContext';
			readonly replyCount: number;
			readonly participantCount: number;
			readonly lastActivity: string;
			readonly missingPosts: number;
			readonly syncStatus: import('./index.js').SyncStatus;
			readonly rootNote: {
				readonly __typename: 'Object';
				readonly id: string;
				readonly type: import('./index.js').ObjectType;
				readonly content: string;
				readonly visibility: import('./index.js').Visibility;
				readonly sensitive: boolean;
				readonly spoilerText?: string | null | undefined;
				readonly createdAt: string;
				readonly updatedAt: string;
				readonly repliesCount: number;
				readonly likesCount: number;
				readonly sharesCount: number;
				readonly estimatedCost: number;
				readonly moderationScore?: number | null | undefined;
				readonly quoteUrl?: string | null | undefined;
				readonly quoteable: boolean;
				readonly quotePermissions: import('./index.js').QuotePermission;
				readonly quoteCount: number;
				readonly boostedObject?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly content: string;
							readonly visibility: import('./index.js').Visibility;
							readonly sensitive: boolean;
							readonly spoilerText?: string | null | undefined;
							readonly createdAt: string;
							readonly updatedAt: string;
							readonly repliesCount: number;
							readonly likesCount: number;
							readonly sharesCount: number;
							readonly estimatedCost: number;
							readonly moderationScore?: number | null | undefined;
							readonly quoteUrl?: string | null | undefined;
							readonly quoteable: boolean;
							readonly quotePermissions: import('./index.js').QuotePermission;
							readonly quoteCount: number;
							readonly contentMap: ReadonlyArray<{
								readonly __typename: 'ContentMap';
								readonly language: string;
								readonly content: string;
							}>;
							readonly attachments: ReadonlyArray<{
								readonly __typename: 'Attachment';
								readonly id: string;
								readonly type: string;
								readonly url: string;
								readonly preview?: string | null | undefined;
								readonly description?: string | null | undefined;
								readonly blurhash?: string | null | undefined;
								readonly width?: number | null | undefined;
								readonly height?: number | null | undefined;
								readonly duration?: number | null | undefined;
							}>;
							readonly tags: ReadonlyArray<{
								readonly __typename: 'Tag';
								readonly name: string;
								readonly url: string;
							}>;
							readonly mentions: ReadonlyArray<{
								readonly __typename: 'Mention';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly url: string;
							}>;
							readonly quoteContext?:
								| {
										readonly __typename: 'QuoteContext';
										readonly quoteAllowed: boolean;
										readonly quoteType: import('./index.js').QuoteType;
										readonly withdrawn: boolean;
										readonly originalAuthor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
										readonly originalNote?:
											| {
													readonly __typename: 'Object';
													readonly id: string;
													readonly type: import('./index.js').ObjectType;
											  }
											| null
											| undefined;
								  }
								| null
								| undefined;
							readonly communityNotes: ReadonlyArray<{
								readonly __typename: 'CommunityNote';
								readonly id: string;
								readonly content: string;
								readonly helpful: number;
								readonly notHelpful: number;
								readonly createdAt: string;
								readonly author: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
							}>;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly inReplyTo?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
										readonly actor: {
											readonly __typename: 'Actor';
											readonly id: string;
											readonly username: string;
											readonly domain?: string | null | undefined;
											readonly displayName?: string | null | undefined;
											readonly summary?: string | null | undefined;
											readonly avatar?: string | null | undefined;
											readonly header?: string | null | undefined;
											readonly followers: number;
											readonly following: number;
											readonly statusesCount: number;
											readonly bot: boolean;
											readonly locked: boolean;
											readonly updatedAt: string;
											readonly trustScore: number;
											readonly fields: ReadonlyArray<{
												readonly __typename: 'Field';
												readonly name: string;
												readonly value: string;
												readonly verifiedAt?: string | null | undefined;
											}>;
										};
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly contentMap: ReadonlyArray<{
					readonly __typename: 'ContentMap';
					readonly language: string;
					readonly content: string;
				}>;
				readonly attachments: ReadonlyArray<{
					readonly __typename: 'Attachment';
					readonly id: string;
					readonly type: string;
					readonly url: string;
					readonly preview?: string | null | undefined;
					readonly description?: string | null | undefined;
					readonly blurhash?: string | null | undefined;
					readonly width?: number | null | undefined;
					readonly height?: number | null | undefined;
					readonly duration?: number | null | undefined;
				}>;
				readonly tags: ReadonlyArray<{
					readonly __typename: 'Tag';
					readonly name: string;
					readonly url: string;
				}>;
				readonly mentions: ReadonlyArray<{
					readonly __typename: 'Mention';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly url: string;
				}>;
				readonly quoteContext?:
					| {
							readonly __typename: 'QuoteContext';
							readonly quoteAllowed: boolean;
							readonly quoteType: import('./index.js').QuoteType;
							readonly withdrawn: boolean;
							readonly originalAuthor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
							readonly originalNote?:
								| {
										readonly __typename: 'Object';
										readonly id: string;
										readonly type: import('./index.js').ObjectType;
								  }
								| null
								| undefined;
					  }
					| null
					| undefined;
				readonly communityNotes: ReadonlyArray<{
					readonly __typename: 'CommunityNote';
					readonly id: string;
					readonly content: string;
					readonly helpful: number;
					readonly notHelpful: number;
					readonly createdAt: string;
					readonly author: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
				}>;
				readonly actor: {
					readonly __typename: 'Actor';
					readonly id: string;
					readonly username: string;
					readonly domain?: string | null | undefined;
					readonly displayName?: string | null | undefined;
					readonly summary?: string | null | undefined;
					readonly avatar?: string | null | undefined;
					readonly header?: string | null | undefined;
					readonly followers: number;
					readonly following: number;
					readonly statusesCount: number;
					readonly bot: boolean;
					readonly locked: boolean;
					readonly updatedAt: string;
					readonly trustScore: number;
					readonly fields: ReadonlyArray<{
						readonly __typename: 'Field';
						readonly name: string;
						readonly value: string;
						readonly verifiedAt?: string | null | undefined;
					}>;
				};
				readonly inReplyTo?:
					| {
							readonly __typename: 'Object';
							readonly id: string;
							readonly type: import('./index.js').ObjectType;
							readonly actor: {
								readonly __typename: 'Actor';
								readonly id: string;
								readonly username: string;
								readonly domain?: string | null | undefined;
								readonly displayName?: string | null | undefined;
								readonly summary?: string | null | undefined;
								readonly avatar?: string | null | undefined;
								readonly header?: string | null | undefined;
								readonly followers: number;
								readonly following: number;
								readonly statusesCount: number;
								readonly bot: boolean;
								readonly locked: boolean;
								readonly updatedAt: string;
								readonly trustScore: number;
								readonly fields: ReadonlyArray<{
									readonly __typename: 'Field';
									readonly name: string;
									readonly value: string;
									readonly verifiedAt?: string | null | undefined;
								}>;
							};
					  }
					| null
					| undefined;
			};
		};
	}>;
	getThreadContext(noteId: string): Promise<
		| {
				readonly __typename: 'ThreadContext';
				readonly replyCount: number;
				readonly participantCount: number;
				readonly lastActivity: string;
				readonly missingPosts: number;
				readonly syncStatus: import('./index.js').SyncStatus;
				readonly rootNote: {
					readonly __typename: 'Object';
					readonly id: string;
					readonly type: import('./index.js').ObjectType;
					readonly content: string;
					readonly visibility: import('./index.js').Visibility;
					readonly sensitive: boolean;
					readonly spoilerText?: string | null | undefined;
					readonly createdAt: string;
					readonly updatedAt: string;
					readonly repliesCount: number;
					readonly likesCount: number;
					readonly sharesCount: number;
					readonly estimatedCost: number;
					readonly moderationScore?: number | null | undefined;
					readonly quoteUrl?: string | null | undefined;
					readonly quoteable: boolean;
					readonly quotePermissions: import('./index.js').QuotePermission;
					readonly quoteCount: number;
					readonly boostedObject?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
								readonly content: string;
								readonly visibility: import('./index.js').Visibility;
								readonly sensitive: boolean;
								readonly spoilerText?: string | null | undefined;
								readonly createdAt: string;
								readonly updatedAt: string;
								readonly repliesCount: number;
								readonly likesCount: number;
								readonly sharesCount: number;
								readonly estimatedCost: number;
								readonly moderationScore?: number | null | undefined;
								readonly quoteUrl?: string | null | undefined;
								readonly quoteable: boolean;
								readonly quotePermissions: import('./index.js').QuotePermission;
								readonly quoteCount: number;
								readonly contentMap: ReadonlyArray<{
									readonly __typename: 'ContentMap';
									readonly language: string;
									readonly content: string;
								}>;
								readonly attachments: ReadonlyArray<{
									readonly __typename: 'Attachment';
									readonly id: string;
									readonly type: string;
									readonly url: string;
									readonly preview?: string | null | undefined;
									readonly description?: string | null | undefined;
									readonly blurhash?: string | null | undefined;
									readonly width?: number | null | undefined;
									readonly height?: number | null | undefined;
									readonly duration?: number | null | undefined;
								}>;
								readonly tags: ReadonlyArray<{
									readonly __typename: 'Tag';
									readonly name: string;
									readonly url: string;
								}>;
								readonly mentions: ReadonlyArray<{
									readonly __typename: 'Mention';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly url: string;
								}>;
								readonly quoteContext?:
									| {
											readonly __typename: 'QuoteContext';
											readonly quoteAllowed: boolean;
											readonly quoteType: import('./index.js').QuoteType;
											readonly withdrawn: boolean;
											readonly originalAuthor: {
												readonly __typename: 'Actor';
												readonly id: string;
												readonly username: string;
												readonly domain?: string | null | undefined;
												readonly displayName?: string | null | undefined;
												readonly summary?: string | null | undefined;
												readonly avatar?: string | null | undefined;
												readonly header?: string | null | undefined;
												readonly followers: number;
												readonly following: number;
												readonly statusesCount: number;
												readonly bot: boolean;
												readonly locked: boolean;
												readonly updatedAt: string;
												readonly trustScore: number;
												readonly fields: ReadonlyArray<{
													readonly __typename: 'Field';
													readonly name: string;
													readonly value: string;
													readonly verifiedAt?: string | null | undefined;
												}>;
											};
											readonly originalNote?:
												| {
														readonly __typename: 'Object';
														readonly id: string;
														readonly type: import('./index.js').ObjectType;
												  }
												| null
												| undefined;
									  }
									| null
									| undefined;
								readonly communityNotes: ReadonlyArray<{
									readonly __typename: 'CommunityNote';
									readonly id: string;
									readonly content: string;
									readonly helpful: number;
									readonly notHelpful: number;
									readonly createdAt: string;
									readonly author: {
										readonly __typename: 'Actor';
										readonly id: string;
										readonly username: string;
										readonly domain?: string | null | undefined;
										readonly displayName?: string | null | undefined;
										readonly summary?: string | null | undefined;
										readonly avatar?: string | null | undefined;
										readonly header?: string | null | undefined;
										readonly followers: number;
										readonly following: number;
										readonly statusesCount: number;
										readonly bot: boolean;
										readonly locked: boolean;
										readonly updatedAt: string;
										readonly trustScore: number;
										readonly fields: ReadonlyArray<{
											readonly __typename: 'Field';
											readonly name: string;
											readonly value: string;
											readonly verifiedAt?: string | null | undefined;
										}>;
									};
								}>;
								readonly actor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
								readonly inReplyTo?:
									| {
											readonly __typename: 'Object';
											readonly id: string;
											readonly type: import('./index.js').ObjectType;
											readonly actor: {
												readonly __typename: 'Actor';
												readonly id: string;
												readonly username: string;
												readonly domain?: string | null | undefined;
												readonly displayName?: string | null | undefined;
												readonly summary?: string | null | undefined;
												readonly avatar?: string | null | undefined;
												readonly header?: string | null | undefined;
												readonly followers: number;
												readonly following: number;
												readonly statusesCount: number;
												readonly bot: boolean;
												readonly locked: boolean;
												readonly updatedAt: string;
												readonly trustScore: number;
												readonly fields: ReadonlyArray<{
													readonly __typename: 'Field';
													readonly name: string;
													readonly value: string;
													readonly verifiedAt?: string | null | undefined;
												}>;
											};
									  }
									| null
									| undefined;
						  }
						| null
						| undefined;
					readonly contentMap: ReadonlyArray<{
						readonly __typename: 'ContentMap';
						readonly language: string;
						readonly content: string;
					}>;
					readonly attachments: ReadonlyArray<{
						readonly __typename: 'Attachment';
						readonly id: string;
						readonly type: string;
						readonly url: string;
						readonly preview?: string | null | undefined;
						readonly description?: string | null | undefined;
						readonly blurhash?: string | null | undefined;
						readonly width?: number | null | undefined;
						readonly height?: number | null | undefined;
						readonly duration?: number | null | undefined;
					}>;
					readonly tags: ReadonlyArray<{
						readonly __typename: 'Tag';
						readonly name: string;
						readonly url: string;
					}>;
					readonly mentions: ReadonlyArray<{
						readonly __typename: 'Mention';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly url: string;
					}>;
					readonly quoteContext?:
						| {
								readonly __typename: 'QuoteContext';
								readonly quoteAllowed: boolean;
								readonly quoteType: import('./index.js').QuoteType;
								readonly withdrawn: boolean;
								readonly originalAuthor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
								readonly originalNote?:
									| {
											readonly __typename: 'Object';
											readonly id: string;
											readonly type: import('./index.js').ObjectType;
									  }
									| null
									| undefined;
						  }
						| null
						| undefined;
					readonly communityNotes: ReadonlyArray<{
						readonly __typename: 'CommunityNote';
						readonly id: string;
						readonly content: string;
						readonly helpful: number;
						readonly notHelpful: number;
						readonly createdAt: string;
						readonly author: {
							readonly __typename: 'Actor';
							readonly id: string;
							readonly username: string;
							readonly domain?: string | null | undefined;
							readonly displayName?: string | null | undefined;
							readonly summary?: string | null | undefined;
							readonly avatar?: string | null | undefined;
							readonly header?: string | null | undefined;
							readonly followers: number;
							readonly following: number;
							readonly statusesCount: number;
							readonly bot: boolean;
							readonly locked: boolean;
							readonly updatedAt: string;
							readonly trustScore: number;
							readonly fields: ReadonlyArray<{
								readonly __typename: 'Field';
								readonly name: string;
								readonly value: string;
								readonly verifiedAt?: string | null | undefined;
							}>;
						};
					}>;
					readonly actor: {
						readonly __typename: 'Actor';
						readonly id: string;
						readonly username: string;
						readonly domain?: string | null | undefined;
						readonly displayName?: string | null | undefined;
						readonly summary?: string | null | undefined;
						readonly avatar?: string | null | undefined;
						readonly header?: string | null | undefined;
						readonly followers: number;
						readonly following: number;
						readonly statusesCount: number;
						readonly bot: boolean;
						readonly locked: boolean;
						readonly updatedAt: string;
						readonly trustScore: number;
						readonly fields: ReadonlyArray<{
							readonly __typename: 'Field';
							readonly name: string;
							readonly value: string;
							readonly verifiedAt?: string | null | undefined;
						}>;
					};
					readonly inReplyTo?:
						| {
								readonly __typename: 'Object';
								readonly id: string;
								readonly type: import('./index.js').ObjectType;
								readonly actor: {
									readonly __typename: 'Actor';
									readonly id: string;
									readonly username: string;
									readonly domain?: string | null | undefined;
									readonly displayName?: string | null | undefined;
									readonly summary?: string | null | undefined;
									readonly avatar?: string | null | undefined;
									readonly header?: string | null | undefined;
									readonly followers: number;
									readonly following: number;
									readonly statusesCount: number;
									readonly bot: boolean;
									readonly locked: boolean;
									readonly updatedAt: string;
									readonly trustScore: number;
									readonly fields: ReadonlyArray<{
										readonly __typename: 'Field';
										readonly name: string;
										readonly value: string;
										readonly verifiedAt?: string | null | undefined;
									}>;
								};
						  }
						| null
						| undefined;
				};
		  }
		| null
		| undefined
	>;
	getSeveredRelationships(
		instance?: string,
		first?: number,
		after?: string
	): Promise<{
		readonly __typename: 'SeveredRelationshipConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'SeveredRelationshipEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'SeveredRelationship';
				readonly id: string;
				readonly localInstance: string;
				readonly remoteInstance: string;
				readonly reason: import('./index.js').SeveranceReason;
				readonly affectedFollowers: number;
				readonly affectedFollowing: number;
				readonly timestamp: string;
				readonly reversible: boolean;
				readonly details?:
					| {
							readonly __typename: 'SeveranceDetails';
							readonly description: string;
							readonly metadata: ReadonlyArray<string>;
							readonly adminNotes?: string | null | undefined;
							readonly autoDetected: boolean;
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	acknowledgeSeverance(id: string): Promise<{
		readonly __typename: 'AcknowledgePayload';
		readonly success: boolean;
		readonly acknowledged: boolean;
		readonly severedRelationship: {
			readonly __typename: 'SeveredRelationship';
			readonly id: string;
			readonly localInstance: string;
			readonly remoteInstance: string;
		};
	}>;
	attemptReconnection(id: string): Promise<{
		readonly __typename: 'ReconnectionPayload';
		readonly success: boolean;
		readonly reconnected: number;
		readonly failed: number;
		readonly errors?: ReadonlyArray<string> | null | undefined;
		readonly severedRelationship: {
			readonly __typename: 'SeveredRelationship';
			readonly id: string;
			readonly localInstance: string;
			readonly remoteInstance: string;
		};
	}>;
	getFederationHealth(threshold?: number): Promise<
		readonly {
			readonly __typename: 'FederationManagementStatus';
			readonly domain: string;
			readonly status: import('./index.js').FederationState;
			readonly reason?: string | null | undefined;
			readonly pausedUntil?: string | null | undefined;
		}[]
	>;
	getFederationStatus(domain: string): Promise<{
		readonly __typename: 'FederationStatus';
		readonly domain: string;
		readonly reachable: boolean;
		readonly lastContact?: string | null | undefined;
		readonly sharedInbox?: string | null | undefined;
		readonly publicKey?: string | null | undefined;
		readonly software?: string | null | undefined;
		readonly version?: string | null | undefined;
	}>;
	pauseFederation(
		domain: string,
		reason: string,
		until?: string
	): Promise<{
		readonly __typename: 'FederationManagementStatus';
		readonly domain: string;
		readonly status: import('./index.js').FederationState;
		readonly reason?: string | null | undefined;
		readonly pausedUntil?: string | null | undefined;
		readonly limits?:
			| {
					readonly __typename: 'FederationLimit';
					readonly domain: string;
					readonly ingressLimitMB: number;
					readonly egressLimitMB: number;
					readonly requestsPerMinute: number;
					readonly monthlyBudgetUSD?: number | null | undefined;
					readonly active: boolean;
					readonly createdAt: string;
					readonly updatedAt: string;
			  }
			| null
			| undefined;
	}>;
	resumeFederation(domain: string): Promise<{
		readonly __typename: 'FederationManagementStatus';
		readonly domain: string;
		readonly status: import('./index.js').FederationState;
		readonly reason?: string | null | undefined;
		readonly pausedUntil?: string | null | undefined;
		readonly limits?:
			| {
					readonly __typename: 'FederationLimit';
					readonly domain: string;
					readonly ingressLimitMB: number;
					readonly egressLimitMB: number;
					readonly requestsPerMinute: number;
					readonly monthlyBudgetUSD?: number | null | undefined;
					readonly active: boolean;
					readonly createdAt: string;
					readonly updatedAt: string;
			  }
			| null
			| undefined;
	}>;
	followHashtag(
		hashtag: string,
		notifyLevel?: 'ALL' | 'MUTUALS' | 'FOLLOWING' | 'NONE'
	): Promise<{
		readonly __typename: 'HashtagFollowPayload';
		readonly success: boolean;
		readonly hashtag: {
			readonly __typename: 'Hashtag';
			readonly name: string;
			readonly url: string;
			readonly isFollowing: boolean;
			readonly followedAt?: string | null | undefined;
			readonly notificationSettings?:
				| {
						readonly __typename: 'HashtagNotificationSettings';
						readonly level: NotificationLevel;
						readonly muted: boolean;
						readonly mutedUntil?: string | null | undefined;
				  }
				| null
				| undefined;
		};
	}>;
	unfollowHashtag(hashtag: string): Promise<{
		readonly __typename: 'UnfollowHashtagPayload';
		readonly success: boolean;
		readonly hashtag: {
			readonly __typename: 'Hashtag';
			readonly name: string;
			readonly url: string;
		};
	}>;
	muteHashtag(
		hashtag: string,
		until?: string
	): Promise<{
		readonly __typename: 'MuteHashtagPayload';
		readonly success: boolean;
		readonly mutedUntil?: string | null | undefined;
		readonly hashtag: {
			readonly __typename: 'Hashtag';
			readonly name: string;
			readonly notificationSettings?:
				| {
						readonly __typename: 'HashtagNotificationSettings';
						readonly muted: boolean;
						readonly mutedUntil?: string | null | undefined;
				  }
				| null
				| undefined;
		};
	}>;
	getFollowedHashtags(
		first?: number,
		after?: string
	): Promise<{
		readonly __typename: 'HashtagConnection';
		readonly totalCount: number;
		readonly edges: ReadonlyArray<{
			readonly __typename: 'HashtagEdge';
			readonly cursor: string;
			readonly node: {
				readonly __typename: 'Hashtag';
				readonly name: string;
				readonly url: string;
				readonly isFollowing: boolean;
				readonly followedAt?: string | null | undefined;
				readonly notificationSettings?:
					| {
							readonly __typename: 'HashtagNotificationSettings';
							readonly level: NotificationLevel;
							readonly muted: boolean;
							readonly mutedUntil?: string | null | undefined;
					  }
					| null
					| undefined;
			};
		}>;
		readonly pageInfo: {
			readonly __typename: 'PageInfo';
			readonly hasNextPage: boolean;
			readonly hasPreviousPage: boolean;
			readonly startCursor?: string | null | undefined;
			readonly endCursor?: string | null | undefined;
		};
	}>;
	updateHashtagNotifications(
		hashtag: string,
		settings: HashtagNotificationSettingsInput
	): Promise<{
		success: boolean;
		hashtag?: {
			name: string;
			notificationSettings?: {
				level: NotificationLevel;
				muted?: boolean | null;
				mutedUntil?: string | null;
			} | null;
		} | null;
	}>;
	unmuteHashtag(
		hashtag: string,
		options?: {
			level?: NotificationLevel;
			mutedUntil?: string | null;
			filters?: HashtagNotificationSettingsInput['filters'];
		}
	): Promise<{
		success: boolean;
		hashtag?: {
			name: string;
			notificationSettings?: {
				level: NotificationLevel;
				muted?: boolean | null;
				mutedUntil?: string | null;
			} | null;
		} | null;
	}>;
	subscribeToTimelineUpdates(
		variables: TimelineUpdatesSubscriptionVariables
	): Observable<FetchResult<TimelineUpdatesSubscription>>;
	subscribeToNotificationStream(
		variables?: NotificationStreamSubscriptionVariables
	): Observable<FetchResult<NotificationStreamSubscription>>;
	subscribeToConversationUpdates(): Observable<FetchResult<ConversationUpdatesSubscription>>;
	subscribeToListUpdates(
		variables: ListUpdatesSubscriptionVariables
	): Observable<FetchResult<ListUpdatesSubscription>>;
	subscribeToQuoteActivity(
		variables: QuoteActivitySubscriptionVariables
	): Observable<FetchResult<QuoteActivitySubscription>>;
	subscribeToHashtagActivity(
		variables: HashtagActivitySubscriptionVariables
	): Observable<FetchResult<HashtagActivitySubscription>>;
	subscribeToActivityStream(
		variables?: ActivityStreamSubscriptionVariables
	): Observable<FetchResult<ActivityStreamSubscription>>;
	subscribeToRelationshipUpdates(
		variables?: RelationshipUpdatesSubscriptionVariables
	): Observable<FetchResult<RelationshipUpdatesSubscription>>;
	subscribeToCostUpdates(
		variables?: CostUpdatesSubscriptionVariables
	): Observable<FetchResult<CostUpdatesSubscription>>;
	subscribeToModerationEvents(
		variables?: ModerationEventsSubscriptionVariables
	): Observable<FetchResult<ModerationEventsSubscription>>;
	subscribeToTrustUpdates(
		variables: TrustUpdatesSubscriptionVariables
	): Observable<FetchResult<TrustUpdatesSubscription>>;
	subscribeToAiAnalysisUpdates(
		variables?: AiAnalysisUpdatesSubscriptionVariables
	): Observable<FetchResult<AiAnalysisUpdatesSubscription>>;
	subscribeToMetricsUpdates(
		variables?: MetricsUpdatesSubscriptionVariables
	): Observable<FetchResult<MetricsUpdatesSubscription>>;
	subscribeToModerationAlerts(
		variables?: ModerationAlertsSubscriptionVariables
	): Observable<FetchResult<ModerationAlertsSubscription>>;
	subscribeToCostAlerts(
		variables: CostAlertsSubscriptionVariables
	): Observable<FetchResult<CostAlertsSubscription>>;
	subscribeToBudgetAlerts(
		variables?: BudgetAlertsSubscriptionVariables
	): Observable<FetchResult<BudgetAlertsSubscription>>;
	subscribeToFederationHealthUpdates(
		variables?: FederationHealthUpdatesSubscriptionVariables
	): Observable<FetchResult<FederationHealthUpdatesSubscription>>;
	subscribeToModerationQueueUpdate(
		variables?: ModerationQueueUpdateSubscriptionVariables
	): Observable<FetchResult<ModerationQueueUpdateSubscription>>;
	subscribeToThreatIntelligence(): Observable<FetchResult<ThreatIntelligenceSubscription>>;
	subscribeToPerformanceAlert(
		variables: PerformanceAlertSubscriptionVariables
	): Observable<FetchResult<PerformanceAlertSubscription>>;
	subscribeToInfrastructureEvent(): Observable<FetchResult<InfrastructureEventSubscription>>;
}
export declare function createLesserGraphQLAdapter(
	config: LesserGraphQLAdapterConfig
): LesserGraphQLAdapter;
//# sourceMappingURL=LesserGraphQLAdapter.d.ts.map
