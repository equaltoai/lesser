export {
	LesserHostSoulClient,
	LesserHostSoulClientError,
	createLesserHostSoulClient,
} from './client.js';
export type {
	ErrorEnvelope,
	FetchLike,
	LesserHostSoulClientConfig,
	ResolveEnsOptions,
	SoulAgentCommActivityItem,
	SoulAgentCommActivityQuery,
	SoulAgentCommActivityResponse,
	SoulAgentChannelsResponse,
	SoulAgentChannelPreferencesRequest,
	SoulAgentChannelPreferencesResponse,
	SoulAgentCommQueueItem,
	SoulAgentCommQueueQuery,
	SoulAgentCommQueueResponse,
	SoulAgentIdentity,
	SoulCommSendErrorEnvelope,
	SoulCommSendRequest,
	SoulCommSendResponse,
	SoulCommStatusErrorEnvelope,
	SoulCommStatusResponse,
	SoulResolveResponse,
	SoulSearchQuery,
	SoulSearchResponse,
	SoulSearchResult,
} from './client.js';

export { resolveSoulAgentIdFromEnsTextRecord } from './ens.js';
export type { ResolveSoulAgentIdFromEnsTextRecordOptions } from './ens.js';
