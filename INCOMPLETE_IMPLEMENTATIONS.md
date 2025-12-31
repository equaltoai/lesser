# Incomplete Implementations Report

_Generated on Wed Dec 31 03:21:11 PM EST 2025_

## "not implemented" occurrences (1)
./pkg/testing/mocks/storage_mock.go:4146:	// Create repository instances with nil DB - they'll return "not implemented" errors

## TODO comments (14)
./graph/model/models_gen.go:3062:	ExportFormatMastodon    ExportFormat = "MASTODON"
./graph/subscription_resolvers_moderation.go:102:	// TODO: Implement proper moderation queue subscription via subscription manager
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsec2/DestinationOptions.go:6:// TODO: there are other destination options, currently they are
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsec2/VpnConnectionType.go:35:	// Dummy member TODO: remove once https://github.com/aws/jsii/issues/231 is fixed.
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsecs/ContainerDefinitionOptions.go:43:	// TODO: Update these to specify using classes of IContainerImage.
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsecs/ContainerDefinitionProps.go:133:	// TODO: Update these to specify using classes of IContainerImage.
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsecs/FirelensLogRouterDefinitionOptions.go:141:	// TODO: Update these to specify using classes of IContainerImage.
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsecs/FirelensLogRouterProps.go:143:	// TODO: Update these to specify using classes of IContainerImage.
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsinspector/AssessmentTemplate.go:14:// TODO: This class should implement IAssessmentTemplate and "construct-ctor-props-type:aws-cdk-lib.aws_inspector.AssessmentTemplate" should be
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/awsinspector/AssessmentTemplateProps.go:4:// Properties for creating an Inspector Assessment Template TODO: Add properties and remove "props-physical-name:aws-cdk-lib.aws_inspector.AssessmentTemplateProps" from `awslint.json` when implementing the L2 construct.
./tmp/go-mod-cache/github.com/aws/aws-cdk-go/awscdk/v2@v2.220.0/Names.go:27:// TODO (v2): replace with API to use `constructs.Node`.
./tmp/go-mod-cache/github.com/aws/jsii-runtime-go@v1.117.0/internal/kernel/conversions.go:286:// TODO: This should return a time.Time instead
./tmp/go-mod-cache/github.com/aws/jsii-runtime-go@v1.117.0/internal/kernel/create.go:12:// TODO extends AnnotatedObjRef?
./tmp/go-mod-cache/github.com/!masterminds/semver/v3@v3.4.0/constraints.go:73:	// TODO(mattfarina): For v4 of this library consolidate the Check and Validate

## context.TODO() occurrences (0)
_None found._

## Authentication repository gaps (0)
_None found._

## Pagination TODO markers (2)
pkg/storage/repositories/account_repository_oauth.go:412:// ListOAuthClients lists OAuth clients with deterministic cursor-based pagination.
pkg/storage/repositories/base_repository.go:968:	UseCursor bool   // Enables cursor-based pagination on the configured sort key

## GraphQL TODOs (0)
_None found._

## "return nil, nil" patterns (32)
pkg/storage/repositories/account_repository.go:739:		return nil, nil
pkg/storage/repositories/account_repository.go:743:		return nil, nil
pkg/storage/repositories/account_repository.go:755:		return nil, nil
pkg/storage/repositories/account_repository.go:759:		return nil, nil
pkg/storage/repositories/account_repository.go:771:		return nil, nil
pkg/storage/repositories/account_repository.go:794:		return nil, nil
pkg/storage/repositories/account_repository.go:828:		return nil, nil
pkg/storage/repositories/actor_repository_additional_coverage_test.go:37:		return nil, nil
pkg/storage/repositories/actor_repository.go:160:			return nil, nil, common.ActorNotFoundError{Username: username}
pkg/storage/repositories/actor_repository.go:162:		return nil, nil, ErrorHandler.HandleGetError(err, EntityActor, username)
pkg/storage/repositories/bookmark_repository.go:640:		return nil, nil
pkg/storage/repositories/bookmark_repository.go:720:		return nil, nil
pkg/storage/repositories/bookmark_repository_round08_error_branches_test.go:309:		return nil, nil
pkg/storage/repositories/bookmark_repository_round08_more_coverage_test.go:97:		return nil, nil
pkg/storage/repositories/bookmark_repository_test.go:60:		return nil, nil
pkg/storage/repositories/federation_instance_repository.go:759:		return nil, nil, err
pkg/storage/repositories/push_subscription_repository.go:294:		return nil, nil
pkg/storage/repositories/search_repository.go:1035:		return nil, nil, ErrorHandler.HandleQueryError(err, "hashtag search", "pagination validation")
pkg/storage/repositories/search_repository.go:1048:		return nil, nil, ErrorHandler.HandleQueryError(err, "hashtag search", "cursor decoding")
pkg/storage/repositories/search_repository.go:1070:		return nil, nil, ErrorHandler.HandleQueryError(err, "hashtag search", "advanced search")
pkg/storage/repositories/search_repository.go:1855:		return nil, nil, err
pkg/storage/repositories/search_repository.go:1862:		return nil, nil, ErrorHandler.HandleQueryError(err, "embedding search", "cursor decoding")
pkg/storage/repositories/search_repository.go:436:		return nil, nil, ErrorHandler.HandleQueryError(err, "search", "pagination validation")
pkg/storage/repositories/search_repository.go:505:		return nil, nil, err
pkg/storage/repositories/search_repository.go:875:		return nil, nil, ErrorHandler.HandleQueryError(err, "search all", "pagination validation")
pkg/storage/repositories/status_repository_additional_coverage_test.go:49:	return nil, nil
pkg/storage/repositories/status_repository.go:1400:		return nil, nil, ErrorHandler.HandleGetError(err, EntityStatus, statusID)
pkg/storage/repositories/status_repository.go:367:			return nil, nil
pkg/storage/repositories/trust_repository.go:370:			return nil, nil
pkg/storage/repositories/trust_repository.go:376:		return nil, nil
pkg/storage/repositories/user_repository_fanout_test.go:40:	return nil, nil
pkg/storage/repositories/user_repository.go:2913:		return nil, nil, ErrorHandler.HandleGetError(storage.ErrInvalidInput, "activity object", fmt.Sprintf("type %T", activity.Object))

