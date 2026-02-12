<script lang="ts">
  /**
   * OAuthConsentScreen - OAuth authorization consent
   * 
   * ⚠️ STATELESS STATIC APPLICATION - NO SESSIONS OR COOKIES PERMITTED ⚠️
   * 
   * This is a static UI component that:
   * - Reads JWT from sessionStorage (set after login)
   * - NEVER sets cookies or creates sessions
   * - Passes JWT back to Lesser via Authorization header for stateless validation
   * 
   * Flow:
   * 1. User logs in → JWT stored in sessionStorage
   * 2. Lesser redirects here for consent
   * 3. User approves/denies → sends JWT to Lesser
   * 4. Lesser validates JWT (stateless) and continues OAuth flow
   * 5. JWT is cleared from sessionStorage after use
   */
  
  import Button from 'src/lib/greater/primitives/components/Button.svelte';
  import { onMount } from 'svelte';
  
  // API base URL - single-domain CloudFront routes API + UI by path.
  // For local dev, optionally set PUBLIC_LESSER_API_ORIGIN (e.g., http://localhost:8080).
  const API_URL = import.meta.env.PUBLIC_LESSER_API_ORIGIN || window.location.origin;
  
  interface Props {
    // Props are optional - component reads from URL directly
  }
  
  let { }: Props = $props();
  
  // State - read from URL params
  let clientId = $state('');
  let clientName = $state('');
  let clientUrl = $state('');
  let scopes = $state<string[]>([]);
  let oauthState = $state('');
  let redirectUri = $state('');
  let error = $state('');
  let isApproving = $state(false);
  let isDenying = $state(false);
  
  // Read OAuth params from URL on mount
  onMount(() => {
    const urlParams = new URLSearchParams(window.location.search);
    
    clientId = urlParams.get('client_id') || '';
    clientName = urlParams.get('client_name') || clientId;
    clientUrl = urlParams.get('client_url') || '';
    oauthState = urlParams.get('state') || '';
    redirectUri = urlParams.get('redirect_uri') || '';
    
    const scopesParam = urlParams.get('scopes') || 'read write';
    scopes = scopesParam.split(' ').filter(s => s.trim());
    
    const jwt = sessionStorage.getItem('lesser_auth_jwt') || '';
    if (!jwt) {
      error = 'Not authenticated. Please log in again.';
    }
    
    // Validate required params
    if (!clientId || !oauthState || !redirectUri) {
      error = 'Invalid authorization request - missing required parameters. Please restart the authorization flow from your application.';
    }
  });
  
  // Map scope names to user-friendly descriptions
  const scopeDescriptions: Record<string, string> = {
    'read': 'Read your profile and timeline',
    'write': 'Create and manage posts',
    'follow': 'Follow and unfollow accounts',
    'push': 'Send push notifications',
    'admin': 'Access admin features',
    'admin:read': 'Read admin data',
    'admin:write': 'Perform admin actions',
  };
  
  function getScopeDescription(scope: string): string {
    return scopeDescriptions[scope] || scope;
  }
  
  async function handleApprove() {
    isApproving = true;
    error = '';
    
    const jwt = sessionStorage.getItem('lesser_auth_jwt') || '';
    
    if (!jwt) {
      error = 'Not authenticated. Please log in again.';
      isApproving = false;
      return;
    }
    
    try {
      const response = await fetch(`${API_URL}/oauth/consent`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Authorization': `Bearer ${jwt}`
        },
        body: new URLSearchParams({
          state: oauthState,
          action: 'approve'
        }),
        redirect: 'manual'
      });
      
      // Clear JWT after use
      sessionStorage.removeItem('lesser_auth_jwt');
      
      // Lesser will redirect to client callback with authorization code
      // Follow the redirect
      if (response.ok) {
        // If response is OK but not a redirect, try to get redirect from response
        const data = await response.json().catch(() => ({}));
        if (data.redirect_uri) {
          window.location.href = data.redirect_uri;
        } else {
          error = 'Unexpected response from server';
          isApproving = false;
        }
      } else {
        // Handle error response
        const errorData = await response.json().catch(() => ({}));
        error = errorData.error_description || errorData.error || 'Failed to approve authorization';
        isApproving = false;
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to approve authorization';
      console.error('Approval error:', err);
      sessionStorage.removeItem('lesser_auth_jwt');
      isApproving = false;
    }
  }
  
  async function handleDeny() {
    isDenying = true;
    error = '';
    
    const jwt = sessionStorage.getItem('lesser_auth_jwt') || '';
    
    if (!jwt) {
      error = 'Not authenticated. Please log in again.';
      isDenying = false;
      return;
    }
    
    try {
      const response = await fetch(`${API_URL}/oauth/consent`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Authorization': `Bearer ${jwt}`
        },
        body: new URLSearchParams({
          state: oauthState,
          action: 'deny'
        }),
        redirect: 'manual'
      });
      
      // Clear JWT after use
      sessionStorage.removeItem('lesser_auth_jwt');
      
      // Lesser will redirect to client callback with error
      // Follow the redirect
      if (response.ok) {
        // If response is OK but not a redirect, try to get redirect from response
        const data = await response.json().catch(() => ({}));
        if (data.redirect_uri) {
          window.location.href = data.redirect_uri;
        } else {
          error = 'Unexpected response from server';
          isDenying = false;
        }
      } else {
        // Handle error response
        const errorData = await response.json().catch(() => ({}));
        error = errorData.error_description || errorData.error || 'Failed to deny authorization';
        isDenying = false;
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to deny authorization';
      console.error('Denial error:', err);
      sessionStorage.removeItem('lesser_auth_jwt');
      isDenying = false;
    }
  }
</script>

<div class="oauth-consent">
    {#if error}
      <div class="alert alert-error">
        {error}
      </div>
    {/if}
    
    <!-- App Information -->
    <div class="oauth-app-info">
      <h2 class="oauth-app-name">{clientName}</h2>
      {#if clientUrl}
        <a href={clientUrl} target="_blank" rel="noopener noreferrer" class="oauth-app-url">
          {clientUrl}
        </a>
      {/if}
      
      <div class="oauth-scopes">
        <p class="oauth-scopes-title">This application would like to:</p>
        <ul class="oauth-scope-list">
          {#each scopes as scope}
            <li class="oauth-scope-item">{getScopeDescription(scope)}</li>
          {/each}
        </ul>
      </div>
    </div>
    
    <!-- Security Notice -->
    <div class="alert alert-info">
      <strong>Security Notice</strong><br>
      Only authorize applications you trust. You can revoke access at any time from your account settings.
    </div>
    
    <!-- Action Buttons -->
    <div class="consent-actions">
      <Button
        type="button"
        variant="solid"
        onclick={handleApprove}
        disabled={isApproving || isDenying}
      >
        {#if isApproving}
          <span class="spinner"></span>
          Authorizing...
        {:else}
          Authorize {clientName}
        {/if}
      </Button>
      
      <Button
        type="button"
        variant="outline"
        onclick={handleDeny}
        disabled={isApproving || isDenying}
        class="deny-button"
      >
        {#if isDenying}
          Canceling...
        {:else}
          Deny
        {/if}
      </Button>
    </div>
    
    <!-- Additional Info -->
    <div class="auth-footnote auth-footnote--xs text-center mt-4">
      By authorizing, you agree to share your profile information and grant the requested permissions.
    </div>
  </div>

<style>
  .oauth-consent {
    width: 100%;
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
    to { transform: rotate(360deg); }
  }
  
  /* Improve contrast for deny button */
  :global(.consent-actions .deny-button) {
    background-color: rgba(255, 255, 255, 0.1) !important;
    border-color: rgba(255, 255, 255, 0.4) !important;
    color: rgba(255, 255, 255, 0.9) !important;
  }
  
  :global(.consent-actions .deny-button:hover:not(:disabled)) {
    background-color: rgba(220, 53, 69, 0.2) !important;
    border-color: rgba(220, 53, 69, 0.6) !important;
    color: rgba(255, 255, 255, 1) !important;
  }
  
  :global(.consent-actions .deny-button:disabled) {
    opacity: 0.5;
  }
</style>
