package stripes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderDockerfile(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "simple from",
			input:  `FROM alpine:3.20`,
			output: `FROM alpine:3.20`,
		},
		{
			name:   "from with as",
			input:  `FROM golang:1.22 AS builder`,
			output: `FROM golang:1.22 AS builder`,
		},
		{
			name:   "run with flag",
			input:  `RUN apk add --no-cache curl`,
			output: `RUN apk add --no-cache curl`,
		},
		{
			name:   "comment only",
			input:  `# build stage`,
			output: `# build stage`,
		},
		{
			name: "parser directive then from",
			input: `# syntax=docker/dockerfile:1
FROM alpine`,
			output: `# syntax=docker/dockerfile:1
FROM alpine`,
		},
		{
			name:   "json-array cmd",
			input:  `CMD ["nginx", "-g", "daemon off;"]`,
			output: `CMD ["nginx", "-g", "daemon off;"]`,
		},
		{
			name: "multi-line run",
			input: `RUN apt-get update \
    && apt-get install -y curl`,
			output: `RUN apt-get update \
    && apt-get install -y curl`,
		},
		{
			name:   "expose port",
			input:  `EXPOSE 8080`,
			output: `EXPOSE 8080`,
		},
		{
			name:   "env kv",
			input:  `ENV PATH=/usr/local/bin:$PATH`,
			output: `ENV PATH=/usr/local/bin:$PATH`,
		},
		{
			name:   "copy with from flag",
			input:  `COPY --from=builder /src/app /app`,
			output: `COPY --from=builder /src/app /app`,
		},
		{
			name:   "lowercase keyword",
			input:  `from alpine`,
			output: `from alpine`,
		},
		{
			name: "blank line between stages",
			input: `FROM alpine AS base

FROM base AS final`,
			output: `FROM alpine AS base

FROM base AS final`,
		},
		{
			name:   "label with quoted value",
			input:  `LABEL maintainer="dev@example.com"`,
			output: `LABEL maintainer="dev@example.com"`,
		},
		{
			name: "heredoc",
			input: `RUN <<EOF
echo hello
echo world
EOF`,
			output: `RUN <<EOF
echo hello
echo world
EOF`,
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
			reader := strings.NewReader(tt.input)
			Dockerfile(&output, reader, DefaultStyles)
			result := output.String()

			stripped := ansi.Strip(result)
			if stripped != tt.output {
				t.Errorf("Dockerfile() output mismatch\nInput:\n%s\nExpected:\n%s\nGot:\n%s\nActual (with ANSI):\n%s",
					tt.input, tt.output, stripped, result)
			}
		})
	}
}
