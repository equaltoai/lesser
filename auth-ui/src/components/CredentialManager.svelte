<script lang="ts">
  /**
   * CredentialManager — first-party management of an account's authenticators.
   *
   * ⚠️ STATELESS STATIC APPLICATION - NO SESSIONS OR COOKIES PERMITTED ⚠️
   *
   * The person is already signed in; the JWT issued by the login ceremony lives
   * in sessionStorage for the length of the visit and is sent as a bearer token.
   * Nothing here creates a session or a cookie.
   *
   * What it manages:
   *  - Passkeys: list, add (a real WebAuthn registration ceremony against the
   *    AUTHENTICATED endpoints, /auth/webauthn/register/{begin,finish}), rename,
   *    remove.
   *  - Wallets: list, link (challenge + personal_sign + /auth/wallet/link),
   *    unlink.
   *
   * Two rules shape everything below.
   *
   * 1. NO CREDENTIAL MATERIAL IS EVER DISPLAYED. The passkey list endpoint
   *    returns only id/name/timestamps by contract. The wallet list endpoint
   *    DOES return a `public_key` field; it is never read, never rendered, and
   *    never logged. Only the address (truncated), chain and timestamps are
   *    shown. A wallet address is a public identifier the person chose to link;
   *    a public key is credential material.
   *
   * 2. REMOVAL FAILURES ARE REPORTED HONESTLY AND DISTINCTLY. The server's
   *    last-authenticator invariant has a specific error contract, and the three
   *    outcomes mean completely different things to the person in front of it:
   *      400 "cannot delete last authentication method" -> the invariant refused
   *          it; the account would have been left unreachable.
   *      404 "credential not found" / "wallet not found" -> it was already gone;
   *          the list was stale, and re-reading it is the whole remedy.
   *      500 (or a transport failure) -> the server could not complete it; the
   *          authenticator is probably still there and it is worth retrying.
   *    Collapsing these into one "removal failed" string would tell someone who
   *    just locked themselves out of a retry that they had lost a passkey, and
   *    tell someone whose network blipped that their account is down to its last
   *    authenticator. The mapping is pinned by tests.
   */

  import { onMount, tick } from 'svelte';
  import type { components } from 'src/lib/greater/adapters/rest/generated/lesser-api.js';
  import Button from 'src/lib/greater/primitives/components/Button.svelte';
  import TextField from 'src/lib/greater/primitives/components/TextField.svelte';
  import Alert from 'src/lib/greater/primitives/components/Alert.svelte';
  import Heading from 'src/lib/greater/primitives/components/Heading.svelte';
  import Text from 'src/lib/greater/primitives/components/Text.svelte';
  import KeyIcon from 'src/lib/greater/icons/icons/key.svelte';
  import WalletIcon from 'src/lib/greater/icons/icons/wallet.svelte';
  import { truncateMiddle } from 'src/lib/greater/utils';

  // Single-domain CloudFront routes API + UI by path; PUBLIC_LESSER_API_ORIGIN
  // is the local-dev escape hatch, exactly as in PasswordlessLogin.
  const API_URL = import.meta.env.PUBLIC_LESSER_API_ORIGIN || window.location.origin;
  const UI_BASE_PATH = (import.meta.env.BASE_URL || '/').replace(/\/$/, '');
  const loginHref = `${UI_BASE_PATH}/login`;

  /**
   * Server-side ceiling on passkeys per account (pkg/auth/webauthn.go,
   * MaxCredentialsPerUser). Reaching it is refused at FinishRegistration, so the
   * button is disabled at the limit rather than starting a ceremony that cannot
   * land.
   */
  const MAX_PASSKEYS = 10;

  // Shapes come from the generated OpenAPI client so UI and contract cannot
  // drift silently.
  type ApiErrorBody = components['schemas']['Error'];
  type Account = components['schemas']['Account'];
  type WebAuthnBeginResponse = components['schemas']['WebAuthnBeginResponse'];
  type WebAuthnCredentialSummary = components['schemas']['WebAuthnCredentialSummary'];
  type WebAuthnCredentialsResponse = components['schemas']['WebAuthnCredentialsResponse'];
  type WebAuthnFinishRegistrationRequest = components['schemas']['WebAuthnFinishRegistrationRequest'];
  type WebAuthnUpdateCredentialRequest = components['schemas']['WebAuthnUpdateCredentialRequest'];
  type MessageResponse = components['schemas']['MessageResponse'];
  type WalletChallengeRequest = components['schemas']['WalletChallengeRequest'];
  type StorageWalletChallenge = components['schemas']['StorageWalletChallenge'];
  type WalletLinkRequest = components['schemas']['WalletLinkRequest'];
  type WalletLinkResponse = components['schemas']['WalletLinkResponse'];
  type WalletListResponse = components['schemas']['WalletListResponse'];
  type WalletUnlinkResponse = components['schemas']['WalletUnlinkResponse'];
  type StorageWalletCredential = components['schemas']['StorageWalletCredential'];

  /**
   * A linked wallet, reduced at the transport boundary to the fields this UI is
   * allowed to know about. `public_key` and anything else the server sends is
   * dropped here so no later change can render it by accident.
   */
  interface WalletView {
    address: string;
    chainId: number | null;
    walletType: string;
    linkedAt: string;
    lastUsed: string;
  }

  /** Error carrying the server's `Error` contract fields so callers can map status+code to advice. */
  class ApiRequestError extends Error {
    readonly status: number;
    readonly errorCode: string;
    readonly serverMessage: string;

    constructor(status: number, body: Partial<ApiErrorBody> | null) {
      const serverMessage = (body?.error ?? '').trim();
      super(serverMessage || `Request failed (${status})`);
      this.name = 'ApiRequestError';
      this.status = status;
      this.errorCode = (body?.error_code ?? '').trim();
      this.serverMessage = serverMessage;
    }
  }

  /**
   * A request never reached the server (offline, DNS, CORS, TLS). This is not
   * the same as the server refusing the operation, and must not be reported as
   * if it were.
   */
  class NetworkError extends Error {
    constructor(cause: unknown) {
      super(cause instanceof Error ? cause.message : 'network request failed');
      this.name = 'NetworkError';
    }
  }

  let token = $state('');

  async function request<TResponse>(
    path: string,
    init: { method: string; body?: unknown } = { method: 'GET' }
  ): Promise<TResponse> {
    const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
    if (init.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    let response: Response;
    try {
      response = await fetch(`${API_URL}${path}`, {
        method: init.method,
        headers,
        body: init.body === undefined ? undefined : JSON.stringify(init.body)
      });
    } catch (err) {
      throw new NetworkError(err);
    }

    const data = await response.json().catch(() => null);
    if (!response.ok) {
      throw new ApiRequestError(response.status, data as Partial<ApiErrorBody> | null);
    }
    return data as TResponse;
  }

  // ---------------------------------------------------------------- ceremony

  /** Decode the base64url fields the server sends inside PublicKeyCredential options. */
  function decodePublicKeyOptions(publicKey: Record<string, unknown>): Record<string, unknown> {
    const options: Record<string, unknown> = { ...publicKey };
    options.challenge = base64ToArrayBuffer(String(options.challenge));

    const user = options.user as { id?: unknown } | undefined;
    if (user && typeof user.id === 'string') {
      options.user = { ...user, id: base64ToArrayBuffer(user.id) };
    }

    for (const key of ['allowCredentials', 'excludeCredentials'] as const) {
      const list = options[key];
      if (Array.isArray(list)) {
        options[key] = list.map((cred: Record<string, unknown>) => ({
          ...cred,
          id: base64ToArrayBuffer(String(cred.id))
        }));
      }
    }

    return options;
  }

  /** Serialize an attestation (registration) credential into the shape the server parses. */
  function encodeAttestation(credential: PublicKeyCredential): Record<string, unknown> {
    const response = credential.response as AuthenticatorAttestationResponse;
    const transports =
      typeof response.getTransports === 'function' ? response.getTransports() : undefined;

    return {
      id: credential.id,
      rawId: arrayBufferToBase64(credential.rawId),
      type: credential.type,
      clientExtensionResults: credential.getClientExtensionResults(),
      response: {
        clientDataJSON: arrayBufferToBase64(response.clientDataJSON),
        attestationObject: arrayBufferToBase64(response.attestationObject),
        ...(transports && transports.length > 0 ? { transports } : {})
      }
    };
  }

  function base64ToArrayBuffer(base64: string): ArrayBuffer {
    const binaryString = atob(base64.replace(/-/g, '+').replace(/_/g, '/'));
    const bytes = new Uint8Array(binaryString.length);
    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i);
    }
    return bytes.buffer;
  }

  function arrayBufferToBase64(buffer: ArrayBuffer): string {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.byteLength; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
  }

  /**
   * True when the ceremony ended because the person dismissed the platform
   * prompt (or we aborted it), rather than because it failed.
   */
  function isCeremonyCancellation(err: unknown): boolean {
    if (!(err instanceof Error)) {
      return false;
    }
    return err.name === 'NotAllowedError' || err.name === 'AbortError';
  }

  // ------------------------------------------------------------------- state

  type LoadState = 'loading' | 'ready' | 'unauthenticated' | 'failed';

  let loadState = $state<LoadState>('loading');
  let loadError = $state('');
  let username = $state('');
  let passkeys = $state<WebAuthnCredentialSummary[]>([]);
  let wallets = $state<WalletView[]>([]);

  /** Which mutation is in flight, so only the affected control is disabled. */
  let busy = $state<string>('');
  let error = $state('');
  let notice = $state('');
  let statusMessage = $state('');
  let errorRegion = $state<HTMLDivElement | null>(null);

  /** Credential id currently being renamed, and the draft name. */
  let renamingId = $state('');
  let renameDraft = $state('');

  let webAuthnSupported = $state(false);
  let ceremonyAbort: AbortController | null = null;

  const atPasskeyLimit = $derived(passkeys.length >= MAX_PASSKEYS);

  /**
   * Surface an error and move focus to it, so keyboard and screen-reader users
   * are told what happened instead of being left on a control that did nothing.
   */
  async function failWith(message: string) {
    error = message;
    notice = '';
    statusMessage = '';
    await tick();
    errorRegion?.focus();
  }

  function succeedWith(message: string) {
    notice = message;
    error = '';
    statusMessage = '';
  }

  onMount(() => {
    webAuthnSupported = !!window.PublicKeyCredential;
    const stored = sessionStorage.getItem('lesser_auth_jwt') ?? '';
    if (!stored) {
      loadState = 'unauthenticated';
      return;
    }
    token = stored;
    void load();
  });

  async function load() {
    try {
      const account = await request<Account>('/api/v1/accounts/verify_credentials');
      username = (account.username ?? account.acct ?? '').trim();
      await Promise.all([loadPasskeys(), loadWallets()]);
      loadState = 'ready';
    } catch (err) {
      if (err instanceof ApiRequestError && (err.status === 401 || err.status === 403)) {
        loadState = 'unauthenticated';
        return;
      }
      loadState = 'failed';
      loadError =
        err instanceof NetworkError
          ? 'We could not reach the server. Check your connection and reload.'
          : 'We could not load your authenticators. Please reload the page.';
    }
  }

  async function loadPasskeys() {
    const data = await request<WebAuthnCredentialsResponse>('/api/v1/auth/webauthn/credentials');
    passkeys = data.credentials ?? [];
  }

  async function loadWallets() {
    const data = await request<WalletListResponse>('/auth/wallet/list');
    // Project to WalletView here, at the boundary: whatever else the server
    // sends (public_key among it) does not survive into component state.
    wallets = (data.wallets ?? [])
      .filter((wallet): wallet is StorageWalletCredential => wallet != null)
      .map((wallet) => ({
        address: wallet.address,
        chainId: typeof wallet.chain_id === 'number' ? wallet.chain_id : null,
        walletType: wallet.wallet_type ?? '',
        linkedAt: wallet.linked_at,
        lastUsed: wallet.last_used
      }));
  }

  /** Re-read both lists; used after every mutation so the view is never stale. */
  async function refresh() {
    await Promise.all([loadPasskeys(), loadWallets()]);
  }

  function formatDate(value: string | null | undefined): string {
    if (!value) {
      return 'Unknown';
    }
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
      return 'Unknown';
    }
    return parsed.toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    });
  }

  function chainLabel(chainId: number | null): string {
    if (chainId == null) {
      return 'Unknown network';
    }
    return chainId === 1 ? 'Ethereum mainnet' : `Chain ${chainId}`;
  }

  // -------------------------------------------------------- error contracts

  /**
   * The last-authenticator error contract, mapped to what the person should do.
   *
   * `kind` distinguishes the two removal surfaces only for wording; the status
   * mapping is identical because the server's contract is identical:
   *   400 invariant / 404 already gone / 500 (or transport) genuine failure.
   */
  function describeRemovalError(err: unknown, kind: 'passkey' | 'wallet'): string {
    if (err instanceof NetworkError) {
      return `We could not reach the server, so your ${kind} was not removed. Check your connection and try again.`;
    }

    if (err instanceof ApiRequestError) {
      const message = err.serverMessage.toLowerCase();

      // The invariant. This is a refusal, not a failure: the account still has
      // exactly the authenticators it had a moment ago.
      if (err.status === 400 && message.includes('last authentication method')) {
        return 'This is your last way to sign in, so it cannot be removed. Add another passkey or link a wallet first, then remove this one.';
      }
      // Already gone. The list was stale; it has just been re-read.
      if (err.status === 404) {
        return kind === 'passkey'
          ? 'That passkey had already been removed. Your list is now up to date.'
          : 'That wallet had already been unlinked. Your list is now up to date.';
      }
      if (err.status === 401 || err.status === 403) {
        return 'Your session is no longer valid. Sign in again to manage your authenticators.';
      }
      if (err.status >= 500) {
        return `The server could not remove that ${kind}. It is probably still there — please try again in a moment.`;
      }
      return err.serverMessage || `That ${kind} could not be removed.`;
    }

    return err instanceof Error ? err.message : `That ${kind} could not be removed.`;
  }

  /** Map add-passkey failures onto advice the person can act on. */
  function describeAddPasskeyError(err: unknown): string {
    if (isCeremonyCancellation(err)) {
      return 'Adding a passkey was cancelled. Nothing changed — you can try again.';
    }
    if (err instanceof NetworkError) {
      return 'We could not reach the server, so no passkey was added. Check your connection and try again.';
    }

    if (err instanceof ApiRequestError) {
      const message = err.serverMessage.toLowerCase();

      if (err.status === 401 || err.status === 403) {
        return 'Your session is no longer valid. Sign in again to add a passkey.';
      }
      if (message.includes('webauthn not configured')) {
        return 'Passkeys are unavailable on this server. You can link a wallet instead.';
      }
      if (message.includes('challenge')) {
        return 'That took too long and the registration expired. Please try again.';
      }
      if (err.status >= 500) {
        return 'The server could not add that passkey. Please try again in a moment.';
      }
      return err.serverMessage || 'That passkey could not be added.';
    }

    return err instanceof Error ? err.message : 'That passkey could not be added.';
  }

  function describeWalletLinkError(err: unknown): string {
    if (err instanceof NetworkError) {
      return 'We could not reach the server, so no wallet was linked. Check your connection and try again.';
    }
    if (err instanceof ApiRequestError) {
      if (err.status === 401 || err.status === 403) {
        return `That wallet was not accepted: ${err.serverMessage || 'signature verification failed'}.`;
      }
      if (err.status >= 500) {
        return 'The server could not link that wallet. Please try again in a moment.';
      }
      return err.serverMessage || 'That wallet could not be linked.';
    }
    return err instanceof Error ? err.message : 'That wallet could not be linked.';
  }

  // ----------------------------------------------------------- passkey verbs

  function cancelCeremony() {
    ceremonyAbort?.abort();
  }

  async function addPasskey() {
    if (atPasskeyLimit) {
      await failWith(
        `You already have the maximum of ${MAX_PASSKEYS} passkeys. Remove one before adding another.`
      );
      return;
    }

    busy = 'add-passkey';
    error = '';
    notice = '';
    statusMessage = 'Starting passkey registration…';

    try {
      // These are the AUTHENTICATED registration endpoints: adding a passkey to
      // an account you are already signed in to. The signup ceremony is for
      // creating an account and is not reachable from here.
      const beginData = await request<WebAuthnBeginResponse>(
        '/api/v1/auth/webauthn/register/begin',
        { method: 'POST' }
      );

      statusMessage = 'Confirm the new passkey on your device…';
      ceremonyAbort = new AbortController();
      const credential = (await navigator.credentials.create({
        publicKey: decodePublicKeyOptions(
          beginData.publicKey
        ) as unknown as PublicKeyCredentialCreationOptions,
        signal: ceremonyAbort.signal
      })) as PublicKeyCredential | null;
      ceremonyAbort = null;

      if (!credential) {
        throw new Error('Passkey registration was cancelled');
      }

      statusMessage = 'Verifying your new passkey…';
      const body: WebAuthnFinishRegistrationRequest = {
        challenge: beginData.challenge,
        credential_name: nextPasskeyName(),
        response: encodeAttestation(credential)
      };
      await request<MessageResponse>('/api/v1/auth/webauthn/register/finish', {
        method: 'POST',
        body
      });

      await refresh();
      succeedWith('Passkey added. You can rename it below.');
    } catch (err) {
      console.error('Add passkey error:', err);
      await failWith(describeAddPasskeyError(err));
    } finally {
      ceremonyAbort = null;
      busy = '';
    }
  }

  /**
   * A default name so a new passkey is never nameless. The server generates one
   * too when the field is blank; sending ours keeps the name predictable for the
   * person who just created it.
   */
  function nextPasskeyName(): string {
    return `Passkey ${passkeys.length + 1}`;
  }

  function startRename(credential: WebAuthnCredentialSummary) {
    renamingId = credential.id;
    renameDraft = credential.name;
    error = '';
    notice = '';
  }

  function cancelRename() {
    renamingId = '';
    renameDraft = '';
  }

  async function saveRename(credentialId: string) {
    const name = renameDraft.trim();
    if (!name) {
      await failWith('Give the passkey a name so you can recognise it later.');
      return;
    }

    busy = `rename-${credentialId}`;
    error = '';
    notice = '';

    try {
      const body: WebAuthnUpdateCredentialRequest = { name };
      await request<MessageResponse>(
        `/api/v1/auth/webauthn/credentials/${encodeURIComponent(credentialId)}`,
        { method: 'PUT', body }
      );
      renamingId = '';
      renameDraft = '';
      await refresh();
      succeedWith(`Renamed to “${name}”.`);
    } catch (err) {
      console.error('Rename passkey error:', err);
      await failWith(describeRenameError(err));
    } finally {
      busy = '';
    }
  }

  function describeRenameError(err: unknown): string {
    if (err instanceof NetworkError) {
      return 'We could not reach the server, so the name was not changed. Check your connection and try again.';
    }
    if (err instanceof ApiRequestError) {
      if (err.status === 404) {
        return 'That passkey no longer exists, so it could not be renamed.';
      }
      if (err.status === 401 || err.status === 403) {
        return 'Your session is no longer valid. Sign in again to rename this passkey.';
      }
      if (err.status >= 500) {
        return 'The server could not save that name. Please try again in a moment.';
      }
      return err.serverMessage || 'That passkey could not be renamed.';
    }
    return err instanceof Error ? err.message : 'That passkey could not be renamed.';
  }

  async function removePasskey(credential: WebAuthnCredentialSummary) {
    busy = `remove-${credential.id}`;
    error = '';
    notice = '';

    try {
      await request<MessageResponse>(
        `/api/v1/auth/webauthn/credentials/${encodeURIComponent(credential.id)}`,
        { method: 'DELETE' }
      );
      await refresh();
      succeedWith(`Removed “${credential.name}”.`);
    } catch (err) {
      console.error('Remove passkey error:', err);
      // A 404 means the list we rendered was stale. Re-read before reporting so
      // the message and the list agree with each other.
      if (err instanceof ApiRequestError && err.status === 404) {
        await refreshQuietly();
      }
      await failWith(describeRemovalError(err, 'passkey'));
    } finally {
      busy = '';
    }
  }

  /** Best-effort re-read used on an error path, where a second failure must not mask the first. */
  async function refreshQuietly() {
    try {
      await refresh();
    } catch (err) {
      console.error('Refresh after failed removal:', err);
    }
  }

  // ------------------------------------------------------------ wallet verbs

  async function linkWallet() {
    if (!window.ethereum) {
      await failWith('No wallet detected. Install MetaMask or another Web3 wallet to link one.');
      return;
    }
    if (!username) {
      await failWith('We could not confirm your username, so a wallet cannot be linked right now.');
      return;
    }

    busy = 'link-wallet';
    error = '';
    notice = '';
    statusMessage = 'Waiting for your wallet…';

    try {
      const accounts = (await window.ethereum.request({
        method: 'eth_requestAccounts'
      })) as string[];
      const address = accounts[0];
      const chainId = (await window.ethereum.request({ method: 'eth_chainId' })) as string;
      const chainIdDecimal = parseInt(chainId, 16);

      const challengeBody: WalletChallengeRequest = {
        address,
        chainId: chainIdDecimal,
        username
      };
      const challenge = await request<StorageWalletChallenge>('/auth/wallet/challenge', {
        method: 'POST',
        body: challengeBody
      });

      statusMessage = 'Sign the message in your wallet…';
      const signature = (await window.ethereum.request({
        method: 'personal_sign',
        params: [challenge.message, address]
      })) as string;

      // Authenticated link: the bearer token identifies the account, and the
      // signature proves control of the wallet.
      const linkBody: WalletLinkRequest = {
        address,
        chainId: chainIdDecimal,
        challengeId: challenge.id,
        message: challenge.message,
        signature,
        walletType: 'ethereum'
      };
      await request<WalletLinkResponse>('/auth/wallet/link', { method: 'POST', body: linkBody });

      await refresh();
      succeedWith('Wallet linked.');
    } catch (err) {
      console.error('Link wallet error:', err);
      await failWith(describeWalletLinkError(err));
    } finally {
      busy = '';
    }
  }

  async function unlinkWallet(wallet: WalletView) {
    busy = `unlink-${wallet.address}`;
    error = '';
    notice = '';

    try {
      await request<WalletUnlinkResponse>(
        `/auth/wallet/unlink/${encodeURIComponent(wallet.address)}`,
        { method: 'DELETE' }
      );
      await refresh();
      succeedWith('Wallet unlinked.');
    } catch (err) {
      console.error('Unlink wallet error:', err);
      if (err instanceof ApiRequestError && err.status === 404) {
        await refreshQuietly();
      }
      await failWith(describeRemovalError(err, 'wallet'));
    } finally {
      busy = '';
    }
  }
</script>

<div class="credential-manager">
  {#if loadState === 'loading'}
    <div class="state-block" role="status" aria-live="polite" data-testid="credentials-loading">
      <Text>Loading your authenticators…</Text>
    </div>
  {:else if loadState === 'unauthenticated'}
    <div class="state-block" data-testid="credentials-signed-out">
      <Alert variant="info" title="Sign in to continue">
        Managing your passkeys and wallets needs you to be signed in.
      </Alert>
      <a class="auth-link" href={loginHref}>Sign in</a>
    </div>
  {:else if loadState === 'failed'}
    <div class="state-block" data-testid="credentials-load-error">
      <Alert variant="error" title="Could not load your authenticators">
        {loadError}
      </Alert>
    </div>
  {:else}
    {#if error}
      <div
        class="alert-slot"
        role="alert"
        aria-live="assertive"
        tabindex="-1"
        bind:this={errorRegion}
        data-testid="credential-error"
      >
        <Alert variant="error" title="Something went wrong">{error}</Alert>
      </div>
    {/if}

    {#if notice}
      <div class="alert-slot" data-testid="credential-notice">
        <Alert variant="success" title="Done">{notice}</Alert>
      </div>
    {/if}

    <!-- Progress for the multi-step ceremonies, announced politely so it does
         not interrupt the platform's own passkey prompt. -->
    <div class="gr-sr-only" role="status" aria-live="polite" data-testid="credential-status">
      {statusMessage}
    </div>

    {#if username}
      <Text size="sm" data-testid="credential-account">Signed in as {username}</Text>
    {/if}

    <section class="credential-section" aria-labelledby="passkeys-heading">
      <div class="section-header">
        <Heading level={2} size="lg" id="passkeys-heading">Passkeys</Heading>
        <Button
          variant="solid"
          size="sm"
          disabled={!webAuthnSupported || atPasskeyLimit || busy !== ''}
          loading={busy === 'add-passkey'}
          aria-label="Add a passkey to your account"
          data-testid="add-passkey-button"
          onclick={addPasskey}
        >
          <KeyIcon class="btn-icon" />
          Add passkey
        </Button>
      </div>

      <Text size="sm" data-testid="passkey-count">
        {passkeys.length} of {MAX_PASSKEYS} passkeys in use.
      </Text>

      {#if atPasskeyLimit}
        <Alert variant="info">
          You have reached the maximum of {MAX_PASSKEYS} passkeys. Remove one before adding another.
        </Alert>
      {/if}

      {#if busy === 'add-passkey'}
        <div class="ceremony-actions">
          <Button variant="ghost" size="sm" onclick={cancelCeremony} data-testid="cancel-ceremony-button">
            Cancel
          </Button>
        </div>
      {/if}

      {#if passkeys.length === 0}
        <Text size="sm" data-testid="passkeys-empty">No passkeys yet.</Text>
      {:else}
        <ul class="credential-list" data-testid="passkey-list">
          {#each passkeys as credential (credential.id)}
            <li class="credential-row" data-testid="passkey-row">
              {#if renamingId === credential.id}
                <div class="rename-form">
                  <TextField
                    id={`rename-${credential.id}`}
                    label="Passkey name"
                    bind:value={renameDraft}
                    disabled={busy !== ''}
                    data-testid="rename-input"
                    required
                  />
                  <div class="row-actions">
                    <Button
                      variant="solid"
                      size="sm"
                      loading={busy === `rename-${credential.id}`}
                      disabled={busy !== ''}
                      data-testid="rename-save-button"
                      onclick={() => saveRename(credential.id)}
                    >
                      Save name
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={busy !== ''}
                      data-testid="rename-cancel-button"
                      onclick={cancelRename}
                    >
                      Cancel
                    </Button>
                  </div>
                </div>
              {:else}
                <div class="credential-detail">
                  <span class="credential-name" data-testid="passkey-name">{credential.name}</span>
                  <span class="credential-meta">
                    Added {formatDate(credential.created_at)} · Last used
                    {formatDate(credential.last_used_at)}
                  </span>
                </div>
                <div class="row-actions">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busy !== ''}
                    aria-label={`Rename passkey ${credential.name}`}
                    data-testid="rename-passkey-button"
                    onclick={() => startRename(credential)}
                  >
                    Rename
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    loading={busy === `remove-${credential.id}`}
                    disabled={busy !== ''}
                    aria-label={`Remove passkey ${credential.name}`}
                    data-testid="remove-passkey-button"
                    onclick={() => removePasskey(credential)}
                  >
                    Remove
                  </Button>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <section class="credential-section" aria-labelledby="wallets-heading">
      <div class="section-header">
        <Heading level={2} size="lg" id="wallets-heading">Wallets</Heading>
        <Button
          variant="solid"
          size="sm"
          disabled={busy !== ''}
          loading={busy === 'link-wallet'}
          aria-label="Link a crypto wallet to your account"
          data-testid="link-wallet-button"
          onclick={linkWallet}
        >
          <WalletIcon class="btn-icon" />
          Link wallet
        </Button>
      </div>

      {#if wallets.length === 0}
        <Text size="sm" data-testid="wallets-empty">No wallets linked. A wallet is optional.</Text>
      {:else}
        <ul class="credential-list" data-testid="wallet-list">
          {#each wallets as wallet (wallet.address)}
            <li class="credential-row" data-testid="wallet-row">
              <div class="credential-detail">
                <span class="credential-name" data-testid="wallet-address">
                  {truncateMiddle(wallet.address, { head: 10, tail: 8 })}
                </span>
                <span class="credential-meta">
                  {chainLabel(wallet.chainId)} · Linked {formatDate(wallet.linkedAt)}
                </span>
              </div>
              <div class="row-actions">
                <Button
                  variant="ghost"
                  size="sm"
                  loading={busy === `unlink-${wallet.address}`}
                  disabled={busy !== ''}
                  aria-label={`Unlink wallet ${wallet.address}`}
                  data-testid="unlink-wallet-button"
                  onclick={() => unlinkWallet(wallet)}
                >
                  Unlink
                </Button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </section>

    <Text size="sm" data-testid="credential-footnote">
      Lesser never shows credential material. Passkey keys stay on your device, and the public keys
      Lesser stores are never displayed here or in your public profile.
    </Text>
  {/if}
</div>

<style>
  .credential-manager {
    width: 100%;
    max-width: 640px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: var(--spacing-md);
  }

  .state-block {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
    align-items: flex-start;
  }

  .alert-slot:focus {
    outline: 2px solid var(--lesser-primary-500, #007bff);
    outline-offset: 2px;
  }

  .credential-section {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-sm);
    flex-wrap: wrap;
  }

  .section-header :global(.btn-icon) {
    width: 1rem;
    height: 1rem;
    flex-shrink: 0;
    display: inline-block;
  }

  .section-header :global(.btn-icon svg) {
    display: block;
    width: 100%;
    height: 100%;
  }

  .credential-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
  }

  .credential-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-sm);
    flex-wrap: wrap;
    padding: var(--spacing-sm);
    border: 1px solid var(--gr-color-border, rgba(255, 255, 255, 0.15));
    border-radius: var(--border-radius-md);
  }

  .credential-detail {
    display: flex;
    flex-direction: column;
    gap: 0.125rem;
    min-width: 0;
  }

  .credential-name {
    font-weight: 600;
    word-break: break-word;
  }

  .credential-meta {
    font-size: 0.8125rem;
    color: var(--text-secondary, rgba(255, 255, 255, 0.7));
  }

  .row-actions {
    display: flex;
    gap: var(--spacing-sm);
    flex-wrap: wrap;
  }

  .rename-form {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
    width: 100%;
  }

  .ceremony-actions {
    display: flex;
  }

  @media (max-width: 640px) {
    .credential-row {
      align-items: flex-start;
      flex-direction: column;
    }
  }
</style>
