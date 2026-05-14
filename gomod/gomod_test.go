package gomod

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRenderRenderMod(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "module only",
			input:  `module example.com/foo`,
			output: `module example.com/foo`,
		},
		{
			name: "module plus go",
			input: `module example.com/foo

go 1.22`,
			output: `module example.com/foo

go 1.22`,
		},
		{
			name:   "single require",
			input:  `require example.com/bar v1.2.3`,
			output: `require example.com/bar v1.2.3`,
		},
		{
			name: "require block with indirect",
			input: `require (
	example.com/a v1.0.0
	example.com/b v0.0.0-20240101120000-abcdef123456 // indirect
)`,
			output: `require (
	example.com/a v1.0.0
	example.com/b v0.0.0-20240101120000-abcdef123456 // indirect
)`,
		},
		{
			name:   "replace with arrow",
			input:  `replace example.com/foo => example.com/fork v1.2.3`,
			output: `replace example.com/foo => example.com/fork v1.2.3`,
		},
		{
			name:   "replace with local path",
			input:  `replace example.com/foo => ../foo`,
			output: `replace example.com/foo => ../foo`,
		},
		{
			name:   "single retract version",
			input:  `retract v1.0.0`,
			output: `retract v1.0.0`,
		},
		{
			name:   "retract range",
			input:  `retract [v1.0.0, v1.1.0]`,
			output: `retract [v1.0.0, v1.1.0]`,
		},
		{
			name:   "exclude",
			input:  `exclude example.com/foo v1.2.3`,
			output: `exclude example.com/foo v1.2.3`,
		},
		{
			name:   "toolchain",
			input:  `toolchain go1.22.3`,
			output: `toolchain go1.22.3`,
		},
		{
			name:   "comment only",
			input:  `// top-level note`,
			output: `// top-level note`,
		},
		{
			name:   "pre-release version",
			input:  `require example.com/foo v1.2.3-rc1`,
			output: `require example.com/foo v1.2.3-rc1`,
		},
		{
			name: "blank line between sections",
			input: `module example.com/foo

go 1.22

require example.com/bar v1.0.0`,
			output: `module example.com/foo

go 1.22

require example.com/bar v1.0.0`,
		},
		{
			name:   "empty input",
			input:  ``,
			output: ``,
		},
		{
			name: "malformed input falls through",
			input: `module example.com/foo
this is not valid go.mod syntax @@@`,
			output: `module example.com/foo
this is not valid go.mod syntax @@@`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			RenderMod(&output, strings.NewReader(tt.input), stripes.DefaultStyles)
			stripped := ansi.Strip(output.String())
			if stripped != tt.output {
				t.Errorf("RenderMod() output mismatch\nInput:\n%s\nExpected:\n%q\nGot:\n%q",
					tt.input, tt.output, stripped)
			}
		})
	}
}
