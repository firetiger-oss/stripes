package stripes

import (
	"bytes"
	"strings"
	"testing"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestProtobufRendering(t *testing.T) {
	tests := []struct {
		name     string
		message  proto.Message
		expected string
	}{
		{
			name:    "StringValue wrapper",
			message: wrapperspb.String("Hello, World!"),
			expected: `value: "Hello, World!"
`,
		},
		{
			name:    "Int64Value wrapper",
			message: wrapperspb.Int64(42),
			expected: `value: 42
`,
		},
		{
			name:    "BoolValue wrapper",
			message: wrapperspb.Bool(true),
			expected: `value: true
`,
		},
		{
			name:    "DoubleValue wrapper",
			message: wrapperspb.Double(3.14159),
			expected: `value: 3.14159
`,
		},
		{
			name:     "Empty message",
			message:  &emptypb.Empty{},
			expected: ``,
		},
		{
			name: "AnyValue with string_value (oneof)",
			message: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_StringValue{
					StringValue: "test string",
				},
			},
			expected: `string_value: "test string"
`,
		},
		{
			name: "AnyValue with int_value (oneof)",
			message: &commonv1.AnyValue{
				Value: &commonv1.AnyValue_IntValue{
					IntValue: 123,
				},
			},
			expected: `int_value: 123
`,
		},
		{
			name: "KeyValue pair",
			message: &commonv1.KeyValue{
				Key: "service.name",
				Value: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{
						StringValue: "my-service",
					},
				},
			},
			expected: `key: "service.name"
value: "my-service"
`,
		},
		{
			name: "Resource with attributes",
			message: &resourcev1.Resource{
				Attributes: []*commonv1.KeyValue{
					{
						Key: "service.name",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{
								StringValue: "test-service",
							},
						},
					},
					{
						Key: "service.version",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{
								StringValue: "1.0.0",
							},
						},
					},
				},
			},
			expected: `attributes: {
  key: "service.name"
  value: "test-service"
}
attributes: {
  key: "service.version"
  value: "1.0.0"
}
`,
		},
		{
			name: "LogRecord with nested fields",
			message: &logsv1.LogRecord{
				TimeUnixNano: 1640995200000000000,
				SeverityText: "ERROR",
				Body: &commonv1.AnyValue{
					Value: &commonv1.AnyValue_StringValue{
						StringValue: "Critical error occurred",
					},
				},
				Attributes: []*commonv1.KeyValue{
					{
						Key: "user.id",
						Value: &commonv1.AnyValue{
							Value: &commonv1.AnyValue_StringValue{
								StringValue: "user123",
							},
						},
					},
				},
			},
			expected: `time_unix_nano: 1640995200000000000
severity_text: "ERROR"
body: "Critical error occurred"
attributes: {
  key: "user.id"
  value: "user123"
}
`,
		},
		{
			name:    "Timestamp well-known type",
			message: timestamppb.New(time.Unix(1700000000, 500000000)),
			expected: `seconds: 1700000000
nanos: 500000000
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := proto.Marshal(tt.message)
			if err != nil {
				t.Fatalf("Failed to marshal message: %v", err)
			}

			desc := tt.message.ProtoReflect().Descriptor()
			var buf bytes.Buffer
			reader := bytes.NewReader(data)

			renderer := Protobuf(desc, protoregistry.GlobalTypes)
			renderer(&buf, reader, DefaultStyles)

			got := buf.String()
			if got != tt.expected {
				t.Errorf("Expected:\n%q\nGot:\n%q", tt.expected, got)
			}
		})
	}
}

func TestProtobufWireFormat(t *testing.T) {
	tests := []struct {
		name     string
		message  proto.Message
		expected string
	}{
		{
			name:    "Simple message wire format",
			message: wrapperspb.String("test"),
			// Expected output for a simple string wrapper - this will be deterministic
			expected: `Wire format (6 bytes):
  field 1 (tag=1, wire_type=length-delimited): "test"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := proto.Marshal(tt.message)
			if err != nil {
				t.Fatalf("Failed to marshal message: %v", err)
			}

			var buf bytes.Buffer
			reader := bytes.NewReader(data)

			// Use nil descriptor to force wire format
			renderer := Protobuf(nil, nil)
			renderer(&buf, reader, DefaultStyles)

			got := buf.String()
			if got != tt.expected {
				t.Errorf("Expected:\n%q\nGot:\n%q", tt.expected, got)
			}
		})
	}
}

func TestProtobufErrorHandling(t *testing.T) {
	t.Run("Invalid protobuf data", func(t *testing.T) {
		invalidData := []byte{0xFF, 0xFF, 0xFF}
		desc := (&emptypb.Empty{}).ProtoReflect().Descriptor()

		var buf bytes.Buffer
		reader := bytes.NewReader(invalidData)

		renderer := Protobuf(desc, protoregistry.GlobalTypes)
		renderer(&buf, reader, DefaultStyles)

		got := buf.String()
		if !strings.HasPrefix(got, "Error unmarshaling protobuf: proto:") {
			t.Errorf("Expected error message to start with 'Error unmarshaling protobuf: proto:', got: %q", got)
		}
	})
}
