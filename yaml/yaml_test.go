package yaml

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	goyaml "gopkg.in/yaml.v3"
)

func TestRenderRender(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name: "simple mapping",
			input: `name: John
age: 30`,
			output: `name: John
age: 30`,
		},
		{
			name: "nested mapping",
			input: `user:
  name: John
  details:
    age: 30
    active: true`,
			output: `user: 
  name: John
  details: 
    age: 30
    active: true`,
		},
		{
			name: "array",
			input: `items:
  - apple
  - banana
  - cherry`,
			output: `items: 
  - apple
  - banana
  - cherry`,
		},
		{
			name: "quoted strings",
			input: `message: "Hello World"
path: 'C:\Windows'`,
			output: `message: Hello World
path: C:\Windows`,
		},
		{
			name: "mixed types",
			input: `name: John
age: 30
active: true
score: 95.5
tags: null`,
			output: `name: John
age: 30
active: true
score: 95.5
tags: null`,
		},
		{
			name: "with comments",
			input: `# This is a comment
name: John  # inline comment
age: 30`,
			output: `# This is a comment
name: John # inline comment
age: 30`,
		},
		{
			name: "folded scalar style",
			input: `description: >
  This is a long
  description that
  spans multiple lines`,
			output: `description: >
  This is a long description that spans multiple lines`,
		},
		{
			name: "literal scalar style",
			input: `script: |
  #!/bin/bash
  echo "Hello World"
  exit 0`,
			output: `script: |
  #!/bin/bash
  echo "Hello World"
  exit 0`,
		},
		{
			name: "nested literal scalar style",
			input: `service:
  firetiger-catalog-server:
    command: |
      run catalog server
      --http=:4327
      --storage-backend=http://localhost:4328`,
			output: `service: 
  firetiger-catalog-server: 
    command: |
      run catalog server
      --http=:4327
      --storage-backend=http://localhost:4328`,
		},
		{
			name: "plain scalars in sequence",
			input: `command:
  - --accesslog=true
  - --api.insecure=true
  - --log.level=INFO`,
			output: `command: 
  - --accesslog=true
  - --api.insecure=true
  - --log.level=INFO`,
		},
		{
			name: "complex command arguments",
			input: `command:
  - --providers.docker=true
  - --providers.docker.exposedbydefault=false
  - --entrypoints.web.address=:4317`,
			output: `command: 
  - --providers.docker=true
  - --providers.docker.exposedbydefault=false
  - --entrypoints.web.address=:4317`,
		},
		{
			name: "traefik config example",
			input: `command:
  - --accesslog=true
  - --api.insecure=true
  - --log.level=INFO
  - --providers.docker=true
  - --providers.docker.exposedbydefault=false
  - --providers.file.directory=/etc/traefik/dynamic
  - --entrypoints.web.address=:4317
  - --entrypoints.grafana.address=:8321
  - --ping=true`,
			output: `command: 
  - --accesslog=true
  - --api.insecure=true
  - --log.level=INFO
  - --providers.docker=true
  - --providers.docker.exposedbydefault=false
  - --providers.file.directory=/etc/traefik/dynamic
  - --entrypoints.web.address=:4317
  - --entrypoints.grafana.address=:8321
  - --ping=true`,
		},
		{
			name: "flow style inline arrays and objects",
			input: `inline_array: [1, 2, 3]
inline_object: {key: value, other: test}`,
			output: `inline_array: 
  - 1
  - 2
  - 3
inline_object: 
  key: value
  other: test`,
		},
		{
			name: "anchors and aliases",
			input: `default: &default
  name: service
  port: 8080
service1:
  <<: *default
  name: api
service2: *default`,
			output: `default: &default
  name: service
  port: 8080
service1: 
  <<: *default
  name: api
service2: *default`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			reader := strings.NewReader(tt.input)
			Render(&output, reader, stripes.DefaultStyles)
			result := output.String()

			// Strip ANSI codes for byte-for-byte comparison
			stripped := ansi.Strip(result)
			if stripped != tt.output {
				t.Errorf("Render() output mismatch\nInput: %s\nExpected:\n%s\nGot:\n%s\nActual (with ANSI):\n%s",
					tt.input, tt.output, stripped, result)
			}
		})
	}
}

const longDescription = "Runs the built-in consistency checks (`iceberg analyze`) and optionally repairs (`--fix`) — what each check covers, what it cannot detect, and the safe order to run a check + fix. Use before/after a risky change, when diagnosing a broken table, or as a periodic audit."

func renderYAML(t *testing.T, input string, width int) string {
	t.Helper()
	styles := stripes.DefaultStyles.Clone()
	styles.Width = width
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), styles)
	return ansi.Strip(buf.String())
}

func TestRenderFoldsLongMappingValue(t *testing.T) {
	input := "description: " + longDescription + "\n"
	got := renderYAML(t, input, 60)

	lines := strings.Split(got, "\n")
	if lines[0] != "description: >" {
		t.Fatalf("expected first line to be `description: >`, got %q\nFull output:\n%s", lines[0], got)
	}
	if len(lines) < 3 {
		t.Fatalf("expected multiple continuation lines, got %d:\n%s", len(lines), got)
	}
	for i, line := range lines[1:] {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("continuation line %d not 2-space indented: %q", i+1, line)
		}
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("continuation line %d exceeds width 60 (got %d): %q", i+1, w, line)
		}
	}
}

func TestRenderShortValueNotFolded(t *testing.T) {
	got := renderYAML(t, "name: rollout\n", 60)
	if got != "name: rollout" {
		t.Errorf("short value should render on one line\nwant: %q\ngot:  %q", "name: rollout", got)
	}
	if strings.Contains(got, ">") {
		t.Errorf("short value should not gain a > fold marker: %q", got)
	}
}

func TestRenderUnwrappedWhenWidthZero(t *testing.T) {
	// Width=0 disables wrapping — the long value stays on one line.
	got := renderYAML(t, "description: "+longDescription+"\n", 0)
	if strings.Contains(got, "\n") {
		t.Errorf("Width=0 must not wrap; got multi-line output:\n%s", got)
	}
	if strings.Contains(got, ">") {
		t.Errorf("Width=0 must not emit > fold marker: %q", got)
	}
}

func TestRenderFoldsLongSequenceItem(t *testing.T) {
	input := "items:\n  - " + longDescription + "\n"
	got := renderYAML(t, input, 60)

	lines := strings.Split(got, "\n")
	// First line is the map key.
	if lines[0] != "items: " {
		t.Fatalf("expected first line `items: `, got %q\nFull:\n%s", lines[0], got)
	}
	// Second line opens the sequence item with `- >`.
	if lines[1] != "  - >" {
		t.Fatalf("expected sequence-item fold marker `  - >`, got %q\nFull:\n%s", lines[1], got)
	}
	// Continuation lines align under the value at 4 spaces (2 for the
	// nested PrefixWriter indent + 2 for the fold's own continuation).
	for i, line := range lines[2:] {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			t.Errorf("sequence continuation line %d not 4-space indented: %q", i+2, line)
		}
		if w := ansi.StringWidth(line); w > 60 {
			t.Errorf("sequence continuation line %d exceeds width 60 (got %d): %q", i+2, w, line)
		}
	}
}

func TestRenderFoldedRoundTrips(t *testing.T) {
	// Safety guarantee: a long description wrapped with > folds back to
	// the original string when re-parsed as YAML. This is what motivated
	// choosing > over plain wrapping or |.
	got := renderYAML(t, "description: "+longDescription+"\n", 60)
	var round struct {
		Description string `yaml:"description"`
	}
	if err := goyaml.Unmarshal([]byte(got), &round); err != nil {
		t.Fatalf("rendered YAML failed to parse: %v\nOutput:\n%s", err, got)
	}
	if round.Description != longDescription {
		t.Errorf("round-trip mismatch\nwant: %q\ngot:  %q", longDescription, round.Description)
	}
}
