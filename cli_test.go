package stripes_test

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var update = flag.Bool("update", false, "update golden files inside testscript archives")

const (
	testBasicUser  = "alice"
	testBasicPass  = "s3cret"
	testBearerTok  = "t0p$ecret"
	testHTTPBody   = `{"ok": true}`
	testHTTPHeader = "application/json"
)

func TestMain(m *testing.M) {
	flag.Parse()

	binDir, err := os.MkdirTemp("", "stripes-bin-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(binDir)

	if err := buildBinary(binDir, "stripes", "./cmd/stripes"); err != nil {
		panic(err)
	}
	if err := buildBinary(binDir, "stub-pager", "./testdata/stubcmd/stub-pager"); err != nil {
		panic(err)
	}
	// stub-buf is built as "buf" so stripes finds it on PATH when
	// handling --registry buf.build/... entries; testscripts then see
	// the stub instead of a real buf install.
	if err := buildBinary(binDir, "buf", "./testdata/stubcmd/stub-buf"); err != nil {
		panic(err)
	}

	// Precomputed protobuf payload + the messageType the server advertises
	// via Content-Type, exercised by detect_protobuf_messagetype.txtar.
	// google.protobuf.StringValue is linked into the stripes binary via
	// the wrapperspb runtime, so the CLI can resolve the schema from the
	// MIME parameter alone — no --schema or --registry needed.
	pbMsg := wrapperspb.String("stripes")
	pbBody, err := proto.Marshal(pbMsg)
	if err != nil {
		panic(err)
	}
	pbType := `application/x-protobuf; messageType="` + string(pbMsg.ProtoReflect().Descriptor().FullName()) + `"`

	// OTLP TracesData payload to exercise the side-effect import in
	// stripes/protobuf/otlp: the CLI binary has the OTLP descriptors
	// pre-registered in protoregistry.GlobalTypes, so the messageType
	// header alone resolves the schema with no --registry.
	otlpMsg := &tracev1.TracesData{ResourceSpans: []*tracev1.ResourceSpans{{
		ScopeSpans: []*tracev1.ScopeSpans{{
			Spans: []*tracev1.Span{{
				Name: "GET /api",
				Attributes: []*commonv1.KeyValue{{
					Key:   "http.method",
					Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}},
				}},
			}},
		}},
	}}}
	otlpBody, err := proto.Marshal(otlpMsg)
	if err != nil {
		panic(err)
	}
	otlpType := `application/x-protobuf; messageType="` + string(otlpMsg.ProtoReflect().Descriptor().FullName()) + `"`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/basic/data.json":
			if user, pass, ok := r.BasicAuth(); !ok || user != testBasicUser || pass != testBasicPass {
				w.Header().Set("WWW-Authenticate", `Basic realm="stripes-test"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		case "/bearer/data.json":
			if r.Header.Get("Authorization") != "Bearer "+testBearerTok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		case "/proto/keyvalue":
			w.Header().Set("Content-Type", pbType)
			_, _ = w.Write(pbBody)
			return
		case "/proto/otlp":
			w.Header().Set("Content-Type", otlpType)
			_, _ = w.Write(otlpBody)
			return
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", testHTTPHeader)
		_, _ = w.Write([]byte(testHTTPBody))
	}))
	defer srv.Close()

	os.Setenv("STRIPES_TEST_BIN", binDir)
	os.Setenv("STRIPES_TEST_HTTP_BASIC", srv.URL+"/basic/data.json")
	os.Setenv("STRIPES_TEST_HTTP_BEARER", srv.URL+"/bearer/data.json")
	os.Setenv("STRIPES_TEST_HTTP_PROTOBUF", srv.URL+"/proto/keyvalue")
	os.Setenv("STRIPES_TEST_HTTP_OTLP", srv.URL+"/proto/otlp")
	os.Setenv("STRIPES_TEST_BASIC_USER", testBasicUser+":"+testBasicPass)
	os.Setenv("STRIPES_TEST_BEARER_TOKEN", testBearerTok)
	os.Exit(m.Run())
}

func buildBinary(dir, name, pkg string) error {
	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestCLI(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:           "testdata/script",
		UpdateScripts: *update,
		Setup: func(env *testscript.Env) error {
			binDir := os.Getenv("STRIPES_TEST_BIN")
			env.Setenv("PATH", binDir+string(os.PathListSeparator)+env.Getenv("PATH"))
			env.Setenv("NO_COLOR", "")
			env.Setenv("PAGER", "")
			env.Setenv("STRIPES_TEST_HTTP_BASIC", os.Getenv("STRIPES_TEST_HTTP_BASIC"))
			env.Setenv("STRIPES_TEST_HTTP_BEARER", os.Getenv("STRIPES_TEST_HTTP_BEARER"))
			env.Setenv("STRIPES_TEST_HTTP_PROTOBUF", os.Getenv("STRIPES_TEST_HTTP_PROTOBUF"))
			env.Setenv("STRIPES_TEST_HTTP_OTLP", os.Getenv("STRIPES_TEST_HTTP_OTLP"))
			env.Setenv("STRIPES_TEST_BASIC_USER", os.Getenv("STRIPES_TEST_BASIC_USER"))
			env.Setenv("STRIPES_TEST_BEARER_TOKEN", os.Getenv("STRIPES_TEST_BEARER_TOKEN"))
			return nil
		},
	})
}
