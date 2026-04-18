package federation

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"go.uber.org/zap"
)

// FollowTraceEnvVar enables the temporary live follow trace for the ops -> steward path.
const FollowTraceEnvVar = "FEDERATION_TRACE_OPS_STEWARD_FOLLOW"

type followTraceContextKey struct{}

// FollowTraceMetadata identifies the single Follow flow we want to trace end to end.
type FollowTraceMetadata struct {
	ActivityID     string
	ActivityType   string
	SenderUsername string
	TargetUsername string
}

// NewFollowTraceMetadata returns trace metadata only for the gated ops -> steward Follow contract.
func NewFollowTraceMetadata(activity *activitypub.Activity, senderIdentifier, targetIdentifier string) *FollowTraceMetadata {
	if !followTraceEnabled() || activity == nil || !strings.EqualFold(strings.TrimSpace(activity.Type), activitypub.FollowType) {
		return nil
	}

	senderUsername := normalizeFollowTraceIdentity(senderIdentifier)
	if senderUsername == "" {
		senderUsername = normalizeFollowTraceIdentity(activity.Actor)
	}

	targetUsername := normalizeFollowTraceIdentity(targetIdentifier)
	if targetUsername == "" {
		if objectID, ok := activity.Object.(string); ok {
			targetUsername = normalizeFollowTraceIdentity(objectID)
		}
	}

	if !strings.EqualFold(senderUsername, "ops") || !strings.EqualFold(targetUsername, "steward") {
		return nil
	}

	return &FollowTraceMetadata{
		ActivityID:     strings.TrimSpace(activity.ID),
		ActivityType:   strings.TrimSpace(activity.Type),
		SenderUsername: senderUsername,
		TargetUsername: targetUsername,
	}
}

// WithFollowTrace carries the trace metadata across sender and receiver verification boundaries.
func WithFollowTrace(ctx context.Context, trace *FollowTraceMetadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, followTraceContextKey{}, trace)
}

// FollowTraceFromContext retrieves any active follow trace metadata from the context.
func FollowTraceFromContext(ctx context.Context) (*FollowTraceMetadata, bool) {
	if ctx == nil {
		return nil, false
	}

	trace, ok := ctx.Value(followTraceContextKey{}).(*FollowTraceMetadata)
	if !ok || trace == nil {
		return nil, false
	}

	return trace, true
}

// FollowTraceFields returns the common log fields for a traced follow contract stage.
func FollowTraceFields(trace *FollowTraceMetadata, stage string) []zap.Field {
	if trace == nil {
		return nil
	}

	return []zap.Field{
		zap.String("trace_stage", stage),
		zap.String("trace_activity_id", trace.ActivityID),
		zap.String("trace_activity_type", trace.ActivityType),
		zap.String("trace_sender_username", trace.SenderUsername),
		zap.String("trace_target_username", trace.TargetUsername),
	}
}

// BuildSignatureTraceFields records the request and canonical signature state without exposing key material.
func BuildSignatureTraceFields(req *http.Request) []zap.Field {
	fields := []zap.Field{}
	if req == nil {
		return append(fields, zap.String("trace_request_error", "nil_request"))
	}

	requestURL := ""
	requestPath := ""
	requestRawQuery := ""
	requestURLHost := ""
	if req.URL != nil {
		requestURL = req.URL.String()
		requestPath = req.URL.Path
		requestRawQuery = req.URL.RawQuery
		requestURLHost = req.URL.Host
	}

	fields = append(fields,
		zap.String("request_method", req.Method),
		zap.String("request_url", requestURL),
		zap.String("request_path", requestPath),
		zap.String("request_raw_query", requestRawQuery),
		zap.Bool("request_host_present", strings.TrimSpace(req.Host) != ""),
		zap.Int("request_host_len", len(strings.TrimSpace(req.Host))),
		zap.String("request_url_host", requestURLHost),
	)
	fields = appendTraceHeaderState(fields, "request_host_header", req.Header.Get("Host"))
	fields = appendTraceHeaderState(fields, "request_date_header", req.Header.Get(DateHeader))
	fields = appendTraceHeaderState(fields, "request_content_type_header", req.Header.Get("Content-Type"))
	fields = appendTraceHeaderState(fields, "request_digest_header", req.Header.Get(DigestHeader))

	signatureHeader := req.Header.Get(SignatureHeader)
	fields = appendTraceHeaderState(fields, "request_signature_header", signatureHeader)
	if signatureHeader == "" {
		return fields
	}

	sig, err := ParseSignatureHeader(signatureHeader)
	if err != nil {
		return append(fields, zap.Bool("signature_parse_failed", true))
	}

	fields = append(fields,
		zap.Bool("signature_parsed", true),
		zap.Bool("signature_key_id_present", strings.TrimSpace(sig.KeyID) != ""),
		zap.Bool("signature_algorithm_present", strings.TrimSpace(sig.Algorithm) != ""),
		zap.Int("signature_header_count", len(sig.Headers)),
	)

	sigString, err := BuildHTTPSignatureString(req, sig.Headers)
	if err != nil {
		return append(fields, zap.Bool("signature_canonical_build_failed", true))
	}

	return append(fields, zap.Int("signature_canonical_len", len(sigString)))
}

func appendTraceHeaderState(fields []zap.Field, fieldPrefix, value string) []zap.Field {
	trimmed := strings.TrimSpace(value)

	return append(fields,
		zap.Bool(fieldPrefix+"_present", trimmed != ""),
		zap.Int(fieldPrefix+"_len", len(trimmed)),
	)
}

func followTraceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(FollowTraceEnvVar))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeFollowTraceIdentity(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "@") {
		trimmed = strings.TrimPrefix(trimmed, "@")
		if user, _, ok := strings.Cut(trimmed, "@"); ok {
			return strings.ToLower(strings.TrimSpace(user))
		}
		return strings.ToLower(strings.TrimSpace(trimmed))
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			if username := usernameFromFollowTracePath(parsed.Path); username != "" {
				return username
			}
			if parsed.Host != "" {
				return strings.ToLower(strings.TrimSpace(parsed.Host))
			}
		}
	}

	if username := usernameFromFollowTracePath(trimmed); username != "" {
		return username
	}

	if user, _, ok := strings.Cut(trimmed, "@"); ok {
		return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(user, "@")))
	}

	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, "@")))
}

func usernameFromFollowTracePath(path string) string {
	clean := strings.Trim(strings.TrimSpace(path), "/")
	if clean == "" {
		return ""
	}

	parts := strings.Split(clean, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "users" || parts[i] == "actors" {
			candidate := strings.TrimSpace(strings.TrimPrefix(parts[i+1], "@"))
			if candidate != "" {
				return strings.ToLower(candidate)
			}
		}
	}

	last := strings.TrimSpace(strings.TrimPrefix(parts[len(parts)-1], "@"))
	if last == "" {
		return ""
	}

	switch last {
	case "inbox", "outbox", "followers", "following", "statuses":
		if len(parts) < 2 {
			return ""
		}
		candidate := strings.TrimSpace(strings.TrimPrefix(parts[len(parts)-2], "@"))
		if candidate == "" || candidate == "users" || candidate == "actors" {
			return ""
		}
		return strings.ToLower(candidate)
	default:
		return strings.ToLower(last)
	}
}
