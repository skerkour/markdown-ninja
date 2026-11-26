module markdown.ninja

go 1.27.0

godebug multipathtcp=0

tool github.com/skerkour/stdx-go/tools/watchgod

require (
	github.com/alecthomas/chroma/v2 v2.27.0
	github.com/aws/aws-sdk-go-v2 v1.48.1
	github.com/aws/aws-sdk-go-v2/config v1.33.22
	github.com/aws/aws-sdk-go-v2/credentials v1.21.4
	github.com/aws/aws-sdk-go-v2/service/s3 v1.116.1
	github.com/aws/aws-sdk-go-v2/service/sesv2 v1.80.1
	github.com/coder/websocket v1.8.15
	github.com/creack/pty v1.1.24
	github.com/fsnotify/fsnotify v1.10.1
	github.com/fxamacker/cbor/v2 v2.10.0
	github.com/go-chi/chi/v5 v5.5.0
	github.com/jackc/pgx/v5 v5.13.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/klauspost/compress v1.20.5
	github.com/klauspost/cpuid/v2 v2.4.2
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/minio/minio-go/v7 v7.3.0
	github.com/mmcloughlin/avo v0.6.0
	github.com/pdfcpu/pdfcpu v0.16.4
	github.com/quic-go/quic-go v0.66.0
	github.com/rogpeppe/go-internal v1.16.0
	github.com/stripe/stripe-go/v81 v81.4.0
	github.com/tetratelabs/wazero v1.12.0
	github.com/yuin/goldmark v1.8.7
	github.com/zeebo/blake3 v0.2.4
	golang.org/x/crypto v0.59.0
	golang.org/x/exp v0.0.0-20261124000051-d6646c509b6d
	golang.org/x/image v0.48.0
	golang.org/x/mod v0.43.0
	golang.org/x/net v0.63.0
	golang.org/x/sync v0.25.0
	golang.org/x/sys v0.50.0
	golang.org/x/term v0.48.0
	golang.org/x/text v0.44.0
	lukechampine.com/blake3 v1.4.1
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.28 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.20.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.13 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.13 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.26 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.11.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.20.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.14.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.39.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.44.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.53.1 // indirect
	github.com/aws/smithy-go v1.28.10 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dlclark/regexp2/v2 v2.13.2 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/hhrutter/tiff v1.0.8 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.4 // indirect
	github.com/klauspost/crc32 v1.3.0 // indirect
	github.com/mattn/go-runewidth v0.0.31 // indirect
	github.com/minio/crc64nvme v1.1.1 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/skerkour/stdx-go v0.0.0-20260825090241-98622c7e7a79 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xdg-go/stringprep v1.0.5 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/tools v0.53.0 // indirect
	gopkg.in/ini.v1 v1.67.3 // indirect
)
