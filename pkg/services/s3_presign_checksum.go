package services

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// hoistPresignedChecksumMiddleware restores the host-only signature contract
// for presigned PUTs. AWS SDK v1.47 added X-Amz-Checksum-* to the signer's
// required-signed-header list, which stops the checksum from being hoisted
// into the query string and starts signing Content-Type with the request.
// A client holding only the presigned URL cannot echo either value, so the
// middleware moves checksum headers into the query (where S3 validates them)
// and drops Content-Type before the request is signed.
type hoistPresignedChecksumMiddleware struct{}

func (hoistPresignedChecksumMiddleware) ID() string {
	return "LesserHoistPresignedChecksum"
}

func (hoistPresignedChecksumMiddleware) HandleFinalize(
	ctx context.Context,
	in middleware.FinalizeInput,
	next middleware.FinalizeHandler,
) (
	middleware.FinalizeOutput,
	middleware.Metadata,
	error,
) {
	request, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return next.HandleFinalize(ctx, in)
	}

	request.Header.Del("Content-Type")

	query := request.URL.Query()
	hoisted := false
	for name, values := range request.Header {
		if !strings.HasPrefix(strings.ToLower(name), "x-amz-checksum-") || len(values) == 0 {
			continue
		}
		query.Set(name, values[0])
		request.Header.Del(name)
		hoisted = true
	}
	if hoisted {
		request.URL.RawQuery = query.Encode()
	}

	return next.HandleFinalize(ctx, in)
}

// withPresignedChecksumHoisting attaches the hoisting middleware to one
// presign invocation only, leaving the shared S3 client untouched.
func withPresignedChecksumHoisting(options *s3.PresignOptions) {
	options.ClientOptions = append(options.ClientOptions, func(clientOptions *s3.Options) {
		clientOptions.APIOptions = append(clientOptions.APIOptions, func(stack *middleware.Stack) error {
			return stack.Finalize.Insert(hoistPresignedChecksumMiddleware{}, "PresignHTTPRequest", middleware.Before)
		})
	})
}
