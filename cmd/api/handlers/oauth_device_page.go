package handlers

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

// HandleOAuthDevicePageLift serves the operator-facing verification page referenced by verification_uri.
func (h *Handler) HandleOAuthDevicePageLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || ctx == nil {
		return common.RespondInternalServerError(ctx)
	}
	if h.cfg == nil || !h.cfg.AllowDeviceFlow {
		return apptheory.JSON(http.StatusForbidden, apimodels.OAuthErrorResponse{
			Error:            "access_denied",
			ErrorDescription: "device authorization is disabled",
		})
	}

	scriptNonce := newCSPNonce()
	htmlContent := h.renderOAuthDevicePage(normalizeOAuthDeviceUserCode(queryValue(ctx, "user_code")), scriptNonce)

	return &apptheory.Response{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"content-type":            {"text/html; charset=utf-8"},
			"content-security-policy": {oauthDevicePageCSP(scriptNonce)},
			"cache-control":           {"no-store"},
			"x-content-type-options":  {"nosniff"},
			"referrer-policy":         {"same-origin"},
			"x-frame-options":         {"DENY"},
		},
		Body: []byte(htmlContent),
	}, nil
}

func oauthDevicePageCSP(scriptNonce string) string {
	nonceDirective := ""
	if strings.TrimSpace(scriptNonce) != "" {
		nonceDirective = " 'nonce-" + scriptNonce + "'"
	}
	return "default-src 'none'; style-src 'unsafe-inline'; script-src" + nonceDirective + "; connect-src 'self'; img-src 'self' data:; form-action 'self'; base-uri 'none'"
}

func (h *Handler) renderOAuthDevicePage(prefilledUserCode, scriptNonce string) string {
	var builder strings.Builder
	builder.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	builder.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	builder.WriteString("<title>Approve Device Authorization</title>\n")
	builder.WriteString("<style>\n")
	builder.WriteString("body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,\"Segoe UI\",sans-serif;background:#f4efe6;color:#1f2933;}")
	builder.WriteString(".page{max-width:760px;margin:0 auto;padding:48px 20px 64px;}.card{background:#fffdf8;border:1px solid #d8cdb9;border-radius:20px;box-shadow:0 18px 60px rgba(31,41,51,.08);padding:28px;}")
	builder.WriteString("h1{font-size:2rem;line-height:1.1;margin:0 0 12px;}p{line-height:1.55;}label{display:block;font-weight:600;margin:18px 0 8px;}input,textarea,button{font:inherit;}")
	builder.WriteString("input,textarea{width:100%;box-sizing:border-box;border:1px solid #c5b79d;border-radius:12px;padding:12px 14px;background:#fff;}")
	builder.WriteString("textarea{min-height:120px;resize:vertical;}button{border:0;border-radius:999px;padding:12px 18px;cursor:pointer;background:#0f766e;color:#fff;font-weight:700;}")
	builder.WriteString("button.secondary{background:#b45309;}button.ghost{background:#334155;}button:disabled{opacity:.55;cursor:wait;}")
	builder.WriteString(".row{display:flex;gap:12px;flex-wrap:wrap;align-items:center;}.row > *{flex:1 1 180px;}")
	builder.WriteString(".status,.panel{margin-top:18px;padding:16px;border-radius:14px;background:#f8f3e9;border:1px solid #dccfb8;}.status strong{display:block;margin-bottom:6px;}")
	builder.WriteString(".muted{color:#52606d;font-size:.95rem;}.scopes{display:flex;flex-wrap:wrap;gap:8px;margin-top:10px;}.scope{background:#e6f4f1;color:#0f5d57;border-radius:999px;padding:6px 10px;font-size:.92rem;font-weight:600;}")
	builder.WriteString(".actions{display:flex;gap:12px;flex-wrap:wrap;margin-top:18px;}.actions button{flex:1 1 180px;}")
	builder.WriteString("code{background:#f1ede5;padding:2px 6px;border-radius:6px;}a{color:#0f766e;}.hidden{display:none;}\n")
	builder.WriteString("</style>\n</head>\n<body>\n")
	builder.WriteString("<main class=\"page\"><section class=\"card\">")
	builder.WriteString("<h1>Approve a device login</h1>")
	builder.WriteString("<p class=\"muted\">Use this page from the verification link shown to the headless client. Enter the <code>user_code</code> to inspect the request, then approve or deny it with an operator access token.</p>")
	builder.WriteString("<form id=\"lookup-form\">")
	builder.WriteString("<label for=\"user-code\">User code</label>")
	fmt.Fprintf(&builder, "<input id=\"user-code\" name=\"user_code\" autocomplete=\"one-time-code\" placeholder=\"ABCD-EFGH\" value=\"%s\">", html.EscapeString(prefilledUserCode))
	builder.WriteString("<div class=\"actions\"><button id=\"lookup-button\" type=\"submit\">Look up request</button></div>")
	builder.WriteString("</form>")
	builder.WriteString("<div id=\"lookup-status\" class=\"status\"><strong>Ready</strong><span>Enter the code from the headless client to inspect the pending authorization.</span></div>")
	builder.WriteString("<section id=\"details\" class=\"panel hidden\">")
	builder.WriteString("<strong id=\"app-name\">Pending request</strong>")
	builder.WriteString("<p class=\"muted\" id=\"app-url\"></p>")
	builder.WriteString("<p id=\"request-status\"></p>")
	builder.WriteString("<div id=\"scope-list\" class=\"scopes\"></div>")
	builder.WriteString("<label for=\"access-token\">Operator access token</label>")
	builder.WriteString("<textarea id=\"access-token\" placeholder=\"Paste a Lesser bearer token for the approving operator\"></textarea>")
	builder.WriteString("<p class=\"muted\">The approval API currently authenticates with a bearer token. Use an operator token that can act for the approving account.</p>")
	builder.WriteString("<div class=\"actions\">")
	builder.WriteString("<button id=\"approve-button\" type=\"button\">Approve</button>")
	builder.WriteString("<button id=\"deny-button\" type=\"button\" class=\"secondary\">Deny</button>")
	builder.WriteString("</div>")
	builder.WriteString("</section>")
	builder.WriteString("</section></main>")
	fmt.Fprintf(&builder, "<script nonce=\"%s\">", html.EscapeString(scriptNonce))
	builder.WriteString(`
const lookupForm = document.getElementById("lookup-form");
const lookupButton = document.getElementById("lookup-button");
const approveButton = document.getElementById("approve-button");
const denyButton = document.getElementById("deny-button");
const userCodeInput = document.getElementById("user-code");
const accessTokenInput = document.getElementById("access-token");
const lookupStatus = document.getElementById("lookup-status");
const details = document.getElementById("details");
const appName = document.getElementById("app-name");
const appURL = document.getElementById("app-url");
const requestStatus = document.getElementById("request-status");
const scopeList = document.getElementById("scope-list");
let verifiedDeviceRequest = null;

function setStatus(title, detail) {
  lookupStatus.innerHTML = "<strong>" + title + "</strong><span>" + detail + "</span>";
}

function normalizedCode() {
  return userCodeInput.value.trim().toUpperCase().replace(/\s+/g, "").replace(/^(.{4})(.{4})$/, "$1-$2");
}

async function postForm(path, fields, bearerToken) {
  const headers = { "Content-Type": "application/x-www-form-urlencoded" };
  if (bearerToken && bearerToken.trim() !== "") {
    headers.Authorization = "Bearer " + bearerToken.trim();
  }
  const response = await fetch(path, {
    method: "POST",
    headers,
    body: new URLSearchParams(fields),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error_description || payload.error || ("Request failed with status " + response.status));
  }
  return payload;
}

function renderScopes(scopes) {
  scopeList.replaceChildren();
  (scopes || []).forEach((scope) => {
    const badge = document.createElement("span");
    badge.className = "scope";
    badge.textContent = scope;
    scopeList.appendChild(badge);
  });
}

async function lookupRequest() {
  const userCode = normalizedCode();
  userCodeInput.value = userCode;
  lookupButton.disabled = true;
  setStatus("Looking up request", "Checking the pending device authorization...");
  try {
    const payload = await postForm("/oauth/device/verify", { user_code: userCode });
    verifiedDeviceRequest = payload;
    details.classList.remove("hidden");
    appName.textContent = payload.client_name || payload.client_id || "Pending request";
    appURL.textContent = payload.client_url ? "Client URL: " + payload.client_url : "Client ID: " + (payload.client_id || "");
    requestStatus.textContent = "Status: " + (payload.status || "pending") + " • Expires in: " + (payload.expires_in || 0) + "s";
    renderScopes(payload.scopes);
    setStatus("Request loaded", "Review the requesting app and scopes, then approve or deny.");
  } catch (error) {
    verifiedDeviceRequest = null;
    details.classList.add("hidden");
    setStatus("Lookup failed", error.message);
  } finally {
    lookupButton.disabled = false;
  }
}

async function submitConsent(action) {
  const userCode = normalizedCode();
  const bearerToken = accessTokenInput.value;
  if (!verifiedDeviceRequest || verifiedDeviceRequest.user_code !== userCode) {
    setStatus("Verify code first", "Look up the current device authorization before approving or denying.");
    return;
  }
  if (!bearerToken.trim()) {
    setStatus("Access token required", "Paste an operator bearer token before approving or denying.");
    return;
  }

  approveButton.disabled = true;
  denyButton.disabled = true;
  setStatus("Submitting decision", "Updating the device authorization...");
  try {
    const payload = await postForm("/oauth/device/consent", {
      user_code: userCode,
      action,
      client_id: verifiedDeviceRequest.client_id,
      scope: (verifiedDeviceRequest.scopes || []).join(" "),
    }, bearerToken);
    setStatus("Decision recorded", payload.message || ("Authorization " + payload.status));
    requestStatus.textContent = "Status: " + (payload.status || action);
  } catch (error) {
    setStatus("Decision failed", error.message);
  } finally {
    approveButton.disabled = false;
    denyButton.disabled = false;
  }
}

lookupForm.addEventListener("submit", (event) => {
  event.preventDefault();
  void lookupRequest();
});
approveButton.addEventListener("click", () => void submitConsent("approve"));
denyButton.addEventListener("click", () => void submitConsent("deny"));

if (normalizedCode()) {
  void lookupRequest();
}
`)
	builder.WriteString("</script></body></html>")
	return builder.String()
}
