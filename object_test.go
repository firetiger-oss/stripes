package stripes

import (
	"testing"
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
			renderFunc := Func(tc.contentType, tc.schemaURL)

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
			renderFunc := Func(tc.contentType, tc.schemaURL)

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
