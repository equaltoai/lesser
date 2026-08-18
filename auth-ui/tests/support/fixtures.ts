/**
 * Test fixtures: a served production build, a contract-shaped API stub, and a
 * Chrome WebAuthn virtual authenticator wired over CDP.
 */
import { test as base, type CDPSession, type Page } from '@playwright/test';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

import { LesserStubServer } from './lesser-stub-server.js';

const here = dirname(fileURLToPath(import.meta.url));
const DIST_DIR = resolve(here, '../../dist');

export interface VirtualAuthenticator {
  client: CDPSession;
  authenticatorId: string;
  /** Stop auto-approving user presence so a ceremony stays pending. */
  suspendPresence(): Promise<void>;
}

export interface Fixtures {
  stub: LesserStubServer;
  baseURL: string;
  authenticator: VirtualAuthenticator;
}

/**
 * Add a virtual authenticator to the page.
 *
 * `automaticPresenceSimulation: false` leaves ceremonies pending until presence
 * is simulated, which is how the cancellation test gets a deterministic window
 * in which to click Cancel.
 */
export async function addVirtualAuthenticator(
  page: Page,
  options: { automaticPresenceSimulation?: boolean } = {}
): Promise<VirtualAuthenticator> {
  const client = await page.context().newCDPSession(page);
  await client.send('WebAuthn.enable', { enableUI: false });

  const { authenticatorId } = await client.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      ctap2Version: 'ctap2_1',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: options.automaticPresenceSimulation ?? true
    }
  });

  return {
    client,
    authenticatorId,
    async suspendPresence() {
      await client.send('WebAuthn.setAutomaticPresenceSimulation', {
        authenticatorId,
        enabled: false
      });
    }
  };
}

/**
 * Inject a minimal EIP-1193 provider. There is no virtual-wallet equivalent of
 * the WebAuthn virtual authenticator, so the browser extension is the one part
 * of the wallet path that has to be stood in for.
 */
export async function installStubWallet(page: Page, address: string): Promise<void> {
  await page.addInitScript((walletAddress: string) => {
    (window as unknown as { ethereum: unknown }).ethereum = {
      isMetaMask: true,
      async request({ method }: { method: string }) {
        switch (method) {
          case 'eth_requestAccounts':
          case 'eth_accounts':
            return [walletAddress];
          case 'eth_chainId':
            return '0x1';
          case 'personal_sign':
            return `0x${'ab'.repeat(65)}`;
          default:
            throw new Error(`unsupported wallet method: ${method}`);
        }
      }
    };
  }, address);
}

export const test = base.extend<Fixtures>({
  stub: async ({}, use) => {
    if (!existsSync(DIST_DIR)) {
      throw new Error(
        `auth-ui/dist not found at ${DIST_DIR}. Run "pnpm build" before the browser tests.`
      );
    }
    const server = new LesserStubServer(DIST_DIR);
    await server.start();
    await use(server);
    await server.stop();
  },

  baseURL: async ({ stub }, use) => {
    await use(stub.origin);
  },

  authenticator: async ({ page }, use) => {
    await use(await addVirtualAuthenticator(page));
  }
});

export { expect } from '@playwright/test';
