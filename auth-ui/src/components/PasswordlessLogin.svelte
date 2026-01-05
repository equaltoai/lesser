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
   * Authentication methods:
   * 1. WebAuthn (passkeys, biometrics, security keys)
   * 2. Crypto wallets (MetaMask, WalletConnect, etc.)
   * 
   * Both methods return JWTs from Lesser's backend, which are used once
   * and discarded. All session management is stateless JWT validation.
   */
  
  import { onMount } from 'svelte';
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
    isRegistration = false 
  }: Props = $props();
  
  // API base URL - single-domain CloudFront routes API + UI by path.
  // For local dev, optionally set PUBLIC_LESSER_API_ORIGIN (e.g., http://localhost:8080).
  const API_URL = import.meta.env.PUBLIC_LESSER_API_ORIGIN || window.location.origin;

  const UI_BASE_PATH = (import.meta.env.BASE_URL || '/').replace(/\/$/, '');
  const loginHref = `${UI_BASE_PATH}/login`;
  const registerHref = `${UI_BASE_PATH}/register`;
  
  // State
  let username = $state('');
  let isLoading = $state(false);
  let error = $state('');
  let authMethod = $state<'webauthn' | 'wallet' | null>(null);
  let webAuthnSupported = $state(false);
  let walletConnected = $state(false);
  let connectedAddress = $state('');
  let walletChallengeId = $state('');
  
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
        const decoded = decodeURIComponent(authRequestParam);
        const oauthParams = JSON.parse(decoded);
        
        // Build query string from parsed params
        const params = new URLSearchParams();
        if (oauthParams.client_id) params.set('client_id', oauthParams.client_id);
        if (oauthParams.redirect_uri) params.set('redirect_uri', oauthParams.redirect_uri);
        if (oauthParams.state) params.set('state', oauthParams.state);
        if (oauthParams.scope) params.set('scope', oauthParams.scope);
        if (oauthParams.response_type) params.set('response_type', oauthParams.response_type);
        if (oauthParams.code_challenge) params.set('code_challenge', oauthParams.code_challenge);
        if (oauthParams.code_challenge_method) params.set('code_challenge_method', oauthParams.code_challenge_method);
        
        authRequest = params.toString();
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
      const params = new URLSearchParams();
      const urlClientId = urlParams.get('client_id') || clientId;
      const urlRedirectUri = urlParams.get('redirect_uri') || redirectUri;
      const urlState = urlParams.get('state') || oauthState;
      
      if (urlClientId && urlRedirectUri && urlState) {
        params.set('client_id', urlClientId);
        params.set('redirect_uri', urlRedirectUri);
        params.set('state', urlState);
        if (urlParams.get('scope') || scope) params.set('scope', urlParams.get('scope') || scope || '');
        if (urlParams.get('response_type') || responseType) params.set('response_type', urlParams.get('response_type') || responseType || '');
        if (urlParams.get('code_challenge') || codeChallenge) params.set('code_challenge', urlParams.get('code_challenge') || codeChallenge || '');
        if (urlParams.get('code_challenge_method') || codeChallengeMethod) params.set('code_challenge_method', urlParams.get('code_challenge_method') || codeChallengeMethod || '');
        
        authRequest = params.toString();
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
  
  // WebAuthn Login
  async function loginWithWebAuthn() {
    if (!username.trim()) {
      error = 'Please enter your username';
      return;
    }
    
    isLoading = true;
    error = '';
    authMethod = 'webauthn';
    
    try {
      // Step 1: Begin WebAuthn login
      const beginResponse = await fetch(`${API_URL}/api/v1/auth/webauthn/login/begin`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username })
      });
      
      const beginData = await beginResponse.json();
      if (!beginResponse.ok) {
        throw new Error(beginData.error || 'Failed to start login');
      }
      
      // Step 2: Convert base64 challenge and credentials
      const publicKeyOptions = beginData.publicKey;
      publicKeyOptions.challenge = base64ToArrayBuffer(publicKeyOptions.challenge);
      
      if (publicKeyOptions.allowCredentials) {
        publicKeyOptions.allowCredentials = publicKeyOptions.allowCredentials.map((cred: any) => ({
          ...cred,
          id: base64ToArrayBuffer(cred.id)
        }));
      }
      
      // Step 3: Get credential from authenticator
      const credential = await navigator.credentials.get({ publicKey: publicKeyOptions }) as PublicKeyCredential;
      
      if (!credential) {
        throw new Error('Authentication cancelled');
      }
      
      // Step 4: Prepare response
      const credentialResponse = {
        id: credential.id,
        rawId: arrayBufferToBase64(credential.rawId),
        type: credential.type,
        response: {
          clientDataJSON: arrayBufferToBase64((credential.response as AuthenticatorAssertionResponse).clientDataJSON),
          authenticatorData: arrayBufferToBase64((credential.response as AuthenticatorAssertionResponse).authenticatorData),
          signature: arrayBufferToBase64((credential.response as AuthenticatorAssertionResponse).signature),
          userHandle: (credential.response as AuthenticatorAssertionResponse).userHandle ? 
            arrayBufferToBase64((credential.response as AuthenticatorAssertionResponse).userHandle!) : null
        }
      };
      
      // Step 5: Complete login
      const finishResponse = await fetch(`${API_URL}/api/v1/auth/webauthn/login/finish`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          username,
          challenge: beginData.challenge,
          response: credentialResponse,
          device_name: 'Web Browser'
        })
      });
      
      const finishData = await finishResponse.json();
      
      if (!finishResponse.ok) {
        throw new Error(finishData.error || 'Login failed');
      }
      
      // Step 6: Store JWT temporarily in sessionStorage ONLY for the redirect
      if (finishData.access_token) {
        sessionStorage.setItem('lesser_auth_jwt', finishData.access_token);
        continueOAuthFlow();
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'WebAuthn login failed';
      console.error('WebAuthn login error:', err);
    } finally {
      isLoading = false;
    }
  }
  
  // Wallet Login
  async function loginWithWallet() {
    if (!window.ethereum) {
      error = 'No wallet detected. Please install MetaMask or another Web3 wallet.';
      return;
    }

    // For registration, require username first (signature must bind to username)
    if (isRegistration && !username.trim()) {
      error = 'Please enter a username first';
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
        error = 'Username is required for wallet authentication';
        isLoading = false;
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
        continueOAuthFlow();
      } else if (isRegistration && verifyData.verified) {
        // Wallet verified for registration - proceed with account creation
        // Username was already collected and bound to signature - proceed directly
        await registerWithWallet(username, address, chainIdDecimal, challengeData.id, signature, challengeData.message);
      } else if (!isRegistration) {
        // Login failed - wallet not linked
        error = 'This wallet is not linked to any account. Please register first.';
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Wallet login failed';
      console.error('Wallet login error:', err);
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
          locale: 'en'
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
        continueOAuthFlow();
      } else {
        // Fallback: if backend doesn't return JWT (shouldn't happen, but handle gracefully)
        error = 'Registration successful, but authentication token missing. Please log in.';
        console.error('Link response missing access_token:', linkData);
        
        setTimeout(() => {
          const loginUrl = new URL(loginHref, window.location.origin);
          window.location.href = loginUrl.toString();
        }, 2000);
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Registration failed';
      console.error('Wallet registration error:', err);
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
      <div class="alert alert-error">
        <h3>Error</h3>
        <p>{error}</p>
        <div class="error-actions">
          <a href={loginHref} class="btn btn-primary">Try Again</a>
        </div>
      </div>
    {/if}
    
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
          onclick={loginWithWebAuthn}
        >
          {#if isLoading && authMethod === 'webauthn'}
            <span class="spinner"></span>
            Authenticating...
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
        <div class="text-center mt-4" style="font-size: 0.875rem; color: var(--text-muted);">
          Don't have an account? <a href={registerHref} style="color: var(--lesser-primary-500); text-decoration: none;">Register</a>
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
