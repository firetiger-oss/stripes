package protobuf

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
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

			renderer := New(desc, protoregistry.GlobalTypes)
			renderer(&buf, reader, stripes.DefaultStyles)

			got := ansi.Strip(buf.String())
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
			renderer := New(nil, nil)
			renderer(&buf, reader, stripes.DefaultStyles)

			got := ansi.Strip(buf.String())
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

		renderer := New(desc, protoregistry.GlobalTypes)
		renderer(&buf, reader, stripes.DefaultStyles)

		got := buf.String()
		if !strings.HasPrefix(got, "Error unmarshaling protobuf: proto:") {
			t.Errorf("Expected error message to start with 'Error unmarshaling protobuf: proto:', got: %q", got)
		}
	})
}

func TestProtobufJSONRendering(t *testing.T) {
	msg := &commonv1.KeyValue{Key: "service.name", Value: &commonv1.AnyValue{
		Value: &commonv1.AnyValue_StringValue{StringValue: "stripes"},
	}}
	data := []byte(`{"key":"service.name","value":{"stringValue":"stripes"}}`)
	desc := msg.ProtoReflect().Descriptor()

	var buf bytes.Buffer
	renderer := NewJSON(desc, protoregistry.GlobalTypes)
	renderer(&buf, bytes.NewReader(data), stripes.DefaultStyles)

	got := ansi.Strip(buf.String())
	want := `key: "service.name"
value: "stripes"
`
	if got != want {
		t.Errorf("Expected:\n%q\nGot:\n%q", want, got)
	}
}

func TestProtobufJSONInvalidPayload(t *testing.T) {
	desc := (&emptypb.Empty{}).ProtoReflect().Descriptor()

	var buf bytes.Buffer
	renderer := NewJSON(desc, protoregistry.GlobalTypes)
	renderer(&buf, bytes.NewReader([]byte("not json")), stripes.DefaultStyles)

	got := buf.String()
	if !strings.HasPrefix(got, "Error unmarshaling protojson:") {
		t.Errorf("Expected error prefix 'Error unmarshaling protojson:', got: %q", got)
	}
}

// TestProtobufMessageTypeParam verifies the renderer falls back to the
// "messageType" MIME parameter when --schema is not supplied — the
// convention application/protobuf; messageType="foo.Bar" uses.
func TestProtobufMessageTypeParam(t *testing.T) {
	msg := &commonv1.KeyValue{Key: "hi"}
	binaryData, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	// Empty schemaURL, but params carry messageType — should decode.
	// mime.ParseMediaType lowercases param names so we look up
	// "messagetype" with the canonical lowercase form.
	r := rendererFor(map[string]string{"messagetype": string(msg.ProtoReflect().Descriptor().FullName())}, "")
	if r == nil {
		t.Fatalf("rendererFor returned nil")
	}
	var buf bytes.Buffer
	r(&buf, bytes.NewReader(binaryData), stripes.DefaultStyles)
	if !strings.Contains(ansi.Strip(buf.String()), "\"hi\"") {
		t.Errorf("decoded output missing field value: %q", buf.String())
	}

	// schemaURL wins over messageType when both are set.
	r = rendererFor(map[string]string{"messagetype": "does.not.exist"}, string(msg.ProtoReflect().Descriptor().FullName()))
	var buf2 bytes.Buffer
	r(&buf2, bytes.NewReader(binaryData), stripes.DefaultStyles)
	if !strings.Contains(ansi.Strip(buf2.String()), "\"hi\"") {
		t.Errorf("explicit schemaURL did not win over messageType: %q", buf2.String())
	}
}

// TestProtobufFuncDispatch verifies the registry dispatch picks the
// binary or protojson renderer based on the +suffix carried in params
// (set by stripes.Func when collapsing application/protobuf+json).
func TestProtobufFuncDispatch(t *testing.T) {
	msg := &commonv1.KeyValue{Key: "hi"}
	desc := msg.ProtoReflect().Descriptor()
	schemaURL := string(desc.FullName())

	binary := rendererFor(nil, schemaURL)
	if binary == nil {
		t.Fatalf("rendererFor(nil, %q) = nil", schemaURL)
	}
	withJSON := rendererFor(map[string]string{stripes.SuffixParam: "json"}, schemaURL)
	if withJSON == nil {
		t.Fatalf("rendererFor(+json, %q) = nil", schemaURL)
	}

	binaryData, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	var bbuf bytes.Buffer
	binary(&bbuf, bytes.NewReader(binaryData), stripes.DefaultStyles)
	if !strings.Contains(ansi.Strip(bbuf.String()), "\"hi\"") {
		t.Errorf("binary renderer output: %q", bbuf.String())
	}

	var jbuf bytes.Buffer
	withJSON(&jbuf, bytes.NewReader([]byte(`{"key":"hi"}`)), stripes.DefaultStyles)
	if !strings.Contains(ansi.Strip(jbuf.String()), "\"hi\"") {
		t.Errorf("protojson renderer output: %q", jbuf.String())
	}
}

// TestBinpbExtensionDetection verifies that the registry picks up the
// .binpb extension registered alongside application/protobuf.
func TestBinpbExtensionDetection(t *testing.T) {
	if ct := stripes.Detect("payload.binpb", nil); ct != "application/protobuf" {
		t.Errorf("Detect(payload.binpb) = %q, want application/protobuf", ct)
	}
}

// TestProtobufWrapRespectsNestingDepth guards against the bug where
// long string values inside deeply-nested messages wrapped to the raw
// terminal width and overflowed once the chain indent was prepended.
// Builds a 4-level-deep message holding a long string and verifies
// every rendered line — including the wrapped continuation lines —
// fits within the requested width.
func TestProtobufWrapRespectsNestingDepth(t *testing.T) {
	// Long sentence with lots of word boundaries so wordwrap has plenty
	// of places to break.
	long := strings.Repeat("the quick brown fox jumps over the lazy dog ", 10)

	// 4 nested levels of LogRecord → Body(AnyValue) → KvlistValue →
	// KeyValue → string. Use real OTLP types so the test exercises the
	// same render path as the user's case.
	msg := &logsv1.LogRecord{
		Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{
			KvlistValue: &commonv1.KeyValueList{Values: []*commonv1.KeyValue{{
				Key: "judgment.output",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{
					KvlistValue: &commonv1.KeyValueList{Values: []*commonv1.KeyValue{{
						Key:   "summary",
						Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: long}},
					}}},
				}},
			}}},
		}},
	}

	const width = 80
	styles := &stripes.Styles{Indent: "  ", Width: width}

	var buf bytes.Buffer
	renderer := New(msg.ProtoReflect().Descriptor(), nil)
	renderer(&buf, bytes.NewReader(mustMarshal(t, msg)), styles)

	for i, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		visible := ansi.StringWidth(line)
		if visible > width {
			t.Errorf("line %d width=%d > %d: %q", i, visible, width, line)
		}
	}
}

func mustMarshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return data
}
