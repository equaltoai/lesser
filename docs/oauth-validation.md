Mastodon implements several validation rules for OAuth clients and redirects to ensure security and proper authentication flow. Here are the key validation rules:

## **Redirect URI Validation**

The redirect URI validation is strict and requires exact matching:[1][2]

- **Must match registered URIs**: The `redirect_uri` parameter used during authorization must exactly match one of the `redirect_uris` declared during app registration
- **Special URIs supported**: The out-of-band URI `urn:ietf:wg:oauth:2.0:oob` is accepted, which displays the authorization code instead of redirecting
- **Multiple URIs allowed during registration**: Apps can register multiple redirect URIs, but only one can be used per authorization request
- **Known issues**: Some URI formats like Android Intent URIs have had validation problems[3]

## **Scope Validation**

Mastodon enforces strict scope validation:[4][1]

- **Subset requirement**: Requested scopes must be a subset of the scopes registered with the application
- **Cannot exceed registered scopes**: You cannot request any scope that wasn't included in the original registration
- **Default scope**: If no scope is specified, it defaults to `read`
- **Parameter naming**: Use `scopes` (plural) during registration but `scope` (singular) when requesting authorization

## **Client Authentication Rules**

OAuth clients must authenticate properly:[2][5]

- **Required credentials**: `client_id` and `client_secret` are required for most flows
- **Credential matching**: The client ID/secret pair must match what was issued during registration
- **Secondary validation**: Mastodon checks that the ID/secret pair matches the redirect URI as an additional security measure
- **Client types**: Currently only supports confidential clients (though public client support is being developed)[1]

## **OAuth Flow Validation**

Mastodon validates OAuth flow parameters:[2][1]

- **Response type**: Must be set to `code` for authorization code flow
- **Grant types supported**: 
  - `authorization_code` for user authorization
  - `client_credentials` for app-only access
  - Password grant (deprecated as of version 4.4.0)
- **PKCE support**: Added in version 4.3.0, requires `code_challenge_method` to be `S256`

## **Additional Security Rules**

- **State parameter**: Supports state parameter for CSRF protection and passing arbitrary data[1]
- **Force login**: Supports `force_login` parameter to require re-authentication[2]
- **Token validation**: Tokens can be revoked and must be validated periodically[6]
- **Domain-specific registration**: Apps must be registered separately for each Mastodon instance[5]

## **Common Implementation Gotchas**

Several common mistakes can cause validation failures:[7][8][1]

- Using `redirect_uris` instead of `redirect_uri` during authorization
- Using `scopes` instead of `scope` during authorization  
- Providing a redirect URI that wasn't registered with the app
- Attempting to use scopes beyond what was registered
- Client ID/secret mismatches when redirect URIs change

These validation rules ensure that OAuth flows are secure and that applications only have the permissions they were explicitly granted during registration.

[1] https://docs.joinmastodon.org/spec/oauth/
[2] https://docs.joinmastodon.org/methods/oauth/
[3] https://github.com/tootsuite/mastodon/issues/2048
[4] https://renchap.github.io/mastodon-documentation/spec/oauth/
[5] https://docs.monado.ren/en/client/token/
[6] https://github.com/mastodon/mastodon/issues/27740
[7] https://community.make.com/t/connection-error-to-mastodon/26883
[8] https://github.com/tuskyapp/Tusky/issues/203
[9] https://renchap.github.io/mastodon-documentation/methods/oauth/
[10] https://mastodonpy.readthedocs.io/en/stable/04_auth.html
[11] https://copeid.ssrc.msstate.edu/wp-content/uploads/2022/06/FINAL-Mastodon-Documentation.pdf
[12] https://www.passportjs.org/packages/passport-mastodon/
[13] https://documentation.sig.gy/client/authorized/
[14] https://documentation.sig.gy/client/guidelines/
[15] https://pvdz.ee/weblog/406
[16] https://mastodon-docs.vercel.app/spec/oauth/
[17] https://socialhub.activitypub.rocks/t/activitypub-and-oauth2-strategies/3312
[18] https://community.n8n.io/t/help-to-connect-to-mastodon-via-oauth2/13066
[19] https://documentation.sig.gy/methods/apps/oauth/