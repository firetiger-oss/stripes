package stripes

import (
	"mime"
	"path"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// ObjectFunc returns the appropriate render function for the given content type and schema URL.
// Returns nil if no suitable renderer is found for the content type.
func ObjectFunc(contentType, schemaURL string) Func {
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
