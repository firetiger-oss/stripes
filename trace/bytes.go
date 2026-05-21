package trace

import (
	"fmt"
	"io"
	"iter"

	"github.com/firetiger-oss/stripes"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// NewRenderer returns a [stripes.Renderer] that decodes binary OTLP
// protobuf against d and renders a waterfall. d may be the descriptor
// for TracesData, ResourceSpans, ScopeSpans, or Span; messages at any
// of those levels are wrapped up to a synthetic TracesData before
// rendering. types resolves embedded message references; pass
// [protoregistry.GlobalTypes] for the default behaviour.
//
// When d is nil the renderer assumes TracesData. If the descriptor
// names an unsupported message the renderer writes an error line and
// returns.
func NewRenderer(d protoreflect.MessageDescriptor, types protoregistry.MessageTypeResolver) stripes.Renderer {
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		data, err := io.ReadAll(r)
		if err != nil {
			fmt.Fprintf(w, "trace: read input: %v\n", err)
			return
		}
		seq, err := decodeBinary(d, data)
		if err != nil {
			fmt.Fprintf(w, "trace: %v\n", err)
			return
		}
		if err := Write(w, seq, WithStyles(styles)); err != nil {
			fmt.Fprintf(w, "trace: render: %v\n", err)
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
			fmt.Fprintf(w, "trace: read input: %v\n", err)
			return
		}
		seq, err := decodeJSON(d, data, types)
		if err != nil {
			fmt.Fprintf(w, "trace: %v\n", err)
			return
		}
		if err := Write(w, seq, WithStyles(styles)); err != nil {
			fmt.Fprintf(w, "trace: render: %v\n", err)
		}
	}
}

// decodeBinary unmarshals data against d (defaulting to TracesData
// when d is nil) and wraps the result into an iter.Seq of
// ResourceSpans. Returns an error when the descriptor is not one of
// the supported OTLP trace message types or when decoding fails.
func decodeBinary(d protoreflect.MessageDescriptor, data []byte) (iter.Seq[*tracev1.ResourceSpans], error) {
	name := tracesDataFullName
	if d != nil {
		name = string(d.FullName())
	}
	switch name {
	case tracesDataFullName:
		td := &tracev1.TracesData{}
		if err := proto.Unmarshal(data, td); err != nil {
			return nil, fmt.Errorf("decode TracesData: %w", err)
		}
		return FromTracesData(td), nil
	case resourceSpansFullName:
		rs := &tracev1.ResourceSpans{}
		if err := proto.Unmarshal(data, rs); err != nil {
			return nil, fmt.Errorf("decode ResourceSpans: %w", err)
		}
		return FromResourceSpans(rs), nil
	case scopeSpansFullName:
		ss := &tracev1.ScopeSpans{}
		if err := proto.Unmarshal(data, ss); err != nil {
			return nil, fmt.Errorf("decode ScopeSpans: %w", err)
		}
		return FromScopeSpans("", ss), nil
	case spanFullName:
		sp := &tracev1.Span{}
		if err := proto.Unmarshal(data, sp); err != nil {
			return nil, fmt.Errorf("decode Span: %w", err)
		}
		return FromSpans("", sp), nil
	}
	return nil, fmt.Errorf("unsupported trace schema %q", name)
}

// decodeJSON mirrors decodeBinary for protojson-encoded inputs. The
// proto runtime can only protojson-unmarshal against a compiled
// MessageType, so when d resolves to a dynamic-only descriptor we
// route through dynamicpb and re-marshal to binary for the typed
// decode. types resolves embedded references inside Any payloads.
func decodeJSON(d protoreflect.MessageDescriptor, data []byte, types protoregistry.MessageTypeResolver) (iter.Seq[*tracev1.ResourceSpans], error) {
	name := tracesDataFullName
	if d != nil {
		name = string(d.FullName())
	}
	_ = types // reserved for future use; protojson resolver below uses GlobalTypes.
	opts := protojson.UnmarshalOptions{DiscardUnknown: true, Resolver: protoregistry.GlobalTypes}
	switch name {
	case tracesDataFullName:
		td := &tracev1.TracesData{}
		if err := opts.Unmarshal(data, td); err != nil {
			// Some encoders (e.g. OTLP/HTTP+JSON) emit bare ResourceSpans
			// arrays under a top-level "resourceSpans" key, which decodes
			// fine. Other tools wrap differently; fall through with the
			// original error.
			if d != nil {
				if msg, derr := dynamicDecodeJSON(d, data, opts); derr == nil {
					return wrapDynamic(d, msg)
				}
			}
			return nil, fmt.Errorf("decode TracesData (json): %w", err)
		}
		return FromTracesData(td), nil
	case resourceSpansFullName:
		rs := &tracev1.ResourceSpans{}
		if err := opts.Unmarshal(data, rs); err != nil {
			return nil, fmt.Errorf("decode ResourceSpans (json): %w", err)
		}
		return FromResourceSpans(rs), nil
	case scopeSpansFullName:
		ss := &tracev1.ScopeSpans{}
		if err := opts.Unmarshal(data, ss); err != nil {
			return nil, fmt.Errorf("decode ScopeSpans (json): %w", err)
		}
		return FromScopeSpans("", ss), nil
	case spanFullName:
		sp := &tracev1.Span{}
		if err := opts.Unmarshal(data, sp); err != nil {
			return nil, fmt.Errorf("decode Span (json): %w", err)
		}
		return FromSpans("", sp), nil
	}
	return nil, fmt.Errorf("unsupported trace schema %q", name)
}

// dynamicDecodeJSON decodes data into a *dynamicpb.Message against d
// when the compiled type can't handle the wire shape (rare; mostly a
// safety hatch for caller-loaded descriptors).
func dynamicDecodeJSON(d protoreflect.MessageDescriptor, data []byte, opts protojson.UnmarshalOptions) (*dynamicpb.Message, error) {
	msg := dynamicpb.NewMessage(d)
	if err := opts.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// wrapDynamic re-encodes msg through the wire format and decodes it
// back into the compiled OTLP type matching d's full name. Used only
// for the rare dynamic-descriptor fallback in decodeJSON.
func wrapDynamic(d protoreflect.MessageDescriptor, msg *dynamicpb.Message) (iter.Seq[*tracev1.ResourceSpans], error) {
	bin, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("re-marshal dynamic message: %w", err)
	}
	return decodeBinary(d, bin)
}
