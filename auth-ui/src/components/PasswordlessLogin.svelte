<script lang="ts">
  /**
   * PasswordlessLogin - WebAuthn + Wallet Authentication
   * 
   * ⚠️ STATELESS STATIC APPLICATION - NO SESSIONS OR COOKIES PERMITTED ⚠️
   * 
   * This is a static UI component that:
   * - Calls Lesser's authentication APIs to obtain JWTs
   * - Uses sessionStorage for temporary OAuth flow state (cleared after completion)
   * - NEVER sets cookies or creates sessions
   * - Continues OAuth via /oauth/authorize UI-mode using Authorization headers
   * 
   * Authentication methods (equal alternatives — neither is required):
   * 1. WebAuthn (passkeys, biometrics, security keys)
   * 2. Crypto wallets (MetaMask, WalletConnect, etc.)
   *
   * Both methods return JWTs from Lesser's backend, which are used once
   * and discarded. All session management is stateless JWT validation.
   *
   * On the registration page (isRegistration=true) the passkey button drives
   * the SIGNUP ceremony — /auth/webauthn/signup/begin + navigator.credentials
   * .create() + /auth/webauthn/signup/finish — and exchanges the resulting
   * single-use proof for an account at POST /api/v1/accounts. It must never
   * call the LOGIN endpoints in that mode: a brand-new username has no
   * credentials, so login/begin deterministically fails with
   * "no passkeys registered for this user".
   */
  
  import { onMount, tick } from 'svelte';
  import type { components } from 'src/lib/greater/adapters/rest/generated/lesser-api.js';
  import Button from 'src/lib/greater/primitives/components/Button.svelte';
  import CopyButton from 'src/lib/greater/primitives/components/CopyButton.svelte';
  import DefinitionItem from 'src/lib/greater/primitives/components/DefinitionItem.svelte';
  import DefinitionList from 'src/lib/greater/primitives/components/DefinitionList.svelte';
  import TextField from 'src/lib/greater/primitives/components/TextField.svelte';
  import KeyIcon from 'src/lib/greater/icons/icons/key.svelte';
  import WalletIcon from 'src/lib/greater/icons/icons/wallet.svelte';
  import { truncateMiddle } from 'src/lib/greater/utils';
  
  interface Props {
    /** OAuth session ID for flow continuation */
    sessionId?: string;
    /** Return URL after successful auth */
    returnTo?: string;
    /** Auth request data (OAuth params as query string) - deprecated, use individual params */
    authRequest?: string;
    /** OAuth client ID */
    clientId?: string;
    /** OAuth redirect URI */
    redirectUri?: string;
    /** OAuth state parameter */
    oauthState?: string;
    /** OAuth scope */
    scope?: string;
    /** OAuth response type */
    responseType?: string;
    /** PKCE code challenge */
    codeChallenge?: string;
    /** PKCE code challenge method */
    codeChallengeMethod?: string;
    /** Hide the register link at the bottom */
    hideRegisterLink?: boolean;
    /** Whether this is a registration flow (allows unlinked wallets) */
    isRegistration?: boolean;
    /** How to proceed after successful authentication */
    authFlow?: 'oauth' | 'redirect';
    /** Redirect destination when authFlow=redirect */
    postAuthRedirectTo?: string;
  }
  
  let { 
    sessionId, 
    returnTo = '/oauth/authorize', 
    authRequest: authRequestProp,
    clientId,
    redirectUri,
    oauthState,
    scope,
    responseType,
    codeChallenge,
    codeChallengeMethod,
    hideRegisterLink = false, 
    isRegistration = false,
    authFlow = 'oauth',
    postAuthRedirectTo = ''
  }: Props = $props();
  
  // API base URL - single-domain CloudFront routes API + UI by path.
  // For local dev, optionally set PUBLIC_LESSER_API_ORIGIN (e.g., http://localhost:8080).
  const API_URL = import.meta.env.PUBLIC_LESSER_API_ORIGIN || window.location.origin;

  const UI_BASE_PATH = (import.meta.env.BASE_URL || '/').replace(/\/$/, '');
  const loginHref = `${UI_BASE_PATH}/login`;
  const registerHref = `${UI_BASE_PATH}/register`;

  // Request/response shapes come from the generated OpenAPI client so the UI
  // and the server contract cannot drift silently.
  type ApiErrorBody = components['schemas']['Error'];
  type WebAuthnBeginLoginRequest = components['schemas']['WebAuthnBeginLoginRequest'];
  type WebAuthnBeginResponse = components['schemas']['WebAuthnBeginResponse'];
  type WebAuthnFinishLoginRequest = components['schemas']['WebAuthnFinishLoginRequest'];
  type WebAuthnSignupFinishRequest = components['schemas']['WebAuthnSignupFinishRequest'];
  type WebAuthnSignupFinishResponse = components['schemas']['WebAuthnSignupFinishResponse'];
  type AccountRegistrationRequest = components['schemas']['AccountRegistrationRequest'];
  type AccountRegistrationResponse = components['schemas']['AccountRegistrationResponse'];
  type AuthAuthResponse = components['schemas']['AuthAuthResponse'];

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

  async function postJson<TRequest, TResponse>(path: string, body: TRequest): Promise<TResponse> {
    const response = await fetch(`${API_URL}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });

    const data = await response.json().catch(() => null);
    if (!response.ok) {
      throw new ApiRequestError(response.status, data as Partial<ApiErrorBody> | null);
    }
    return data as TResponse;
  }

  /**
   * True when the WebAuthn ceremony ended because the person dismissed the
   * platform prompt (or we aborted it), rather than because it failed.
   */
  function isCeremonyCancellation(err: unknown): boolean {
    if (!(err instanceof Error)) {
      return false;
    }
    return err.name === 'NotAllowedError' || err.name === 'AbortError';
  }

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

  /** Serialize an assertion (login) credential into the shape the server parses. */
  function encodeAssertion(credential: PublicKeyCredential): Record<string, unknown> {
    const response = credential.response as AuthenticatorAssertionResponse;

    return {
      id: credential.id,
      rawId: arrayBufferToBase64(credential.rawId),
      type: credential.type,
      response: {
        clientDataJSON: arrayBufferToBase64(response.clientDataJSON),
        authenticatorData: arrayBufferToBase64(response.authenticatorData),
        signature: arrayBufferToBase64(response.signature),
        userHandle: response.userHandle ? arrayBufferToBase64(response.userHandle) : null
      }
    };
  }

  // State
  let username = $state('');
  let isLoading = $state(false);
  let error = $state('');
  let authMethod = $state<'webauthn' | 'wallet' | null>(null);
  let webAuthnSupported = $state(false);
  let walletConnected = $state(false);
  let connectedAddress = $state('');
  let walletChallengeId = $state('');
  // Progress text for the multi-step passkey signup, announced politely.
  let statusMessage = $state('');
  let errorRegion = $state<HTMLDivElement | null>(null);
  // Aborts an in-flight WebAuthn ceremony when the person cancels explicitly.
  let ceremonyAbort: AbortController | null = null;
  const nonOAuthURLParams = new Set(['auth_request', 'return_to', 'session_id']);

  /**
   * Surface an error and move focus to it, so keyboard and screen-reader users
   * are told what happened instead of being left on a button that did nothing.
   */
  async function failWith(message: string) {
    error = message;
    statusMessage = '';
    await tick();
    errorRegion?.focus();
  }

  /** Reset every transient bit of ceremony state. Cancellation must not strand the form. */
  function resetCeremonyState() {
    ceremonyAbort = null;
    isLoading = false;
    statusMessage = '';
  }

  /**
   * Abort a passkey ceremony that is waiting on the authenticator. Without this
   * a keyboard user who cannot dismiss the platform dialog has no way back to
   * the form. The ceremony's catch block does the actual state cleanup.
   */
  function cancelCeremony() {
    ceremonyAbort?.abort();
  }

  function setOAuthParam(params: URLSearchParams, key: string, value: unknown) {
    if (value == null) {
      return;
    }

    const stringValue = String(value);
    if (stringValue === '') {
      return;
    }

    params.set(key, stringValue);
  }

  function parseAuthRequestJson(authRequestParam: string): Record<string, unknown> {
    try {
      return JSON.parse(authRequestParam) as Record<string, unknown>;
    } catch {
      return JSON.parse(decodeURIComponent(authRequestParam)) as Record<string, unknown>;
    }
  }

  function buildAuthRequestFromJson(oauthParams: Record<string, unknown>): string {
    const params = new URLSearchParams();

    for (const [key, value] of Object.entries(oauthParams)) {
      setOAuthParam(params, key, value);
    }

    return params.toString();
  }

  function buildAuthRequestFromURLParams(
    urlParams: URLSearchParams,
    fallbacks: {
      clientId?: string;
      redirectUri?: string;
      oauthState?: string;
      scope?: string;
      responseType?: string;
      codeChallenge?: string;
      codeChallengeMethod?: string;
    }
  ): string {
    const params = new URLSearchParams();
    const urlClientId = urlParams.get('client_id') || fallbacks.clientId;
    const urlRedirectUri = urlParams.get('redirect_uri') || fallbacks.redirectUri;
    const urlState = urlParams.get('state') || fallbacks.oauthState;

    if (!urlClientId || !urlRedirectUri || !urlState) {
      return '';
    }

    setOAuthParam(params, 'client_id', urlClientId);
    setOAuthParam(params, 'redirect_uri', urlRedirectUri);
    setOAuthParam(params, 'state', urlState);
    setOAuthParam(params, 'scope', urlParams.get('scope') || fallbacks.scope);
    setOAuthParam(params, 'response_type', urlParams.get('response_type') || fallbacks.responseType);
    setOAuthParam(params, 'code_challenge', urlParams.get('code_challenge') || fallbacks.codeChallenge);
    setOAuthParam(
      params,
      'code_challenge_method',
      urlParams.get('code_challenge_method') || fallbacks.codeChallengeMethod
    );

    for (const [key, value] of urlParams.entries()) {
      if (nonOAuthURLParams.has(key) || params.has(key) || value === '') {
        continue;
      }

      params.set(key, value);
    }

    return params.toString();
  }

  function handleAuthSuccess() {
    if (authFlow === 'redirect') {
      const target = postAuthRedirectTo || window.location.href;
      try {
        window.location.href = new URL(target, window.location.origin).toString();
      } catch (e) {
        console.error('Invalid postAuthRedirectTo:', target, e);
        window.location.href = window.location.href;
      }
      return;
    }

    continueOAuthFlow();
  }
  
  // Build authRequest query string from OAuth params
  // Read directly from URL since this is client-side only
  let authRequest = $state('');
  
  // Check capabilities on mount and build authRequest from OAuth params
  onMount(() => {
    webAuthnSupported = !!window.PublicKeyCredential;
    checkWalletAvailability();
    
    // Read OAuth params directly from URL (single source of truth)
    const urlParams = new URLSearchParams(window.location.search);
    
    // Try to parse auth_request JSON first (from backend redirect)
    const authRequestParam = urlParams.get('auth_request');
    if (authRequestParam) {
      try {
        const oauthParams = parseAuthRequestJson(authRequestParam);
        authRequest = buildAuthRequestFromJson(oauthParams);
        console.log('PasswordlessLogin - built authRequest from URL JSON:', authRequest);
        
        // Store in sessionStorage for navigation persistence
        sessionStorage.setItem('lesser_oauth_auth_request', authRequest);
        
        // Also set individual props if provided (for backwards compatibility)
        if (!clientId && oauthParams.client_id) {
          // Note: Can't mutate props in Svelte 5, but we don't need to since we're using authRequest directly
        }
      } catch (e) {
        console.error('Failed to parse auth_request JSON:', e);
      }
    }
    
    // If authRequest is still empty, check if we have individual URL params
    if (!authRequest) {
      authRequest = buildAuthRequestFromURLParams(urlParams, {
        clientId,
        redirectUri,
        oauthState,
        scope,
        responseType,
        codeChallenge,
        codeChallengeMethod
      });

      if (authRequest) {
        console.log('PasswordlessLogin - built authRequest from URL params:', authRequest);
        
        // Store in sessionStorage for navigation persistence
        sessionStorage.setItem('lesser_oauth_auth_request', authRequest);
      }
    }
    
    // If authRequest is still empty, try to restore from sessionStorage (e.g., after navigating to register)
    if (!authRequest) {
      const storedAuthRequest = sessionStorage.getItem('lesser_oauth_auth_request');
      if (storedAuthRequest) {
        authRequest = storedAuthRequest;
        console.log('PasswordlessLogin - restored authRequest from sessionStorage:', authRequest);
      }
    }
    
    // Legacy: Use authRequest prop if provided and nothing else worked
    if (!authRequest && authRequestProp) {
      authRequest = authRequestProp;
      console.log('PasswordlessLogin - using authRequestProp:', authRequest);
      
      // Store in sessionStorage for navigation persistence
      sessionStorage.setItem('lesser_oauth_auth_request', authRequest);
    }
    
    // Also store returnTo in sessionStorage if provided
    const urlReturnTo = urlParams.get('return_to') || returnTo;
    if (urlReturnTo && urlReturnTo !== '/oauth/authorize') {
      sessionStorage.setItem('lesser_oauth_return_to', urlReturnTo);
    } else {
      // Restore from sessionStorage if available
      const storedReturnTo = sessionStorage.getItem('lesser_oauth_return_to');
      if (storedReturnTo) {
        // Update returnTo if it's not already set
        if (!returnTo || returnTo === '/oauth/authorize') {
          // Note: Can't mutate props, but we can use storedReturnTo directly in continueOAuthFlow
        }
      }
    }
    
    // Debug: log what we have
    console.log('PasswordlessLogin mounted - authRequest:', authRequest);
    console.log('PasswordlessLogin mounted - returnTo:', returnTo);
    console.log('PasswordlessLogin mounted - sessionId:', sessionId);
    
    if (!authRequest && !sessionId) {
      console.error('PasswordlessLogin - Missing OAuth params! URL:', window.location.href);
    }
  });
  
  function checkWalletAvailability() {
    if (typeof window !== 'undefined' && window.ethereum) {
      walletConnected = true;
    }
  }
  
  /**
   * Passkey button entry point. Registration and login are different ceremonies
   * against different endpoints; the label the person clicked has to match the
   * request that goes out.
   */
  async function loginWithWebAuthn() {
    if (isRegistration) {
      await registerWithPasskey();
      return;
    }
    await signInWithPasskey();
  }

  /**
   * Run the WebAuthn assertion ceremony and return the issued JWT.
   * Shared by passkey login and the sign-in that completes passkey signup.
   */
  async function runPasskeyLoginCeremony(usernameValue: string): Promise<string> {
    const beginData = await postJson<WebAuthnBeginLoginRequest, WebAuthnBeginResponse>(
      '/api/v1/auth/webauthn/login/begin',
      { username: usernameValue }
    );

    ceremonyAbort = new AbortController();
    const credential = (await navigator.credentials.get({
      publicKey: decodePublicKeyOptions(beginData.publicKey) as unknown as PublicKeyCredentialRequestOptions,
      signal: ceremonyAbort.signal
    })) as PublicKeyCredential | null;
    ceremonyAbort = null;

    if (!credential) {
      throw new Error('Authentication cancelled');
    }

    const finishData = await postJson<WebAuthnFinishLoginRequest, AuthAuthResponse>(
      '/api/v1/auth/webauthn/login/finish',
      {
        username: usernameValue,
        challenge: beginData.challenge,
        response: encodeAssertion(credential),
        device_name: 'Web Browser'
      }
    );

    return finishData.access_token;
  }

  // WebAuthn Login (existing account)
  async function signInWithPasskey() {
    if (!username.trim()) {
      await failWith('Please enter your username');
      return;
    }

    isLoading = true;
    error = '';
    authMethod = 'webauthn';
    statusMessage = 'Waiting for your passkey…';

    try {
      const accessToken = await runPasskeyLoginCeremony(username.trim());
      if (!accessToken) {
        throw new Error('Login succeeded but no access token was returned');
      }

      // Store JWT temporarily in sessionStorage ONLY for the redirect
      sessionStorage.setItem('lesser_auth_jwt', accessToken);
      statusMessage = 'Signed in. Continuing…';
      handleAuthSuccess();
    } catch (err) {
      console.error('WebAuthn login error:', err);
      await failWith(describePasskeyLoginError(err));
    } finally {
      resetCeremonyState();
    }
  }

  /**
   * Passkey-first account creation. No wallet, no extension, no existing
   * credential: signup ceremony -> single-use proof -> account -> signed in.
   */
  async function registerWithPasskey() {
    const usernameValue = username.trim();
    if (!usernameValue) {
      await failWith('Please choose a username');
      return;
    }

    isLoading = true;
    error = '';
    authMethod = 'webauthn';
    statusMessage = 'Starting passkey registration…';

    try {
      // Step 1: begin the SIGNUP ceremony (not login — this username has no credentials yet)
      const beginData = await postJson<WebAuthnBeginLoginRequest, WebAuthnBeginResponse>(
        '/api/v1/auth/webauthn/signup/begin',
        { username: usernameValue }
      );

      // Step 2: create the credential on the authenticator
      statusMessage = 'Confirm the new passkey on your device…';
      ceremonyAbort = new AbortController();
      const credential = (await navigator.credentials.create({
        publicKey: decodePublicKeyOptions(beginData.publicKey) as unknown as PublicKeyCredentialCreationOptions,
        signal: ceremonyAbort.signal
      })) as PublicKeyCredential | null;
      ceremonyAbort = null;

      if (!credential) {
        throw new Error('Passkey registration was cancelled');
      }

      // Step 3: exchange the attestation for a single-use registration proof
      statusMessage = 'Verifying your new passkey…';
      const finishData = await postJson<WebAuthnSignupFinishRequest, WebAuthnSignupFinishResponse>(
        '/api/v1/auth/webauthn/signup/finish',
        {
          username: usernameValue,
          challenge: beginData.challenge,
          response: encodeAttestation(credential)
        }
      );

      // Step 4: create the account with the proof (exactly one of
      // wallet_challenge_id / passkey_registration_proof is permitted)
      statusMessage = 'Creating your account…';
      await postJson<AccountRegistrationRequest, AccountRegistrationResponse>('/api/v1/accounts', {
        username: usernameValue,
        agreement: true,
        locale: 'en',
        passkey_registration_proof: finishData.passkey_registration_proof
      });

      // Step 5: the account now owns the passkey — sign in with it so the
      // person lands authenticated rather than on a login form.
      statusMessage = 'Signing you in with your new passkey…';
      const accessToken = await runPasskeyLoginCeremony(usernameValue);
      if (!accessToken) {
        throw new Error('Account created but no access token was returned');
      }

      sessionStorage.setItem('lesser_auth_jwt', accessToken);
      statusMessage = 'Account created. Continuing…';
      handleAuthSuccess();
    } catch (err) {
      console.error('Passkey registration error:', err);
      await failWith(describePasskeySignupError(err));
    } finally {
      resetCeremonyState();
    }
  }

  /**
   * Map signup failures onto advice the person can act on. The distinctions
   * matter: a taken username needs a different username, an expired proof
   * needs the ceremony run again.
   */
  function describePasskeySignupError(err: unknown): string {
    if (isCeremonyCancellation(err)) {
      return 'Passkey registration was cancelled. Your account was not created — you can try again.';
    }

    if (err instanceof ApiRequestError) {
      const message = err.serverMessage.toLowerCase();

      if (err.status === 409 || message.includes('already exists')) {
        return 'That username is already taken. Choose a different username and try again.';
      }
      if (message.includes('passkey_registration_proof') || message.includes('passkey registration proof')) {
        return 'Your passkey registration expired before the account was created. Please register the passkey again.';
      }
      if (message.includes('challenge')) {
        return 'The passkey registration timed out. Please try again.';
      }
      if (message.includes('webauthn not configured')) {
        return 'Passkey sign-up is unavailable on this server. You can register with a wallet instead.';
      }
      if (err.status === 422) {
        return `That username was rejected: ${err.serverMessage}`;
      }
      if (err.status >= 500) {
        return 'The server could not complete registration. Please try again in a moment.';
      }
      return err.serverMessage || 'Registration failed. Please try again.';
    }

    return err instanceof Error ? err.message : 'Passkey registration failed';
  }

  /** Map login failures onto advice, including the "you may need to register" case. */
  function describePasskeyLoginError(err: unknown): string {
    if (isCeremonyCancellation(err)) {
      return 'Passkey sign-in was cancelled. You can try again.';
    }

    if (err instanceof ApiRequestError) {
      const message = err.serverMessage.toLowerCase();

      if (message.includes('no passkeys registered')) {
        return 'No passkey is registered for that username. Register first, or sign in another way.';
      }
      if (message.includes('user not found')) {
        return 'We could not find that username.';
      }
      if (err.status === 401) {
        return 'That passkey was not accepted. Please try again.';
      }
      if (err.status >= 500) {
        return 'The server could not complete sign-in. Please try again in a moment.';
      }
      return err.serverMessage || 'Sign-in failed. Please try again.';
    }

    return err instanceof Error ? err.message : 'WebAuthn login failed';
  }


  // Wallet Login
  async function loginWithWallet() {
    if (!window.ethereum) {
      await failWith('No wallet detected. Please install MetaMask or another Web3 wallet.');
      return;
    }

    // For registration, require username first (signature must bind to username)
    if (isRegistration && !username.trim()) {
      await failWith('Please enter a username first');
      return;
    }

    isLoading = true;
    error = '';
    authMethod = 'wallet';

    try {
      // Step 1: Request wallet connection
      const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
      const address = accounts[0];
      connectedAddress = address;
      
      // Step 2: Get chain ID
      const chainId = await window.ethereum.request({ method: 'eth_chainId' });
      const chainIdDecimal = parseInt(chainId, 16);
      
      // Step 3: Create challenge (includes username for security)
      // Username MUST be provided - signature binds to username
      if (!username.trim()) {
        isLoading = false;
        await failWith('Username is required for wallet authentication');
        return;
      }

      const challengeResponse = await fetch(`${API_URL}/auth/wallet/challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          address, 
          chainId: chainIdDecimal,
          username: username.trim()
        })
      });
      
      const challengeData = await challengeResponse.json();
      if (!challengeResponse.ok) {
        throw new Error(challengeData.error || 'Failed to create challenge');
      }
      
      // Step 4: Sign message
      const signature = await window.ethereum.request({
        method: 'personal_sign',
        params: [challengeData.message, address]
      });
      
      // Step 5: Login or verify based on context
      // For registration: verify signature only
      // For login: verify signature AND create session
      const endpoint = isRegistration ? '/auth/wallet/verify' : '/auth/wallet/login';
      const verifyResponse = await fetch(`${API_URL}${endpoint}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          challengeId: challengeData.id,
          address,
          signature,
          message: challengeData.message
        })
      });
      
      const verifyData = await verifyResponse.json();
      
      if (!verifyResponse.ok) {
        throw new Error(verifyData.error || 'Authentication failed');
      }
      
      // Step 6: Handle response
      if (verifyData.access_token) {
        // Login successful - store JWT temporarily for OAuth flow
        sessionStorage.setItem('lesser_auth_jwt', verifyData.access_token);
        handleAuthSuccess();
      } else if (isRegistration && verifyData.verified) {
        // Wallet verified for registration - proceed with account creation
        // Username was already collected and bound to signature - proceed directly
        await registerWithWallet(username, address, chainIdDecimal, challengeData.id, signature, challengeData.message);
      } else if (!isRegistration) {
        // Login failed - wallet not linked
        await failWith('This wallet is not linked to any account. Please register first.');
      }
    } catch (err) {
      console.error('Wallet login error:', err);
      await failWith(err instanceof Error ? err.message : 'Wallet login failed');
    } finally {
      isLoading = false;
    }
  }
  
  // Register with wallet (called after wallet verification on registration page)
  async function registerWithWallet(usernameValue: string, address: string, chainId: number, challengeId: string, signature: string, message: string) {
    isLoading = true;
    error = '';
    
    try {
      // Step 1: Register account (email is deprecated and disallowed)
      const registerResponse = await fetch(`${API_URL}/api/v1/accounts`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username: usernameValue,
          agreement: true,
          locale: 'en',
          wallet_challenge_id: challengeId
        })
      });
      
      const registerData = await registerResponse.json();
      if (!registerResponse.ok) {
        // Show detailed error message from API
        let errorMsg = 'Registration failed';
        if (registerData.error) {
          errorMsg = registerData.error;
        } else if (registerData.message) {
          errorMsg = registerData.message;
        } else if (registerData.errors && Array.isArray(registerData.errors)) {
          errorMsg = registerData.errors.join(', ');
        } else if (typeof registerData === 'string') {
          errorMsg = registerData;
        } else {
          errorMsg = `Registration failed: ${registerResponse.status}`;
        }
        console.error('Registration error:', registerData);
        throw new Error(errorMsg);
      }
      
      // Step 2: Link wallet to the new account
      // Registration doesn't return an access token, so we pass username
      const linkResponse = await fetch(`${API_URL}/auth/wallet/link`, {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json'
          // No Authorization header - username provided in body for registration flow
        },
        body: JSON.stringify({
          username: usernameValue, // Required for registration flow
          address,
          chainId,
          walletType: 'ethereum',
          challengeId,
          signature,
          message
        })
      });
      
      const linkData = await linkResponse.json();
      if (!linkResponse.ok) {
        throw new Error(linkData.error || 'Failed to link wallet');
      }
      
      // Step 3: Wallet linked successfully - backend now returns JWT in response
      const jwt = linkData.access_token;
      
      if (jwt) {
        // Store JWT and continue OAuth flow
        sessionStorage.setItem('lesser_auth_jwt', jwt);
        handleAuthSuccess();
      } else {
        // Fallback: if backend doesn't return JWT (shouldn't happen, but handle gracefully)
        console.error('Link response missing access_token:', linkData);
        await failWith('Registration successful, but authentication token missing. Please log in.');

        setTimeout(() => {
          const loginUrl = new URL(loginHref, window.location.origin);
          window.location.href = loginUrl.toString();
        }, 2000);
      }
    } catch (err) {
      console.error('Wallet registration error:', err);
      await failWith(err instanceof Error ? err.message : 'Registration failed');
    } finally {
      isLoading = false;
    }
  }
  
  async function continueOAuthFlow() {
    // Debug: log what we have
    console.log('continueOAuthFlow - authRequest:', authRequest);
    console.log('continueOAuthFlow - sessionId:', sessionId);
    console.log('continueOAuthFlow - returnTo:', returnTo);
    
    // Try to restore authRequest from sessionStorage if not available (e.g., after registration)
    let effectiveAuthRequest = authRequest;
    if (!effectiveAuthRequest) {
      const storedAuthRequest = sessionStorage.getItem('lesser_oauth_auth_request');
      if (storedAuthRequest) {
        effectiveAuthRequest = storedAuthRequest;
        console.log('continueOAuthFlow - restored authRequest from sessionStorage');
      }
    }
    
    // Validate OAuth params exist before redirecting
    if (!effectiveAuthRequest && !sessionId) {
      error = 'Missing OAuth parameters. Please restart the authorization flow.';
      console.error('Missing OAuth params - authRequest:', effectiveAuthRequest, 'sessionId:', sessionId);
      return;
    }
    
    // Get JWT from sessionStorage (set after successful login)
    const jwt = sessionStorage.getItem('lesser_auth_jwt') || '';
    if (!jwt) {
      error = 'Not authenticated. Please log in again.';
      return;
    }
    
    // Validate API_URL is valid
    try {
      new URL(API_URL);
    } catch (e) {
      error = 'Invalid API configuration. Please contact support.';
      console.error('Invalid API_URL:', API_URL, e);
      return;
    }
    
    // Parse returnTo: extract path if it's a full URL, otherwise use as-is
    // Also check sessionStorage for returnTo
    let effectiveReturnTo = returnTo || sessionStorage.getItem('lesser_oauth_return_to') || '/oauth/authorize';
    let redirectPath = effectiveReturnTo;
    try {
      // If it's a full URL, extract just the pathname
      const url = new URL(effectiveReturnTo);
      redirectPath = url.pathname;
    } catch {
      // If it's not a valid URL, treat as relative path
      redirectPath = effectiveReturnTo.startsWith('/') ? effectiveReturnTo : `/${effectiveReturnTo}`;
    }
    
    // Default to OAuth authorize if path is root or empty
    if (!redirectPath || redirectPath === '/') {
      redirectPath = '/oauth/authorize';
    }
    
    const authorizeURL = new URL(`${API_URL}${redirectPath}`);
    
    // Build query string with OAuth params
    let params: URLSearchParams;
    if (effectiveAuthRequest) {
      // authRequest already contains all OAuth params including PKCE:
      // client_id, redirect_uri, state, scope, response_type, code_challenge, code_challenge_method
      // Validate that authRequest contains required params before using it
      params = new URLSearchParams(effectiveAuthRequest);
      if (!params.get('client_id') || !params.get('redirect_uri') || !params.get('state')) {
        error = 'Invalid OAuth parameters. Please restart the authorization flow.';
        return;
      }
    } else if (sessionId) {
      // Fallback to session_id if no authRequest (legacy flow)
      params = new URLSearchParams({ session_id: sessionId });
    } else {
      error = 'Missing OAuth parameters. Please restart the authorization flow.';
      return;
    }
    
    // UI-mode: server returns { next_url } instead of issuing 302 redirects.
    params.set('mode', 'ui');
    authorizeURL.search = params.toString();
    
    try {
      const response = await fetch(authorizeURL.toString(), {
        method: 'GET',
        headers: {
          'Accept': 'application/json',
          'Authorization': `Bearer ${jwt}`,
        },
      });
      
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        error = data.error_description || data.error || 'Authorization failed';
        return;
      }
      
      const nextURL = data.next_url;
      if (!nextURL || typeof nextURL !== 'string') {
        error = 'Unexpected response from server';
        return;
      }
      
      // Clear OAuth params now that the server has progressed the flow.
      sessionStorage.removeItem('lesser_oauth_auth_request');
      sessionStorage.removeItem('lesser_oauth_return_to');
      
      let parsedNextURL: URL;
      try {
        parsedNextURL = new URL(nextURL, window.location.origin);
      } catch (e) {
        error = 'Invalid redirect URL. Please try again or contact support.';
        console.error('Invalid next_url:', nextURL, e);
        return;
      }
      
      const isSameOrigin = parsedNextURL.origin === window.location.origin;
      const consentPath = `${UI_BASE_PATH}/consent`;
      const isAuthConsent = isSameOrigin && (parsedNextURL.pathname === consentPath || parsedNextURL.pathname === `${consentPath}/`);
      
      // Keep the JWT only while navigating within the auth UI (e.g., to consent).
      if (!isAuthConsent) {
        sessionStorage.removeItem('lesser_auth_jwt');
      }
      
      window.location.href = parsedNextURL.toString();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Authorization failed';
      console.error('continueOAuthFlow error:', err);
    }
  }
  
  // Helper functions for WebAuthn
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
    return btoa(binary)
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=/g, '');
  }
</script>

<div class="passwordless-login">
    {#if error}
      <div
        class="alert alert-error"
        role="alert"
        aria-live="assertive"
        tabindex="-1"
        bind:this={errorRegion}
        data-testid="auth-error"
      >
        <h3>Error</h3>
        <p>{error}</p>
        <div class="error-actions">
          {#if isRegistration}
            <!-- Stay on the register page: sending a registering person to
                 /login strands them mid-signup with no account to log in to. -->
            <a href={loginHref} class="btn btn-primary">Sign in instead</a>
          {:else}
            <a href={loginHref} class="btn btn-primary">Try Again</a>
          {/if}
        </div>
      </div>
    {/if}

    <!-- Progress for the multi-step passkey ceremony. Polite: it must not
         interrupt the platform's own passkey prompt. -->
    <div class="gr-sr-only" role="status" aria-live="polite" data-testid="auth-status">
      {statusMessage}
    </div>

    <!-- Single username field -->
    <div class="form-group">
        <TextField
          id="username"
          type="text"
          label="Username"
          bind:value={username}
          placeholder={isRegistration ? "Choose a username" : "Enter your username"}
          disabled={isLoading}
          autocomplete="username webauthn"
          required
        />
      </div>
      
      <!-- Auth buttons side by side -->
      <div class="auth-buttons">
        <Button
          type="button"
          variant="solid"
          disabled={!webAuthnSupported || isLoading}
          aria-busy={isLoading && authMethod === 'webauthn'}
          aria-label={isRegistration
            ? 'Register a passkey and create your account'
            : 'Sign in with your passkey'}
          data-testid="passkey-button"
          onclick={loginWithWebAuthn}
        >
          {#if isLoading && authMethod === 'webauthn'}
            <span class="spinner"></span>
            {isRegistration ? 'Registering...' : 'Authenticating...'}
          {:else if !webAuthnSupported}
            Passkeys Not Supported
          {:else}
            <KeyIcon class="btn-icon" />
            {isRegistration ? 'Register Passkey' : 'Passkey Login'}
          {/if}
        </Button>

        <Button
          type="button"
          variant="solid"
          onclick={loginWithWallet}
          disabled={isLoading}
          aria-busy={isLoading && authMethod === 'wallet'}
          aria-label={isRegistration
            ? 'Register a crypto wallet and create your account'
            : 'Sign in with your crypto wallet'}
          data-testid="wallet-button"
        >
          {#if isLoading && authMethod === 'wallet'}
            <span class="spinner"></span>
            Connecting Wallet...
          {:else}
            <WalletIcon class="btn-icon" />
            {isRegistration ? 'Register Wallet' : 'Wallet Login'}
          {/if}
        </Button>
      </div>

      {#if isLoading && authMethod === 'webauthn'}
        <div class="ceremony-actions">
          <Button
            type="button"
            variant="ghost"
            onclick={cancelCeremony}
            data-testid="cancel-ceremony-button"
          >
            Cancel
          </Button>
        </div>
      {/if}

      {#if connectedAddress}
        <div class="wallet-details">
          <DefinitionList density="sm" dividers>
            <DefinitionItem label="Connected Wallet" monospace wrap={false}>
              {truncateMiddle(connectedAddress, { head: 10, tail: 8 })}
              {#snippet actions()}
                <CopyButton text={connectedAddress} />
              {/snippet}
            </DefinitionItem>
          </DefinitionList>
        </div>
      {/if}
      
      <!-- Info -->
      {#if !webAuthnSupported}
        <div class="alert alert-info mt-3">
          <strong>Passkeys not available</strong><br>
          Please use a modern browser (Chrome, Safari, Edge) or sign in with your crypto wallet.
        </div>
      {/if}
      
      {#if !hideRegisterLink}
        <div class="auth-footnote text-center mt-4">
          Don't have an account? <a class="auth-link" href={registerHref}>Register</a>
        </div>
      {/if}
  </div>

<style>
  .passwordless-login {
    width: 100%;
    max-width: 400px;
    margin: 0 auto;
  }
  
  .form-group {
    margin-bottom: var(--spacing-md);
    width: 100%;
  }
  
  .form-group :global(input),
  .form-group :global(.text-field) {
    width: 100%;
  }
  
  .form-group :global(label) {
    color: var(--text-secondary, rgba(255, 255, 255, 0.8));
  }
  
  .auth-buttons {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-md);
  }
  
  .auth-buttons :global(.gr-button__content) {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
  }
  
  .auth-buttons :global(.btn-icon) {
    width: 1.25rem;
    height: 1.25rem;
    flex-shrink: 0;
    display: inline-block;
  }
  
  .auth-buttons :global(.btn-icon svg) {
    display: block;
    width: 100%;
    height: 100%;
  }

  .ceremony-actions {
    display: flex;
    justify-content: center;
    margin-bottom: var(--spacing-md);
  }

  .wallet-details {
    margin-top: var(--spacing-sm);
  }
  
  @media (max-width: 640px) {
    .auth-buttons {
      grid-template-columns: 1fr;
    }
  }
  
  .spinner {
    display: inline-block;
    width: 1rem;
    height: 1rem;
    border: 2px solid currentColor;
    border-top-color: transparent;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
    margin-right: 0.5rem;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  .alert-error {
    background-color: rgba(220, 53, 69, 0.1);
    border: 1px solid rgba(220, 53, 69, 0.3);
    color: var(--text-primary, #fff);
    padding: var(--spacing-lg);
    border-radius: var(--border-radius-md);
    margin-bottom: var(--spacing-md);
  }
  
  .alert-error h3 {
    margin-top: 0;
    margin-bottom: var(--spacing-sm);
    font-size: 1.25rem;
    font-weight: 600;
  }
  
  .alert-error p {
    margin-bottom: var(--spacing-md);
    line-height: 1.5;
  }
  
  .error-actions {
    display: flex;
    gap: var(--spacing-sm);
    flex-wrap: wrap;
    margin-top: var(--spacing-md);
  }
  
  .btn {
    display: inline-block;
    padding: var(--spacing-sm) var(--spacing-md);
    border-radius: var(--border-radius-sm);
    text-decoration: none;
    font-weight: 500;
    transition: opacity 0.2s;
    cursor: pointer;
    border: none;
  }
  
  .btn:hover {
    opacity: 0.8;
  }
  
  .btn-primary {
    background-color: var(--lesser-primary-500, #007bff);
    color: white;
  }
  
</style>
