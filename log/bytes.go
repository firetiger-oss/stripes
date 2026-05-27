package log

import (
	"fmt"
	"io"
	"iter"

	"github.com/firetiger-oss/stripes"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// NewRenderer returns a [stripes.Renderer] that decodes binary OTLP
// protobuf against d and renders a per-resource table. d may be the
// descriptor for LogsData, ResourceLogs, ScopeLogs, or LogRecord;
// messages at any of those levels are wrapped up to a synthetic
// LogsData before rendering. types resolves embedded message
// references; pass [protoregistry.GlobalTypes] for the default
// behaviour.
//
// When d is nil the renderer assumes LogsData. If the descriptor
// names an unsupported message the renderer writes an error line and
// returns.
func NewRenderer(d protoreflect.MessageDescriptor, types protoregistry.MessageTypeResolver) stripes.Renderer {
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		data, err := io.ReadAll(r)
		if err != nil {
			fmt.Fprintf(w, "logs: read input: %v\n", err)
			return
		}
		seq, err := decodeBinary(d, data)
		if err != nil {
			fmt.Fprintf(w, "logs: %v\n", err)
			return
		}
		if err := Write(w, seq, WithStyles(styles)); err != nil {
			fmt.Fprintf(w, "logs: render: %v\n", err)
		}
	}
}

// NewJSONRenderer is the protojson counterpart of [NewRenderer]. The
// input is parsed against d via [protojson.Unmarshal]; same wrapping
// rules apply.
func NewJSONRenderer(d protoreflect.MessageDescriptor, types protoregistry.MessageTypeResolver) stripes.Renderer {
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		data, err := io.ReadAll(r)
		if err != nil {
			fmt.Fprintf(w, "logs: read input: %v\n", err)
			return
		}
		seq, err := decodeJSON(d, data, types)
		if err != nil {
			fmt.Fprintf(w, "logs: %v\n", err)
			return
		}
		if err := Write(w, seq, WithStyles(styles)); err != nil {
			fmt.Fprintf(w, "logs: render: %v\n", err)
		}
	}
}

// decodeBinary unmarshals data against d (defaulting to LogsData when
// d is nil) and wraps the result into an iter.Seq of ResourceLogs.
// Returns an error when the descriptor is not one of the supported
// OTLP log message types or when decoding fails.
func decodeBinary(d protoreflect.MessageDescriptor, data []byte) (iter.Seq[*logsv1.ResourceLogs], error) {
	name := logsDataFullName
	if d != nil {
		name = string(d.FullName())
	}
	switch name {
	case logsDataFullName:
		ld := &logsv1.LogsData{}
		if err := proto.Unmarshal(data, ld); err != nil {
			return nil, fmt.Errorf("decode LogsData: %w", err)
		}
		return FromLogsData(ld), nil
	case resourceLogsFullName:
		rl := &logsv1.ResourceLogs{}
		if err := proto.Unmarshal(data, rl); err != nil {
			return nil, fmt.Errorf("decode ResourceLogs: %w", err)
		}
		return FromResourceLogs(rl), nil
	case scopeLogsFullName:
		sl := &logsv1.ScopeLogs{}
		if err := proto.Unmarshal(data, sl); err != nil {
			return nil, fmt.Errorf("decode ScopeLogs: %w", err)
		}
		return FromScopeLogs("", sl), nil
	case logRecordFullName:
		lr := &logsv1.LogRecord{}
		if err := proto.Unmarshal(data, lr); err != nil {
			return nil, fmt.Errorf("decode LogRecord: %w", err)
		}
		return FromLogRecords("", lr), nil
	}
	return nil, fmt.Errorf("unsupported logs schema %q", name)
}

// decodeJSON mirrors decodeBinary for protojson-encoded inputs.
func decodeJSON(d protoreflect.MessageDescriptor, data []byte, types protoregistry.MessageTypeResolver) (iter.Seq[*logsv1.ResourceLogs], error) {
	name := logsDataFullName
	if d != nil {
		name = string(d.FullName())
	}
	_ = types // reserved for future use; protojson resolver below uses GlobalTypes.
	opts := protojson.UnmarshalOptions{DiscardUnknown: true, Resolver: protoregistry.GlobalTypes}
	switch name {
	case logsDataFullName:
		ld := &logsv1.LogsData{}
		if err := opts.Unmarshal(data, ld); err != nil {
			if d != nil {
				if msg, derr := dynamicDecodeJSON(d, data, opts); derr == nil {
					return wrapDynamic(d, msg)
				}
			}
			return nil, fmt.Errorf("decode LogsData (json): %w", err)
		}
		return FromLogsData(ld), nil
	case resourceLogsFullName:
		rl := &logsv1.ResourceLogs{}
		if err := opts.Unmarshal(data, rl); err != nil {
			return nil, fmt.Errorf("decode ResourceLogs (json): %w", err)
		}
		return FromResourceLogs(rl), nil
	case scopeLogsFullName:
		sl := &logsv1.ScopeLogs{}
		if err := opts.Unmarshal(data, sl); err != nil {
			return nil, fmt.Errorf("decode ScopeLogs (json): %w", err)
		}
		return FromScopeLogs("", sl), nil
	case logRecordFullName:
		lr := &logsv1.LogRecord{}
		if err := opts.Unmarshal(data, lr); err != nil {
			return nil, fmt.Errorf("decode LogRecord (json): %w", err)
		}
		return FromLogRecords("", lr), nil
	}
	return nil, fmt.Errorf("unsupported logs schema %q", name)
}

func dynamicDecodeJSON(d protoreflect.MessageDescriptor, data []byte, opts protojson.UnmarshalOptions) (*dynamicpb.Message, error) {
	msg := dynamicpb.NewMessage(d)
	if err := opts.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func wrapDynamic(d protoreflect.MessageDescriptor, msg *dynamicpb.Message) (iter.Seq[*logsv1.ResourceLogs], error) {
	bin, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("re-marshal dynamic message: %w", err)
	}
	return decodeBinary(d, bin)
}
