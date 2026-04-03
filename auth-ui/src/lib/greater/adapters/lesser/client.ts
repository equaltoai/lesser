import type { components } from '../rest/generated/lesser-api.js';
import { resolveFetchLike, type FetchLike } from '../fetch.js';

export type LesserSoulAgentIdentity = components['schemas']['SoulAgentIdentity'];
export type LesserSoulAgentBinding = components['schemas']['SoulAgentBinding'];
export type LesserSoulBodyBinding = LesserSoulAgentBinding;
export type LesserSoulInventoryItem = components['schemas']['SoulInventoryItem'];
export type LesserSoulsMineResponse = components['schemas']['SoulsMineResponse'];
export type LesserSoulIncorporateResponse = components['schemas']['SoulIncorporateResponse'];

export type LesserFetchLike = FetchLike;

export interface LesserSoulClientConfig {
	baseUrl: string;
	fetch?: LesserFetchLike;
	headers?: Record<string, string>;
}

export class LesserSoulClientError extends Error {
	readonly status: number;
	readonly details?: unknown;

	constructor(options: { status: number; message: string; details?: unknown }) {
		super(options.message);
		this.name = 'LesserSoulClientError';
		this.status = options.status;
		this.details = options.details;
	}
}

interface RequestJsonOptions {
	method?: 'GET' | 'POST';
	headers?: HeadersInit;
	body?: BodyInit | null;
}

export function createLesserSoulClient(config: LesserSoulClientConfig): LesserSoulClient {
	return new LesserSoulClient(config);
}

export class LesserSoulClient {
	private readonly baseUrl: string;
	private readonly fetch: LesserFetchLike;
	private readonly headers: Record<string, string>;

	constructor(config: LesserSoulClientConfig) {
		const baseUrl = config.baseUrl.trim();
		if (!baseUrl) {
			throw new Error('LesserSoulClient requires baseUrl');
		}

		this.baseUrl = trimTrailingSlashes(baseUrl);
		this.fetch = resolveFetchLike(config.fetch);
		this.headers = {
			accept: 'application/json',
			...(config.headers ?? {}),
		};
	}

	async getMySouls(): Promise<LesserSoulsMineResponse> {
		return this.requestJson('/api/v1/souls/mine');
	}

	async incorporateSoul(
		agentId: string,
		targetAgentUsername: string
	): Promise<LesserSoulIncorporateResponse> {
		const id = agentId.trim();
		if (!id) throw new Error('agentId is required');
		const targetUsername = targetAgentUsername.trim();
		if (!targetUsername) throw new Error('targetAgentUsername is required');

		return this.requestJson(`/api/v1/souls/${encodeURIComponent(id)}/incorporate`, {
			method: 'POST',
			headers: {
				'content-type': 'application/json',
			},
			body: JSON.stringify({
				target_agent_username: targetUsername,
			}),
		});
	}

	private async requestJson<T>(pathname: string, options: RequestJsonOptions = {}): Promise<T> {
		const response = await this.fetch(`${this.baseUrl}${pathname}`, {
			method: options.method,
			headers: mergeHeaders(this.headers, options.headers),
			body: options.body,
		});

		if (!response.ok) {
			throw await createLesserSoulClientError(response);
		}

		return (await response.json()) as T;
	}
}

function trimTrailingSlashes(value: string): string {
	let trimmed = value;
	while (trimmed.endsWith('/')) {
		trimmed = trimmed.slice(0, -1);
	}
	return trimmed;
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

async function createLesserSoulClientError(response: Response): Promise<LesserSoulClientError> {
	const fallbackMessage = `Request failed (${response.status})`;

	let details: unknown;
	let message = fallbackMessage;

	try {
		details = await response.json();
	} catch {
		try {
			details = await response.text();
		} catch {
			details = undefined;
		}
	}

	if (typeof details === 'string' && details.trim().length > 0) {
		message = details.trim();
	} else if (details && typeof details === 'object') {
		const record = details as Record<string, unknown>;
		const nestedError =
			record['error'] && typeof record['error'] === 'object'
				? (record['error'] as Record<string, unknown>)
				: null;

		if (typeof record['message'] === 'string' && record['message'].trim().length > 0) {
			message = record['message'];
		} else if (typeof record['error'] === 'string' && record['error'].trim().length > 0) {
			message = record['error'];
		} else if (
			nestedError &&
			typeof nestedError['message'] === 'string' &&
			nestedError['message'].trim().length > 0
		) {
			message = nestedError['message'];
		}
	}

	return new LesserSoulClientError({
		status: response.status,
		message,
		details,
	});
}
