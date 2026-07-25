package routing

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

func (ih *InboxHandler) followTraceForRequest(req *InboxRequest) *federation.FollowTraceMetadata {
	if req == nil {
		return nil
	}
	return federation.NewFollowTraceMetadata(req.Activity, req.Activity.Actor, req.Username)
}

func (ih *InboxHandler) logFollowTrace(trace *federation.FollowTraceMetadata, stage string, extra ...zap.Field) {
	if trace == nil {
		return
	}

	fields := append(federation.FollowTraceFields(trace, stage), extra...)
	ih.logger.Info("federation follow trace", fields...)
}

func (ih *InboxHandler) logFollowTraceRawRequest(ctx *apptheory.Context, trace *federation.FollowTraceMetadata, username string) {
	if trace == nil || ctx == nil {
		return
	}

	rawHost := headerValue(ctx, "Host")
	rawXLesserForwardedHost := headerValue(ctx, common.XLesserForwardedHost)
	rawXLesserForwardedProto := headerValue(ctx, common.XLesserForwardedProto)
	rawXForwardedHost := headerValue(ctx, "X-Forwarded-Host")
	rawXForwardedProto := headerValue(ctx, "X-Forwarded-Proto")
	rawForwarded := headerValue(ctx, "Forwarded")
	rawContentType := headerValue(ctx, "Content-Type")
	rawDate := headerValue(ctx, "Date")
	rawDigest := headerValue(ctx, "Digest")
	rawSignature := headerValue(ctx, "Signature")

	ih.logFollowTrace(
		trace,
		"receiver.raw_request",
		zap.String("request_id", ctx.RequestID),
		zap.String("target_username", username),
		zap.String("raw_method", ctx.Request.Method),
		zap.String("raw_path", ctx.Request.Path),
		zap.Any("raw_query", ctx.Request.Query),
		zap.Bool("raw_host_present", rawHost != ""),
		zap.Int("raw_host_len", len(rawHost)),
		zap.Bool("raw_x_lesser_forwarded_host_present", rawXLesserForwardedHost != ""),
		zap.Int("raw_x_lesser_forwarded_host_len", len(rawXLesserForwardedHost)),
		zap.Bool("raw_x_lesser_forwarded_proto_present", rawXLesserForwardedProto != ""),
		zap.Int("raw_x_lesser_forwarded_proto_len", len(rawXLesserForwardedProto)),
		zap.Bool("raw_x_forwarded_host_present", rawXForwardedHost != ""),
		zap.Int("raw_x_forwarded_host_len", len(rawXForwardedHost)),
		zap.Bool("raw_x_forwarded_proto_present", rawXForwardedProto != ""),
		zap.Int("raw_x_forwarded_proto_len", len(rawXForwardedProto)),
		zap.Bool("raw_forwarded_present", rawForwarded != ""),
		zap.Int("raw_forwarded_len", len(rawForwarded)),
		zap.Bool("raw_content_type_present", rawContentType != ""),
		zap.Int("raw_content_type_len", len(rawContentType)),
		zap.Bool("raw_date_present", rawDate != ""),
		zap.Int("raw_date_len", len(rawDate)),
		zap.Bool("raw_digest_present", rawDigest != ""),
		zap.Int("raw_digest_len", len(rawDigest)),
		zap.Bool("raw_signature_present", rawSignature != ""),
		zap.Int("raw_signature_len", len(rawSignature)),
	)
}

func (ih *InboxHandler) logFollowTraceReconstructedRequest(trace *federation.FollowTraceMetadata, req *http.Request) {
	if trace == nil {
		return
	}

	ih.logFollowTrace(trace, "receiver.reconstructed_request", federation.BuildSignatureTraceFields(req)...)
}
