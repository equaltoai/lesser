/**
 * Browser-level coverage for the M3 exit criterion: a person clicks
 * "Register Passkey", completes a real WebAuthn ceremony against Chrome's
 * virtual authenticator, and lands logged in with a passkey-only account.
 *
 * These tests drive the production build of the component through a real
 * browser. The only stand-ins are the API (a contract-shaped stub that
 * validates the ceremony binding) and the wallet extension (no virtual
 * equivalent exists). The WebAuthn ceremony itself is real.
 */
import { createHash } from 'node:crypto';

import { test, expect, addVirtualAuthenticator, installStubWallet } from './support/fixtures.js';
import type { Page } from '@playwright/test';

const OAUTH_QUERY =
  'client_id=test-client&redirect_uri=https%3A%2F%2Fclient.example%2Fcb&state=xyz789&response_type=code&scope=read+write';

function registerUrl(baseURL: string): string {
  return `${baseURL}/auth/register?${OAUTH_QUERY}`;
}

function decodeBase64Url(value: string): Buffer {
  return Buffer.from(value.replace(/-/g, '+').replace(/_/g, '/'), 'base64');
}

function encodeBase64Url(value: Buffer): string {
  return value.toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** Flip one byte of a base64url payload, leaving its length and framing intact. */
function tamperByte(value: string, index: number): string {
  const bytes = decodeBase64Url(value);
  const at = index < 0 ? bytes.length + index : index;
  bytes[at] ^= 0xff;
  return encodeBase64Url(bytes);
}

/**
 * Rewrite one field of a ceremony-finish request on the wire, so a serializer
 * defect can be simulated without touching the component.
 */
async function tamperFinishRequest(
  page: Page,
  urlGlob: string,
  mutate: (response: Record<string, string>) => void
): Promise<void> {
  await page.route(urlGlob, async (route) => {
    const body = JSON.parse(route.request().postData() ?? '{}') as {
      response?: { response?: Record<string, string> };
    };
    const inner = body.response?.response;
    if (!inner) {
      throw new Error('ceremony request had no response payload to tamper with');
    }
    mutate(inner);
    await route.continue({ postData: JSON.stringify(body) });
  });
}

test.describe('passkey-first registration', () => {
  test('happy path: Register Passkey creates the account and signs in — no wallet involved', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('ada');
    await page.getByTestId('passkey-button').click();

    // Landing point is the OAuth continuation the stub hands back.
    await page.waitForURL(/\/auth\/consent/);

    // The account exists and owns the credential minted during the ceremony.
    const account = stub.getAccount('ada');
    expect(account).toBeDefined();
    expect(account?.credentialIds.length).toBe(1);

    // The signup ceremony ran; the LOGIN-begin call that used to be the bug
    // only appears after the account exists, as the sign-in step.
    const calls = stub.pathsCalled();
    expect(calls).toContain('POST /api/v1/auth/webauthn/signup/begin');
    expect(calls).toContain('POST /api/v1/auth/webauthn/signup/finish');
    expect(calls).toContain('POST /api/v1/accounts');
    expect(calls.indexOf('POST /api/v1/auth/webauthn/signup/begin')).toBeLessThan(
      calls.indexOf('POST /api/v1/auth/webauthn/login/begin')
    );

    // No wallet endpoint was touched: passkey signup requires no extension.
    expect(calls.some((call) => call.includes('/auth/wallet/'))).toBe(false);

    // Signed in: a JWT was issued and handed to the OAuth continuation.
    const jwt = await page.evaluate(() => sessionStorage.getItem('lesser_auth_jwt'));
    expect(jwt).toContain('stub-jwt-ada');
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_oauth_auth_request'))).toBeNull();
  });

  test('OAuth continuation state survives the whole ceremony', async ({
    page,
    baseURL,
    stub,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    await page.goto(registerUrl(baseURL));

    // Captured on mount, before any ceremony runs. The island hydrates
    // asynchronously (client:only), so poll rather than read once.
    await expect
      .poll(() => page.evaluate(() => sessionStorage.getItem('lesser_oauth_auth_request')))
      .toContain('client_id=test-client');
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_oauth_auth_request'))).toContain(
      'state=xyz789'
    );

    await page.getByLabel('Username').fill('grace');
    await page.getByTestId('passkey-button').click();
    await page.waitForURL(/\/auth\/consent/);

    // The authorize call carried the OAuth params through the ceremony rather
    // than dropping them, so the flow can continue where it left off.
    expect(page.url()).toContain('state=xyz789');
    expect(stub.pathsCalled()).toContain('GET /oauth/authorize');
  });

  test('cancel path: aborting the ceremony leaves the form usable and creates nothing', async ({
    page,
    stub,
    baseURL
  }) => {
    // Presence is never auto-simulated, so create() stays pending until we cancel.
    const authenticator = await addVirtualAuthenticator(page, {
      automaticPresenceSimulation: false
    });
    expect(authenticator.authenticatorId).toBeTruthy();

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('hopper');
    await page.getByTestId('passkey-button').click();

    const cancelButton = page.getByTestId('cancel-ceremony-button');
    await expect(cancelButton).toBeVisible();
    await cancelButton.click();

    const errorRegion = page.getByTestId('auth-error');
    await expect(errorRegion).toContainText('cancelled');
    await expect(errorRegion).toContainText('not created');

    // Not stranded: focus is on the message, the button works again, the
    // in-flight status is cleared, and no account or token exists.
    await expect(errorRegion).toBeFocused();
    await expect(page.getByTestId('passkey-button')).toBeEnabled();
    await expect(page.getByTestId('cancel-ceremony-button')).toHaveCount(0);
    await expect(page.getByTestId('auth-status')).toHaveText('');

    expect(stub.getAccount('hopper')).toBeUndefined();
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_auth_jwt'))).toBeNull();

    // The OAuth continuation state is still there to retry with.
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_oauth_auth_request'))).toContain(
      'client_id=test-client'
    );

    // Cancellation happened before any account write was attempted.
    expect(stub.pathsCalled()).not.toContain('POST /api/v1/accounts');
  });

  test('error path: a taken username is distinguishable from an invalid proof', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();
    stub.usernameTakenOverride = 'taken';

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('taken');
    await page.getByTestId('passkey-button').click();

    const errorRegion = page.getByTestId('auth-error');
    await expect(errorRegion).toContainText('already taken');
    await expect(errorRegion).toContainText('Choose a different username');
    await expect(errorRegion).toBeFocused();

    // Registration failures must not bounce a registering person to /login.
    await expect(page).toHaveURL(/\/auth\/register/);
    await expect(page.getByTestId('passkey-button')).toBeEnabled();

    // The message is specific to the conflict, not the generic proof failure.
    await expect(errorRegion).not.toContainText('expired');
  });

  test('error path: an expired registration proof reports differently from a taken username', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    // Drop the proof between signup/finish and account creation so
    // /api/v1/accounts answers with the real 422 proof rejection.
    await page.route('**/api/v1/auth/webauthn/signup/finish', async (route) => {
      const response = await route.fetch();
      const body = await response.json();
      await route.fulfill({
        response,
        json: { ...body, passkey_registration_proof: 'a-proof-that-was-never-issued' }
      });
    });

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('lovelace');
    await page.getByTestId('passkey-button').click();

    const errorRegion = page.getByTestId('auth-error');
    await expect(errorRegion).toContainText('expired');
    await expect(errorRegion).not.toContainText('already taken');
    await expect(errorRegion).toBeFocused();

    expect(stub.getAccount('lovelace')).toBeUndefined();
  });

  test('passkey login for an existing account still uses the login endpoints', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    // Register first so the account owns a credential.
    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('turing');
    await page.getByTestId('passkey-button').click();
    await page.waitForURL(/\/auth\/consent/);

    const afterSignup = stub.requestLog.length;

    // Now sign in from the login page with the same authenticator.
    await page.goto(`${baseURL}/auth/login?${OAUTH_QUERY}`);
    await page.getByLabel('Username').fill('turing');
    await page.getByTestId('passkey-button').click();
    await page.waitForURL(/\/auth\/consent/);

    const loginCalls = stub.requestLog.slice(afterSignup).map((e) => `${e.method} ${e.path}`);
    expect(loginCalls).toContain('POST /api/v1/auth/webauthn/login/begin');
    expect(loginCalls).toContain('POST /api/v1/auth/webauthn/login/finish');
    // Signing in must never run the signup ceremony.
    expect(loginCalls.some((call) => call.includes('/signup/'))).toBe(false);
  });
});

/**
 * These pin the stub's cryptographic verification. Without it, the suite could
 * not tell a working assertion serializer from a broken one: the happy path
 * would pass with a constant signature on the wire.
 *
 * Each mutation is applied to the request the component actually sends, so the
 * component stays untouched and the check being exercised is the server-side
 * one.
 */
test.describe('the suite rejects a forged ceremony', () => {
  const CONSTANT_SIGNATURE = 'not-a-real-signature';

  test('a constant assertion signature fails the registration happy path', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    // Exactly the mutation the review proposed: replace whatever the
    // authenticator signed with a constant, non-empty string.
    await tamperFinishRequest(page, '**/api/v1/auth/webauthn/login/finish', (response) => {
      response.signature = CONSTANT_SIGNATURE;
    });

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('curie');
    await page.getByTestId('passkey-button').click();

    const errorRegion = page.getByTestId('auth-error');
    await expect(errorRegion).toBeVisible();
    await expect(errorRegion).not.toHaveText('');
    await expect(errorRegion).toBeFocused();

    // The forged assertion is what stopped it: everything up to the sign-in
    // step succeeded, so the account exists and owns the credential...
    const account = stub.getAccount('curie');
    expect(account).toBeDefined();
    expect(account?.credentialIds.length).toBe(1);
    expect(stub.pathsCalled()).toContain('POST /api/v1/auth/webauthn/login/finish');

    // ...but no token was issued and the OAuth continuation never ran.
    await expect(page).toHaveURL(/\/auth\/register/);
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_auth_jwt'))).toBeNull();
    expect(stub.pathsCalled()).not.toContain('GET /oauth/authorize');
  });

  test('a single flipped byte in a real assertion signature is rejected at sign-in', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    // Register cleanly first, so the rejection below can only be the signature.
    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('noether');
    await page.getByTestId('passkey-button').click();
    await page.waitForURL(/\/auth\/consent/);

    // sessionStorage survives the navigation, so drop the token registration
    // just issued: whatever is there after the next ceremony came from it.
    await page.evaluate(() => sessionStorage.removeItem('lesser_auth_jwt'));
    const afterSignup = stub.requestLog.length;

    // A well-formed signature of the right length over the right structure,
    // differing from the authenticator's by one byte. Only a real signature
    // check can tell this apart from the genuine article.
    await tamperFinishRequest(page, '**/api/v1/auth/webauthn/login/finish', (response) => {
      response.signature = tamperByte(response.signature, -1);
    });

    await page.goto(`${baseURL}/auth/login?${OAUTH_QUERY}`);
    await page.getByLabel('Username').fill('noether');
    await page.getByTestId('passkey-button').click();

    const errorRegion = page.getByTestId('auth-error');
    await expect(errorRegion).toContainText('That passkey was not accepted');
    await expect(page).toHaveURL(/\/auth\/login/);
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_auth_jwt'))).toBeNull();

    // The tampered assertion did reach the server and was refused there.
    const loginCalls = stub.requestLog.slice(afterSignup).map((e) => `${e.method} ${e.path}`);
    expect(loginCalls).toContain('POST /api/v1/auth/webauthn/login/finish');
  });

  test('an attestation bound to a different RP ID is rejected at signup', async ({
    page,
    stub,
    baseURL,
    authenticator
  }) => {
    expect(authenticator.authenticatorId).toBeTruthy();

    // The RP ID hash is the first 32 bytes of the authenticator data, which
    // sits verbatim inside the attestation object. Flipping a byte of it in
    // place keeps the CBOR framing valid, so only the RP binding changes.
    const rpIdHash = createHash('sha256').update('localhost').digest();
    await tamperFinishRequest(page, '**/api/v1/auth/webauthn/signup/finish', (response) => {
      const attestation = decodeBase64Url(response.attestationObject);
      const at = attestation.indexOf(rpIdHash);
      if (at < 0) {
        throw new Error('attestation object did not contain the expected RP ID hash');
      }
      attestation[at] ^= 0xff;
      response.attestationObject = encodeBase64Url(attestation);
    });

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('franklin');
    await page.getByTestId('passkey-button').click();

    const errorRegion = page.getByTestId('auth-error');
    await expect(errorRegion).toContainText('attestation rejected');

    // Rejected before any account write, so nothing was created.
    expect(stub.getAccount('franklin')).toBeUndefined();
    expect(stub.pathsCalled()).not.toContain('POST /api/v1/accounts');
    expect(await page.evaluate(() => sessionStorage.getItem('lesser_auth_jwt'))).toBeNull();
  });
});

test.describe('wallet registration remains an equal alternative', () => {
  test('wallet-first signup is unchanged by the passkey work', async ({ page, stub, baseURL }) => {
    await installStubWallet(page, '0x1234567890abcdef1234567890abcdef12345678');

    await page.goto(registerUrl(baseURL));
    await page.getByLabel('Username').fill('satoshi');
    await page.getByTestId('wallet-button').click();

    await page.waitForURL(/\/auth\/consent/);

    const account = stub.getAccount('satoshi');
    expect(account).toBeDefined();
    // Wallet signup creates no passkey credential.
    expect(account?.credentialIds.length).toBe(0);

    const calls = stub.pathsCalled();
    expect(calls).toContain('POST /auth/wallet/challenge');
    expect(calls).toContain('POST /auth/wallet/verify');
    expect(calls).toContain('POST /api/v1/accounts');
    expect(calls).toContain('POST /auth/wallet/link');
    // The wallet path must not have been rerouted through the passkey ceremony.
    expect(calls.some((call) => call.includes('/webauthn/'))).toBe(false);

    expect(await page.evaluate(() => sessionStorage.getItem('lesser_auth_jwt'))).toContain(
      'stub-jwt-wallet-satoshi'
    );
  });

  test('the register page offers both paths without requiring a wallet extension', async ({
    page,
    baseURL
  }) => {
    // No window.ethereum installed at all.
    await page.goto(registerUrl(baseURL));

    const passkeyButton = page.getByTestId('passkey-button');
    const walletButton = page.getByTestId('wallet-button');

    await expect(passkeyButton).toBeEnabled();
    await expect(passkeyButton).toHaveText(/Register Passkey/);
    await expect(walletButton).toHaveText(/Register Wallet/);

    // Accessible names describe the outcome, not just the mechanism.
    await expect(passkeyButton).toHaveAccessibleName('Register a passkey and create your account');
    await expect(walletButton).toHaveAccessibleName(
      'Register a crypto wallet and create your account'
    );

    // Both paths share the same labelled, required username field.
    const username = page.getByLabel('Username');
    await expect(username).toBeVisible();
    await expect(username).toHaveAttribute('required', '');
  });
});
