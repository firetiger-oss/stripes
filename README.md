# stripes [![CI](https://github.com/firetiger-oss/stripes/actions/workflows/ci.yml/badge.svg)](https://github.com/firetiger-oss/stripes/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/firetiger-oss/stripes.svg)](https://pkg.go.dev/github.com/firetiger-oss/stripes)

<p align="center">
  <img width="300" height="255" alt="stripes" src="stripes.png" />
</p>

Streaming pretty-printer for structured data formats — JSON, YAML, XML, HTML, CSV, Dockerfile, markdown, protobuf, parquet, plain text, source code (via [chroma](https://github.com/alecthomas/chroma)), txtar archives, WebAssembly — usable as a Go library or as a standalone CLI.

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

### Format sub-packages

Each format lives in its own sub-package that registers itself with the
root `stripes` package at init. Import the formats you need for their
side effects — this keeps your dependency graph free of the parsers you
don't use:

```go
import (
    "github.com/firetiger-oss/stripes"
    _ "github.com/firetiger-oss/stripes/json"
    _ "github.com/firetiger-oss/stripes/yaml"
)
```

Or import everything with the umbrella package:

```go
import _ "github.com/firetiger-oss/stripes/all"
```

| Content type                     | Sub-package  | Renderer(s)                                  |
|----------------------------------|--------------|----------------------------------------------|
| `application/json`               | `stripes/json`       | `Render`                             |
| `application/yaml`               | `stripes/yaml`       | `Render`                             |
| `application/xml`                | `stripes/xml`        | `Render`                             |
| `text/html`                      | `stripes/html`       | `Render`                             |
| `text/csv`                       | `stripes/csv`        | `Render`                             |
| `text/x-dockerfile`              | `stripes/dockerfile` | `Render`                             |
| `text/x-go-mod` etc.             | `stripes/gomod`      | `RenderMod`, `RenderSum`, `RenderWork`, `RenderVendorModules` |
| `text/markdown`                  | `stripes/markdown`   | `Render`                             |
| `text/x-source-code`             | `stripes/code`       | `New` (factory; pass chroma lexer name) |
| `application/wasm`               | `stripes/code`       | `RenderWasm` (requires `wasm-tools`, or `wasm2wat` from WABT as fallback) |
| `application/protobuf`           | `stripes/protobuf`   | `New` (factory; pass message descriptor) |
| `application/vnd.opentelemetry.trace` | `stripes/trace` | `Write` / `New` (structured); `NewRenderer` / `NewJSONRenderer` (byte stream) |
| `application/vnd.apache.parquet` | `stripes/parquet`    | `Render`                             |
| `text/x-txtar`                   | `stripes/txtar`      | `Render` (recursive per-file dispatch) |
| `text/plain`                     | `stripes` (root)     | `Text`, `Plain`                      |

The plain renderers share the
[`Renderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Renderer)
signature: `func(io.Writer, io.Reader, *stripes.Styles)`. The two `New`
factories (`stripes/code`, `stripes/protobuf`) take format-specific
parameters and return a `Renderer`.

`.wat`/`.wast` text-format WebAssembly is detected automatically and
routed through chroma's `wat` lexer. Binary `.wasm` rendering shells
out to `wasm-tools print` from
[wasm-tools](https://github.com/bytecodealliance/wasm-tools), which
tracks the current WebAssembly specification (component model, GC,
exception handling, …); install via `brew install wasm-tools` or
`cargo install wasm-tools`. When `wasm-tools` is not on `$PATH`,
rendering falls back to `wasm2wat` from
[WABT](https://github.com/WebAssembly/wabt) (`brew install wabt` or
`apt install wabt`), which may fail on modules using newer spec
features.

Terraform `.tf` and `.hcl` files are picked up by chroma's built-in
filename match; `.tfvars` is routed to the same `terraform` lexer.
`.tfstate` and `.tfstate.backup` are routed to the JSON renderer.

### [stripes.Func](https://pkg.go.dev/github.com/firetiger-oss/stripes#Func)

Pick a [`Renderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Renderer)
by MIME type. Returns `nil` if no imported sub-package handles the
content type.

```go
import (
    "github.com/firetiger-oss/stripes"
    _ "github.com/firetiger-oss/stripes/json"
)

renderer := stripes.Func("application/json", "")
renderer(os.Stdout, body, stripes.DefaultStyles)
```

For `application/protobuf`, pass the message's full name as the second
argument so the dynamic descriptor lookup can resolve fields.

### [stripes.Detect](https://pkg.go.dev/github.com/firetiger-oss/stripes#Detect)

Resolve a content type from a filename and/or the leading bytes of a
stream, using the filenames, extensions, magic bytes, and heuristics
registered by the imported sub-packages.

```go
buf, _ := bufio.NewReader(input).Peek(512)
ct := stripes.Detect("payload.yaml", buf)
renderer := stripes.Func(ct, "")
```

### [stripes.Register](https://pkg.go.dev/github.com/firetiger-oss/stripes#Register)

Third-party code can register additional formats by calling
`stripes.Register` with a [`Format`](https://pkg.go.dev/github.com/firetiger-oss/stripes#Format)
from an `init` function — the same mechanism the built-in sub-packages
use.

### [stripes.Styles](https://pkg.go.dev/github.com/firetiger-oss/stripes#Styles)

Pass [`stripes.DefaultStyles`](https://pkg.go.dev/github.com/firetiger-oss/stripes#DefaultStyles)
for the built-in grayscale theme, a `Clone()` to customize, or `&stripes.Styles{}`
for unstyled output.

## [stripes/trace](https://pkg.go.dev/github.com/firetiger-oss/stripes/trace)

Render OpenTelemetry trace data as a terminal waterfall. Each
`trace_id` becomes its own block: a top rule, a `Key: value`
metadata table (`Trace ID`, `Root`, `Duration`, `Spans`, `Errors`),
a bottom rule with embedded time-axis tick marks and labels, then
one row per span with `tree+kind+name · duration · bar`. The bar is
a coloured rectangle whose background is the service's hash-stable
hue, with the bold `service.namespace/service.name` label overlaid
at the left edge — so the bar simultaneously shows duration,
position, and ownership. Right-edge sub-cell precision uses
eighth-block glyphs; the left edge always snaps to a whole cell for
cross-terminal robustness. Span names and the tree connectors are
rendered in the terminal's default colour, keeping the focus on the
coloured bars.

```go
import (
    "iter"
    "os"

    "github.com/firetiger-oss/stripes"
    "github.com/firetiger-oss/stripes/trace"
    tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

func render(td *tracev1.TracesData) error {
    return trace.Write(os.Stdout, trace.FromTracesData(td),
        trace.WithStyles(stripes.DefaultStyles),
        trace.WithWidth(120),
        trace.WithVerbose(true), // expand attributes + events
    )
}
```

`Write` / `Format` are one-shot helpers; `New(opts...) *Formatter`
precomputes the options for hot loops. The structured API accepts
`iter.Seq[*tracev1.ResourceSpans]` — `FromTracesData`,
`FromResourceSpans`, `FromScopeSpans`, and `FromSpans(serviceName,
spans...)` adapt the common OTLP wrapper levels into that shape.

For MIME-routed byte-stream use,
[`NewRenderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes/trace#NewRenderer)
and
[`NewJSONRenderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes/trace#NewJSONRenderer)
mirror `protobuf.New` / `protobuf.NewJSON` — they accept a message
descriptor (TracesData / ResourceSpans / ScopeSpans / Span) and return
a `stripes.Renderer`. The CLI's `--format=trace` flag picks them
automatically; in `--format=auto`, a `--schema` that names an
OpenTelemetry trace message also routes here instead of the generic
protobuf text renderer.

## [stripes/log](https://pkg.go.dev/github.com/firetiger-oss/stripes/log)

Render log data — both OpenTelemetry log batches (binary or
protojson) and line-oriented text log formats — through one
shared rendering pipeline. Every record produces the same shape:

    yyyy/mm/dd hh:mm:ss.mmm LEVEL metadata message
      attr1      = value
      attr2.key1 = value

Single line when there are no attrs — the per-record vertical cost
is paid only when there's structured data to surface. Timestamps
are normalised to the host's local time (parsed from RFC 3339,
log4j/python comma-millisecond, Apache combined, BSD/journald,
etc.). The 4-character `LEVEL` column is coloured by class
(`TRAC`/`DEBU` cyan, `INFO` green, `WARN` yellow, `ERRO` red,
`FATA` purple); formats with no level concept (BSD syslog, macOS)
drop the column entirely. `metadata` is the format's per-line
visual identity (HTTP access logs land on a coloured "HTTP
status" pair with the status code class-tinted; log4j shows
`logger[thread]`; syslog shows `tag[pid]`; python shows
`logger:line`). Attributes are alphabetically sorted with keys
aligned on the `=`.

### OpenTelemetry logs

```go
import (
    "iter"
    "os"

    "github.com/firetiger-oss/stripes"
    "github.com/firetiger-oss/stripes/log"
    logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
)

func render(ld *logsv1.LogsData) error {
    return log.Write(os.Stdout, log.FromLogsData(ld),
        log.WithStyles(stripes.DefaultStyles),
        log.WithWidth(120),
        log.WithVerbose(true), // expand attributes + trace correlation
    )
}
```

`Write` / `Format` are one-shot helpers; `New(opts...) *Formatter`
precomputes the options for hot loops. The structured API accepts
`iter.Seq[*logsv1.ResourceLogs]` — `FromLogsData`,
`FromResourceLogs`, `FromScopeLogs`, and `FromLogRecords(serviceName,
records...)` adapt the common OTLP wrapper levels into that shape.
`-v` folds `trace_id`, `span_id`, and the instrumentation scope into
the same indented attrs block.

For MIME-routed byte-stream use,
[`NewRenderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes/log#NewRenderer)
and
[`NewJSONRenderer`](https://pkg.go.dev/github.com/firetiger-oss/stripes/log#NewJSONRenderer)
accept a message descriptor (LogsData / ResourceLogs / ScopeLogs /
LogRecord) and return a `stripes.Renderer`. The CLI's `--format=logs`
flag picks them automatically; in `--format=auto`, a `--schema` that
names an OpenTelemetry log message also routes here instead of the
generic protobuf text renderer.

### Text log formats

| Format            | CLI alias         | Detection                                          |
|-------------------|-------------------|---------------------------------------------------|
| logfmt            | `logfmt`          | ≥2 `key=value` tokens on the first line           |
| JSON-per-line     | `jsonlog`         | JSON object with a recognised time/level/msg key  |
| AWS ALB access    | `alb-access`      | `^(h2\|http\|https\|ws\|wss) <iso-timestamp>`     |
| NGINX/Apache combined | `nginx-access` | Apache common-log shape with bracketed date       |
| log4j / Kafka     | `log4j`           | Bracketed date OR plain date with `[thread]`/`- ` |
| Python `logging`  | `python-log`      | `YYYY-MM-DD HH:MM:SS,sss LEVEL logger:line msg`   |
| Go stdlib `log`   | `go-log`          | `YYYY/MM/DD HH:MM:SS …`                           |
| BSD syslog/journald/macOS | `syslog`  | `Mon DD HH:MM:SS hostname tag[pid]: …`            |
| RFC 5424 syslog   | `syslog-rfc5424`  | `<PRI>1 <iso-timestamp> hostname app procid …`    |

Detection is content-based (no format claims `.log`), so dropping
a `.log` file with no flag uses the first matching `Detect`. The
classifiers are registered most-specific first; logfmt and
jsonlog register last because their predicates are the most
permissive.

**Note on JSONL auto-detection:** the Go runtime initializes
sub-packages in alphabetical order, so the `json` renderer's
`Detect` (which requires a parseable first JSON value) is
consulted before `jsonlog`'s. JSON Lines payloads detect as plain
JSON unless you pass `--format=jsonlog` explicitly.

Adding a new text log format is one file: define a `LineFormat`
value and call [`log.Register`](https://pkg.go.dev/github.com/firetiger-oss/stripes/log#Register)
from `init()`. The registry wires it into both the byte-stream
renderer and `stripes.Detect`. The shipped formats register in a
deterministic priority order from the package's own `init`;
externally-added formats run after the built-ins, so write a
`Detect` predicate strict enough to avoid false positives.

## [stripes/table](https://pkg.go.dev/github.com/firetiger-oss/stripes/table)

Render typed iterators of struct values as styled CLI tables. Columns are
derived by reflection from exported fields: headers come from field names
(or a `table:"NAME"` tag), cell formatters from field types
(`time.Time`/`time.Duration` get dedicated formats, numerics are
right-aligned). Tag modifiers like `table:",bytes"`, `table:",count"`, and
`table:",0-100%"` pin specific formatters and suffixes.

```go
import (
    "iter"
    "os"
    "time"

    "github.com/firetiger-oss/stripes/table"
)

type Pod struct {
    Name     string
    Status   string
    Restarts int
    Memory   int64 `table:"MEM,bytes"`
    Age      time.Time
}

func render(seq iter.Seq2[Pod, error]) error {
    return table.Write(os.Stdout, seq, table.WithNow(time.Now))
}
```

`Write` / `Format` are one-shot helpers; `NewWriter[T]` / `NewFormatter[T]`
precompute the schema and are appropriate for hot loops. For non-struct
rows (`[]string`, `[]any`, …) pass [`WithColumns`](https://pkg.go.dev/github.com/firetiger-oss/stripes/table#WithColumns)
or [`WithHeaders`](https://pkg.go.dev/github.com/firetiger-oss/stripes/table#WithHeaders).
Borders, viewports/scrollbars, row selectors, and per-cell or per-row
style callbacks are available via the
[`Option`](https://pkg.go.dev/github.com/firetiger-oss/stripes/table#Option)
constructors.

## [stripes/cobra](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra)

Drop-in styled help, usage, and error output for CLIs built with
[`spf13/cobra`](https://github.com/spf13/cobra). The palette is sourced
from [`stripes.DefaultStyles`](https://pkg.go.dev/github.com/firetiger-oss/stripes#DefaultStyles)
so help text matches the rest of the project's output. ANSI is downgraded
or stripped automatically when stdout/stderr is not a terminal.

```go
import (
    "context"
    "errors"
    "os"

    "github.com/spf13/cobra"

    stripescobra "github.com/firetiger-oss/stripes/cobra"
)

func main() {
    root := &cobra.Command{
        Use:   "mytool",
        Short: "A demo CLI",
    }
    root.PersistentFlags().StringP("config", "c", "/etc/mytool.cfg", "config `file` path")

    root.AddCommand(&cobra.Command{
        Use:   "serve",
        Short: "Start the server",
        RunE: func(*cobra.Command, []string) error {
            return errors.New("not implemented")
        },
    })

    if err := stripescobra.Execute(context.Background(), root); err != nil {
        os.Exit(1)
    }
}
```

[`Execute`](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra#Execute)
installs styled help/usage/error rendering on `root` and every subcommand
before calling `root.ExecuteContext`. Use
[`Apply`](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra#Apply)
to install the renderers without running the command. The palette,
output writers, and error handler are overridable via
[`WithStyles`](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra#WithStyles),
[`WithOutput`](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra#WithOutput),
[`WithErrorOutput`](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra#WithErrorOutput),
and [`WithErrorHandler`](https://pkg.go.dev/github.com/firetiger-oss/stripes/cobra#WithErrorHandler).

## CLI

```
go install github.com/firetiger-oss/stripes/cmd/stripes@latest
```

```
$ stripes --help
Usage: stripes [flags] [file...]

Pretty-print structured data (JSON, YAML, XML, HTML, CSV, Dockerfile, markdown,
protobuf, parquet, text, source code, txtar, wasm) with ANSI colors and optional paging.

When multiple files are given, each is preceded by a centered rule
(───── filename ─────) so the source is visible inline. --format,
--content-type, and --schema apply to all of them.

Flags:
  -f, --format string         json|yaml|xml|html|csv|dockerfile|markdown|text|code|protobuf|trace|parquet|txtar|wasm|table|auto (default auto)
                              "table" routes CSV/TSV/JSONL/parquet through the
                              new typed-table renderer with width-fitting,
                              JSON-cell colorization, and (for parquet) schema-
                              driven column formatting.
      --content-type string   Override MIME type (e.g. application/vnd.foo+json)
      --schema string         Schema URL (protobuf full name)
      --color string          always|never|auto (default auto)
      --paging string         always|never|auto (default auto). In "auto",
                              the pager is spawned only when the rendered
                              output is wider or taller than the terminal,
                              or when more than one file is rendered.
      --profile string        Color profile name or path. Bare names resolve
                              against $XDG_CONFIG_HOME/stripes/profiles
                              (~/.config/stripes/profiles) and the built-in
                              set. A value containing "/" or ending in
                              .yaml/.yml is loaded as a file directly.
  -w, --width int             Output width in columns. 0 (default) =
                              auto-detect from the terminal; falls back
                              to no wrap when stdout is not a TTY.
  -p, --pager string          Pager command (e.g. "less -R", "bat --plain").
                              Use --paging=never to bypass paging.
  -n, --line-numbers          Show line numbers in a left-aligned gutter.
  -v, --verbose               Expand per-row detail (currently used by
                              the trace format to reveal attributes,
                              events, and status messages under each
                              span).

Pager resolution: -p flag > $PAGER > "less -R"
Profile resolution: --profile flag > $STRIPES_PROFILE > built-in default
Color is auto-disabled when NO_COLOR is set or stdout is not a terminal.
```

### Shell aliases

```sh
alias scat='stripes'                 # auto-paging: pages only when content overflows
alias spcat='stripes --paging=never' # always-stream, never page
```

![stripes CLI screenshot](assets/screenshot.png)

## Contributing

Contributions are welcome! To get started:

1. Ensure you have Go 1.25+ installed
2. Run `go test ./...` to verify tests pass

Please report bugs and feature requests via [GitHub Issues](https://github.com/firetiger-oss/stripes/issues).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
