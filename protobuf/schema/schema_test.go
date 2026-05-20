package schema_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes/protobuf/schema"
	_ "github.com/firetiger-oss/tigerblock/storage/file"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const fooProto = `syntax = "proto3";
package foo.v1;

message Foo {
  string name = 1;
  int32 count = 2;
}
`

const wktProto = `syntax = "proto3";
package wkt.v1;

import "google/protobuf/timestamp.proto";

message Event {
  google.protobuf.Timestamp at = 1;
  string note = 2;
}
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestLoadRegistryProtoFile(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "foo.proto", fooProto)

	files, err := schema.LoadRegistry(context.Background(), []string{p}, nil)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	desc, err := files.FindDescriptorByName("foo.v1.Foo")
	if err != nil {
		t.Fatalf("FindDescriptorByName: %v", err)
	}
	if _, ok := desc.(protoreflect.MessageDescriptor); !ok {
		t.Fatalf("foo.v1.Foo is not a MessageDescriptor")
	}
}

func TestLoadRegistryProtoWithWKT(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "evt.proto", wktProto)

	files, err := schema.LoadRegistry(context.Background(), []string{p}, nil)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if _, err := files.FindDescriptorByName("wkt.v1.Event"); err != nil {
		t.Fatalf("FindDescriptorByName: %v", err)
	}
}

func TestLoadRegistryDescriptorSet(t *testing.T) {
	tsDesc := (&timestamppb.Timestamp{}).ProtoReflect().Descriptor()
	fdProto := protodesc.ToFileDescriptorProto(tsDesc.ParentFile())
	fdset := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fdProto}}
	data, err := proto.Marshal(fdset)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "ts.binpbset")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	files, err := schema.LoadRegistry(context.Background(), []string{p}, nil)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if _, err := files.FindDescriptorByName("google.protobuf.Timestamp"); err != nil {
		t.Fatalf("FindDescriptorByName: %v", err)
	}
}

// TestLoadRegistryImportRelative exercises the protoc -I model: the
// registry value is an import path resolved against a single include
// root, and a cross-directory import resolves under that same root.
func TestLoadRegistryImportRelative(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "a"), "foo.proto", `syntax = "proto3";
package a.v1;
import "b/bar.proto";
message Foo { b.v1.Bar bar = 1; }
`)
	writeFile(t, filepath.Join(root, "b"), "bar.proto", `syntax = "proto3";
package b.v1;
message Bar { string id = 1; }
`)

	files, err := schema.LoadRegistry(context.Background(),
		[]string{"a/foo.proto"}, []string{root})
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if _, err := files.FindDescriptorByName("a.v1.Foo"); err != nil {
		t.Errorf("FindDescriptorByName(a.v1.Foo): %v", err)
	}
	if _, err := files.FindDescriptorByName("b.v1.Bar"); err != nil {
		t.Errorf("FindDescriptorByName(b.v1.Bar): %v", err)
	}
}

func TestLoadRegistryMissingFile(t *testing.T) {
	_, err := schema.LoadRegistry(context.Background(), []string{filepath.Join(t.TempDir(), "nope.proto")}, nil)
	if err == nil {
		t.Fatalf("expected error for missing file, got nil")
	}
}

func TestLoadRegistryUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "junk.txt", "junk")
	_, err := schema.LoadRegistry(context.Background(), []string{p}, nil)
	if err == nil {
		t.Fatalf("expected error for unsupported extension, got nil")
	}
	if !strings.Contains(err.Error(), "unrecognized input") {
		t.Errorf("error %q does not mention unrecognized input", err)
	}
}

func TestLoadRegistryConflictingSymbols(t *testing.T) {
	dir := t.TempDir()
	const dup = `syntax = "proto3";
package dup.v1;

message Thing {
  string id = 1;
}
`
	p1 := writeFile(t, dir, "c1.proto", dup)
	p2 := writeFile(t, dir, "c2.proto", dup)
	_, err := schema.LoadRegistry(context.Background(), []string{p1, p2}, nil)
	if err == nil {
		t.Fatalf("expected error for conflicting symbol registration, got nil")
	}
}

func TestLoadRegistrySameFileTwiceIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "foo.proto", fooProto)
	files, err := schema.LoadRegistry(context.Background(), []string{p, p}, nil)
	if err != nil {
		t.Fatalf("LoadRegistry with the same file twice: %v", err)
	}
	if _, err := files.FindDescriptorByName("foo.v1.Foo"); err != nil {
		t.Errorf("FindDescriptorByName: %v", err)
	}
}

// TestLoadRegistryBufModule exercises the buf-CLI subprocess branch
// of LoadRegistry by building the stub-buf binary at the repo root,
// dropping it onto PATH as "buf", and asking LoadRegistry to resolve a
// reference the stub recognizes. Confirms (a) the subprocess is
// invoked with the expected argv, (b) its stdout (a FileDescriptorSet)
// round-trips through loadDescriptorSetBytes, (c) the message it
// declares is name-resolvable in the returned files.
func TestLoadRegistryBufModule(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess PATH override is not portable to windows in this test")
	}
	stubBin := buildStubBuf(t)

	t.Setenv("PATH", filepath.Dir(stubBin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	files, err := schema.LoadRegistry(context.Background(),
		[]string{"buf.build/stub/hello"}, nil)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	desc, err := files.FindDescriptorByName("stub.v1.Hello")
	if err != nil {
		t.Fatalf("FindDescriptorByName: %v", err)
	}
	if _, ok := desc.(protoreflect.MessageDescriptor); !ok {
		t.Fatalf("stub.v1.Hello is not a MessageDescriptor")
	}
}

func TestLoadRegistryBufModuleNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess PATH override is not portable to windows in this test")
	}
	stubBin := buildStubBuf(t)
	t.Setenv("PATH", filepath.Dir(stubBin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := schema.LoadRegistry(context.Background(),
		[]string{"buf.build/stub/missing"}, nil)
	if err == nil {
		t.Fatalf("expected error for unknown buf module, got nil")
	}
	if !strings.Contains(err.Error(), "module buf.build/stub/missing not found") {
		t.Errorf("error does not include buf's stderr: %v", err)
	}
}

// buildStubBuf compiles ./testdata/stubcmd/stub-buf into a temp
// directory as a binary named "buf" so PATH-based dispatch picks it up
// instead of any real buf on the developer's machine.
func buildStubBuf(t *testing.T) string {
	t.Helper()
	// schema_test.go runs from .../protobuf/schema/; the stub source
	// lives at the repo root under testdata/stubcmd/stub-buf/.
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs ../..: %v", err)
	}
	stubSrc := filepath.Join(repoRoot, "testdata", "stubcmd", "stub-buf")
	if _, err := os.Stat(stubSrc); err != nil {
		t.Fatalf("locate stub-buf source: %v", err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "buf")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = stubSrc
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build stub-buf: %v", err)
	}
	return bin
}

func TestNewTypeResolver(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "foo.proto", fooProto)
	files, err := schema.LoadRegistry(context.Background(), []string{p}, nil)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}

	resolver := schema.NewTypeResolver(files)
	mt, err := resolver.FindMessageByName("foo.v1.Foo")
	if err != nil {
		t.Fatalf("FindMessageByName: %v", err)
	}
	if got := string(mt.Descriptor().FullName()); got != "foo.v1.Foo" {
		t.Errorf("FullName = %q, want foo.v1.Foo", got)
	}
}
