module github.com/TuSKan/astrogo

// Go 1.27 is the floor: remote/api uses encoding/json/v2, standard from 1.27.
// No dependency needs more than 1.26 (#1198).
go 1.27

require (
	github.com/apache/arrow-go/v18 v18.9.0
	github.com/hebl/gofa v1.19.1
	github.com/klauspost/pgzip v1.3.0
	github.com/scigolib/hdf5 v0.15.0
	golang.org/x/exp v0.0.0-20260410095643-746e56fc9e2f
	golang.org/x/sync v0.25.0
	golang.org/x/tools v0.52.0
)

require (
	github.com/andybalholm/brotli v1.3.0 // indirect
	github.com/apache/thrift v0.25.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/goccy/go-json v0.12.0 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/klauspost/cpuid/v2 v2.4.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.33 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260817212433-ac3dfec99bb1 // indirect
	google.golang.org/grpc v1.85.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
