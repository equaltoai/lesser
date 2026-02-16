module github.com/equaltoai/lesser

go 1.25.0

toolchain go1.25.7

require (
	github.com/99designs/gqlgen v0.17.85
	github.com/aws/aws-lambda-go v1.52.0
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/config v1.32.7
	github.com/aws/aws-sdk-go-v2/credentials v1.19.7
	github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign v1.9.11
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.20.1
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.38.3
	github.com/aws/aws-sdk-go-v2/service/bedrock v1.48.2
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.41.2
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.55.2
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.51.4
	github.com/aws/aws-sdk-go-v2/service/comprehend v1.40.8
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.55.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.53.1
	github.com/aws/aws-sdk-go-v2/service/kms v1.49.5
	github.com/aws/aws-sdk-go-v2/service/lambda v1.80.0
	github.com/aws/aws-sdk-go-v2/service/mediaconvert v1.83.1
	github.com/aws/aws-sdk-go-v2/service/rekognition v1.51.7
	github.com/aws/aws-sdk-go-v2/service/route53 v1.62.0
	github.com/aws/aws-sdk-go-v2/service/s3 v1.95.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.39.9
	github.com/aws/aws-sdk-go-v2/service/sns v1.39.11
	github.com/aws/aws-sdk-go-v2/service/sqs v1.42.11
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.6
	github.com/aws/aws-sdk-go-v2/service/transcribe v1.53.2
	github.com/aws/aws-sdk-go-v2/service/translate v1.33.8
	github.com/aws/aws-xray-sdk-go/v2 v2.0.0
	github.com/aws/smithy-go v1.24.0
	github.com/bbrks/go-blurhash v1.1.1
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/disintegration/imaging v1.6.2
	github.com/ethereum/go-ethereum v1.16.8
	github.com/go-webauthn/webauthn v0.14.0
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/graph-gophers/dataloader v5.0.0+incompatible
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/oklog/ulid/v2 v2.1.1
	github.com/shopspring/decimal v1.4.0
	github.com/spruceid/siwe-go v0.2.1
	github.com/stretchr/testify v1.11.1
	github.com/theory-cloud/apptheory v0.8.0
	github.com/theory-cloud/tabletheory v1.3.0
	github.com/tyler-smith/go-bip32 v1.0.0
	github.com/tyler-smith/go-bip39 v1.1.0
	github.com/vektah/gqlparser/v2 v2.5.31
	go.uber.org/zap v1.27.1
	golang.org/x/crypto v0.46.0
	golang.org/x/image v0.32.0
	golang.org/x/net v0.48.0
	golang.org/x/sync v0.19.0
	golang.org/x/text v0.32.0
	golang.org/x/tools v0.40.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2 // indirect
	github.com/FactomProject/basen v0.0.0-20150613233007-fe3947df716e // indirect
	github.com/FactomProject/btcutilecc v0.0.0-20130527213604-d3a63a5752ec // indirect
	github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime v0.0.0-20251001021608-1fe7b43fc4d6 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.20.32 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi v1.29.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.32.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.13 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bits-and-blooms/bitset v1.24.3 // indirect
	github.com/consensys/gnark-crypto v0.19.2 // indirect
	github.com/crate-crypto/go-eth-kzg v1.4.0 // indirect
	github.com/crate-crypto/go-ipa v0.0.0-20240724233137-53bbb0ceb27a // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dchest/uniuri v1.2.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/ethereum/c-kzg-4844/v2 v2.1.5 // indirect
	github.com/ethereum/go-verkle v0.2.2 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/go-webauthn/x v0.1.25 // indirect
	github.com/google/go-tpm v0.9.6 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/relvacode/iso8601 v1.7.0 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/supranational/blst v0.3.16 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.68.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xyproto/randomstring v1.2.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.38.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
