// Package protobuf registers the Protobuf renderer with the stripes
// registry. Import for side effects to enable application/protobuf
// support:
//
//	import _ "github.com/firetiger-oss/stripes/protobuf"
//
// For application/protobuf, [stripes.Func]'s schemaURL argument is
// interpreted as the full message name used to look up the descriptor
// in [protoregistry.GlobalTypes] / [protoregistry.GlobalFiles]. Without
// a resolvable schema, the renderer falls back to wire-format display.
//
// application/protobuf+json (protojson encoding) is recognized through
// the stripes registry's +suffix convention and decoded with
// [protojson]. Output is always protobuf text format, regardless of the
// input encoding.
package protobuf

import (
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/firetiger-oss/stripes"
	"github.com/muesli/reflow/wordwrap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "protobuf",
		ContentType: "application/protobuf",
		Extensions:  []string{".binpb"},
		RendererFor: rendererFor,
	})
}

// rendererFor resolves a Protobuf renderer from the schema URL passed to
// [stripes.Func]. schemaURL is treated as a full message name; its base
// is looked up in protoregistry.GlobalTypes, then GlobalFiles. When no
// descriptor resolves, the wire-format renderer is returned.
//
// When params[[stripes.SuffixParam]] is "json" (i.e. the original MIME
// type was application/protobuf+json), the returned renderer decodes
// protojson; otherwise it decodes binary protobuf. Output is the same
// protobuf text format in both cases.
func rendererFor(params map[string]string, schemaURL string) stripes.Renderer {
	types := protoregistry.GlobalTypes
	protojsonEncoded := params[stripes.SuffixParam] == "json"
	if schemaURL == "" {
		return New(nil, types)
	}
	fullName := protoreflect.FullName(path.Base(schemaURL))

	messageType, err := types.FindMessageByName(fullName)
	if err == nil {
		desc := messageType.New().Descriptor()
		if protojsonEncoded {
			return NewJSON(desc, types)
		}
		return New(desc, types)
	}

	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(fullName)
	if err != nil {
		return New(nil, types)
	}
	if msgDesc, ok := desc.(protoreflect.MessageDescriptor); ok {
		if protojsonEncoded {
			return NewJSON(msgDesc, types)
		}
		return New(msgDesc, types)
	}
	return New(nil, types)
}

var (
	protoKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // Yellow bold for keys
	protoStringStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))             // Green for string values
	protoDelimStyle  = lipgloss.NewStyle().Bold(true)                                  // Bold delimiters
	protoWireStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // Grey for wire format
)

// New returns a [stripes.Renderer] for binary-encoded protobuf
// messages. When d is nil the renderer displays the raw wire format;
// otherwise it decodes against the descriptor d, resolving nested
// message types through t.
func New(d protoreflect.MessageDescriptor, t protoregistry.MessageTypeResolver) stripes.Renderer {
	if d == nil {
		return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
			printProtobufWire(w, r, styles)
		}
	}
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		printProtobufMessage(w, r, d, t, styles)
	}
}

// NewJSON returns a [stripes.Renderer] for protojson-encoded protobuf
// messages. The decoded message is rendered in the same protobuf text
// format as [New]. d must not be nil; without a schema there is no
// useful protojson decoding to attempt.
func NewJSON(d protoreflect.MessageDescriptor, t protoregistry.MessageTypeResolver) stripes.Renderer {
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		printProtobufJSON(w, r, d, t, styles)
	}
}

func printProtobufMessage(w io.Writer, r io.Reader, d protoreflect.MessageDescriptor, t protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "Error reading protobuf data: %v", err)
		return
	}

	// Try to find message type in GlobalTypes first
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(d.FullName())
	if err != nil {
		// If not found in GlobalTypes, create a dynamic message from the descriptor
		msg := dynamicpb.NewMessage(d)
		if err := proto.Unmarshal(data, msg); err != nil {
			fmt.Fprintf(w, "Error unmarshaling protobuf: %v", err)
			return
		}
		// Render in protobuf text format
		renderMessageFields(w, msg, d, t, styles)
		return
	}

	msg := messageType.New()
	if err := proto.Unmarshal(data, msg.Interface()); err != nil {
		fmt.Fprintf(w, "Error unmarshaling protobuf: %v", err)
		return
	}

	// Render in protobuf text format
	renderMessageFields(w, msg, d, t, styles)
}

// printProtobufJSON unmarshals data as protojson against descriptor d
// and renders the resulting message in protobuf text format. Any
// resolution falls back to protoregistry.GlobalTypes — callers that
// have loaded descriptors via stripes/protobuf/schema rely on the CLI
// to have populated GlobalFiles; embedded Any payloads still need
// matching MessageTypes in GlobalTypes to resolve cleanly.
func printProtobufJSON(w io.Writer, r io.Reader, d protoreflect.MessageDescriptor, t protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "Error reading protobuf data: %v", err)
		return
	}

	msg := dynamicpb.NewMessage(d)
	opts := protojson.UnmarshalOptions{
		DiscardUnknown: true,
		Resolver:       protoregistry.GlobalTypes,
	}
	if err := opts.Unmarshal(data, msg); err != nil {
		fmt.Fprintf(w, "Error unmarshaling protojson: %v", err)
		return
	}

	renderMessageFields(w, msg, d, t, styles)
}

func renderMessageFields(w io.Writer, msg protoreflect.Message, desc protoreflect.MessageDescriptor, typeResolver protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	fields := desc.Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if !msg.Has(field) && !field.IsList() && !field.IsMap() {
			continue
		}

		renderField(w, msg, field, typeResolver, styles)
	}
}

func renderField(w io.Writer, msg protoreflect.Message, field protoreflect.FieldDescriptor, typeResolver protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	fieldName := styles.Name.Render(string(field.Name()))

	if field.IsMap() {
		renderMapField(w, msg, field, fieldName, styles)
	} else if field.IsList() {
		renderRepeatedField(w, msg, field, typeResolver, fieldName, styles)
	} else {
		value := msg.Get(field)
		fmt.Fprintf(w, "%s: ", fieldName)
		renderFieldValue(w, value, field, typeResolver, styles)
		fmt.Fprint(w, "\n")
	}
}

// shouldRenderMessageAsOneof determines if a message should be rendered as a oneof value
// This happens when:
// 1. The message has exactly one oneof
// 2. Only one field in that oneof is set
// 3. No other fields are set
// 4. All fields in the message are part of the oneof (no regular fields)
func shouldRenderMessageAsOneof(msg protoreflect.Message, desc protoreflect.MessageDescriptor) bool {
	oneofs := desc.Oneofs()
	if oneofs.Len() != 1 {
		return false // Must have exactly one oneof
	}

	oneof := oneofs.Get(0)
	setFields := 0
	var setOneofField protoreflect.FieldDescriptor

	// Check that only one field in the oneof is set and no other fields are set
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if msg.Has(field) {
			setFields++
			if field.ContainingOneof() == oneof {
				setOneofField = field
			} else {
				// A non-oneof field is set, so we should render normally
				return false
			}
		}
	}

	// Also check that ALL fields are part of the oneof (no standalone fields)
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.ContainingOneof() != oneof {
			// There's a field that's not part of the oneof, so render normally
			return false
		}
	}

	// Should render as oneof if exactly one field is set and it's in the oneof
	return setFields == 1 && setOneofField != nil
}

// renderOneofMessage renders a message that contains oneof fields directly
func renderOneofMessage(w io.Writer, msg protoreflect.Message, desc protoreflect.MessageDescriptor, styles *stripes.Styles) {
	oneofs := desc.Oneofs()
	if oneofs.Len() != 1 {
		return // Should not happen if shouldRenderMessageAsOneof returned true
	}

	oneof := oneofs.Get(0)
	fields := oneof.Fields()

	// Find the set field in the oneof
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if msg.Has(field) {
			value := msg.Get(field)
			if field.Kind() == protoreflect.MessageKind {
				// For nested messages, render them normally
				renderMessageValue(w, value.Message(), field.Message(), nil, styles)
			} else {
				// For scalar values, render them directly
				renderScalarValueWithStyles(w, value, field.Kind(), styles)
			}
			return
		}
	}
}

func renderMapField(w io.Writer, msg protoreflect.Message, field protoreflect.FieldDescriptor, fieldName string, styles *stripes.Styles) {
	mapValue := msg.Get(field).Map()
	if mapValue.Len() == 0 {
		return // Don't render empty maps
	}

	keyKind := field.MapKey().Kind()

	// In protobuf text format, maps are rendered as repeated entries with key/value structure
	mapValue.Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
		fmt.Fprintf(w, "%s: %s\n", fieldName, protoDelimStyle.Render("{"))

		indentWriter := stripes.NewPrefixWriter(w, "  ")
		fmt.Fprintf(indentWriter, "%s: %s\n", protoKeyStyle.Render("key"), formatMapKey(key, keyKind))
		fmt.Fprintf(indentWriter, "%s: ", protoKeyStyle.Render("value"))
		renderMapValue(indentWriter, value, field.MapValue(), styles)
		fmt.Fprint(indentWriter, "\n")

		fmt.Fprintf(w, "%s\n", protoDelimStyle.Render("}"))
		return true
	})
}

func renderRepeatedField(w io.Writer, msg protoreflect.Message, field protoreflect.FieldDescriptor, typeResolver protoregistry.MessageTypeResolver, fieldName string, styles *stripes.Styles) {
	list := msg.Get(field).List()
	if list.Len() == 0 {
		return // Don't render empty repeated fields
	}

	// In protobuf text format, repeated fields are rendered as multiple entries with the same field name
	for i := 0; i < list.Len(); i++ {
		value := list.Get(i)
		fmt.Fprintf(w, "%s: ", fieldName)
		renderFieldValue(w, value, field, typeResolver, styles)
		fmt.Fprint(w, "\n")
	}
}

func renderFieldValue(w io.Writer, value protoreflect.Value, field protoreflect.FieldDescriptor, typeResolver protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	if field.Kind() == protoreflect.MessageKind {
		renderMessageValue(w, value.Message(), field.Message(), typeResolver, styles)
	} else {
		renderScalarValueWithStyles(w, value, field.Kind(), styles)
	}
}

func renderMapValue(w io.Writer, value protoreflect.Value, field protoreflect.FieldDescriptor, styles *stripes.Styles) {
	if field.Kind() == protoreflect.MessageKind {
		renderMessageValue(w, value.Message(), field.Message(), nil, styles)
	} else {
		renderScalarValueWithStyles(w, value, field.Kind(), styles)
	}
}

func renderMessageValue(w io.Writer, msg protoreflect.Message, desc protoreflect.MessageDescriptor, typeResolver protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	fullName := desc.FullName()

	// Handle well-known types with special formatting
	switch fullName {
	case "google.protobuf.Timestamp":
		renderTimestampValue(w, msg)
		return
	case "google.protobuf.Duration":
		renderDurationValue(w, msg)
		return
	case "google.protobuf.Any":
		renderAnyValue(w, msg, typeResolver, styles)
		return
	}

	// Check if this message has oneof fields and should be rendered directly
	if shouldRenderMessageAsOneof(msg, desc) {
		renderOneofMessage(w, msg, desc, styles)
		return
	}

	fmt.Fprint(w, protoDelimStyle.Render("{"))
	if hasAnyFields(msg, desc) {
		fmt.Fprint(w, "\n")

		indentWriter := stripes.NewPrefixWriter(w, "  ")
		renderMessageFields(indentWriter, msg, desc, typeResolver, styles)

		fmt.Fprintf(w, "%s", protoDelimStyle.Render("}"))
	} else {
		fmt.Fprint(w, protoDelimStyle.Render("}"))
	}
}

func hasAnyFields(msg protoreflect.Message, desc protoreflect.MessageDescriptor) bool {
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if msg.Has(field) || field.IsList() || field.IsMap() {
			return true
		}
	}
	return false
}

// renderTimestampValue renders google.protobuf.Timestamp in a readable format
func renderTimestampValue(w io.Writer, msg protoreflect.Message) {
	fields := msg.Descriptor().Fields()
	secondsField := fields.ByName("seconds")
	nanosField := fields.ByName("nanos")

	if secondsField != nil && nanosField != nil {
		seconds := msg.Get(secondsField).Int()
		nanos := msg.Get(nanosField).Int()

		fmt.Fprint(w, protoDelimStyle.Render("{"))
		fmt.Fprint(w, "\n")

		indentWriter := stripes.NewPrefixWriter(w, "  ")
		fmt.Fprintf(indentWriter, "%s: %d\n", protoKeyStyle.Render("seconds"), seconds)
		fmt.Fprintf(indentWriter, "%s: %d\n", protoKeyStyle.Render("nanos"), nanos)

		fmt.Fprint(w, protoDelimStyle.Render("}"))
	}
}

// renderDurationValue renders google.protobuf.Duration in a readable format
func renderDurationValue(w io.Writer, msg protoreflect.Message) {
	fields := msg.Descriptor().Fields()
	secondsField := fields.ByName("seconds")
	nanosField := fields.ByName("nanos")

	if secondsField != nil && nanosField != nil {
		seconds := msg.Get(secondsField).Int()
		nanos := msg.Get(nanosField).Int()

		fmt.Fprint(w, protoDelimStyle.Render("{"))
		fmt.Fprint(w, "\n")

		indentWriter := stripes.NewPrefixWriter(w, "  ")
		fmt.Fprintf(indentWriter, "%s: %d\n", protoKeyStyle.Render("seconds"), seconds)
		fmt.Fprintf(indentWriter, "%s: %d\n", protoKeyStyle.Render("nanos"), nanos)

		fmt.Fprint(w, protoDelimStyle.Render("}"))
	}
}

func renderAnyValue(w io.Writer, msg protoreflect.Message, typeResolver protoregistry.MessageTypeResolver, styles *stripes.Styles) {
	// Extract type URL and value from Any message
	fields := msg.Descriptor().Fields()
	typeUrlField := fields.ByName("type_url")
	valueField := fields.ByName("value")

	if typeUrlField == nil || valueField == nil {
		fmt.Fprint(w, protoDelimStyle.Render("{}"))
		return
	}

	typeURL := msg.Get(typeUrlField).String()
	valueBytes := msg.Get(valueField).Bytes()

	if typeURL != "" && len(valueBytes) > 0 {
		// Try to resolve and unmarshal the embedded message
		messageName := typeURL
		if idx := strings.LastIndex(typeURL, "/"); idx >= 0 {
			messageName = typeURL[idx+1:]
		}
		fullName := protoreflect.FullName(messageName)

		// Try GlobalTypes first (compiled types)
		var messageType protoreflect.MessageType
		if typeResolver != nil {
			messageType, _ = typeResolver.FindMessageByName(fullName)
		}

		// Fallback to GlobalFiles (dynamic descriptors from session)
		if messageType == nil {
			if desc, err := protoregistry.GlobalFiles.FindDescriptorByName(fullName); err == nil {
				if msgDesc, ok := desc.(protoreflect.MessageDescriptor); ok {
					messageType = dynamicpb.NewMessageType(msgDesc)
				}
			}
		}

		if messageType != nil {
			embeddedMsg := messageType.New()
			if err := proto.Unmarshal(valueBytes, embeddedMsg.Interface()); err == nil {
				// Render with special Any syntax: [type_url]: { ... }
				fmt.Fprint(w, protoDelimStyle.Render("{"))
				fmt.Fprint(w, "\n")

				indentWriter := stripes.NewPrefixWriter(w, "  ")
				fmt.Fprintf(indentWriter, "%s: ", protoKeyStyle.Render("["+typeURL+"]"))
				renderMessageValue(indentWriter, embeddedMsg, messageType.Descriptor(), typeResolver, styles)
				fmt.Fprint(indentWriter, "\n")

				fmt.Fprint(w, protoDelimStyle.Render("}"))
				return
			}
		}
	}

	// Fallback to regular message rendering
	fmt.Fprint(w, protoDelimStyle.Render("{"))
	fmt.Fprint(w, "\n")

	indentWriter := stripes.NewPrefixWriter(w, "  ")
	if typeURL != "" {
		fmt.Fprintf(indentWriter, "%s: %s\n", protoKeyStyle.Render("type_url"), protoStringStyle.Render(strconv.Quote(typeURL)))
	}
	if len(valueBytes) > 0 {
		fmt.Fprintf(indentWriter, "%s: %s\n", protoKeyStyle.Render("value"), protoStringStyle.Render(fmt.Sprintf("\"%x\"", valueBytes)))
	}

	fmt.Fprint(w, protoDelimStyle.Render("}"))
}

func renderScalarValueWithStyles(w io.Writer, value protoreflect.Value, kind protoreflect.Kind, styles *stripes.Styles) {
	switch kind {
	case protoreflect.StringKind:
		str := value.String()
		if styles != nil && styles.Width > 0 {
			quotedValue := strconv.Quote(str)
			wrappedQuoted := wrapProtobufString(quotedValue, styles.Width)
			fmt.Fprint(w, protoStringStyle.Render(wrappedQuoted))
		} else {
			fmt.Fprint(w, protoStringStyle.Render(strconv.Quote(str)))
		}
	case protoreflect.BytesKind:
		// Render bytes as hex string
		fmt.Fprint(w, protoStringStyle.Render(fmt.Sprintf("\"%x\"", value.Bytes())))
	case protoreflect.BoolKind:
		fmt.Fprint(w, value.Bool())
	case protoreflect.EnumKind:
		// For enums, show the numeric value (protobuf text format standard)
		fmt.Fprint(w, value.Enum())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		fmt.Fprint(w, value.Int())
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		fmt.Fprint(w, value.Int())
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		fmt.Fprint(w, value.Uint())
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		fmt.Fprint(w, value.Uint())
	case protoreflect.FloatKind:
		fmt.Fprint(w, value.Float())
	case protoreflect.DoubleKind:
		fmt.Fprint(w, value.Float())
	default:
		fmt.Fprint(w, value.String())
	}
}

func formatMapKey(key protoreflect.MapKey, kind protoreflect.Kind) string {
	switch kind {
	case protoreflect.StringKind:
		return protoStringStyle.Render(strconv.Quote(key.String()))
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return strconv.FormatInt(key.Int(), 10)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return strconv.FormatInt(key.Int(), 10)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return strconv.FormatUint(key.Uint(), 10)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return strconv.FormatUint(key.Uint(), 10)
	case protoreflect.BoolKind:
		return strconv.FormatBool(key.Bool())
	default:
		return key.String()
	}
}

func printProtobufWire(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "Error reading protobuf data: %v", err)
		return
	}

	fmt.Fprint(w, styles.Text.Render("Wire format ("))
	fmt.Fprint(w, styles.Number.Render(fmt.Sprintf("%d bytes", len(data))))
	fmt.Fprint(w, styles.Text.Render("):"))
	fmt.Fprint(w, "\n")

	if len(data) == 0 {
		fmt.Fprint(w, protoWireStyle.Render("(empty)"))
		return
	}

	fieldNum := 1
	for len(data) > 0 {
		// Parse the field tag and wire type
		fieldNumber, wireType, tagSize := protowire.ConsumeTag(data)
		if tagSize < 0 {
			fmt.Fprintf(w, protoWireStyle.Render("Error parsing field %d: invalid tag"), fieldNum)
			break
		}
		data = data[tagSize:]

		// Parse the field value based on wire type
		value, valueSize := parseWireValue(data, wireType)
		if valueSize < 0 {
			fmt.Fprintf(w, protoWireStyle.Render("Error parsing field %d: invalid value"), fieldNum)
			break
		}
		data = data[valueSize:]

		fmt.Fprintf(w, protoWireStyle.Render("  field %d (tag=%d, wire_type=%s): %s"),
			fieldNum, fieldNumber, getWireTypeName(wireType), value)
		fmt.Fprint(w, "\n")
		fieldNum++
	}
}

func parseWireValue(data []byte, wireType protowire.Type) (string, int) {
	switch wireType {
	case protowire.VarintType:
		val, n := protowire.ConsumeVarint(data)
		if n < 0 {
			return "", n
		}
		return fmt.Sprintf("%d", val), n
	case protowire.Fixed64Type:
		val, n := protowire.ConsumeFixed64(data)
		if n < 0 {
			return "", n
		}
		return fmt.Sprintf("0x%016x", val), n
	case protowire.BytesType:
		val, n := protowire.ConsumeBytes(data)
		if n < 0 {
			return "", n
		}
		// Try to display as string if printable, otherwise as hex
		if isPrintable(val) {
			return fmt.Sprintf("\"%s\"", string(val)), n
		}
		return fmt.Sprintf("%d bytes (hex: %x)", len(val), val), n
	case protowire.Fixed32Type:
		val, n := protowire.ConsumeFixed32(data)
		if n < 0 {
			return "", n
		}
		return fmt.Sprintf("0x%08x", val), n
	default:
		return "unknown wire type", 0
	}
}

func isPrintable(data []byte) bool {
	for _, b := range data {
		if b < 32 || b > 126 {
			return false
		}
	}
	return true
}

func getWireTypeName(wireType protowire.Type) string {
	switch wireType {
	case protowire.VarintType:
		return "varint"
	case protowire.Fixed64Type:
		return "64-bit"
	case protowire.BytesType:
		return "length-delimited"
	case protowire.Fixed32Type:
		return "32-bit"
	default:
		return "unknown"
	}
}

// wrapProtobufString wraps protobuf string values with proper indentation
func wrapProtobufString(quotedValue string, width int) string {
	if width <= 0 || len(quotedValue) <= width-4 {
		return quotedValue
	}

	// Calculate available width (reserve some space for field name and indentation)
	availableWidth := width - 4
	if availableWidth < 20 {
		return quotedValue
	}

	// Apply word wrapping to the quoted content
	wrappedContent := wordwrap.String(quotedValue, availableWidth)

	// Handle multi-line by adding proper indentation for continuation lines
	if strings.Contains(wrappedContent, "\n") {
		// Use minimal indentation for continuation lines (2 spaces like field values)
		indent := "  "
		return strings.ReplaceAll(wrappedContent, "\n", "\n"+indent)
	}

	return wrappedContent
}
