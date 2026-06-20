package main

import (
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"

	// Side-effect import: registers text/markdown (and friends) so
	// stripes.Detect/Func can route by extension.
	_ "github.com/firetiger-oss/stripes/all"
)

// TestRenderOneWeakTextPlainHint covers servers (e.g. raw.githubusercontent.com)
// that serve every file as "text/plain; charset=utf-8". A bare text/plain hint
// must not beat a more specific filename/content detection, so a .md URL still
// renders as Markdown while a genuine .txt stays plain.
func TestRenderOneWeakTextPlainHint(t *testing.T) {
	const body = "# Title\n\nsome text\n"

	cases := []struct {
		name        string
		file        string
		hint        string
		wantHeading bool // true: rendered as markdown ("# " stripped)
	}{
		{"markdown over text/plain", "README.md", "text/plain; charset=utf-8", true},
		{"markdown over bare text/plain", "README.md", "text/plain", true},
		{"plain stays plain", "notes.txt", "text/plain; charset=utf-8", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			cfg := &config{format: "auto", color: "never"}
			renderOne(&sb, tc.file, tc.hint, strings.NewReader(body), cfg, stripes.DefaultStyles)
			out := sb.String()

			// The Markdown renderer strips the leading "# " from a heading;
			// the plain renderer emits it verbatim.
			gotMarkdown := !strings.Contains(out, "# Title")
			if gotMarkdown != tc.wantHeading {
				t.Errorf("file %q hint %q: rendered as markdown=%v, want markdown=%v\noutput:\n%s",
					tc.file, tc.hint, gotMarkdown, tc.wantHeading, out)
			}
		})
	}
}
