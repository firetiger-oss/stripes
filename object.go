package stripes

import (
	"mime"
	"path"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Func returns the [Renderer] matching contentType (a MIME media type), or
// nil if the content type is unsupported. For application/protobuf,
// schemaURL is interpreted as the full message name used to look up the
// descriptor in [protoregistry.GlobalTypes] / [protoregistry.GlobalFiles];
// for other formats it is ignored.
func Func(contentType, schemaURL string) Renderer {
	mediaType, _, _ := mime.ParseMediaType(contentType)

	if strings.HasPrefix(mediaType, "application/") {
		if i := strings.LastIndexByte(mediaType, '+'); i >= 0 {
			mediaType = "application/" + mediaType[i+1:]
		}
	}

	switch mediaType {
	case "application/protobuf":
		types := protoregistry.GlobalTypes
		if schemaURL == "" {
			return Protobuf(nil, types)
		}
		fullName := protoreflect.FullName(path.Base(schemaURL))

		messageType, err := types.FindMessageByName(fullName)
		if err == nil {
			return Protobuf(messageType.New().Descriptor(), types)
		}

		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(fullName)
		if err != nil {
			return Protobuf(nil, types)
		}
		if msgDesc, ok := desc.(protoreflect.MessageDescriptor); ok {
			return Protobuf(msgDesc, types)
		}

		return Protobuf(nil, types)
	case "application/json":
		return JSON
	case "application/yaml":
		return YAML
	case "application/xml":
		return XML
	case "text/html":
		return HTML
	case "text/csv":
		return CSV
	default:
		if strings.HasPrefix(mediaType, "text/") {
			return Text
		}
	}
	return nil
}
