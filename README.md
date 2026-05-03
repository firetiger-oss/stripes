# stripes [![CI](https://github.com/firetiger-oss/stripes/actions/workflows/ci.yml/badge.svg)](https://github.com/firetiger-oss/stripes/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/firetiger-oss/stripes.svg)](https://pkg.go.dev/github.com/firetiger-oss/stripes)

<p align="center">
  <img width="300" height="255" alt="stripes" src="stripes.png" />
</p>

Streaming pretty-printer for structured data formats — JSON, YAML, XML, HTML, CSV, Dockerfile, protobuf, plain text — usable as a Go library or as a standalone CLI.

## Motivation

Pretty-printers in the Unix toolchain are fragmented: `jq` for JSON, `yq` for
YAML, browsers for HTML, no canonical option for protobuf. They share neither
flags nor styling, which makes uniform terminal output hard to assemble inside
a single Go program emitting mixed structured payloads (logs, traces, debug
dumps, RPC responses).

`stripes` collapses that surface to a single library and CLI:

- Renderers stream — bytes are emitted as they arrive, no whole-input load. Works for `tail -f`, large objects, and HTTP response bodies.
- Format dispatch is by MIME type, so any program already carrying a content type can pick a renderer without parsing its own input.
- The same library powers the CLI; no separate process required when a Go program wants colored output.
- One binary, one set of flags, one styling model across all supported formats.

## Library

### [stripes.Func](https://pkg.go.dev/github.com/firetiger-oss/stripes#Func)

Pick a [`Renderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Renderer)
by MIME type. Returns `nil` if the content type is unsupported.

```go
import "github.com/firetiger-oss/stripes"

renderer := stripes.Func("application/json", "")
renderer(os.Stdout, body, stripes.DefaultStyles)
```

For `application/protobuf`, pass the message's full name as the second
argument so the dynamic descriptor lookup can resolve fields.

### [stripes.Detect](https://pkg.go.dev/github.com/firetiger-oss/stripes#Detect)

Resolve a content type from a filename and/or the leading bytes of a stream.

```go
buf, _ := bufio.NewReader(input).Peek(512)
ct := stripes.Detect("payload.yaml", buf)
renderer := stripes.Func(ct, "")
```

### Format functions

| Content type             | Function                                                                                                  |
|--------------------------|-----------------------------------------------------------------------------------------------------------|
| `application/json`       | [`JSON`](https://pkg.go.dev/github.com/firetiger-oss/stripes#JSON)                                        |
| `application/yaml`       | [`YAML`](https://pkg.go.dev/github.com/firetiger-oss/stripes#YAML)                                        |
| `application/xml`        | [`XML`](https://pkg.go.dev/github.com/firetiger-oss/stripes#XML)                                          |
| `text/html`              | [`HTML`](https://pkg.go.dev/github.com/firetiger-oss/stripes#HTML)                                        |
| `text/csv`               | [`CSV`](https://pkg.go.dev/github.com/firetiger-oss/stripes#CSV)                                          |
| `text/x-dockerfile`      | [`Dockerfile`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Dockerfile)                            |
| `text/plain`             | [`Text`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Text)                                        |
| `application/protobuf`   | [`Protobuf`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Protobuf)                                |
| (passthrough)            | [`Plain`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Plain)                                      |

All share the [`Renderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Renderer)
signature: `func(io.Writer, io.Reader, *Styles)`.

### [stripes.Styles](https://pkg.go.dev/github.com/firetiger-oss/stripes#Styles)

Pass [`stripes.DefaultStyles`](https://pkg.go.dev/github.com/firetiger-oss/stripes#DefaultStyles)
for the built-in grayscale theme, a `Clone()` to customize, or `&stripes.Styles{}`
for unstyled output.

## CLI

```
go install github.com/firetiger-oss/stripes/cmd/stripes@latest
```

```
$ stripes --help
Usage: stripes [flags] [file]

Pretty-print structured data (JSON, YAML, XML, HTML, CSV, Dockerfile, protobuf,
text) with ANSI colors and optional paging.

Flags:
  -f, --format string         json|yaml|xml|html|csv|dockerfile|text|protobuf|auto (default auto)
      --content-type string   Override MIME type (e.g. application/vnd.foo+json)
      --schema string         Schema URL (protobuf full name)
      --color string          always|never|auto (default auto)
  -w, --width int             Output width (default: terminal width or 100)
  -p, --pager string          Pager command (e.g. "less -R", "bat --plain").
                              Use "cat" to bypass paging on a TTY.

Pager resolution: -p flag > $STRIPES_PAGER > $PAGER > "less -R"
Color is auto-disabled when NO_COLOR is set or stdout is not a terminal.
```

### Shell aliases

```sh
alias scat='stripes'              # cat-like, with paging on a TTY
alias spcat='stripes -p cat'      # always-stream, never page
```

## Contributing

Contributions are welcome! To get started:

1. Ensure you have Go 1.25+ installed
2. Run `go test ./...` to verify tests pass

Please report bugs and feature requests via [GitHub Issues](https://github.com/firetiger-oss/stripes/issues).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
