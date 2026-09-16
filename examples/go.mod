// examples is a separate module on purpose.
//
// Its 32 main packages are demos, not API. Inside the root module they were
// 32 of the 84 rows in astrogo's pkg.go.dev directory listing — 38% of the
// front door pointing at programs nobody imports (#124).
//
// The replace directive is what keeps them honest: they build against the
// working tree, so an API change that breaks an example breaks CI, exactly as
// it did when they lived in the root module. The require line names a real
// released version so the module still resolves for anyone who fetches it
// directly, where a replace directive is ignored.
module github.com/TuSKan/astrogo/examples

go 1.27

require github.com/TuSKan/astrogo v0.19.0

require (
	github.com/andybalholm/brotli v1.2.3 // indirect
	github.com/apache/arrow-go/v18 v18.8.0 // indirect
	github.com/apache/thrift v0.24.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hebl/gofa v1.19.1 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/pierrec/lz4/v4 v4.1.29 // indirect
	github.com/scigolib/hdf5 v0.14.1 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260724162435-b2f20204f0df // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	resty.dev/v3 v3.0.0-rc.4 // indirect
)

replace github.com/TuSKan/astrogo => ../
