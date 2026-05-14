package gomod

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRenderGoVendorModules(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name: "simple module with packages",
			input: `# example.com/foo v1.2.3
## explicit
example.com/foo
example.com/foo/sub`,
			output: `# example.com/foo v1.2.3
## explicit
example.com/foo
example.com/foo/sub`,
		},
		{
			name: "explicit with go version",
			input: `# example.com/foo v1.2.3
## explicit; go 1.21
example.com/foo`,
			output: `# example.com/foo v1.2.3
## explicit; go 1.21
example.com/foo`,
		},
		{
			name: "module with replacement",
			input: `# example.com/foo v1.2.3 => example.com/fork v0.0.0-20240101120000-abcdef
## explicit
example.com/foo`,
			output: `# example.com/foo v1.2.3 => example.com/fork v0.0.0-20240101120000-abcdef
## explicit
example.com/foo`,
		},
		{
			name: "module with local replacement",
			input: `# example.com/foo v1.2.3 => ../foo
## explicit
example.com/foo`,
			output: `# example.com/foo v1.2.3 => ../foo
## explicit
example.com/foo`,
		},
		{
			name: "multiple modules",
			input: `# example.com/a v1.0.0
## explicit
example.com/a
# example.com/b v2.0.0
example.com/b
example.com/b/sub`,
			output: `# example.com/a v1.0.0
## explicit
example.com/a
# example.com/b v2.0.0
example.com/b
example.com/b/sub`,
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
			GoVendorModules(&output, strings.NewReader(tt.input), stripes.DefaultStyles)
			stripped := ansi.Strip(output.String())
			if stripped != tt.output {
				t.Errorf("GoVendorModules() output mismatch\nInput:\n%s\nExpected:\n%q\nGot:\n%q",
					tt.input, tt.output, stripped)
			}
		})
	}
}
