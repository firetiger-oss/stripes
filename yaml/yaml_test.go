package yaml

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRenderYAML(t *testing.T) {
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
			YAML(&output, reader, stripes.DefaultStyles)
			result := output.String()

			// Strip ANSI codes for byte-for-byte comparison
			stripped := ansi.Strip(result)
			if stripped != tt.output {
				t.Errorf("YAML() output mismatch\nInput: %s\nExpected:\n%s\nGot:\n%s\nActual (with ANSI):\n%s",
					tt.input, tt.output, stripped, result)
			}
		})
	}
}
