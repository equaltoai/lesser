package common

// HTTP Headers
const (
	// Standard headers
	AuthorizationHeader = "Authorization"
	ContentTypeHeader   = "Content-Type"
	UserAgentHeader     = "User-Agent"
	AcceptHeader        = "Accept"
	
	// Custom headers
	XTestUsernameHeader    = "X-Test-Username"
	XTenantIDHeader        = "X-Tenant-ID"
	XRequestIDHeader       = "X-Request-ID"
	XRealIPHeader          = "X-Real-IP"
	XForwardedForHeader    = "X-Forwarded-For"
	XProcessingDelayHeader = "X-Processing-Delay"
	XNextCursorHeader      = "X-Next-Cursor"
	XInboxBacklogHeader    = "X-Inbox-Backlog"
	
	// Security headers
	XFrameOptionsHeader         = "X-Frame-Options"
	XContentTypeOptionsHeader   = "X-Content-Type-Options"
	XXSSProtectionHeader        = "X-XSS-Protection"
)

// Content Types
const (
	ContentTypeJSON           = "application/json"
	ContentTypeHTML           = "text/html"
	ContentTypeActivityPubJSON = "application/activity+json"
	ContentTypeLDJSON         = "application/ld+json"
	
	// Media types
	ImageGIF  = "image/gif"
	ImagePNG  = "image/png"
	ImageJPEG = "image/jpeg"
	ImageJPG  = "image/jpg"
	ImageWEBP = "image/webp"
	VideoMP4  = "video/mp4"
	VideoWEBM = "video/webm"
	VideoQuickTime = "video/quicktime"
)