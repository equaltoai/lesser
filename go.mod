module github.com/equaltoai/lesser

go 1.24

toolchain go1.24.2

require (
	github.com/99designs/gqlgen v0.17.74
	github.com/aws/aws-lambda-go v1.49.0
	github.com/aws/aws-sdk-go v1.55.7
	github.com/aws/aws-sdk-go-v2 v1.37.1
	github.com/aws/aws-sdk-go-v2/config v1.29.16
	github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign v1.9.1
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.19.2
	github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi v1.24.3
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.7.2
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.46.3
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.45.3
	github.com/aws/aws-sdk-go-v2/service/comprehend v1.36.4
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.43.3
	github.com/aws/aws-sdk-go-v2/service/lambda v1.72.0
	github.com/aws/aws-sdk-go-v2/service/mediaconvert v1.30.0
	github.com/aws/aws-sdk-go-v2/service/rekognition v1.46.3
	github.com/aws/aws-sdk-go-v2/service/s3 v1.83.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.35.7
	github.com/aws/aws-sdk-go-v2/service/ses v1.30.5
	github.com/aws/aws-sdk-go-v2/service/sns v1.34.7
	github.com/aws/aws-sdk-go-v2/service/sqs v1.38.5
	github.com/aws/aws-sdk-go-v2/service/translate v1.29.2
	github.com/aws/aws-xray-sdk-go v1.8.5
	github.com/aws/smithy-go v1.22.5
	github.com/bbrks/go-blurhash v1.1.1
	github.com/bytedance/sonic v1.13.3
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/disintegration/imaging v1.6.2
	github.com/ethereum/go-ethereum v1.13.15
	github.com/go-webauthn/webauthn v0.13.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/graph-gophers/dataloader v5.0.0+incompatible
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/pay-theory/dynamorm v1.0.29
	github.com/pay-theory/lift v1.0.55
	github.com/shopspring/decimal v1.4.0
	github.com/spruceid/siwe-go v0.2.1
	github.com/stretchr/testify v1.10.0
	github.com/vektah/gqlparser/v2 v2.5.27
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.39.0
	golang.org/x/image v0.28.0
)

require (
	github.com/PaesslerAG/gval v1.0.0 // indirect
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.11 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.69 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/appconfig v1.38.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/appmesh v1.30.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.45.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.25.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.10.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.35.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.60.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.21 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bits-and-blooms/bitset v1.10.0 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/bytedance/sonic/loader v0.2.4 // indirect
	github.com/cloudwego/base64x v0.1.5 // indirect
	github.com/consensys/bavard v0.1.13 // indirect
	github.com/consensys/gnark-crypto v0.12.1 // indirect
	github.com/crate-crypto/go-kzg-4844 v0.7.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dchest/uniuri v1.2.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/ethereum/c-kzg-4844 v0.4.0 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/go-webauthn/x v0.1.21 // indirect
	github.com/google/go-tpm v0.9.5 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/holiman/uint256 v1.2.4 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.17.6 // indirect
	github.com/klauspost/cpuid/v2 v2.2.4 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mmcloughlin/addchain v0.4.0 // indirect
	github.com/opentracing/opentracing-go v1.1.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pay-theory/limited v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/relvacode/iso8601 v1.1.1-0.20210511065120-b30b151cc433 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/supranational/blst v0.3.11 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.52.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/arch v0.3.0 // indirect
	golang.org/x/exp v0.0.0-20240112132812-db7319d0e0e3 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/grpc v1.64.1 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	rsc.io/tmplfunc v0.0.3 // indirect
)
