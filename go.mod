module github.com/equaltoai/lesser

go 1.26.2

require (
	github.com/99designs/gqlgen v0.17.88
	github.com/aws/aws-lambda-go v1.54.0
	github.com/aws/aws-sdk-go-v2 v1.41.6
	github.com/aws/aws-sdk-go-v2/config v1.32.12
	github.com/aws/aws-sdk-go-v2/credentials v1.19.12
	github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign v1.9.20
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.20.35
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.1.10
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.39.0
	github.com/aws/aws-sdk-go-v2/service/bedrock v1.56.1
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.50.5
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.60.3
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.55.2
	github.com/aws/aws-sdk-go-v2/service/comprehend v1.40.20
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.56.2
	github.com/aws/aws-sdk-go-v2/service/iam v1.53.6
	github.com/aws/aws-sdk-go-v2/service/kms v1.50.3
	github.com/aws/aws-sdk-go-v2/service/lambda v1.89.1
	github.com/aws/aws-sdk-go-v2/service/mediaconvert v1.88.0
	github.com/aws/aws-sdk-go-v2/service/rekognition v1.51.19
	github.com/aws/aws-sdk-go-v2/service/route53 v1.62.4
	github.com/aws/aws-sdk-go-v2/service/s3 v1.99.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.41.4
	github.com/aws/aws-sdk-go-v2/service/sns v1.39.14
	github.com/aws/aws-sdk-go-v2/service/sqs v1.42.24
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.9
	github.com/aws/aws-sdk-go-v2/service/transcribe v1.54.3
	github.com/aws/aws-sdk-go-v2/service/translate v1.33.20
	github.com/aws/aws-xray-sdk-go/v2 v2.0.0
	github.com/aws/smithy-go v1.25.0
	github.com/bbrks/go-blurhash v1.2.0
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/disintegration/imaging v1.6.2
	github.com/ethereum/go-ethereum v1.17.1
	github.com/go-webauthn/webauthn v0.16.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.3
	github.com/graph-gophers/dataloader v5.0.0+incompatible
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/oklog/ulid/v2 v2.1.1
	github.com/shopspring/decimal v1.4.0
	github.com/spruceid/siwe-go v0.2.1
	github.com/stretchr/testify v1.11.1
	github.com/theory-cloud/apptheory v1.2.0
	github.com/theory-cloud/tabletheory v1.8.0-rc
	github.com/tyler-smith/go-bip32 v1.0.0
	github.com/tyler-smith/go-bip39 v1.1.0
	github.com/vektah/gqlparser/v2 v2.5.32
	go.uber.org/zap v1.27.1
	golang.org/x/crypto v0.49.0
	golang.org/x/image v0.39.0
	golang.org/x/net v0.52.0
	golang.org/x/sync v0.20.0
	golang.org/x/text v0.36.0
	golang.org/x/tools v0.43.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/aws/aws-sdk-go-v2/service/cloudformation v1.71.9

require github.com/aws/aws-sdk-go-v2/service/ssm v1.60.2

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2 // indirect
	github.com/FactomProject/basen v0.0.0-20150613233007-fe3947df716e // indirect
	github.com/FactomProject/btcutilecc v0.0.0-20130527213604-d3a63a5752ec // indirect
	github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime v0.0.0-20260225065256-91dd007ecddc // indirect
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.9 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.6 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.23 // indirect
	github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi v1.29.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.32.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.17 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bits-and-blooms/bitset v1.24.4 // indirect
	github.com/consensys/gnark-crypto v0.19.2 // indirect
	github.com/crate-crypto/go-eth-kzg v1.5.0 // indirect
	github.com/cyberphone/json-canonicalization v0.0.0-20241213102144-19d51d7fe467
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dchest/uniuri v1.2.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/dgryski/trifles v0.0.0-20240922021506-5ecb8eeff266 // indirect
	github.com/emicklei/dot v1.11.0 // indirect
	github.com/ethereum/c-kzg-4844/v2 v2.1.6 // indirect
	github.com/ferranbt/fastssz v1.0.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/go-webauthn/x v0.2.2 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/go-tpm-tools v0.4.7 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/relvacode/iso8601 v1.7.0 // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/sosodev/duration v1.4.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/supranational/blst v0.3.16 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.69.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xyproto/randomstring v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.43.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/mod v0.34.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
