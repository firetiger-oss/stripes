package stripes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderGoWork(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "go version",
			input:  `go 1.22`,
			output: `go 1.22`,
		},
		{
			name: "single use",
			input: `go 1.22

use ./foo`,
			output: `go 1.22

use ./foo`,
		},
		{
			name: "use block",
			input: `go 1.22

use (
	./foo
	./bar
)`,
			output: `go 1.22

use (
	./foo
	./bar
)`,
		},
		{
			name:   "replace with arrow",
			input:  `replace example.com/foo => ../foo`,
			output: `replace example.com/foo => ../foo`,
		},
		{
			name:   "toolchain",
			input:  `toolchain go1.22.3`,
			output: `toolchain go1.22.3`,
		},
		{
			name:   "comment only",
			input:  `// workspace note`,
			output: `// workspace note`,
		},
		{
			name:   "empty input",
			input:  ``,
			output: ``,
		},
		{
			name: "malformed input falls through",
			input: `go 1.22
this is not valid go.work syntax @@@`,
			output: `go 1.22
this is not valid go.work syntax @@@`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			GoWork(&output, strings.NewReader(tt.input), DefaultStyles)
			stripped := ansi.Strip(output.String())
			if stripped != tt.output {
				t.Errorf("GoWork() output mismatch\nInput:\n%s\nExpected:\n%q\nGot:\n%q",
					tt.input, tt.output, stripped)
			}
		})
	}
}
