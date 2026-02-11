<script lang="ts">
  /**
   * OAuthDeviceVerificationScreen - OAuth device authorization approval (CLI bootstrap)
   *
   * ⚠️ STATELESS STATIC APPLICATION - NO SESSIONS OR COOKIES PERMITTED ⚠️
   *
   * This component:
   * - Lets the user enter a short user_code (or deep-links via ?user_code=...)
   * - Fetches device session metadata from Lesser
   * - Requires wallet/WebAuthn login (JWT in sessionStorage) to approve/deny
   * - Never sets cookies or server-side sessions
   */

  import { onMount } from 'svelte';
  import Button from 'src/lib/greater/primitives/components/Button.svelte';
  import TextField from 'src/lib/greater/primitives/components/TextField.svelte';
  import PasswordlessLogin from './PasswordlessLogin.svelte';

  // API base URL - single-domain CloudFront routes API + UI by path.
  // For local dev, optionally set PUBLIC_LESSER_API_ORIGIN (e.g., http://localhost:8080).
  const API_URL = import.meta.env.PUBLIC_LESSER_API_ORIGIN || window.location.origin;
  const UI_BASE_PATH = (import.meta.env.BASE_URL || '/').replace(/\/$/, '');

  type VerifyResponse = {
    user_code: string;
    client_id: string;
    client_name?: string;
    client_url?: string;
    scopes?: string[];
    status: string;
    expires_in: number;
    interval: number;
  };

  type ConsentResponse = {
    status: string;
    message?: string;
  };

  type OAuthError = {
    error?: string;
    error_description?: string;
  };

  let userCode = $state('');
  let verifyResult = $state<VerifyResponse | null>(null);
  let jwt = $state('');
  let authorizedUsername = $state('');

  let error = $state('');
  let isVerifying = $state(false);
  let isApproving = $state(false);
  let isDenying = $state(false);

  let doneStatus = $state('');
  let doneMessage = $state('');

  // Map scope names to user-friendly descriptions
  const scopeDescriptions: Record<string, string> = {
    read: 'Read your profile and timeline',
    write: 'Create and manage posts',
    follow: 'Follow and unfollow accounts',
    push: 'Send push notifications',
    admin: 'Access admin features',
    'admin:read': 'Read admin data',
    'admin:write': 'Perform admin actions',
  };

  function getScopeDescription(scope: string): string {
    return scopeDescriptions[scope] || scope;
  }

  function updateURLUserCode(code: string) {
    try {
      const url = new URL(window.location.href);
      url.searchParams.set('user_code', code);
      window.history.replaceState(null, '', url.toString());
    } catch (e) {
      console.error('Failed to update URL user_code', e);
    }
  }

  async function refreshAuthState() {
    jwt = sessionStorage.getItem('lesser_auth_jwt') || '';
    if (!jwt) {
      authorizedUsername = '';
      return;
    }

    try {
      const resp = await fetch(`${API_URL}/api/v1/accounts/verify_credentials`, {
        method: 'GET',
        headers: {
          Accept: 'application/json',
          Authorization: `Bearer ${jwt}`,
        },
      });

      if (!resp.ok) {
        authorizedUsername = '';
        return;
      }

      const data = await resp.json().catch(() => ({} as any));
      authorizedUsername = data.username || data.acct || '';
    } catch (e) {
      console.error('Failed to load authorized user', e);
      authorizedUsername = '';
    }
  }

  async function verifyUserCode() {
    error = '';
    doneStatus = '';
    doneMessage = '';
    verifyResult = null;

    const trimmed = userCode.trim();
    if (!trimmed) {
      error = 'Please enter the code shown in your CLI.';
      return;
    }

    isVerifying = true;
    try {
      const resp = await fetch(`${API_URL}/oauth/device/verify`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          Accept: 'application/json',
        },
        body: new URLSearchParams({
          user_code: trimmed,
        }),
      });

      const data = (await resp.json().catch(() => ({}))) as VerifyResponse & OAuthError;
      if (!resp.ok) {
        error = data.error_description || data.error || 'Unable to verify code';
        return;
      }

      verifyResult = data as VerifyResponse;
      if (verifyResult?.user_code) {
        userCode = verifyResult.user_code;
        updateURLUserCode(verifyResult.user_code);
      }

      // If the session is already complete, display a friendly message.
      const status = (verifyResult?.status || '').toLowerCase();
      if (status === 'approved') {
        doneStatus = 'approved';
        doneMessage = 'This device has already been approved. Return to your CLI to continue.';
      } else if (status === 'denied') {
        doneStatus = 'denied';
        doneMessage = 'This device authorization was denied. Return to your CLI to retry.';
      }
    } catch (e) {
      console.error('verifyUserCode error', e);
      error = e instanceof Error ? e.message : 'Unable to verify code';
    } finally {
      isVerifying = false;
    }
  }

  async function submitConsent(action: 'approve' | 'deny') {
    error = '';

    const token = sessionStorage.getItem('lesser_auth_jwt') || '';
    if (!token) {
      error = 'Not authenticated. Please sign in to approve this device.';
      return;
    }

    if (!verifyResult?.user_code) {
      error = 'Missing verification code. Please verify again.';
      return;
    }

    if (action === 'approve') {
      isApproving = true;
    } else {
      isDenying = true;
    }

    try {
      const resp = await fetch(`${API_URL}/oauth/device/consent`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          Accept: 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: new URLSearchParams({
          user_code: verifyResult.user_code,
          action,
        }),
      });

      const data = (await resp.json().catch(() => ({}))) as ConsentResponse & OAuthError;
      if (!resp.ok) {
        error = data.error_description || data.error || 'Unable to update device authorization';
        return;
      }

      doneStatus = (data.status || action).toLowerCase();
      doneMessage =
        doneStatus === 'approved'
          ? 'Approved. Return to your CLI to finish signing in.'
          : 'Denied. Return to your CLI to retry.';

      // Clear JWT after use
      sessionStorage.removeItem('lesser_auth_jwt');
      jwt = '';
      authorizedUsername = '';
    } catch (e) {
      console.error('submitConsent error', e);
      error = e instanceof Error ? e.message : 'Unable to update device authorization';
    } finally {
      isApproving = false;
      isDenying = false;
    }
  }

  onMount(async () => {
    const urlParams = new URLSearchParams(window.location.search);
    const fromURL = urlParams.get('user_code') || '';
    if (fromURL) {
      userCode = fromURL;
    }

    await refreshAuthState();

    if (fromURL) {
      await verifyUserCode();
    }
  });
</script>

<div class="device-verify">
  {#if error}
    <div class="alert alert-error">{error}</div>
  {/if}

  {#if doneStatus}
    <div class="alert alert-info">{doneMessage}</div>
  {/if}

  <!-- Step 1: enter/verify code -->
  <div class="form-group">
    <TextField
      id="user_code"
      type="text"
      label="Verification Code"
      placeholder="ABCD-EFGH"
      bind:value={userCode}
      disabled={isVerifying || isApproving || isDenying}
      autocomplete="one-time-code"
      required
    />
  </div>

  <div class="actions">
    <Button type="button" variant="solid" onclick={verifyUserCode} disabled={isVerifying || isApproving || isDenying}>
      {#if isVerifying}
        <span class="spinner"></span>
        Verifying...
      {:else}
        Verify Code
      {/if}
    </Button>
  </div>

  {#if verifyResult}
    <div class="device-session">
      <h2 class="section-title">Request Details</h2>

      <div class="oauth-app-info">
        <h3 class="oauth-app-name">{verifyResult.client_name || verifyResult.client_id}</h3>
        {#if verifyResult.client_url}
          <a href={verifyResult.client_url} target="_blank" rel="noopener noreferrer" class="oauth-app-url">
            {verifyResult.client_url}
          </a>
        {/if}

        <div class="oauth-scopes">
          <p class="oauth-scopes-title">This CLI client would like to:</p>
          <ul class="oauth-scope-list">
            {#each (verifyResult.scopes || []) as scope}
              <li class="oauth-scope-item">{getScopeDescription(scope)}</li>
            {/each}
          </ul>
        </div>
      </div>

      {#if !doneStatus && (verifyResult.status || '').toLowerCase() === 'pending'}
        <div class="alert alert-info">
          <strong>Heads up</strong><br />
          Approving will authorize your CLI to act as your account with the requested permissions.
        </div>

        {#if jwt}
          <div class="alert alert-info">
            Authorizing as <strong>@{authorizedUsername || 'your account'}</strong>.
          </div>

          <div class="consent-actions">
            <Button type="button" variant="solid" onclick={() => submitConsent('approve')} disabled={isApproving || isDenying}>
              {#if isApproving}
                <span class="spinner"></span>
                Approving...
              {:else}
                Approve
              {/if}
            </Button>

            <Button type="button" variant="outline" onclick={() => submitConsent('deny')} disabled={isApproving || isDenying}>
              {#if isDenying}
                Denying...
              {:else}
                Deny
              {/if}
            </Button>
          </div>
        {:else}
          <div class="login-block">
            <h3 class="section-title">Sign in to approve</h3>
            <PasswordlessLogin
              hideRegisterLink={true}
              authFlow="redirect"
              postAuthRedirectTo={`${UI_BASE_PATH}/device?user_code=${encodeURIComponent(userCode)}`}
            />
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .device-verify {
    width: 100%;
    max-width: 520px;
    margin: 0 auto;
  }

  .form-group {
    margin-bottom: var(--spacing-md);
    width: 100%;
  }

  .actions {
    display: flex;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-lg);
  }

  .device-session {
    margin-top: var(--spacing-lg);
  }

  .section-title {
    font-size: 1rem;
    font-weight: 600;
    margin-bottom: var(--spacing-sm);
    color: var(--text-primary, #fff);
  }

  .consent-actions {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-md);
    margin-top: var(--spacing-lg);
  }

  .spinner {
    display: inline-block;
    width: 1rem;
    height: 1rem;
    border: 2px solid currentColor;
    border-top-color: transparent;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .login-block {
    margin-top: var(--spacing-lg);
  }
</style>
