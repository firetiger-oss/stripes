package stripes_test

import (
	"io"
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/all"
)

func TestFunc(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
		schemaURL   string
		expectNil   bool
		expectType  string
	}{
		{
			name:        "JSON content type",
			contentType: "application/json",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "JSON",
		},
		{
			name:        "YAML content type",
			contentType: "application/yaml",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "YAML",
		},
		{
			name:        "XML content type",
			contentType: "application/xml",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "XML",
		},
		{
			name:        "HTML content type",
			contentType: "text/html",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "HTML",
		},
		{
			name:        "CSV content type",
			contentType: "text/csv",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "CSV",
		},
		{
			name:        "Plain text content type",
			contentType: "text/plain",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "Text",
		},
		{
			name:        "Dockerfile content type",
			contentType: "text/x-dockerfile",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "Dockerfile",
		},
		{
			name:        "go.mod content type",
			contentType: "text/x-go-mod",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "GoMod",
		},
		{
			name:        "go.sum content type",
			contentType: "text/x-go-sum",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "GoSum",
		},
		{
			name:        "go.work content type",
			contentType: "text/x-go-work",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "GoWork",
		},
		{
			name:        "vendor/modules.txt content type",
			contentType: "text/x-go-vendor-modules",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "GoVendorModules",
		},
		{
			name:        "Generic text content type",
			contentType: "text/markdown",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "Text",
		},
		{
			name:        "Protobuf without schema",
			contentType: "application/protobuf",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "Protobuf",
		},
		{
			name:        "Protobuf with schema",
			contentType: "application/protobuf",
			schemaURL:   "type.googleapis.com/google.protobuf.StringValue",
			expectNil:   false,
			expectType:  "Protobuf",
		},
		{
			name:        "Unknown binary content type",
			contentType: "application/octet-stream",
			schemaURL:   "",
			expectNil:   true,
		},
		{
			name:        "Unknown binary content type with charset",
			contentType: "application/pdf; charset=utf-8",
			schemaURL:   "",
			expectNil:   true,
		},
		{
			name:        "Image content type",
			contentType: "image/png",
			schemaURL:   "",
			expectNil:   true,
		},
		{
			name:        "JSON with charset parameter",
			contentType: "application/json; charset=utf-8",
			schemaURL:   "",
			expectNil:   false,
			expectType:  "JSON",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			renderFunc := stripes.Func(tc.contentType, tc.schemaURL)

			if tc.expectNil {
				if renderFunc != nil {
					t.Errorf("Expected Func to return nil for content type %q, but got a function", tc.contentType)
				}
			} else {
				if renderFunc == nil {
					t.Errorf("Expected Func to return a function for content type %q, but got nil", tc.contentType)
				}
				// We can't easily test the exact function type without more complex reflection,
				// but we can at least verify a function was returned
			}
		})
	}
}

// TestFuncXPrefixAlias verifies that the legacy application/x-FOO form
// resolves to a handler registered for application/FOO. The x- form is
// what plenty of tools still emit for protobuf payloads (e.g.
// application/x-protobuf), even though RFC 6648 deprecated the prefix.
func TestFuncXPrefixAlias(t *testing.T) {
	cases := []struct {
		contentType string
		wantNil     bool
	}{
		{"application/protobuf", false},
		{"application/x-protobuf", false},                              // alias
		{`application/x-protobuf; messageType="foo.Bar"`, false},       // alias + params
		{"application/json", false},                                    // direct hit unaffected
		{"application/x-still-not-registered", true},                   // alias still doesn't invent handlers
	}
	for _, tc := range cases {
		r := stripes.Func(tc.contentType, "")
		if (r == nil) != tc.wantNil {
			t.Errorf("Func(%q) nil=%v, wantNil=%v", tc.contentType, r == nil, tc.wantNil)
		}
	}
}

func TestFuncSuffixParam(t *testing.T) {
	var got map[string]string
	stripes.Register(stripes.Format{
		Name:        "_suffixparamtest",
		ContentType: "application/x-suffixparamtest",
		RendererFor: func(params map[string]string, _ string) stripes.Renderer {
			got = params
			return func(io.Writer, io.Reader, *stripes.Styles) {}
		},
	})

	cases := []struct {
		contentType string
		wantSuffix  string
	}{
		{"application/x-suffixparamtest", ""},
		{"application/x-suffixparamtest+json", "json"},
		{"application/x-suffixparamtest+xml; charset=utf-8", "xml"},
	}
	for _, tc := range cases {
		got = nil
		if r := stripes.Func(tc.contentType, ""); r == nil {
			t.Fatalf("Func(%q) = nil", tc.contentType)
		}
		if got[stripes.SuffixParam] != tc.wantSuffix {
			t.Errorf("Func(%q): %s = %q, want %q", tc.contentType, stripes.SuffixParam, got[stripes.SuffixParam], tc.wantSuffix)
		}
	}
}

func TestFuncEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
		schemaURL   string
		expectNil   bool
	}{
		{
			name:        "Empty content type",
			contentType: "",
			schemaURL:   "",
			expectNil:   true,
		},
		{
			name:        "Invalid content type",
			contentType: "invalid/type/too/many/parts",
			schemaURL:   "",
			expectNil:   true,
		},
		{
			name:        "Content type with multiple parameters",
			contentType: "text/plain; charset=utf-8; boundary=something",
			schemaURL:   "",
			expectNil:   false, // Should still match text/plain
		},
		{
			name:        "Case sensitivity",
			contentType: "TEXT/PLAIN",
			schemaURL:   "",
			expectNil:   false, // mime.ParseMediaType should handle this
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			renderFunc := stripes.Func(tc.contentType, tc.schemaURL)

			if tc.expectNil {
				if renderFunc != nil {
					t.Errorf("Expected Func to return nil for content type %q, but got a function", tc.contentType)
				}
			} else {
				if renderFunc == nil {
					t.Errorf("Expected Func to return a function for content type %q, but got nil", tc.contentType)
				}
			}
		})
	}
}
