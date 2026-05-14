package stripes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderGoSum(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "single module both lines",
			input:  "example.com/foo v1.2.3 h1:abc=\nexample.com/foo v1.2.3/go.mod h1:def=",
			output: "example.com/foo v1.2.3 h1:abc=\nexample.com/foo v1.2.3/go.mod h1:def=",
		},
		{
			name: "multiple modules",
			input: `example.com/a v1.0.0 h1:aaa=
example.com/a v1.0.0/go.mod h1:bbb=
example.com/b v2.3.4 h1:ccc=
example.com/b v2.3.4/go.mod h1:ddd=`,
			output: `example.com/a v1.0.0 h1:aaa=
example.com/a v1.0.0/go.mod h1:bbb=
example.com/b v2.3.4 h1:ccc=
example.com/b v2.3.4/go.mod h1:ddd=`,
		},
		{
			name: "blank line between entries",
			input: `example.com/a v1.0.0 h1:aaa=

example.com/b v2.0.0 h1:bbb=`,
			output: `example.com/a v1.0.0 h1:aaa=

example.com/b v2.0.0 h1:bbb=`,
		},
		{
			name:   "pseudo-version",
			input:  `example.com/foo v0.0.0-20240101120000-abcdef123456 h1:xyz=`,
			output: `example.com/foo v0.0.0-20240101120000-abcdef123456 h1:xyz=`,
		},
		{
			name:   "malformed line falls back",
			input:  `just two tokens`,
			output: `just two tokens`,
		},
		{
			name:   "empty input",
			input:  ``,
			output: ``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			GoSum(&output, strings.NewReader(tt.input), DefaultStyles)
			stripped := ansi.Strip(output.String())
			if stripped != tt.output {
				t.Errorf("GoSum() output mismatch\nInput:\n%s\nExpected:\n%q\nGot:\n%q",
					tt.input, tt.output, stripped)
			}
		})
	}
}
