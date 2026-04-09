package routing

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/federation"
	apptheory "github.com/theory-cloud/apptheory/runtime"
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

	ih.logFollowTrace(
		trace,
		"receiver.raw_request",
		zap.String("request_id", ctx.RequestID),
		zap.String("target_username", username),
		zap.String("raw_method", ctx.Request.Method),
		zap.String("raw_path", ctx.Request.Path),
		zap.Any("raw_query", ctx.Request.Query),
		zap.String("raw_host", headerValue(ctx, "Host")),
		zap.String("raw_x_lesser_forwarded_host", headerValue(ctx, common.XLesserForwardedHost)),
		zap.String("raw_x_lesser_forwarded_proto", headerValue(ctx, common.XLesserForwardedProto)),
		zap.String("raw_x_forwarded_host", headerValue(ctx, "X-Forwarded-Host")),
		zap.String("raw_x_forwarded_proto", headerValue(ctx, "X-Forwarded-Proto")),
		zap.String("raw_forwarded", headerValue(ctx, "Forwarded")),
		zap.String("raw_content_type", headerValue(ctx, "Content-Type")),
		zap.String("raw_date", headerValue(ctx, "Date")),
		zap.String("raw_digest", headerValue(ctx, "Digest")),
		zap.String("raw_signature", headerValue(ctx, "Signature")),
	)
}

func (ih *InboxHandler) logFollowTraceReconstructedRequest(trace *federation.FollowTraceMetadata, req *http.Request) {
	if trace == nil {
		return
	}

	ih.logFollowTrace(trace, "receiver.reconstructed_request", federation.BuildSignatureTraceFields(req)...)
}
