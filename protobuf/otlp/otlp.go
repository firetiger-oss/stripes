// Package otlp side-effect-imports the OpenTelemetry protobuf Go
// bindings so their descriptors are registered in
// [protoregistry.GlobalTypes] / [protoregistry.GlobalFiles] at process
// start. The stripes protobuf renderer then resolves OTLP message
// types by name — from --schema or from an inbound
// application/protobuf; messageType="..." header — with no --registry
// flag or buf CLI involvement.
//
// Import for side effects only:
//
//	import _ "github.com/firetiger-oss/stripes/protobuf/otlp"
//
// The stripes CLI binary imports this package directly. Other Go
// programs embedding stripes opt in the same way when they want OTLP
// descriptors built in.
//
// Coverage: the data types in opentelemetry.proto.{trace,logs,metrics}.v1
// plus their transitive imports (common, resource). The
// collector/v1 ExportXxxServiceRequest wrappers are intentionally
// excluded for now to avoid pulling grpc-gateway into the runtime
// closure; pass --registry buf.build/opentelemetry/opentelemetry to
// resolve those.
package otlp

import (
	_ "go.opentelemetry.io/proto/otlp/logs/v1"
	_ "go.opentelemetry.io/proto/otlp/metrics/v1"
	_ "go.opentelemetry.io/proto/otlp/trace/v1"
)
