import { getEnsText } from 'viem/actions';
import { createPublicClient, http } from 'viem';
import { mainnet } from 'viem/chains';

export interface ResolveSoulAgentIdFromEnsTextRecordOptions {
	ensName: string;
	rpcUrl: string;
	textKey?: string;
}

export async function resolveSoulAgentIdFromEnsTextRecord(
	options: ResolveSoulAgentIdFromEnsTextRecordOptions
): Promise<string | null> {
	const { ensName, rpcUrl, textKey = 'soul.agentId' } = options;
	const name = ensName.trim();
	if (!name) return null;
	if (!rpcUrl.trim()) return null;

	try {
		const client = createPublicClient({
			chain: mainnet,
			transport: http(rpcUrl),
		});
		const value = await getEnsText(client, { name, key: textKey });
		if (typeof value !== 'string') return null;

		const trimmed = value.trim();
		return trimmed ? trimmed : null;
	} catch {
		return null;
	}
}
