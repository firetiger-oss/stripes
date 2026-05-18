package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "git format simple modification",
			input: `diff --git a/foo.txt b/foo.txt
index 0000001..0000002 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
 line one
-old line
+new line
 line three
`,
		},
		{
			name: "plain unified diff",
			input: `--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
 alpha
-bravo
+charlie
 delta
`,
		},
		{
			name: "new file",
			input: `diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..0123456
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+hello
+world
`,
		},
		{
			name: "deleted file",
			input: `diff --git a/gone.txt b/gone.txt
deleted file mode 100644
index 0123456..0000000
--- a/gone.txt
+++ /dev/null
@@ -1,2 +0,0 @@
-bye
-world
`,
		},
		{
			name: "rename",
			input: `diff --git a/old.txt b/new.txt
similarity index 95%
rename from old.txt
rename to new.txt
index 1234567..abcdefg 100644
--- a/old.txt
+++ b/new.txt
@@ -1 +1 @@
-foo
+bar
`,
		},
		{
			name: "multi-file diff",
			input: `diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-one
+ONE
diff --git a/b.txt b/b.txt
index 3333333..4444444 100644
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-two
+TWO
`,
		},
		{
			name: "no newline at end of file",
			input: `diff --git a/foo.txt b/foo.txt
index 1111111..2222222 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1 +1 @@
-one line
\ No newline at end of file
+one line
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			Render(&buf, strings.NewReader(tt.input), stripes.DefaultStyles)
			got := ansi.Strip(buf.String())
			if got != tt.input {
				t.Errorf("ANSI-stripped output should equal input verbatim\ninput:\n%s\ngot:\n%s", tt.input, got)
			}
		})
	}
}

func TestRenderAppliesStyles(t *testing.T) {
	input := `diff --git a/foo.txt b/foo.txt
index 0000001..0000002 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1,3 +1,3 @@
 ctx
-del
+add
 tail
`
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	out := buf.String()

	checks := []struct {
		label string
		want  string
	}{
		{"insertion", stripes.DefaultStyles.Insertion.Render("+add")},
		{"deletion", stripes.DefaultStyles.Deletion.Render("-del")},
		{"hunk header", stripes.DefaultStyles.Name.Render("@@ -1,3 +1,3 @@")},
		{"file header diff line", stripes.DefaultStyles.Title.Render("diff --git a/foo.txt b/foo.txt")},
		{"old file marker", stripes.DefaultStyles.Title.Render("--- a/foo.txt")},
		{"new file marker", stripes.DefaultStyles.Title.Render("+++ b/foo.txt")},
		{"context line", stripes.DefaultStyles.Text.Render(" ctx")},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: expected styled fragment %q in output\nfull output:\n%s", c.label, c.want, out)
		}
	}
}

func TestRenderNoEOLMarkerStyledAsComment(t *testing.T) {
	input := `--- a/foo.txt
+++ b/foo.txt
@@ -1 +1 @@
-one
\ No newline at end of file
+one
`
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	want := stripes.DefaultStyles.Comment.Render(`\ No newline at end of file`)
	if !strings.Contains(buf.String(), want) {
		t.Errorf("expected comment-styled NoEOL marker %q in output\noutput:\n%s", want, buf.String())
	}
}

func TestRenderBinaryFileHeader(t *testing.T) {
	input := `diff --git a/img.png b/img.png
index 1234567..abcdefg 100644
Binary files a/img.png and b/img.png differ
`
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	want := stripes.DefaultStyles.Title.Render("Binary files a/img.png and b/img.png differ")
	if !strings.Contains(buf.String(), want) {
		t.Errorf("expected title-styled binary marker in output\noutput:\n%s", buf.String())
	}
}

func TestRenderNonDiffFallsBackToPlain(t *testing.T) {
	input := "just some text\nthat is not a diff\n"
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	if buf.String() != input {
		t.Errorf("non-diff input should fall through verbatim\nwant:\n%s\ngot:\n%s", input, buf.String())
	}
}

func TestLooksLikeDiff(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"git format", "diff --git a/x b/x\nindex 1..2 100644\n--- a/x\n+++ b/x\n", true},
		{"unified diff", "--- a/x\n+++ b/x\n@@ -1 +1 @@\n", true},
		{"bare hunk", "@@ -1,3 +1,3 @@\n", true},
		{"unified with comma-less hunk", "--- a/x\n+++ b/x\n@@ -1 +1 @@\n", true},
		{"yaml", "name: foo\nage: 30\n", false},
		{"markdown", "# hello\n\nparagraph\n", false},
		{"empty", "", false},
		{"--- alone", "--- not a diff\nthis is some other format\n", false},
		{"leading blank lines", "\n\ndiff --git a/x b/x\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeDiff([]byte(c.input)); got != c.want {
				t.Errorf("looksLikeDiff(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

const benchInput = `diff --git a/foo.go b/foo.go
index 1111111..2222222 100644
--- a/foo.go
+++ b/foo.go
@@ -10,7 +10,7 @@ func compute(x int) int {
 	if x < 0 {
 		return 0
 	}
-	return x * 2
+	return x*2 + 1
 }

-func helper() {}
+func helper() int { return 0 }
diff --git a/bar.go b/bar.go
index 3333333..4444444 100644
--- a/bar.go
+++ b/bar.go
@@ -1,5 +1,6 @@
 package bar

 const (
-	max = 100
+	max     = 200
+	maxName = "big"
 )
`

func BenchmarkRender(b *testing.B) {
	b.ReportAllocs()
	r := strings.NewReader(benchInput)
	for b.Loop() {
		r.Reset(benchInput)
		Render(discard{}, r, stripes.DefaultStyles)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
