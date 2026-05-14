package stripes

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"charm.land/lipgloss/v2"
)

// renderProbe renders a fixed token through a lipgloss.Style. Two styles
// that produce identical output for every probe input are equivalent for
// the purposes of this package, even if their internal representation
// differs.
func renderProbe(s lipgloss.Style) string {
	return s.Render("x")
}

func assertStylesEquivalent(t *testing.T, got, want *Styles) {
	t.Helper()

	pairs := []struct {
		name string
		g, w lipgloss.Style
	}{
		{"Name", got.Name, want.Name},
		{"Text", got.Text, want.Text},
		{"String", got.String, want.String},
		{"Number", got.Number, want.Number},
		{"Boolean", got.Boolean, want.Boolean},
		{"Null", got.Null, want.Null},
		{"Syntax", got.Syntax, want.Syntax},
		{"Code", got.Code, want.Code},
		{"Anchor", got.Anchor, want.Anchor},
		{"Comment", got.Comment, want.Comment},
		{"Title", got.Title, want.Title},
		{"Columns", got.Columns, want.Columns},
		{"Rows", got.Rows, want.Rows},
	}
	for _, p := range pairs {
		if renderProbe(p.g) != renderProbe(p.w) {
			t.Errorf("%s differs: got=%q want=%q", p.name, renderProbe(p.g), renderProbe(p.w))
		}
	}
	for i := range got.Heading {
		if renderProbe(got.Heading[i]) != renderProbe(want.Heading[i]) {
			t.Errorf("Heading[%d] differs: got=%q want=%q", i, renderProbe(got.Heading[i]), renderProbe(want.Heading[i]))
		}
	}
	if got.Indent != want.Indent {
		t.Errorf("Indent differs: got=%q want=%q", got.Indent, want.Indent)
	}
	if !reflect.DeepEqual(got.Border, want.Border) {
		t.Errorf("Border differs:\n got=%+v\nwant=%+v", got.Border, want.Border)
	}
}

// TestBundledDefaultMatchesDefaultStyles guards against drift between the
// Go literal stripes.DefaultStyles and the bundled assets/profiles/default.yaml.
// They must produce identical visual output.
func TestBundledDefaultMatchesDefaultStyles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // make sure no real user config interferes

	prof, err := LoadProfile("default")
	if err != nil {
		t.Fatalf("LoadProfile(default): %v", err)
	}
	got := prof.ToStyles()
	assertStylesEquivalent(t, got, DefaultStyles)
}

func TestAllBundledProfilesLoad(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, name := range ListProfiles() {
		t.Run(name, func(t *testing.T) {
			prof, err := LoadProfile(name)
			if err != nil {
				t.Fatalf("LoadProfile(%q): %v", name, err)
			}
			s := prof.ToStyles()
			if s == nil {
				t.Fatal("ToStyles returned nil")
			}
			if s.Indent == "" {
				t.Errorf("Indent is empty after ToStyles")
			}
			// If the profile names a chroma code-style, that style must
			// actually exist in the bundled chroma styles.
			if prof.CodeStyle != "" {
				if got := chromastyles.Get(prof.CodeStyle); got == nil || got == chromastyles.Fallback {
					t.Errorf("code-style %q is not a known chroma style", prof.CodeStyle)
				}
			}
		})
	}
}

func TestLoadProfileNotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_, err := LoadProfile("does-not-exist-zzz")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("want ErrProfileNotFound, got %v", err)
	}
}

func TestLoadProfileEmptyName(t *testing.T) {
	if _, err := LoadProfile(""); err == nil {
		t.Fatal("want error for empty name")
	}
}

func TestLoadProfileFromFilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	const body = `description: from-file
code-style: monokai
styles:
  name: {color: "#abcdef"}
  text: {}
  string: {}
  number: {}
  boolean: {}
  "null": {}
  syntax: {}
  code: {}
  anchor: {}
  comment: {}
  title: {}
  columns: {}
  rows: {}
  headings: []
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Absolute path.
	prof, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile(absolute): %v", err)
	}
	if prof.CodeStyle != "monokai" {
		t.Errorf("absolute load: code-style=%q want=monokai", prof.CodeStyle)
	}

	// Relative path with separator.
	rel := filepath.Join(filepath.Base(dir), "custom.yaml")
	saved, _ := os.Getwd()
	if err := os.Chdir(filepath.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(saved) })
	if _, err := LoadProfile(rel); err != nil {
		t.Errorf("LoadProfile(relative %q): %v", rel, err)
	}

	// Path with .yaml extension but no separator (current-dir relative)
	// — should be treated as a path because of the suffix.
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile("custom.yaml"); err != nil {
		t.Errorf("LoadProfile(custom.yaml): %v", err)
	}
}

func TestLoadProfilePathNotFound(t *testing.T) {
	_, err := LoadProfile("/nope/missing.yaml")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("want ErrProfileNotFound, got %v", err)
	}
}

func TestLoadProfileTildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "tilde.yaml")
	if err := os.WriteFile(path, []byte("styles: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile("~/tilde.yaml"); err != nil {
		t.Errorf("LoadProfile(~/tilde.yaml): %v", err)
	}
}

func TestLooksLikeProfilePath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"default", false},
		{"iterm-default", false},
		{"foo.bar", false},
		{"foo.yaml", true},
		{"foo.yml", true},
		{"a/b", true},
		{"./foo", true},
		{"/abs/foo", true},
		{"~/foo.yaml", true},
		{"~/foo", true},
	}
	for _, c := range cases {
		if got := looksLikeProfilePath(c.in); got != c.want {
			t.Errorf("looksLikeProfilePath(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestUserProfileOverridesBundled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	profDir := filepath.Join(dir, "stripes", "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Override the bundled "default" with a profile that has a unique
	// code-style we can detect.
	const userYAML = `description: user override
code-style: dracula
styles:
  name: {color: "#ff00ff"}
  text: {}
  string: {}
  number: {}
  boolean: {}
  "null": {}
  syntax: {}
  code: {}
  anchor: {}
  comment: {}
  title: {}
  columns: {}
  rows: {}
  headings: []
border: rounded
indent: "    "
`
	if err := os.WriteFile(filepath.Join(profDir, "default.yaml"), []byte(userYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	prof, err := LoadProfile("default")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if prof.CodeStyle != "dracula" {
		t.Errorf("user override not respected: code-style=%q want=%q", prof.CodeStyle, "dracula")
	}
	if prof.Indent != "    " {
		t.Errorf("user override not respected: indent=%q want=%q", prof.Indent, "    ")
	}
}

func TestListProfilesMergesBundledAndUser(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	profDir := filepath.Join(dir, "stripes", "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "user-only.yaml"), []byte("styles: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ListProfiles()
	for _, want := range []string{"default", "iterm-default", "user-only"} {
		if !slices.Contains(got, want) {
			t.Errorf("ListProfiles missing %q (got %v)", want, got)
		}
	}
}

func TestLoadProfileMalformedYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	profDir := filepath.Join(dir, "stripes", "profiles")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "broken.yaml"), []byte("styles: {name: {color: [1,2"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProfile("broken")
	if err == nil {
		t.Fatal("want parse error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parse: %v", err)
	}
}

func TestProfileToStylesAttributes(t *testing.T) {
	p := &Profile{
		Styles: ProfileStyles{
			Title: StyleSpec{Color: "#abcdef", Bold: true, Faint: true, Italic: true, Underline: true},
		},
	}
	s := p.ToStyles()
	rendered := s.Title.Render("x")
	if rendered == "x" {
		t.Fatal("expected ANSI styling on Title; got plain text. Is lipgloss profile suppressed?")
	}
}

func TestParseBorderFallsBackToNormal(t *testing.T) {
	if !reflect.DeepEqual(parseBorder(""), lipgloss.NormalBorder()) {
		t.Error("empty border should be normal")
	}
	if !reflect.DeepEqual(parseBorder("garbage"), lipgloss.NormalBorder()) {
		t.Error("unknown border should fall back to normal")
	}
	if reflect.DeepEqual(parseBorder("rounded"), lipgloss.NormalBorder()) {
		t.Error("rounded border should differ from normal")
	}
}
