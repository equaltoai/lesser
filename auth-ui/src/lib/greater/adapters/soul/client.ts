import type { components, operations } from '../rest/generated/lesser-host-api.js';
import { resolveFetchLike, type FetchLike } from '../fetch.js';
import { resolveSoulAgentIdFromEnsTextRecord } from './ens.js';

export type { FetchLike } from '../fetch.js';

export type ErrorEnvelope = components['schemas']['ErrorEnvelope'];
export type SoulAgentChannelsResponse = components['schemas']['SoulAgentChannelsResponse'];
export type SoulAgentChannelPreferencesRequest =
	components['schemas']['SoulAgentChannelPreferencesRequest'];
export type SoulAgentChannelPreferencesResponse =
	components['schemas']['SoulAgentChannelPreferencesResponse'];
export type SoulAgentIdentity = components['schemas']['SoulAgentIdentity'];
export type SoulAgentCommActivityQuery = NonNullable<
	operations['soulAgentCommActivity']['parameters']['query']
>;
export type SoulAgentCommActivityItem = components['schemas']['SoulAgentCommActivityItem'];
export type SoulAgentCommActivityResponse = components['schemas']['SoulAgentCommActivityResponse'];
export type SoulAgentCommQueueQuery = NonNullable<
	operations['soulAgentCommQueue']['parameters']['query']
>;
export type SoulAgentCommQueueItem = components['schemas']['SoulAgentCommQueueItem'];
export type SoulAgentCommQueueResponse = components['schemas']['SoulAgentCommQueueResponse'];
export type SoulMintConversationSSEInput = components['schemas']['SoulMintConversationSSEInput'];
export type SoulMintConversation = components['schemas']['SoulMintConversation'];
export type SoulMintConversationCompleteRequest =
	components['schemas']['SoulMintConversationCompleteRequest'];
export type SoulMintConversationFinalizeBeginRequest =
	components['schemas']['SoulMintConversationFinalizeBeginRequest'];
export type SoulMintConversationFinalizePreflightResponse =
	components['schemas']['SoulMintConversationFinalizePreflightResponse'];
export type SoulMintConversationFinalizeRequest =
	components['schemas']['SoulMintConversationFinalizeRequest'];
export type SoulMintConversationFinalizeResponse =
	components['schemas']['SoulMintConversationFinalizeResponse'];
export type SoulAgentMintConversationsQuery = NonNullable<
	operations['soulAgentListMintConversations']['parameters']['query']
>;
export type SoulAgentMintConversationsResponse =
	components['schemas']['SoulAgentMintConversationsResponse'];
export type SoulResolveResponse = components['schemas']['SoulResolveResponse'];
export type SoulSearchQuery = NonNullable<operations['soulSearch']['parameters']['query']>;
export type SoulSearchResult = components['schemas']['SoulSearchResult'];
export type SoulSearchResponse = components['schemas']['SoulSearchResponse'];
export type SoulCommSendRequest = components['schemas']['SoulCommSendRequest'];
export type SoulCommSendResponse = components['schemas']['SoulCommSendResponse'];
export type SoulCommSendErrorEnvelope = components['schemas']['SoulCommSendErrorEnvelope'];
export type SoulCommStatusResponse = components['schemas']['SoulCommStatusResponse'];
export type SoulCommStatusErrorEnvelope = components['schemas']['SoulCommStatusErrorEnvelope'];

export interface LesserHostSoulClientConfig {
	baseUrl: string;
	fetch?: FetchLike;
	headers?: Record<string, string>;
}

export class LesserHostSoulClientError extends Error {
	readonly status: number;
	readonly code: string;
	readonly requestId?: string;

	constructor(options: { status: number; code: string; message: string; requestId?: string }) {
		super(options.message);
		this.name = 'LesserHostSoulClientError';
		this.status = options.status;
		this.code = options.code;
		this.requestId = options.requestId;
	}
}

export interface ResolveEnsOptions {
	ensName: string;
	rpcUrl?: string;
	textKey?: string;
}

type QueryParamPrimitive = string | number | boolean;
type QueryParamValue =
	| QueryParamPrimitive
	| null
	| undefined
	| ReadonlyArray<QueryParamPrimitive | null | undefined>;

interface RequestOptions {
	method?: 'GET' | 'POST' | 'PUT';
	body?: unknown;
	query?: Record<string, QueryParamValue>;
	headers?: HeadersInit;
	accept?: string;
}

export function createLesserHostSoulClient(
	config: LesserHostSoulClientConfig
): LesserHostSoulClient {
	return new LesserHostSoulClient(config);
}

export class LesserHostSoulClient {
	private readonly baseUrl: string;
	private readonly fetch: FetchLike;
	private readonly headers: Record<string, string>;

	constructor(config: LesserHostSoulClientConfig) {
		const baseUrl = config.baseUrl.trim();
		if (!baseUrl) {
			throw new Error('LesserHostSoulClient requires baseUrl');
		}
		this.baseUrl = trimTrailingSlashes(baseUrl);
		this.fetch = resolveFetchLike(config.fetch);
		this.headers = {
			accept: 'application/json',
			...(config.headers ?? {}),
		};
	}

	async getAgentChannels(agentId: string): Promise<SoulAgentChannelsResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestJson(`/api/v1/soul/agents/${encodeURIComponent(id)}/channels`);
	}

	async getAgentChannelPreferences(agentId: string): Promise<SoulAgentChannelPreferencesResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestJson(`/api/v1/soul/agents/${encodeURIComponent(id)}/channels/preferences`);
	}

	async updateAgentChannelPreferences(
		agentId: string,
		request: SoulAgentChannelPreferencesRequest
	): Promise<SoulAgentChannelPreferencesResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestJson(`/api/v1/soul/agents/${encodeURIComponent(id)}/channels/preferences`, {
			method: 'PUT',
			body: request,
		});
	}

	async resolveEns(ensName: string): Promise<SoulResolveResponse> {
		const name = ensName.trim();
		if (!name) throw new Error('ensName is required');

		return this.requestJson(`/api/v1/soul/resolve/ens/${encodeURIComponent(name)}`);
	}

	async resolveEmail(emailAddress: string): Promise<SoulResolveResponse> {
		const email = emailAddress.trim();
		if (!email) throw new Error('emailAddress is required');

		return this.requestJson(`/api/v1/soul/resolve/email/${encodeURIComponent(email)}`);
	}

	async resolvePhone(phoneNumber: string): Promise<SoulResolveResponse> {
		const phone = phoneNumber.trim();
		if (!phone) throw new Error('phoneNumber is required');

		return this.requestJson(`/api/v1/soul/resolve/phone/${encodeURIComponent(phone)}`);
	}

	async searchAgents(query: SoulSearchQuery = {}): Promise<SoulSearchResponse> {
		return this.requestJson('/api/v1/soul/search', { query });
	}

	async sendCommunication(request: SoulCommSendRequest): Promise<SoulCommSendResponse> {
		return this.requestJson('/api/v1/soul/comm/send', {
			method: 'POST',
			body: request,
		});
	}

	async getAgentCommunicationActivity(
		agentId: string,
		query: SoulAgentCommActivityQuery = {}
	): Promise<SoulAgentCommActivityResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestJson(`/api/v1/soul/agents/${encodeURIComponent(id)}/comm/activity`, {
			query,
		});
	}

	async getAgentCommunicationQueue(
		agentId: string,
		query: SoulAgentCommQueueQuery = {}
	): Promise<SoulAgentCommQueueResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestJson(`/api/v1/soul/agents/${encodeURIComponent(id)}/comm/queue`, {
			query,
		});
	}

	async getAgentCommunicationStatus(
		agentId: string,
		messageId: string
	): Promise<SoulCommStatusResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		const message = messageId.trim();
		if (!message) throw new Error('messageId is required');

		return this.requestJson(
			`/api/v1/soul/agents/${encodeURIComponent(id)}/comm/status/${encodeURIComponent(message)}`
		);
	}

	async getCommunicationStatus(messageId: string): Promise<SoulCommStatusResponse> {
		const id = messageId.trim();
		if (!id) throw new Error('messageId is required');

		return this.requestJson(`/api/v1/soul/comm/status/${encodeURIComponent(id)}`);
	}

	async startMintConversationStream(
		registrationId: string,
		request: SoulMintConversationSSEInput
	): Promise<Response> {
		const id = registrationId.trim();
		if (!id) throw new Error('registrationId is required');

		return this.requestEventStream(
			`/api/v1/soul/agents/register/${encodeURIComponent(id)}/mint-conversation`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async getMintConversation(
		registrationId: string,
		conversationId: string
	): Promise<SoulMintConversation> {
		const id = registrationId.trim();
		if (!id) throw new Error('registrationId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/register/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}`
		);
	}

	async completeMintConversation(
		registrationId: string,
		conversationId: string,
		request: SoulMintConversationCompleteRequest = {}
	): Promise<SoulMintConversation> {
		const id = registrationId.trim();
		if (!id) throw new Error('registrationId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/register/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}/complete`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async buildMintConversationFinalizePreflight(
		registrationId: string,
		conversationId: string,
		request: SoulMintConversationFinalizeBeginRequest
	): Promise<SoulMintConversationFinalizePreflightResponse> {
		const id = registrationId.trim();
		if (!id) throw new Error('registrationId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/register/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}/finalize/preflight`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async finalizeMintConversation(
		registrationId: string,
		conversationId: string,
		request: SoulMintConversationFinalizeRequest
	): Promise<SoulMintConversationFinalizeResponse> {
		const id = registrationId.trim();
		if (!id) throw new Error('registrationId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/register/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}/finalize`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async listAgentMintConversations(
		agentId: string,
		query: SoulAgentMintConversationsQuery = {}
	): Promise<SoulAgentMintConversationsResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestJson(`/api/v1/soul/agents/${encodeURIComponent(id)}/mint-conversations`, {
			query,
		});
	}

	async startAgentMintConversationStream(
		agentId: string,
		request: SoulMintConversationSSEInput
	): Promise<Response> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		return this.requestEventStream(
			`/api/v1/soul/agents/${encodeURIComponent(id)}/mint-conversation`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async getAgentMintConversation(
		agentId: string,
		conversationId: string
	): Promise<SoulMintConversation> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}`
		);
	}

	async completeAgentMintConversation(
		agentId: string,
		conversationId: string,
		request: SoulMintConversationCompleteRequest = {}
	): Promise<SoulMintConversation> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}/complete`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async buildAgentMintConversationFinalizePreflight(
		agentId: string,
		conversationId: string,
		request: SoulMintConversationFinalizeBeginRequest
	): Promise<SoulMintConversationFinalizePreflightResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}/finalize/preflight`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	async finalizeAgentMintConversation(
		agentId: string,
		conversationId: string,
		request: SoulMintConversationFinalizeRequest
	): Promise<SoulMintConversationFinalizeResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');

		const conversation = conversationId.trim();
		if (!conversation) throw new Error('conversationId is required');

		return this.requestJson(
			`/api/v1/soul/agents/${encodeURIComponent(id)}/mint-conversation/${encodeURIComponent(conversation)}/finalize`,
			{
				method: 'POST',
				body: request,
			}
		);
	}

	/**
	 * Resolve `*.lessersoul.eth` to an agentId via ENS text records if possible,
	 * otherwise fall back to lesser-host's resolve endpoint.
	 */
	async resolveEnsAgentId(options: ResolveEnsOptions): Promise<string> {
		const { ensName, rpcUrl, textKey } = options;

		if (rpcUrl) {
			const agentId = await resolveSoulAgentIdFromEnsTextRecord({
				ensName,
				rpcUrl,
				textKey,
			});
			if (agentId) return agentId;
		}

		const resolved = await this.resolveEns(ensName);
		const agentId = resolved?.agent?.agent_id?.trim();
		if (!agentId) {
			throw new Error('Resolve response missing agent_id');
		}
		return agentId;
	}

	async getAgentChannelsByEnsName(options: ResolveEnsOptions): Promise<SoulAgentChannelsResponse> {
		const agentId = await this.resolveEnsAgentId(options);
		return this.getAgentChannels(agentId);
	}

	async getAgentChannelPreferencesByEnsName(
		options: ResolveEnsOptions
	): Promise<SoulAgentChannelPreferencesResponse> {
		const agentId = await this.resolveEnsAgentId(options);
		return this.getAgentChannelPreferences(agentId);
	}

	async updateAgentChannelPreferencesByEnsName(
		options: ResolveEnsOptions,
		request: SoulAgentChannelPreferencesRequest
	): Promise<SoulAgentChannelPreferencesResponse> {
		const agentId = await this.resolveEnsAgentId(options);
		return this.updateAgentChannelPreferences(agentId, request);
	}

	private async requestJson<T>(pathname: string, options: RequestOptions = {}): Promise<T> {
		const response = await this.request(pathname, options);
		return (await response.json()) as T;
	}

	private async requestEventStream(
		pathname: string,
		options: RequestOptions = {}
	): Promise<Response> {
		return this.request(pathname, {
			...options,
			accept: 'text/event-stream',
		});
	}

	private async request(pathname: string, options: RequestOptions = {}): Promise<Response> {
		const url = buildUrl(this.baseUrl, pathname, options.query);
		const headers = mergeHeaders(this.headers, options.headers);
		if (options.accept) {
			headers.set('accept', options.accept);
		}

		let body: BodyInit | undefined;
		if (options.body !== undefined) {
			if (!headers.has('content-type')) {
				headers.set('content-type', 'application/json');
			}
			body = JSON.stringify(options.body);
		}

		const response = await this.fetch(url, {
			method: options.method,
			headers,
			body,
		});

		if (!response.ok) {
			throw await createLesserHostSoulClientError(response);
		}

		return response;
	}
}

function trimTrailingSlashes(value: string): string {
	let trimmed = value;
	while (trimmed.endsWith('/')) {
		trimmed = trimmed.slice(0, -1);
	}
	return trimmed;
}

function buildUrl(
	baseUrl: string,
	pathname: string,
	query?: Record<string, QueryParamValue>
): string {
	const url = new URL(pathname, `${baseUrl}/`);
	if (!query) {
		return url.toString();
	}

	for (const [key, value] of Object.entries(query)) {
		appendQueryParam(url.searchParams, key, value);
	}

	return url.toString();
}

function appendQueryParam(
	searchParams: URLSearchParams,
	key: string,
	value: QueryParamValue
): void {
	if (value == null) {
		return;
	}

	if (Array.isArray(value)) {
		for (const item of value) {
			if (item != null) {
				searchParams.append(key, String(item));
			}
		}
		return;
	}

	searchParams.append(key, String(value));
}

function mergeHeaders(baseHeaders: Record<string, string>, headers?: HeadersInit): Headers {
	const merged = new Headers(baseHeaders);
	if (!headers) {
		return merged;
	}

	const normalized = new Headers(headers);
	normalized.forEach((value, key) => {
		merged.set(key, value);
	});
	return merged;
}

async function createLesserHostSoulClientError(
	response: Response
): Promise<LesserHostSoulClientError> {
	const fallbackMessage = `Request failed (${response.status})`;

	let body: unknown = null;
	try {
		body = await response.json();
	} catch {
		// ignore
	}

	if (isErrorEnvelope(body)) {
		return new LesserHostSoulClientError({
			status: response.status,
			code: body.error.code,
			message: body.error.message || fallbackMessage,
			requestId: body.error.request_id,
		});
	}

	return new LesserHostSoulClientError({
		status: response.status,
		code: 'unknown',
		message: fallbackMessage,
	});
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
	if (!value || typeof value !== 'object') return false;
	const error = (value as { error?: unknown }).error;
	if (!error || typeof error !== 'object') return false;

	const record = error as Record<string, unknown>;
	return typeof record['code'] === 'string' && typeof record['message'] === 'string';
}
