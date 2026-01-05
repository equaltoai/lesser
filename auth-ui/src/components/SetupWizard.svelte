<script lang="ts">
  import { onMount } from 'svelte';
  import Alert from 'src/lib/greater/primitives/components/Alert.svelte';
  import Button from 'src/lib/greater/primitives/components/Button.svelte';
  import Checkbox from 'src/lib/greater/primitives/components/Checkbox.svelte';
  import DefinitionItem from 'src/lib/greater/primitives/components/DefinitionItem.svelte';
  import DefinitionList from 'src/lib/greater/primitives/components/DefinitionList.svelte';
  import CopyButton from 'src/lib/greater/primitives/components/CopyButton.svelte';
  import Modal from 'src/lib/greater/primitives/components/Modal.svelte';
  import StepIndicator from 'src/lib/greater/primitives/components/StepIndicator.svelte';
  import TextField from 'src/lib/greater/primitives/components/TextField.svelte';
  import { truncateMiddle } from 'src/lib/greater/utils';

  type WizardStep =
    | 'status'
    | 'bootstrap_login'
    | 'create_admin'
    | 'admin_login'
    | 'passkey'
    | 'finalize'
    | 'done';

  interface SetupStatusResponse {
    locked: boolean;
    instance_state?: string;
    finalize_allowed?: boolean;
    bootstrap_actor?: {
      username: string;
      acct: string;
      actor: string;
    };
    bootstrap?: {
      username: string;
      wallet_address_set: boolean;
      wallet_address: string;
      primary_admin_set: boolean;
      primary_admin: string;
    };
    urls?: Record<string, unknown>;
    activated_at?: string | null;
  }

  interface WalletChallengeResponse {
    id: string;
    username?: string;
    address: string;
    chainId: number;
    nonce: string;
    message: string;
    issuedAt: string;
    expiresAt: string;
  }

  interface SetupBootstrapChallengeResponse {
    challenge_id?: string;
    challenge?: string;
    issued_at?: string;
    expires_at?: string;

    // Backwards-compatible fields.
    id?: string;
    message?: string;
  }

  interface SetupBootstrapVerifyResponse {
    token_type?: string;
    token?: string;
    setup_token?: string;
    expires_at?: string;
  }

  interface AuthResponse {
    access_token: string;
    token_type: string;
    expires_in: number;
    refresh_token: string;
    scope: string;
    created_at: number;
    me?: string;
  }

  interface ApiError {
    message: string;
    url: string;
    status?: number;
    requestId?: string;
    responseBody?: unknown;
  }

  const STORAGE_PREFIX = 'lesser.setup.v1.';

  const apiOrigin =
    import.meta.env.PUBLIC_LESSER_API_ORIGIN || (typeof window !== 'undefined' ? window.location.origin : '');

  let step = $state<WizardStep>('status');

  let status = $state<SetupStatusResponse | null>(null);
  let loading = $state(false);
  let error = $state<ApiError | null>(null);
  let retry = $state<null | (() => Promise<void>)>(null);

  let actionBusy = $state(false);

  let setupToken = $state('');
  let bootstrapAddress = $state('');
  let bootstrapChainId = $state<number>(1);
  let adminUsername = $state('');
  let adminDisplayName = $state('');
  let adminWalletAddress = $state('');
  let adminChainId = $state<number>(1);
  let adminJwt = $state('');

  let passkeyCredentialName = $state('Primary admin passkey');
  let passkeyRegistered = $state(false);
  let skipPasskeyAcknowledged = $state(false);

  let finalizeConfirmOpen = $state(false);

  const stageDomain = $derived(typeof window !== 'undefined' ? window.location.host : '');
  const authBase = $derived(`${apiOrigin}/auth`);
  const clientUrl = $derived(`${apiOrigin}/l`);
  const setupUrl = $derived(`${apiOrigin}/auth/setup`);
  const setupStatusUrl = $derived(`${apiOrigin}/setup/status`);

  const steps = $derived([
    { key: 'status' as const, label: 'Status' },
    { key: 'bootstrap_login' as const, label: 'Bootstrap Login' },
    { key: 'create_admin' as const, label: 'Create Admin' },
    { key: 'admin_login' as const, label: 'Admin Login' },
    { key: 'passkey' as const, label: 'Passkey' },
    { key: 'finalize' as const, label: 'Finalize' },
  ]);

  function stepIndex(key: WizardStep): number {
    const idx = steps.findIndex((s) => s.key === key);
    return idx === -1 ? 0 : idx;
  }

  const currentIndex = $derived(stepIndex(step));
  const stepperIndex = $derived(step === 'done' ? steps.length : currentIndex);

  function readStorage(key: string): string {
    if (typeof window === 'undefined') return '';
    return window.sessionStorage.getItem(`${STORAGE_PREFIX}${key}`) || '';
  }

  function writeStorage(key: string, value: string) {
    if (typeof window === 'undefined') return;
    window.sessionStorage.setItem(`${STORAGE_PREFIX}${key}`, value);
  }

  function setStep(next: WizardStep) {
    step = next;
    writeStorage('step', next);
  }

  function clearWizardStorage() {
    if (typeof window === 'undefined') return;

    for (let i = window.sessionStorage.length - 1; i >= 0; i -= 1) {
      const key = window.sessionStorage.key(i);
      if (key && key.startsWith(STORAGE_PREFIX)) {
        window.sessionStorage.removeItem(key);
      }
    }
  }

  function setLocalError(message: string) {
    error = {
      message,
      url: 'local',
    };
  }

  function normalizeError(err: unknown): ApiError {
    if (err && typeof err === 'object') {
      const maybeErr = err as Partial<ApiError> & { message?: unknown };
      const message = typeof maybeErr.message === 'string' ? maybeErr.message : 'Unexpected error';
      const url = typeof maybeErr.url === 'string' && maybeErr.url ? maybeErr.url : 'local';
      const status = typeof maybeErr.status === 'number' ? maybeErr.status : undefined;
      const requestId = typeof maybeErr.requestId === 'string' ? maybeErr.requestId : undefined;
      const responseBody = 'responseBody' in maybeErr ? maybeErr.responseBody : undefined;

      return { message, url, status, requestId, responseBody };
    }

    if (typeof err === 'string') {
      return { message: err, url: 'local' };
    }

    return { message: 'Unexpected error', url: 'local' };
  }

  async function readJson(response: Response): Promise<unknown> {
    const contentType = response.headers.get('content-type') || '';
    if (contentType.includes('application/json')) {
      return response.json();
    }
    return response.text();
  }

  function extractRequestId(headers: Headers): string {
    return (
      headers.get('x-correlation-id') ||
      headers.get('x-request-id') ||
      headers.get('x-amzn-requestid') ||
      headers.get('x-amzn-trace-id') ||
      ''
    );
  }

  async function fetchJson<T>(url: string, options?: RequestInit): Promise<T> {
    const response = await fetch(url, options);
    const requestId = extractRequestId(response.headers);
    const body = await readJson(response);

    if (!response.ok) {
      const message =
        typeof body === 'object' && body && 'error' in body
          ? String((body as any).error)
          : `Request failed (${response.status})`;
      throw {
        message,
        url,
        status: response.status,
        requestId,
        responseBody: body,
      } satisfies ApiError;
    }

    return body as T;
  }

  async function connectEthereumWallet(): Promise<{ address: string; chainId: number }> {
    const ethereum = (window as any)?.ethereum;
    if (!ethereum?.request) {
      throw new Error('No wallet detected. Please install MetaMask or another EVM wallet.');
    }

    const accounts = (await ethereum.request({ method: 'eth_requestAccounts' })) as string[];
    const address = (accounts?.[0] || '').trim();
    if (!address) {
      throw new Error('Wallet did not return an address.');
    }

    const chainIdHex = (await ethereum.request({ method: 'eth_chainId' })) as string;
    const chainId = chainIdHex ? parseInt(chainIdHex, 16) : 1;

    return { address: address.toLowerCase(), chainId: Number.isFinite(chainId) && chainId > 0 ? chainId : 1 };
  }

  async function signEthereumMessage(address: string, message: string): Promise<string> {
    const ethereum = (window as any)?.ethereum;
    if (!ethereum?.request) {
      throw new Error('No wallet detected. Please install MetaMask or another EVM wallet.');
    }

    try {
      return (await ethereum.request({
        method: 'personal_sign',
        params: [message, address],
      })) as string;
    } catch {
      // Some wallets expect the reverse parameter order.
      return (await ethereum.request({
        method: 'personal_sign',
        params: [address, message],
      })) as string;
    }
  }

  async function connectBootstrapWallet() {
    const { address, chainId } = await connectEthereumWallet();
    bootstrapAddress = address;
    bootstrapChainId = chainId;
    writeStorage('bootstrapAddress', address);
    writeStorage('bootstrapChainId', String(chainId));
  }

  async function connectAdminWallet() {
    const { address, chainId } = await connectEthereumWallet();
    adminWalletAddress = address;
    adminChainId = chainId;
    writeStorage('adminWalletAddress', address);
    writeStorage('adminChainId', String(chainId));
  }

  async function bootstrapLogin() {
    retry = bootstrapLogin;
    error = null;
    actionBusy = true;

    try {
      if (!status?.locked) {
        await refreshStatus();
        return;
      }
      if (!status.bootstrap?.wallet_address_set) {
        setLocalError('This instance does not have a bootstrap wallet configured.');
        return;
      }

      if (!bootstrapAddress) {
        await connectBootstrapWallet();
      }

      const challenge = await fetchJson<SetupBootstrapChallengeResponse>(`${apiOrigin}/setup/bootstrap/challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          address: bootstrapAddress,
          chainId: bootstrapChainId || 1,
        }),
      });

      const challengeId = (challenge.challenge_id || challenge.id || '').trim();
      const message = (challenge.challenge || challenge.message || '').trim();
      if (!challengeId || !message) {
        setLocalError('Bootstrap challenge response was missing required fields.');
        return;
      }

      const signature = await signEthereumMessage(bootstrapAddress, message);

      const verify = await fetchJson<SetupBootstrapVerifyResponse>(`${apiOrigin}/setup/bootstrap/verify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          challengeId,
          address: bootstrapAddress,
          signature,
          message,
        }),
      });

      const token = (verify.setup_token || verify.token || '').trim();
      if (!token) {
        setLocalError('Bootstrap verification succeeded but no setup token was returned.');
        return;
      }

      setupToken = token;
      writeStorage('setupToken', token);
      setStep('create_admin');
    } catch (err) {
      error = normalizeError(err);
    } finally {
      actionBusy = false;
    }
  }

  async function createAdmin() {
    retry = createAdmin;
    error = null;
    actionBusy = true;

    try {
      if (!setupToken) {
        setupToken = readStorage('setupToken');
      }
      if (!setupToken) {
        setLocalError('Missing setup token. Please complete bootstrap login first.');
        setStep('bootstrap_login');
        return;
      }
      if (!adminUsername.trim()) {
        setLocalError('Admin username is required.');
        return;
      }
      if (adminUsername.trim().toLowerCase() === (status?.bootstrap?.username || '').trim().toLowerCase()) {
        setLocalError('Admin username cannot be the reserved bootstrap username.');
        return;
      }

      if (!adminWalletAddress) {
        await connectAdminWallet();
      }

      const challenge = await fetchJson<WalletChallengeResponse>(`${apiOrigin}/auth/wallet/challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          address: adminWalletAddress,
          chainId: adminChainId || 1,
          username: adminUsername.trim(),
        }),
      });

      const signature = await signEthereumMessage(adminWalletAddress, challenge.message);

      await fetchJson(`${apiOrigin}/setup/admin`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${setupToken}`,
        },
        body: JSON.stringify({
          username: adminUsername.trim(),
          displayName: adminDisplayName.trim(),
          wallet: {
            challengeId: challenge.id,
            address: adminWalletAddress,
            signature,
            message: challenge.message,
          },
        }),
      });

      writeStorage('adminUsername', adminUsername.trim());
      setStep('admin_login');
      await refreshStatus();
    } catch (err) {
      const apiErr = normalizeError(err);
      if (apiErr?.status === 409) {
        // Primary admin already created: treat as resumable.
        await refreshStatus();
        setStep('admin_login');
      } else if (apiErr?.status === 401 || apiErr?.status === 403) {
        // Setup token likely expired/invalid.
        setupToken = '';
        writeStorage('setupToken', '');
        setStep('bootstrap_login');
        error = apiErr;
      } else {
        error = apiErr;
      }
    } finally {
      actionBusy = false;
    }
  }

  async function loginAdmin() {
    retry = loginAdmin;
    error = null;
    actionBusy = true;

    try {
      if (!adminUsername.trim()) {
        const fromStatus = status?.bootstrap?.primary_admin || '';
        if (fromStatus.trim()) {
          adminUsername = fromStatus.trim();
        } else {
          setLocalError('Missing admin username. Create the primary admin first.');
          setStep('create_admin');
          return;
        }
      }

      if (!adminWalletAddress) {
        await connectAdminWallet();
      }

      const challenge = await fetchJson<WalletChallengeResponse>(`${apiOrigin}/auth/wallet/challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          address: adminWalletAddress,
          chainId: adminChainId || 1,
          username: adminUsername.trim(),
        }),
      });

      const signature = await signEthereumMessage(adminWalletAddress, challenge.message);

      const auth = await fetchJson<AuthResponse>(`${apiOrigin}/auth/wallet/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          challengeId: challenge.id,
          address: adminWalletAddress,
          signature,
          message: challenge.message,
        }),
      });

      adminJwt = auth.access_token;
      writeStorage('adminJwt', adminJwt);
      setStep('passkey');
    } catch (err) {
      error = normalizeError(err);
    } finally {
      actionBusy = false;
    }
  }

  function base64ToArrayBuffer(base64: string): ArrayBuffer {
    const base64Normalized = base64.replace(/-/g, '+').replace(/_/g, '/');
    const padded = base64Normalized + '='.repeat((4 - (base64Normalized.length % 4)) % 4);
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes.buffer;
  }

  function arrayBufferToBase64(buffer: ArrayBuffer): string {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.byteLength; i += 1) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
  }

  async function registerPasskey() {
    retry = registerPasskey;
    error = null;
    actionBusy = true;

    try {
      if (!adminJwt) {
        adminJwt = readStorage('adminJwt');
      }
      if (!adminJwt) {
        setLocalError('Missing admin session. Please log in as the real admin first.');
        setStep('admin_login');
        return;
      }

      const begin = await fetchJson<{ publicKey: any; challenge: string }>(`${apiOrigin}/api/v1/auth/webauthn/register/begin`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${adminJwt}`,
        },
        body: JSON.stringify({}),
      });

      const options = begin.publicKey;
      if (typeof options.challenge === 'string') {
        options.challenge = base64ToArrayBuffer(options.challenge);
      }
      if (options.user?.id && typeof options.user.id === 'string') {
        options.user.id = base64ToArrayBuffer(options.user.id);
      }
      if (Array.isArray(options.excludeCredentials)) {
        options.excludeCredentials = options.excludeCredentials.map((cred: any) => ({
          ...cred,
          id: typeof cred.id === 'string' ? base64ToArrayBuffer(cred.id) : cred.id,
        }));
      }

      const credential = (await navigator.credentials.create({ publicKey: options })) as PublicKeyCredential;
      if (!credential) {
        setLocalError('Passkey registration was cancelled.');
        return;
      }

      const response = credential.response as AuthenticatorAttestationResponse;
      const payload = {
        id: credential.id,
        rawId: arrayBufferToBase64(credential.rawId),
        type: credential.type,
        response: {
          clientDataJSON: arrayBufferToBase64(response.clientDataJSON),
          attestationObject: arrayBufferToBase64(response.attestationObject),
        },
        clientExtensionResults: credential.getClientExtensionResults?.() || {},
      };

      await fetchJson(`${apiOrigin}/api/v1/auth/webauthn/register/finish`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${adminJwt}`,
        },
        body: JSON.stringify({
          challenge: begin.challenge,
          response: payload,
          credential_name: passkeyCredentialName.trim(),
        }),
      });

      passkeyRegistered = true;
      setStep('finalize');
    } catch (err) {
      error = normalizeError(err);
    } finally {
      actionBusy = false;
    }
  }

  function skipPasskey() {
    if (!skipPasskeyAcknowledged) {
      setLocalError('Acknowledge the recovery warning to continue without a passkey.');
      return;
    }
    setStep('finalize');
  }

  async function finalizeActivation() {
    retry = finalizeActivation;
    error = null;
    actionBusy = true;

    try {
      if (!adminJwt) {
        adminJwt = readStorage('adminJwt');
      }
      if (!adminJwt) {
        setLocalError('Missing admin session. Please log in as the real admin first.');
        setStep('admin_login');
        return;
      }

      await fetchJson(`${apiOrigin}/setup/finalize`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${adminJwt}`,
        },
        body: JSON.stringify({}),
      });

      clearWizardStorage();
      setupToken = '';
      bootstrapAddress = '';
      adminUsername = '';
      adminDisplayName = '';
      adminWalletAddress = '';
      adminJwt = '';
      passkeyRegistered = false;
      skipPasskeyAcknowledged = false;

      setStep('done');
      await refreshStatus();
    } catch (err) {
      const apiErr = normalizeError(err);
      if (apiErr?.status === 409) {
        // Already activated: treat as done.
        clearWizardStorage();
        setStep('done');
        await refreshStatus();
      } else {
        error = apiErr;
      }
    } finally {
      actionBusy = false;
      finalizeConfirmOpen = false;
    }
  }

  function deriveStep(data: SetupStatusResponse): WizardStep {
    const stored = (readStorage('step') as WizardStep) || 'status';
    const storedSetupToken = readStorage('setupToken');
    const storedAdminJwt = readStorage('adminJwt');

    if (!data.locked) return 'done';

    const primaryAdmin = (data.bootstrap?.primary_admin || '').trim();
    if (primaryAdmin && !adminUsername.trim()) {
      adminUsername = primaryAdmin;
      writeStorage('adminUsername', primaryAdmin);
    }

    if (storedAdminJwt) {
      return stored === 'finalize' || stored === 'passkey' ? stored : 'passkey';
    }

    if (data.bootstrap?.primary_admin_set) {
      return stored === 'admin_login' || stored === 'passkey' || stored === 'finalize' ? stored : 'admin_login';
    }

    if (storedSetupToken) {
      return stored === 'create_admin' || stored === 'admin_login' ? stored : 'create_admin';
    }

    return stored === 'bootstrap_login' ? 'bootstrap_login' : 'status';
  }

  async function refreshStatus() {
    loading = true;
    error = null;
    retry = null;

    try {
      const data = await fetchJson<SetupStatusResponse>(`${apiOrigin}/setup/status`, {
        method: 'GET',
        headers: { Accept: 'application/json' },
      });
      status = data;

      setupToken = readStorage('setupToken');
      bootstrapAddress = readStorage('bootstrapAddress');
      bootstrapChainId = parseInt(readStorage('bootstrapChainId') || '1', 10) || 1;
      adminUsername = readStorage('adminUsername');
      adminDisplayName = readStorage('adminDisplayName');
      adminWalletAddress = readStorage('adminWalletAddress');
      adminChainId = parseInt(readStorage('adminChainId') || '1', 10) || 1;
      adminJwt = readStorage('adminJwt');

      setStep(deriveStep(data));
    } catch (err) {
      error = normalizeError(err);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    writeStorage('stageDomain', stageDomain);
    writeStorage('systemBase', apiOrigin);

    if (!import.meta.env.PUBLIC_LESSER_API_ORIGIN) {
      const path = window.location.pathname || '';
      if (!(path === '/auth' || path.startsWith('/auth/'))) {
        setLocalError('This page must be served from /auth on your Lesser domain.');
        return;
      }
    }

    refreshStatus();
  });
</script>

<div class="setup-wizard">
  <div class="stepper">
    {#each steps as s, index}
      <StepIndicator
        number={index + 1}
        label={s.label}
        state={index < stepperIndex ? 'completed' : index === stepperIndex ? 'active' : 'pending'}
        size="sm"
      />
    {/each}
  </div>

  {#if error}
    <Alert variant="error" title="Setup wizard error">
      <div class="alert-body">
        <div>{error.message}</div>
        <DefinitionList density="sm" dividers>
          <DefinitionItem label="URL" monospace>
            {error.url}
            {#snippet actions()}
              <CopyButton text={error.url} />
            {/snippet}
          </DefinitionItem>
          {#if error.status}
            <DefinitionItem label="Status" monospace>
              {String(error.status)}
            </DefinitionItem>
          {/if}
          {#if error.requestId}
            <DefinitionItem label="Request ID" monospace>
              {truncateMiddle(error.requestId, { head: 12, tail: 12 })}
              {#snippet actions()}
                <CopyButton text={error.requestId} />
              {/snippet}
            </DefinitionItem>
          {/if}
        </DefinitionList>
      </div>

      {#snippet actions()}
        <CopyButton
          text={JSON.stringify(
            { message: error.message, url: error.url, status: error.status, requestId: error.requestId, body: error.responseBody },
            null,
            2
          )}
          variant="icon-text"
          labels={{ default: 'Copy debug', success: 'Copied' }}
        />
        <Button variant="outline" size="sm" onclick={() => (retry ? retry() : refreshStatus())} disabled={actionBusy}>
          Retry
        </Button>
      {/snippet}
    </Alert>
  {/if}

  {#if loading && !status}
    <div class="loading">Loading setup status…</div>
  {:else if status}
    {#if step === 'done' || status.locked === false}
      <Alert variant="success" title="Instance is active">
        This Lesser instance is already activated.
      </Alert>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Stage Domain" monospace wrap={false}>
          {truncateMiddle(stageDomain, { head: 18, tail: 12 })}
          {#snippet actions()}
            <CopyButton text={stageDomain} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Client" monospace wrap={false}>
          {truncateMiddle(clientUrl, { head: 24, tail: 12 })}
          {#snippet actions()}
            <CopyButton text={clientUrl} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Auth" monospace wrap={false}>
          {truncateMiddle(authBase, { head: 24, tail: 12 })}
          {#snippet actions()}
            <CopyButton text={authBase} />
          {/snippet}
        </DefinitionItem>
      </DefinitionList>
    {:else if step === 'status'}
      <Alert variant="info" title="Instance is locked">
        Complete setup to enable publishing and signups. The bootstrap actor exists only for initialization and will be
        deleted when you finalize activation.
      </Alert>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Setup Wizard" monospace>
          {setupUrl}
          {#snippet actions()}
            <CopyButton text={setupUrl} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Setup Status" monospace>
          {setupStatusUrl}
          {#snippet actions()}
            <CopyButton text={setupStatusUrl} />
          {/snippet}
        </DefinitionItem>
        {#if status.bootstrap_actor}
          <DefinitionItem label="Bootstrap Actor" monospace wrap={false}>
            {truncateMiddle(status.bootstrap_actor.actor, { head: 18, tail: 12 })}
            {#snippet actions()}
              <CopyButton text={status.bootstrap_actor.actor} />
            {/snippet}
          </DefinitionItem>
        {/if}
        {#if status.bootstrap?.wallet_address}
          <DefinitionItem label="Bootstrap Wallet" monospace wrap={false}>
            {truncateMiddle(status.bootstrap.wallet_address, { head: 10, tail: 8 })}
            {#snippet actions()}
              <CopyButton text={status.bootstrap.wallet_address} />
            {/snippet}
          </DefinitionItem>
        {/if}
        {#if status.bootstrap?.primary_admin_set}
          <DefinitionItem label="Primary Admin" monospace>
            {status.bootstrap?.primary_admin}
          </DefinitionItem>
        {/if}
      </DefinitionList>

      <div class="actions">
        <Button
          variant="solid"
          onclick={() => setStep(status.bootstrap?.primary_admin_set ? 'admin_login' : 'bootstrap_login')}
        >
          Continue
        </Button>
        <Button variant="outline" onclick={refreshStatus} disabled={loading}>Refresh</Button>
      </div>
    {:else if step === 'bootstrap_login'}
      <Alert variant="info" title="Bootstrap login (wallet only)">
        Connect your bootstrap wallet and sign the challenge. This only creates a short-lived setup token used to create
        the real admin.
      </Alert>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Expected Bootstrap Wallet" monospace wrap={false}>
          {truncateMiddle(status.bootstrap?.wallet_address || '', { head: 10, tail: 8 })}
          {#snippet actions()}
            <CopyButton text={status.bootstrap?.wallet_address || ''} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Connected Wallet" monospace wrap={false}>
          {bootstrapAddress ? truncateMiddle(bootstrapAddress, { head: 10, tail: 8 }) : 'not connected'}
          {#snippet actions()}
            {#if bootstrapAddress}
              <CopyButton text={bootstrapAddress} />
            {/if}
          {/snippet}
        </DefinitionItem>
      </DefinitionList>

      <div class="actions">
        <Button variant="solid" onclick={bootstrapLogin} disabled={actionBusy}>
          {actionBusy ? 'Working…' : 'Sign in as bootstrap'}
        </Button>
        <Button variant="outline" onclick={connectBootstrapWallet} disabled={actionBusy}>
          Connect wallet
        </Button>
        <Button variant="ghost" onclick={() => setStep('status')} disabled={actionBusy}>
          Back
        </Button>
      </div>
    {:else if step === 'create_admin'}
      <Alert variant="info" title="Create the real admin">
        Choose a username and bind it to an Ethereum wallet. This admin will be required to finalize activation.
      </Alert>

      <div class="form">
        <TextField label="Admin username" bind:value={adminUsername} placeholder="e.g. owner" disabled={actionBusy} />
        <TextField
          label="Display name (optional)"
          bind:value={adminDisplayName}
          placeholder="e.g. Instance Owner"
          disabled={actionBusy}
        />
      </div>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Connected Wallet" monospace wrap={false}>
          {adminWalletAddress ? truncateMiddle(adminWalletAddress, { head: 10, tail: 8 }) : 'not connected'}
          {#snippet actions()}
            {#if adminWalletAddress}
              <CopyButton text={adminWalletAddress} />
            {/if}
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Setup Token" monospace wrap={false}>
          {setupToken ? truncateMiddle(setupToken, { head: 12, tail: 12 }) : 'missing'}
          {#snippet actions()}
            {#if setupToken}
              <CopyButton text={setupToken} />
            {/if}
          {/snippet}
        </DefinitionItem>
      </DefinitionList>

      <div class="actions">
        <Button variant="solid" onclick={createAdmin} disabled={actionBusy}>
          {actionBusy ? 'Working…' : 'Create admin'}
        </Button>
        <Button variant="outline" onclick={connectAdminWallet} disabled={actionBusy}>
          Connect wallet
        </Button>
        <Button variant="ghost" onclick={() => setStep('bootstrap_login')} disabled={actionBusy}>
          Back
        </Button>
      </div>
    {:else if step === 'admin_login'}
      <Alert variant="info" title="Log in as the real admin (wallet)">
        This creates an admin session used for passkey enrollment (optional) and final activation.
      </Alert>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Admin username" monospace>
          {adminUsername || status.bootstrap?.primary_admin || 'missing'}
        </DefinitionItem>
        <DefinitionItem label="Connected Wallet" monospace wrap={false}>
          {adminWalletAddress ? truncateMiddle(adminWalletAddress, { head: 10, tail: 8 }) : 'not connected'}
          {#snippet actions()}
            {#if adminWalletAddress}
              <CopyButton text={adminWalletAddress} />
            {/if}
          {/snippet}
        </DefinitionItem>
      </DefinitionList>

      <div class="actions">
        <Button variant="solid" onclick={loginAdmin} disabled={actionBusy}>
          {actionBusy ? 'Working…' : 'Login as admin'}
        </Button>
        <Button variant="outline" onclick={connectAdminWallet} disabled={actionBusy}>
          Connect wallet
        </Button>
        <Button variant="ghost" onclick={() => setStep(status.bootstrap?.primary_admin_set ? 'status' : 'create_admin')} disabled={actionBusy}>
          Back
        </Button>
      </div>
    {:else if step === 'passkey'}
      <Alert variant="info" title="Add a passkey (recommended)">
        A passkey is optional, but strongly recommended. If you lose the admin wallet credential, teardown may be
        required.
      </Alert>

      <div class="form">
        <TextField
          label="Passkey name"
          bind:value={passkeyCredentialName}
          placeholder="Primary admin passkey"
          disabled={actionBusy}
        />
      </div>

      {#if passkeyRegistered}
        <Alert variant="success" title="Passkey registered">
          You can finalize activation now.
        </Alert>
      {/if}

      <div class="actions">
        <Button variant="solid" onclick={registerPasskey} disabled={actionBusy}>
          {actionBusy ? 'Working…' : 'Register passkey'}
        </Button>
        <div class="skip-passkey">
          <label class="skip-passkey__label">
            <Checkbox bind:checked={skipPasskeyAcknowledged} disabled={actionBusy} />
            <span>I understand: if I lose the wallet credential, teardown is required.</span>
          </label>
          <Button variant="outline" onclick={skipPasskey} disabled={actionBusy || !skipPasskeyAcknowledged}>
            Continue without passkey
          </Button>
        </div>
        <Button variant="ghost" onclick={() => setStep('admin_login')} disabled={actionBusy}>
          Back
        </Button>
      </div>
    {:else if step === 'finalize'}
      <Alert variant="warning" title="Finalize activation (destructive)">
        Finalizing will unlock the instance (liveness enabled) and delete the bootstrap actor.
      </Alert>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Client" monospace>
          {clientUrl}
          {#snippet actions()}
            <CopyButton text={clientUrl} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Auth" monospace>
          {authBase}
          {#snippet actions()}
            <CopyButton text={authBase} />
          {/snippet}
        </DefinitionItem>
      </DefinitionList>

      <div class="actions">
        <Button variant="solid" onclick={() => (finalizeConfirmOpen = true)} disabled={actionBusy}>
          Finalize activation
        </Button>
        <Button variant="ghost" onclick={() => setStep('passkey')} disabled={actionBusy}>
          Back
        </Button>
      </div>

      <Modal bind:open={finalizeConfirmOpen} title="Finalize activation" closeOnEscape closeOnBackdrop>
        <p>
          This will unlock the instance and permanently delete the bootstrap actor. Continue only if you have created and
          verified your real admin.
        </p>

        {#snippet footer()}
          <div class="modal-actions">
            <Button variant="outline" onclick={() => (finalizeConfirmOpen = false)} disabled={actionBusy}>
              Cancel
            </Button>
            <Button variant="solid" onclick={finalizeActivation} disabled={actionBusy}>
              {actionBusy ? 'Working…' : 'Confirm finalize'}
            </Button>
          </div>
        {/snippet}
      </Modal>
    {:else if step === 'done'}
      <Alert variant="success" title="Setup complete">
        Activation is finalized. You can now publish content and accept signups.
      </Alert>

      <DefinitionList density="sm" dividers>
        <DefinitionItem label="Client" monospace>
          {clientUrl}
          {#snippet actions()}
            <CopyButton text={clientUrl} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Auth" monospace>
          {authBase}
          {#snippet actions()}
            <CopyButton text={authBase} />
          {/snippet}
        </DefinitionItem>
        <DefinitionItem label="Setup Status" monospace>
          {setupStatusUrl}
          {#snippet actions()}
            <CopyButton text={setupStatusUrl} />
          {/snippet}
        </DefinitionItem>
      </DefinitionList>
    {:else}
      <Alert variant="warning" title="Unknown setup step">
        Current step: <code>{step}</code>
      </Alert>
    {/if}
  {/if}
</div>

<style>
  .setup-wizard {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg);
  }

  .stepper {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--spacing-sm);
    justify-items: center;
  }

  @media (min-width: 640px) {
    .stepper {
      grid-template-columns: repeat(6, minmax(0, 1fr));
    }
  }

  .loading {
    text-align: center;
    color: var(--text-muted);
    font-size: 0.875rem;
  }

  .actions {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--spacing-sm);
    margin-top: var(--spacing-md);
  }

  @media (min-width: 640px) {
    .actions {
      grid-template-columns: 1fr 1fr;
    }
  }

  .alert-body {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-md);
  }

  .skip-passkey {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
    width: 100%;
  }

  .skip-passkey__label {
    display: flex;
    gap: var(--spacing-sm);
    align-items: flex-start;
    color: var(--text-muted);
    font-size: 0.875rem;
    line-height: 1.4;
  }

  .skip-passkey__label :global(.gr-checkbox) {
    margin-top: 0.15rem;
  }

  .modal-actions {
    display: flex;
    gap: var(--spacing-sm);
    justify-content: flex-end;
    width: 100%;
  }
</style>
