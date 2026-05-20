// stub-buf is a deterministic buf-CLI replacement used by stripes'
// integration tests. It mimics just enough of "buf build <ref> -o -"
// to return a FileDescriptorSet on stdout: for the recognized test
// module reference it emits a descriptor set containing a small
// message ("stub.v1.Hello"); for anything else it exits non-zero with
// a buf-shaped error message so the CLI can surface it as the real buf
// would.
package main

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
)

const knownRef = "buf.build/stub/hello"

func main() {
	if len(os.Args) < 5 || os.Args[1] != "build" || os.Args[3] != "-o" || os.Args[4] != "-" {
		fmt.Fprintf(os.Stderr, "stub-buf: unexpected argv: %v\n", os.Args)
		os.Exit(2)
	}
	ref := os.Args[2]
	if ref != knownRef {
		fmt.Fprintf(os.Stderr, "Failure: module %s not found\n", ref)
		os.Exit(1)
	}

	fdProto := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("stub/v1/hello.proto"),
		Package: proto.String("stub.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Hello"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     proto.String("greeting"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("greeting"),
			}},
		}},
	}
	// Round-trip through protodesc so the FileDescriptorProto is
	// validated before we hand it back to stripes.
	if _, err := protodesc.NewFile(fdProto, nil); err != nil {
		fmt.Fprintf(os.Stderr, "stub-buf: invalid FileDescriptorProto: %v\n", err)
		os.Exit(2)
	}

	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fdProto}}
	data, err := proto.Marshal(set)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub-buf: marshal: %v\n", err)
		os.Exit(2)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "stub-buf: write: %v\n", err)
		os.Exit(2)
	}
}
