// Package schema loads protobuf descriptor sources (FileDescriptorSet
// binaries, .proto source files, and Buf Schema Registry module
// references) into a [*protoregistry.Files] usable by the
// stripes/protobuf renderer for descriptor lookups.
//
// Inputs are addressed by path, URI, or BSR module reference:
//   - "*.binpbset" / "*.protoset" / "*.pb": tigerblock storage handles
//     scheme dispatch (s3://, gs://, https://, file://, local paths).
//   - "*.proto": compiled in-process via
//     [github.com/bufbuild/protocompile]; imports resolve against the
//     supplied include roots ("protoc -I" model) and protocompile's
//     bundled well-known types as a last-resort fallback.
//   - "buf.build/<owner>/<module>[:ref]": shells out to the buf CLI
//     ("buf build <ref> -o -") to produce a FileDescriptorSet, then
//     loads it like any other descriptor set. Requires the buf CLI to
//     be installed and (for private modules) authenticated.
package schema

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/firetiger-oss/tigerblock/storage"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// LoadRegistry loads each entry in paths into a fresh
// [*protoregistry.Files] and returns it.
//
// Paths are dispatched by value shape:
//
//   - .binpbset, .protoset, .pb      → read as a FileDescriptorSet
//   - .proto                          → compiled via protocompile
//   - buf.build/<owner>/<module>[:ref] → fetched via the buf CLI
//
// includes are import-path roots for .proto resolution, following the
// "protoc -I" model. A .proto path given as a bare relative path is
// treated as an import path resolved against includes — so a tree of
// .proto files needs its root supplied only once. A .proto path given
// as an absolute path or URI is opened directly and its parent
// directory is added to includes. The two forms may be mixed.
//
// Paths and includes may be either local filesystem paths or URIs
// accepted by tigerblock storage. Errors are returned for missing or
// unparseable inputs, duplicate file registrations across inputs, and
// .proto imports that cannot be resolved within includes or
// protocompile's bundled well-known types.
func LoadRegistry(ctx context.Context, paths []string, includes []string) (*protoregistry.Files, error) {
	files := new(protoregistry.Files)

	var protoPaths []string
	for _, p := range paths {
		if isBufModuleRef(p) {
			if err := loadBufModule(ctx, p, files); err != nil {
				return nil, err
			}
			continue
		}
		switch ext := lowerExt(p); ext {
		case ".binpbset", ".protoset", ".pb":
			if err := loadDescriptorSet(ctx, p, files); err != nil {
				return nil, err
			}
		case ".proto":
			protoPaths = append(protoPaths, p)
		default:
			return nil, fmt.Errorf("schema: %s: unrecognized input (want a .binpbset/.protoset/.pb path, a .proto path, or a buf.build/<owner>/<module> reference)", p)
		}
	}

	if len(protoPaths) == 0 {
		return files, nil
	}

	compileNames, allIncludes := planProtoCompile(protoPaths, includes)
	resolver := protocompile.WithStandardImports(&uriResolver{ctx: ctx, includes: allIncludes})
	compiler := protocompile.Compiler{Resolver: resolver}
	compiled, err := compiler.Compile(ctx, compileNames...)
	if err != nil {
		return nil, fmt.Errorf("schema: compile .proto: %w", err)
	}
	for _, fd := range compiled {
		if err := registerFileClosure(files, fd); err != nil {
			return nil, fmt.Errorf("schema: register %s: %w", fd.Path(), err)
		}
	}
	return files, nil
}

// registerFileClosure registers fd and its transitive imports into
// files, skipping any file already present. protocompile.Compile
// returns only the explicitly named targets; their imported files must
// be registered too, or messages those imports declare (e.g. a nested
// type in another directory) are not name-resolvable.
func registerFileClosure(files *protoregistry.Files, fd protoreflect.FileDescriptor) error {
	if _, err := files.FindFileByPath(fd.Path()); err == nil {
		return nil
	}
	imports := fd.Imports()
	for i := 0; i < imports.Len(); i++ {
		if err := registerFileClosure(files, imports.Get(i).FileDescriptor); err != nil {
			return err
		}
	}
	return files.RegisterFile(fd)
}

// NewTypeResolver returns a [protoregistry.MessageTypeResolver] backed
// by files. Lookups produce dynamicpb-backed message types on demand,
// suitable for use as protobuf.Renderer's Any-payload resolver when the
// enclosing message tree comes from a loaded descriptor set.
func NewTypeResolver(files *protoregistry.Files) protoregistry.MessageTypeResolver {
	return &filesTypeResolver{files: files}
}

type filesTypeResolver struct {
	files *protoregistry.Files
}

func (r *filesTypeResolver) FindMessageByName(name protoreflect.FullName) (protoreflect.MessageType, error) {
	desc, err := r.files.FindDescriptorByName(name)
	if err != nil {
		return nil, err
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, protoregistry.NotFound
	}
	return dynamicpb.NewMessageType(md), nil
}

func (r *filesTypeResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	name := url
	if i := strings.LastIndex(url, "/"); i >= 0 {
		name = url[i+1:]
	}
	return r.FindMessageByName(protoreflect.FullName(name))
}

func loadDescriptorSet(ctx context.Context, p string, files *protoregistry.Files) error {
	data, err := readURI(ctx, p)
	if err != nil {
		return fmt.Errorf("schema: read %s: %w", p, err)
	}
	return loadDescriptorSetBytes(p, data, files)
}

// loadDescriptorSetBytes parses data as a FileDescriptorSet and
// registers each file into files. source is used only for error
// messages (a path, URI, or buf module reference). Files already
// present in files are skipped, so the same well-known types pulled in
// by overlapping inputs do not produce duplicate-registration errors.
func loadDescriptorSetBytes(source string, data []byte, files *protoregistry.Files) error {
	var fdset descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &fdset); err != nil {
		return fmt.Errorf("schema: parse %s as FileDescriptorSet: %w", source, err)
	}
	for _, fdp := range fdset.File {
		if _, err := files.FindFileByPath(fdp.GetName()); err == nil {
			continue
		}
		fd, err := protodesc.NewFile(fdp, files)
		if err != nil {
			return fmt.Errorf("schema: %s: build descriptor for %s: %w", source, fdp.GetName(), err)
		}
		if err := files.RegisterFile(fd); err != nil {
			return fmt.Errorf("schema: %s: register %s: %w", source, fd.Path(), err)
		}
	}
	return nil
}

// isBufModuleRef reports whether p looks like a Buf Schema Registry
// module reference (buf.build/<owner>/<module>[:ref]). Self-hosted BSR
// hostnames are not yet recognized; broaden the predicate if/when that
// matters.
func isBufModuleRef(p string) bool {
	return strings.HasPrefix(p, "buf.build/")
}

// loadBufModule shells out to the buf CLI to produce a FileDescriptorSet
// for a BSR module reference, then loads it into files like any other
// descriptor set source. The buf CLI handles BSR fetching, dependency
// resolution, caching (~/.cache/buf/), and auth — features we get for
// free by delegating instead of speaking the BSR API ourselves.
func loadBufModule(ctx context.Context, ref string, files *protoregistry.Files) error {
	data, err := bufBuild(ctx, ref)
	if err != nil {
		return err
	}
	return loadDescriptorSetBytes(ref, data, files)
}

// bufBuild invokes "buf build <ref> -o -" and returns the resulting
// FileDescriptorSet bytes. A missing buf binary produces an error
// pointing the user at the install docs; any other failure forwards
// buf's stderr verbatim so the user sees buf's own diagnostics.
func bufBuild(ctx context.Context, ref string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "buf", "build", ref, "-o", "-")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("schema: %s: buf CLI not found; install from https://buf.build/docs/installation", ref)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, fmt.Errorf("schema: buf build %s: %w", ref, err)
		}
		return nil, fmt.Errorf("schema: buf build %s: %s", ref, msg)
	}
	return stdout.Bytes(), nil
}

// planProtoCompile turns a list of .proto paths and explicit include
// roots into the (compileNames, includes) pair protocompile expects.
//
// When includes are present, a bare relative .proto path is used
// verbatim as a compile target: it names an import path resolved
// against the include roots ("protoc -I" semantics), and the compiled
// descriptor keeps that canonical path. Otherwise the path is split
// into a directory (added to includes) and a basename. User-supplied
// includes come first so they win resolution conflicts.
func planProtoCompile(protoPaths, userIncludes []string) (compileNames, includes []string) {
	seen := map[string]bool{}
	addInclude := func(inc string) {
		if !seen[inc] {
			seen[inc] = true
			includes = append(includes, inc)
		}
	}
	for _, inc := range userIncludes {
		addInclude(canonInclude(inc))
	}
	for _, p := range protoPaths {
		if len(userIncludes) > 0 && isImportRelative(p) {
			compileNames = append(compileNames, p)
			continue
		}
		dir, base := splitPath(p)
		addInclude(dir)
		compileNames = append(compileNames, base)
	}
	return compileNames, includes
}

// isImportRelative reports whether p is a bare relative path — neither a
// URI nor an absolute filesystem path — and so can be interpreted as a
// protoc-style import path resolved against include roots.
func isImportRelative(p string) bool {
	return !isURI(p) && !filepath.IsAbs(p)
}

// uriResolver implements protocompile.Resolver by searching a list of
// include roots (local paths or URIs) via tigerblock storage.
type uriResolver struct {
	ctx      context.Context
	includes []string
}

func (r *uriResolver) FindFileByPath(filename string) (protocompile.SearchResult, error) {
	for _, inc := range r.includes {
		data, err := readURI(r.ctx, joinPath(inc, filename))
		if err == nil {
			return protocompile.SearchResult{Source: bytes.NewReader(data)}, nil
		}
	}
	return protocompile.SearchResult{}, protoregistry.NotFound
}

func readURI(ctx context.Context, p string) ([]byte, error) {
	rc, _, err := storage.GetObject(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func lowerExt(p string) string {
	if isURI(p) {
		return strings.ToLower(path.Ext(p))
	}
	return strings.ToLower(filepath.Ext(p))
}

func isURI(p string) bool {
	return strings.Contains(p, "://")
}

func canonInclude(inc string) string {
	if isURI(inc) {
		if !strings.HasSuffix(inc, "/") {
			inc += "/"
		}
		return inc
	}
	return filepath.Clean(inc)
}

func splitPath(p string) (dir, base string) {
	if isURI(p) {
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			return p, ""
		}
		return p[:i+1], p[i+1:]
	}
	return filepath.Dir(p), filepath.Base(p)
}

func joinPath(base, rel string) string {
	if isURI(base) {
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		return base + rel
	}
	return filepath.Join(base, rel)
}
