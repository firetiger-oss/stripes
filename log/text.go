package log

// The text-log half of the package: shared infrastructure plus
// per-format files (alb.go, nginx.go, logfmt.go, …) that each
// declare a [LineFormat] value. The package init in this file
// registers the built-ins in priority order; external callers can
// add more formats by calling [Register] from their own init().
// Detection is content-based: no format claims a file extension
// (multiple log shapes share .log), so each format's Detect
// callback participates during stripes.Detect's content-sniffing
// phase.

import (
	"bufio"
	"io"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// LineFormat describes one text log format. Implementations live as
// package-level values in their own file, registered via [Register]
// from init().
type LineFormat struct {
	// Name is the unique stripes.Format.Name (e.g. "logfmt"). Used
	// as the CLI's --format alias.
	Name string

	// ContentType is the canonical MIME type for this format
	// (e.g. "application/vnd.logfmt"). Must be unique across the
	// registry.
	ContentType string

	// Extensions are file extensions to claim, if any. Most text
	// log formats leave this empty — .log is shared, so detection
	// relies on Detect.
	Extensions []string

	// HasLevel toggles the severity column. true (the default for
	// formats with structured levels: logfmt, log4j, RFC 5424
	// syslog, etc.) reserves the 4-char column and renders a
	// coloured label when [Row.Level] is recognised, blank
	// otherwise — preserving alignment when only some lines in a
	// stream carry a level. false (BSD syslog without PRI, macOS
	// system logs) drops the column entirely so level-less streams
	// don't show a permanently empty slot.
	HasLevel bool

	// Detect inspects the first ~512 bytes of input and returns
	// true when those bytes look like this format. Called during
	// content-sniffing in registration order, so more specific
	// formats should register before more permissive ones.
	Detect func(peek []byte) bool

	// Format parses one log line into a [Row]. The shared renderer
	// turns the row into a small block:
	//
	//	yyyy/mm/dd hh:mm:ss.mmm LEVEL metadata message
	//	  attr1      = value1
	//	  attr2.key1 = value2
	//
	// The format owns styling of its metadata — call
	// [StyleMetadata] for the default purple, or build a
	// multi-coloured string directly (e.g. HTTP access logs colour
	// the status code by class). Plain attribute keys/values get
	// the shared styling; the format only needs to fill them in.
	//
	// Returning ok=false means "this line doesn't match my shape"
	// — the renderer falls back to a dim, pass-through line. The
	// line passed in has its trailing newline stripped.
	Format func(line string, styles *stripes.Styles) (Row, bool)
}

// scannerBufSize bounds individual log lines. 1 MiB accommodates
// stack traces dumped on one line by some loggers without making
// the default buffer wastefully large.
const scannerBufSize = 1 << 20

// Register wraps lf into a [stripes.Format] and adds it to the
// global registry. The package's own init() calls Register on every
// built-in LineFormat in a deterministic priority order — most
// specific Detect predicates first, most permissive (logfmt) last —
// so the order of file definitions doesn't decide which format wins
// for an ambiguous payload. External callers can add more formats
// after init(), but those then run AFTER the built-ins.
//
// Panics on validation errors (missing fields, duplicate
// name/content-type) — see [stripes.Register].
func Register(lf LineFormat) {
	stripes.Register(stripes.Format{
		Name:        lf.Name,
		ContentType: lf.ContentType,
		Extensions:  lf.Extensions,
		Detect:      lf.Detect,
		RendererFor: stripes.Simple(lineRenderer(lf)),
	})
}

// init registers the built-in text log formats in priority order:
// most specific first (so they win ambiguous detection), permissive
// formats (logfmt, jsonlog) last.
func init() {
	for _, lf := range builtins {
		Register(lf)
	}
}

// builtins is the ordered list of text log formats this package
// ships with. Defined here rather than in each format's file so the
// priority order is explicit and easy to audit. Add new formats at
// the position that matches their detection specificity.
var builtins = []LineFormat{
	syslogRFC5424Format,
	syslogBSDFormat,
	albAccessFormat,
	nginxAccessFormat,
	log4jFormat,
	pythonFormat,
	goLogFormat,
	jsonLogFormat,
	logfmtFormat,
}

// lineRenderer returns the stripes.Renderer for lf. It runs a small
// state machine over the input lines so multi-line records (Go
// panics dumped under a log line, Java exception traces, etc.) fold
// into the message of the most recent parseable line. The rules:
//
//   - A line that lf.Format accepts ends any pending record and
//     starts a new one.
//   - A line that lf.Format rejects (or is whitespace-prefixed and
//     so obviously not a header) is appended to the pending
//     record's continuation buffer. Pre-pending orphan lines pass
//     through dim, as before.
//   - An empty line ends the pending record when the record has no
//     continuation yet (the common "blank between records" case),
//     but folds in when the record is already multi-line (the
//     common "blank in the middle of a stack trace" case).
//   - EOF flushes any pending record.
//
// Continuation lines are styled the same as the message body so
// the folded content reads as a single logical record (Go panic
// traces, multi-line exception messages, etc. are part of the
// log line, not overflow detail).
func lineRenderer(lf LineFormat) stripes.Renderer {
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 64<<10), scannerBufSize)
		bw := bufio.NewWriter(w)
		defer bw.Flush()

		var pending *Row
		var extras []string // raw continuation lines, dim-styled when flushed

		flush := func() {
			if pending == nil {
				return
			}
			if len(extras) > 0 {
				// Style each line independently with the body text
				// style so continuation lines blend in with the
				// rest of the message; styling per line keeps SGR
				// escapes from being cut by the renderer's later
				// \n-split.
				styled := make([]string, len(extras))
				for i, line := range extras {
					styled[i] = StyleText(styles).Render(line)
				}
				cont := strings.Join(styled, "\n")
				if pending.Message != "" {
					pending.Message = pending.Message + "\n" + cont
				} else {
					pending.Message = cont
				}
				extras = nil
			}
			bw.WriteString(renderRow(*pending, lf, styles))
			bw.WriteByte('\n')
			pending = nil
		}

		fold := func(line string) {
			if pending != nil {
				extras = append(extras, line)
				return
			}
			bw.WriteString(styles.Comment.Render(line))
			bw.WriteByte('\n')
		}

		for s.Scan() {
			line := s.Text()
			if line == "" {
				if pending != nil && len(extras) > 0 {
					// Already collecting multi-line content — keep
					// the blank as part of it (Go panic between
					// "panic:" and "goroutine" is the motivating
					// case).
					extras = append(extras, "")
					continue
				}
				flush()
				bw.WriteByte('\n')
				continue
			}
			if isContinuation(line) {
				fold(line)
				continue
			}
			row, ok := lf.Format(line, styles)
			if !ok {
				fold(line)
				continue
			}
			flush()
			r := row
			pending = &r
		}
		flush()

		if err := s.Err(); err != nil {
			bw.WriteString(styles.Comment.Render("log: read error: " + err.Error()))
			bw.WriteByte('\n')
		}
	}
}

// isContinuation reports whether line is whitespace-prefixed —
// stack trace frames, indented sub-fields under a header, etc.
// Treated as continuation of the previous record unconditionally;
// formats whose headers can themselves start with whitespace would
// need to bypass this, but none currently do.
func isContinuation(line string) bool {
	if line == "" {
		return false
	}
	c := line[0]
	return c == ' ' || c == '\t'
}

// FirstNonEmptyLines returns up to n leading non-empty lines from
// peek (lines split on '\n', with trailing '\r' trimmed). Useful for
// Detect callbacks that want to inspect content past leading blank
// lines without committing to a full scan.
func FirstNonEmptyLines(peek []byte, n int) []string {
	out := make([]string, 0, n)
	rest := string(peek)
	for len(out) < n && rest != "" {
		var line string
		if i := strings.IndexByte(rest, '\n'); i >= 0 {
			line, rest = rest[:i], rest[i+1:]
		} else {
			line, rest = rest, ""
		}
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
