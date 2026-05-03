# stripes [![Go Reference](https://pkg.go.dev/badge/github.com/firetiger-oss/stripes.svg)](https://pkg.go.dev/github.com/firetiger-oss/stripes)

A streaming, ANSI-colored pretty-printer for structured data — JSON, YAML,
XML, HTML, CSV, protobuf, and plain text — usable as a Go library or as a
standalone CLI.

## Install

```sh
# CLI
go install github.com/firetiger-oss/stripes/cmd/stripes@latest

# Library
go get github.com/firetiger-oss/stripes
```

## CLI

```
stripes [flags] [file]
```

If no file is given, `stripes` reads from stdin. When stdout is a terminal it
pipes the styled output through a pager (`less -R` by default); otherwise it
writes directly. Format is auto-detected from filename extension or content
sniffing.

### Flags

| Flag | Description |
|---|---|
| `-f`, `--format` | `json`, `yaml`, `xml`, `html`, `csv`, `text`, `protobuf`, `auto` |
| `--content-type` | Override MIME type (e.g. `application/vnd.foo+json`) |
| `--schema` | Schema URL (protobuf full message name) |
| `--color` | `always`, `never`, `auto` (default `auto`) |
| `-w`, `--width` | Output width (default: terminal width or 100) |
| `-p`, `--pager` | Pager command override. Use `cat` to bypass paging on a TTY. |

### Pager resolution

`--pager` flag → `$STRIPES_PAGER` → `$PAGER` → built-in default `less -R`.

The pager string is split on whitespace; no shell quoting is supported. To
pass arguments containing spaces, wrap your own script and point `-p` at it.

### Color

Auto-disabled when `NO_COLOR` is set or stdout is not a terminal.

### Shell snippets

```sh
# Quick aliases — don't clobber core tools
alias scat='stripes'              # cat-like, with paging on TTY
alias spcat='stripes -p cat'      # always-stream, never page

# Override pager
stripes -p 'less -R --tabs=2' foo.json

# Pipe-friendly (paging auto-disables when stdout isn't a TTY)
curl https://api.example.com/foo | stripes --color=always | head -20
```

## Library

All renderers share a common shape:

```go
import "github.com/firetiger-oss/stripes"

stripes.JSON(os.Stdout, r, stripes.DefaultStyles)
```

| Function | Input |
|---|---|
| `stripes.JSON(w, r, s)` | JSON bytes |
| `stripes.YAML(w, r, s)` | YAML bytes |
| `stripes.XML(w, r, s)` | XML bytes |
| `stripes.HTML(w, r, s)` | HTML bytes |
| `stripes.CSV(w, r, s)` | CSV bytes |
| `stripes.Text(w, r, s)` | Plain text |
| `stripes.Plain(w, r, s)` | Pass-through |
| `stripes.Protobuf(desc, types)(w, r, s)` | Protobuf wire format |

### Dispatch by MIME type

```go
fn := stripes.ObjectFunc("application/json", "")
fn(os.Stdout, r, stripes.DefaultStyles)
```

`ObjectFunc` returns `nil` if the MIME type is unsupported.

### Format detection

```go
ct := stripes.Detect("foo.yaml", peek)   // returns "application/yaml"
fn := stripes.ObjectFunc(ct, "")
```

`Detect` uses filename extension first, then sniffs leading bytes, then falls
back to `net/http.DetectContentType`. Returns `"text/plain"` if nothing else
matched.

### Custom styles

```go
styles := stripes.DefaultStyles.Clone()
styles.Width = 120
styles.String = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
```

Pass a zero-value `*stripes.Styles{}` for unstyled output.

## License

MIT
