/**
 * Browser-level coverage for the M4 exit criterion: a signed-in person manages
 * the ways they can sign in — adds a second passkey through a real WebAuthn
 * ceremony, renames it, removes it, is honestly refused when they try to remove
 * their last one, links and unlinks a wallet, and is never shown credential
 * material.
 *
 * Everything runs against the production build in a real browser. The only
 * stand-ins are the API (a contract-shaped stub that enforces the bearer token,
 * the ceremony binding and the last-authenticator invariant) and the wallet
 * extension, which has no virtual equivalent.
 */
import { test, expect, installStubWallet } from './support/fixtures.js';
import type { LesserStubServer } from './support/lesser-stub-server.js';
import type { Page } from '@playwright/test';

const OAUTH_QUERY =
  'client_id=test-client&redirect_uri=https%3A%2F%2Fclient.example%2Fcb&state=xyz789&response_type=code&scope=read+write';

const WALLET_ADDRESS = '0x1234567890abcdef1234567890abcdef12345678';

/**
 * Sign up with a passkey and land on the credential page, authenticated.
 *
 * Registration is the only way to obtain a token here, and it is the honest
 * one: it runs the real ceremony and leaves the JWT in sessionStorage exactly
 * as production does, so the page under test is reached the way a person
 * reaches it.
 */
async function signUpAndOpenCredentials(page: Page, baseURL: string, username: string) {
  await page.goto(`${baseURL}/auth/register?${OAUTH_QUERY}`);
  await page.getByLabel('Username').fill(username);
  await page.getByTestId('passkey-button').click();
  await page.waitForURL(/\/auth\/consent/);

  await page.goto(`${baseURL}/auth/credentials`);
  await expect(page.getByTestId('credential-account')).toContainText(username);
}

/** Rows currently rendered in the passkey list. */
function passkeyRows(page: Page) {
  return page.getByTestId('passkey-row');
}

test.describe('credential management', () => {
  test('a signed-in person sees their authenticators and no credential material', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    // Capture what the server actually sends, so "the key is not on the page"
    // is a claim about redaction rather than about an empty payload.
    let walletListBody = '';
    let credentialListBody = '';
    page.on('response', async (response) => {
      const url = response.url();
      if (url.endsWith('/auth/wallet/list')) {
        walletListBody = await response.text();
      }
      if (url.endsWith('/api/v1/auth/webauthn/credentials')) {
        credentialListBody = await response.text();
      }
    });

    await signUpAndOpenCredentials(page, baseURL, 'ada');
    stub.seedWallet('ada', WALLET_ADDRESS);
    await page.reload();

    await expect(passkeyRows(page)).toHaveCount(1);
    await expect(page.getByTestId('wallet-row')).toHaveCount(1);
    await expect(page.getByTestId('passkey-count')).toContainText('1 of 10');

    // The wallet response really does carry a public key...
    expect(walletListBody).toContain('public_key');
    const publicKey = JSON.parse(walletListBody).wallets[0].public_key as string;
    expect(publicKey.length).toBeGreaterThan(16);

    // ...and none of it reaches the page, in text or in markup.
    const rendered = await page.content();
    expect(rendered).not.toContain(publicKey);
    expect(rendered).not.toContain('public_key');
    expect(await page.getByTestId('wallet-address').innerText()).not.toContain(publicKey);

    // The passkey summary contract carries no key material at all.
    expect(credentialListBody).not.toContain('public_key');
    expect(credentialListBody).not.toContain('publicKey');

    // The address is shown truncated — it is a public identifier, not a secret,
    // but the full string is not needed to recognise it.
    await expect(page.getByTestId('wallet-address')).toContainText('0x12345678');
    await expect(page.getByTestId('wallet-address')).not.toContainText(WALLET_ADDRESS);
  });

  test('adding a second passkey runs the full authenticated ceremony', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'grace');
    await expect(passkeyRows(page)).toHaveCount(1);

    const before = stub.requestLog.length;
    await page.getByTestId('add-passkey-button').click();

    await expect(passkeyRows(page)).toHaveCount(2);
    await expect(page.getByTestId('credential-notice')).toContainText('Passkey added');
    expect(stub.passkeyNames('grace')).toHaveLength(2);

    // It used the AUTHENTICATED registration endpoints, not the signup ceremony
    // that creates accounts.
    const calls = stub.requestLog.slice(before).map((entry) => `${entry.method} ${entry.path}`);
    expect(calls).toContain('POST /api/v1/auth/webauthn/register/begin');
    expect(calls).toContain('POST /api/v1/auth/webauthn/register/finish');
    expect(calls.some((call) => call.includes('/signup/'))).toBe(false);
    expect(calls).toContain('GET /api/v1/auth/webauthn/credentials');
  });

  test('renaming a passkey persists across a reload', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'hopper');

    await page.getByTestId('rename-passkey-button').first().click();
    const input = page.getByTestId('rename-input');
    await expect(input).toBeVisible();
    // The field is labelled, so it is reachable by name and not just by test id.
    await expect(page.getByLabel('Passkey name')).toBeVisible();

    await input.fill('Work laptop');
    await page.getByTestId('rename-save-button').click();

    await expect(page.getByTestId('passkey-name').first()).toHaveText('Work laptop');
    expect(stub.passkeyNames('hopper')).toEqual(['Work laptop']);

    // Persisted server-side, not just in component state.
    await page.reload();
    await expect(page.getByTestId('passkey-name').first()).toHaveText('Work laptop');
  });

  test('removing the last authenticator is refused with the invariant message', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'turing');
    await expect(passkeyRows(page)).toHaveCount(1);

    await page.getByTestId('remove-passkey-button').first().click();

    const errorRegion = page.getByTestId('credential-error');
    // The message names the reason and the way out. It is not a generic failure.
    await expect(errorRegion).toContainText('last way to sign in');
    await expect(errorRegion).toContainText('Add another passkey or link a wallet first');
    await expect(errorRegion).toBeFocused();

    // Distinctness: this must not read like a transport failure or a 500.
    await expect(errorRegion).not.toContainText('could not reach the server');
    await expect(errorRegion).not.toContainText('try again in a moment');
    await expect(errorRegion).not.toContainText('already been removed');

    // Nothing was removed, and the list still says so.
    await expect(passkeyRows(page)).toHaveCount(1);
    expect(stub.passkeyNames('turing')).toHaveLength(1);
  });

  test('a passkey that was already removed reports differently from the invariant', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'lovelace');
    // A second authenticator, so the invariant is not what answers.
    stub.seedWallet('lovelace', WALLET_ADDRESS);
    await page.reload();
    // Wait for the island to have finished reading, or the removal below would
    // race the page's own load and there would be nothing stale to click.
    await expect(passkeyRows(page)).toHaveCount(1);

    // Remove it out from under the page: the rendered list is now stale, which
    // is exactly the situation 404 exists for.
    removeEveryPasskeyBehindThePage(stub, 'lovelace');

    await page.getByTestId('remove-passkey-button').first().click();

    const errorRegion = page.getByTestId('credential-error');
    await expect(errorRegion).toContainText('already been removed');
    await expect(errorRegion).toContainText('up to date');
    await expect(errorRegion).not.toContainText('last way to sign in');

    // The stale row is gone, because a 404 triggers a re-read.
    await expect(passkeyRows(page)).toHaveCount(0);
  });

  test('a server failure is reported as a failure, not as the invariant', async ({
    page,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'noether');

    // The server's genuine-failure arm.
    await page.route('**/api/v1/auth/webauthn/credentials/*', async (route) => {
      if (route.request().method() !== 'DELETE') {
        return route.continue();
      }
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'failed to delete credential', error_code: 'INTERNAL_ERROR' })
      });
    });

    await page.getByTestId('remove-passkey-button').first().click();

    const errorRegion = page.getByTestId('credential-error');
    await expect(errorRegion).toContainText('try again in a moment');
    await expect(errorRegion).toContainText('probably still there');
    // The three arms stay apart.
    await expect(errorRegion).not.toContainText('last way to sign in');
    await expect(errorRegion).not.toContainText('already been removed');
  });

  test('a transport failure is reported as a transport failure', async ({
    page,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'franklin');

    await page.route('**/api/v1/auth/webauthn/credentials/*', async (route) => {
      if (route.request().method() !== 'DELETE') {
        return route.continue();
      }
      await route.abort('failed');
    });

    await page.getByTestId('remove-passkey-button').first().click();

    const errorRegion = page.getByTestId('credential-error');
    await expect(errorRegion).toContainText('could not reach the server');
    await expect(errorRegion).not.toContainText('last way to sign in');
    await expect(errorRegion).not.toContainText('already been removed');
  });

  test('a wallet can be linked and unlinked from the management page', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();
    await installStubWallet(page, WALLET_ADDRESS);

    await signUpAndOpenCredentials(page, baseURL, 'satoshi');
    await expect(page.getByTestId('wallets-empty')).toBeVisible();

    const before = stub.requestLog.length;
    await page.getByTestId('link-wallet-button').click();

    await expect(page.getByTestId('wallet-row')).toHaveCount(1);
    await expect(page.getByTestId('credential-notice')).toContainText('Wallet linked');
    expect(stub.walletAddresses('satoshi')).toEqual([WALLET_ADDRESS]);

    const linkCalls = stub.requestLog.slice(before).map((entry) => `${entry.method} ${entry.path}`);
    expect(linkCalls).toContain('POST /auth/wallet/challenge');
    expect(linkCalls).toContain('POST /auth/wallet/link');

    await page.getByTestId('unlink-wallet-button').click();

    await expect(page.getByTestId('wallets-empty')).toBeVisible();
    await expect(page.getByTestId('credential-notice')).toContainText('Wallet unlinked');
    expect(stub.walletAddresses('satoshi')).toEqual([]);
  });

  test('the last wallet cannot be unlinked either', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'curie');
    stub.seedWallet('curie', WALLET_ADDRESS);
    await page.reload();
    await expect(page.getByTestId('wallet-row')).toHaveCount(1);

    // Leave the wallet as the only authenticator.
    await page.getByTestId('remove-passkey-button').first().click();
    await expect(passkeyRows(page)).toHaveCount(0);

    await page.getByTestId('unlink-wallet-button').click();

    const errorRegion = page.getByTestId('credential-error');
    await expect(errorRegion).toContainText('last way to sign in');
    await expect(errorRegion).toBeFocused();
    await expect(page.getByTestId('wallet-row')).toHaveCount(1);
    expect(stub.walletAddresses('curie')).toEqual([WALLET_ADDRESS]);
  });

  test('the page is usable without a session and does not call the API', async ({
    page,
    stub,
    baseURL
  }) => {
    await page.goto(`${baseURL}/auth/credentials`);

    await expect(page.getByTestId('credentials-signed-out')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Sign in' })).toBeVisible();

    // No token, so no authenticated call was attempted at all.
    expect(stub.pathsCalled().some((call) => call.includes('/webauthn/credentials'))).toBe(false);
    expect(stub.pathsCalled().some((call) => call.includes('/wallet/list'))).toBe(false);
  });

  test('every control carries an accessible name that says what it does', async ({
    page,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await signUpAndOpenCredentials(page, baseURL, 'meitner');

    await expect(page.getByTestId('add-passkey-button')).toHaveAccessibleName(
      'Add a passkey to your account'
    );
    await expect(page.getByTestId('link-wallet-button')).toHaveAccessibleName(
      'Link a crypto wallet to your account'
    );
    await expect(page.getByTestId('rename-passkey-button').first()).toHaveAccessibleName(
      /^Rename passkey /
    );
    await expect(page.getByTestId('remove-passkey-button').first()).toHaveAccessibleName(
      /^Remove passkey /
    );

    // Both sections are headed, so the list is navigable by landmark/heading.
    await expect(page.getByRole('heading', { name: 'Passkeys' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Wallets' })).toBeVisible();
  });
});

/**
 * Delete an account's passkeys server-side, bypassing the page, so the rendered
 * list becomes stale. Uses the stub's own state rather than the HTTP surface,
 * because the HTTP surface would refuse the last one.
 */
function removeEveryPasskeyBehindThePage(stub: LesserStubServer, username: string): void {
  const account = stub.getAccount(username);
  if (!account) {
    throw new Error(`no stub account for ${username}`);
  }
  account.credentialIds = [];
}
