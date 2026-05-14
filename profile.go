package stripes

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"
)

//go:embed assets/profiles/*.yaml
var bundledProfiles embed.FS

const bundledProfileDir = "assets/profiles"

// ErrProfileNotFound is returned by [LoadProfile] when no profile with the
// given name exists in either the user config dir or the bundled set.
var ErrProfileNotFound = errors.New("profile not found")

// Profile is the on-disk schema for a stripes color profile.
type Profile struct {
	Description string        `yaml:"description,omitempty"`
	CodeStyle   string        `yaml:"code-style,omitempty"`
	Styles      ProfileStyles `yaml:"styles"`
	Border      string        `yaml:"border,omitempty"`
	Indent      string        `yaml:"indent,omitempty"`
}

// ProfileStyles is the per-element style table within a [Profile].
type ProfileStyles struct {
	Name       StyleSpec   `yaml:"name"`
	Text       StyleSpec   `yaml:"text"`
	String     StyleSpec   `yaml:"string"`
	Number     StyleSpec   `yaml:"number"`
	Boolean    StyleSpec   `yaml:"boolean"`
	Null       StyleSpec   `yaml:"null"`
	Syntax     StyleSpec   `yaml:"syntax"`
	Code       StyleSpec   `yaml:"code"`
	Anchor     StyleSpec   `yaml:"anchor"`
	Comment    StyleSpec   `yaml:"comment"`
	Title      StyleSpec   `yaml:"title"`
	LineNumber StyleSpec   `yaml:"line-number"`
	Columns    StyleSpec   `yaml:"columns"`
	Rows       StyleSpec   `yaml:"rows"`
	Headings   []StyleSpec `yaml:"headings"`
}

// StyleSpec describes one styled element. Color may be a hex value
// (#rrggbb), an ANSI palette index ("0".."255"), or any color name
// recognised by lipgloss.
type StyleSpec struct {
	Color      string `yaml:"color,omitempty"`
	Background string `yaml:"background,omitempty"`
	Bold       bool   `yaml:"bold,omitempty"`
	Faint      bool   `yaml:"faint,omitempty"`
	Italic     bool   `yaml:"italic,omitempty"`
	Underline  bool   `yaml:"underline,omitempty"`
}

// LoadProfile resolves a profile reference. The reference may be:
//
//   - A bare name (e.g. "iterm-default") — searched first in
//     $XDG_CONFIG_HOME/stripes/profiles (defaulting to
//     ~/.config/stripes/profiles), then in the bundled set.
//   - A file path (anything containing a separator, starting with ~,
//     absolute, or ending in .yaml/.yml) — loaded directly. Tilde
//     expansion is applied.
//
// [ErrProfileNotFound] wraps the underlying error when the reference
// resolves to nothing.
func LoadProfile(ref string) (*Profile, error) {
	if ref == "" {
		return nil, errors.New("profile name is empty")
	}
	if looksLikeProfilePath(ref) {
		return loadProfileFromFile(expandTilde(ref))
	}

	if dir, err := userProfileDir(); err == nil {
		path := filepath.Join(dir, ref+".yaml")
		if data, err := os.ReadFile(path); err == nil {
			return parseProfile(data, path)
		}
	}

	data, err := bundledProfiles.ReadFile(bundledProfileDir + "/" + ref + ".yaml")
	if err == nil {
		return parseProfile(data, "<bundled "+ref+".yaml>")
	}
	return nil, fmt.Errorf("%w: %q", ErrProfileNotFound, ref)
}

func looksLikeProfilePath(s string) bool {
	if strings.HasPrefix(s, "~") || filepath.IsAbs(s) {
		return true
	}
	if strings.ContainsRune(s, '/') || strings.ContainsRune(s, filepath.Separator) {
		return true
	}
	if strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml") {
		return true
	}
	return false
}

func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func loadProfileFromFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", ErrProfileNotFound, path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseProfile(data, path)
}

// ListProfiles returns the sorted union of profile names available in the
// user config directory and the bundled set.
func ListProfiles() []string {
	seen := map[string]struct{}{}
	add := func(filename string) {
		name, ok := strings.CutSuffix(filename, ".yaml")
		if !ok {
			return
		}
		seen[name] = struct{}{}
	}

	if dir, err := userProfileDir(); err == nil {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					add(e.Name())
				}
			}
		}
	}

	_ = fs.WalkDir(bundledProfiles, bundledProfileDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		add(d.Name())
		return nil
	})

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ToStyles converts a [Profile] into a runtime [*Styles]. Width is left at
// 0; callers are expected to populate it from terminal/CLI context.
func (p *Profile) ToStyles() *Styles {
	s := &Styles{
		CodeStyle: p.CodeStyle,
		Indent:    p.Indent,
		Border:    parseBorder(p.Border),
	}
	if s.Indent == "" {
		s.Indent = "  "
	}

	s.Name = applyStyleSpec(p.Styles.Name)
	s.Text = applyStyleSpec(p.Styles.Text)
	s.String = applyStyleSpec(p.Styles.String)
	s.Number = applyStyleSpec(p.Styles.Number)
	s.Boolean = applyStyleSpec(p.Styles.Boolean)
	s.Null = applyStyleSpec(p.Styles.Null)
	s.Syntax = applyStyleSpec(p.Styles.Syntax)
	s.Code = applyStyleSpec(p.Styles.Code)
	s.Anchor = applyStyleSpec(p.Styles.Anchor)
	s.Comment = applyStyleSpec(p.Styles.Comment)
	s.Title = applyStyleSpec(p.Styles.Title)
	if p.Styles.LineNumber == (StyleSpec{}) {
		s.LineNumber = DefaultStyles.LineNumber
	} else {
		s.LineNumber = applyStyleSpec(p.Styles.LineNumber)
	}
	s.Columns = applyStyleSpec(p.Styles.Columns)
	s.Rows = applyStyleSpec(p.Styles.Rows)

	for i := range s.Heading {
		if i < len(p.Styles.Headings) {
			s.Heading[i] = applyStyleSpec(p.Styles.Headings[i])
		} else {
			s.Heading[i] = lipgloss.NewStyle()
		}
	}
	return s
}

func parseProfile(data []byte, src string) (*Profile, error) {
	p := &Profile{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	return p, nil
}

func userProfileDir() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "stripes", "profiles"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "stripes", "profiles"), nil
}

func applyStyleSpec(spec StyleSpec) lipgloss.Style {
	style := lipgloss.NewStyle()
	if spec.Color != "" {
		style = style.Foreground(lipgloss.Color(spec.Color))
	}
	if spec.Background != "" {
		style = style.Background(lipgloss.Color(spec.Background))
	}
	if spec.Bold {
		style = style.Bold(true)
	}
	if spec.Faint {
		style = style.Faint(true)
	}
	if spec.Italic {
		style = style.Italic(true)
	}
	if spec.Underline {
		style = style.Underline(true)
	}
	return style
}

func parseBorder(name string) lipgloss.Border {
	switch strings.ToLower(name) {
	case "rounded":
		return lipgloss.RoundedBorder()
	case "thick":
		return lipgloss.ThickBorder()
	case "double":
		return lipgloss.DoubleBorder()
	case "hidden":
		return lipgloss.HiddenBorder()
	default:
		return lipgloss.NormalBorder()
	}
}
