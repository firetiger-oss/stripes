package cobra

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// renderHelp writes a fully styled help page for cmd to w. The plain-text
// output (ANSI stripped) matches cobra's default help output byte-for-byte;
// styles are layered on top via [Styles].
func renderHelp(cmd *cobra.Command, w io.Writer, s *Styles) {
	desc := trimRightSpace(cmd.Long)
	if desc == "" {
		desc = trimRightSpace(cmd.Short)
	}
	if desc != "" {
		first, rest, more := strings.Cut(desc, "\n")
		fmt.Fprintln(w, s.Description.Bold(true).Render(first))
		if more {
			fmt.Fprintln(w, s.Description.Render(rest))
		}
		fmt.Fprintln(w)
	}
	if cmd.Runnable() || cmd.HasSubCommands() {
		writeUsage(w, cmd, s)
	}
}

// renderUsage writes the usage block for cmd to w. The plain-text output
// matches cobra's default usage output (defaultUsageFunc) byte-for-byte.
func renderUsage(cmd *cobra.Command, w io.Writer, s *Styles) {
	writeUsage(w, cmd, s)
}

// writeUsage mirrors cobra's defaultUsageFunc: the section ordering, the
// inter-section "\n\n" prefixing, and the trailing newline all match.
func writeUsage(w io.Writer, cmd *cobra.Command, s *Styles) {
	fmt.Fprint(w, s.Title.Render("Usage:"))
	if cmd.Runnable() {
		fmt.Fprintf(w, "\n  %s", styleUseLine(cmd, s))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(w, "\n  %s %s",
			s.Program.Render(cmd.CommandPath()),
			s.Argument.Render("[command]"),
		)
	}
	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(w, "\n\n%s\n  %s",
			s.Title.Render("Aliases:"),
			cmd.NameAndAliases(),
		)
	}
	if cmd.HasExample() {
		fmt.Fprintf(w, "\n\n%s\n%s",
			s.Title.Render("Examples:"),
			styleExample(cmd.Example, s),
		)
	}
	if cmd.HasAvailableSubCommands() {
		writeSubcommands(w, cmd, s)
	}
	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintf(w, "\n\n%s\n%s",
			s.Title.Render("Flags:"),
			trimRightSpace(renderFlagUsages(cmd.LocalFlags(), s)),
		)
	}
	if cmd.HasAvailableInheritedFlags() {
		fmt.Fprintf(w, "\n\n%s\n%s",
			s.Title.Render("Global Flags:"),
			trimRightSpace(renderFlagUsages(cmd.InheritedFlags(), s)),
		)
	}
	if cmd.HasHelpSubCommands() {
		fmt.Fprintf(w, "\n\n%s", s.Title.Render("Additional help topics:"))
		for _, sub := range cmd.Commands() {
			if sub.IsAdditionalHelpTopicCommand() {
				fmt.Fprintf(w, "\n  %s %s",
					rpad(sub.CommandPath(), sub.CommandPathPadding()),
					sub.Short,
				)
			}
		}
	}
	if cmd.HasAvailableSubCommands() {
		hint := fmt.Sprintf(`Use "%s [command] --help" for more information about a command.`, cmd.CommandPath())
		fmt.Fprintf(w, "\n\n%s", s.Hint.Render(hint))
	}
	// Two trailing newlines (vs. cobra's one) deliberately leave a blank
	// line after the last section so help output isn't visually flush
	// against the next shell prompt.
	fmt.Fprint(w, "\n\n")
}

// styleUseLine restyles the result of cmd.UseLine: program name in Program
// style, anything that looks like a flag in Flag style, placeholders in
// Argument style.
func styleUseLine(cmd *cobra.Command, s *Styles) string {
	use := cmd.UseLine()
	path := cmd.CommandPath()
	rest := strings.TrimPrefix(use, path)
	rest = strings.TrimPrefix(rest, " ")

	out := s.Program.Render(path)
	if rest == "" {
		return out
	}
	parts := strings.Fields(rest)
	styled := make([]string, len(parts))
	for i, p := range parts {
		if strings.HasPrefix(p, "-") {
			styled[i] = s.Flag.Render(p)
		} else {
			styled[i] = s.Argument.Render(p)
		}
	}
	return out + " " + strings.Join(styled, " ")
}

// styleExample wraps each line of example text in the Example style,
// preserving the original line breaks so the plain-text output is
// byte-identical to cmd.Example.
func styleExample(example string, s *Styles) string {
	lines := strings.Split(example, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = s.Example.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

// writeSubcommands renders the subcommand sections, mirroring cobra's
// defaultUsageFunc: a single "Available Commands:" block when no groups,
// otherwise per-group blocks plus an "Additional Commands:" block for
// ungrouped commands.
func writeSubcommands(w io.Writer, cmd *cobra.Command, s *Styles) {
	cmds := cmd.Commands()
	if len(cmd.Groups()) == 0 {
		fmt.Fprintf(w, "\n\n%s", s.Title.Render("Available Commands:"))
		for _, sub := range cmds {
			if sub.IsAvailableCommand() || sub.Name() == "help" {
				writeSubcommandLine(w, sub, s)
			}
		}
		return
	}
	for _, group := range cmd.Groups() {
		fmt.Fprintf(w, "\n\n%s", s.Title.Render(group.Title))
		for _, sub := range cmds {
			if sub.GroupID == group.ID && (sub.IsAvailableCommand() || sub.Name() == "help") {
				writeSubcommandLine(w, sub, s)
			}
		}
	}
	if !cmd.AllChildCommandsHaveGroup() {
		fmt.Fprintf(w, "\n\n%s", s.Title.Render("Additional Commands:"))
		for _, sub := range cmds {
			if sub.GroupID == "" && (sub.IsAvailableCommand() || sub.Name() == "help") {
				writeSubcommandLine(w, sub, s)
			}
		}
	}
}

func writeSubcommandLine(w io.Writer, sub *cobra.Command, s *Styles) {
	name := sub.Name()
	pad := sub.NamePadding() - len(name)
	if pad < 0 {
		pad = 0
	}
	fmt.Fprintf(w, "\n  %s%s %s",
		s.Command.Render(name),
		strings.Repeat(" ", pad),
		sub.Short,
	)
}

// renderFlagUsages mirrors pflag.FlagSet.FlagUsagesWrapped(0), applying
// styles to recognized tokens. The alignment math uses plain-text widths
// (matching pflag's \x00-based maxlen-sidx formula), then styles are
// layered onto the emitted tokens — so ANSI escapes don't perturb the
// column alignment.
func renderFlagUsages(set *pflag.FlagSet, s *Styles) string {
	type row struct {
		flagToken  string // "-X, --name" or "    --name"
		typeToken  string // typename ("string", "int", "file", ...) or ""
		noOptToken string // "[=val]" or `[="val"]` or ""
		usage      string
		defValue   string // `(default ...)` or ""
		deprecated string // `(DEPRECATED: ...)` or ""
		sidx       int    // plain-text width of the name column
	}

	var rows []row
	maxlen := 0
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		r := row{}
		if f.Shorthand != "" && f.ShorthandDeprecated == "" {
			r.flagToken = fmt.Sprintf("  -%s, --%s", f.Shorthand, f.Name)
		} else {
			r.flagToken = fmt.Sprintf("      --%s", f.Name)
		}

		typeName, usage := pflag.UnquoteUsage(f)
		r.typeToken = typeName
		r.usage = usage

		if f.NoOptDefVal != "" {
			switch f.Value.Type() {
			case "string":
				r.noOptToken = fmt.Sprintf("[=%q]", f.NoOptDefVal)
			case "bool", "boolfunc":
				if f.NoOptDefVal != "true" {
					r.noOptToken = fmt.Sprintf("[=%s]", f.NoOptDefVal)
				}
			case "count":
				if f.NoOptDefVal != "+1" {
					r.noOptToken = fmt.Sprintf("[=%s]", f.NoOptDefVal)
				}
			default:
				r.noOptToken = fmt.Sprintf("[=%s]", f.NoOptDefVal)
			}
		}

		// Plain-text width of the name column.
		w := len(r.flagToken)
		if r.typeToken != "" {
			w += 1 + len(r.typeToken)
		}
		w += len(r.noOptToken)
		r.sidx = w
		// pflag tracks the line length with a trailing \x00 sentinel, so
		// effective maxlen is sidx+1.
		if w+1 > maxlen {
			maxlen = w + 1
		}

		if !defaultIsZeroValue(f) {
			if f.Value.Type() == "string" {
				r.defValue = fmt.Sprintf(" (default %q)", f.DefValue)
			} else {
				r.defValue = fmt.Sprintf(" (default %s)", f.DefValue)
			}
		}
		if f.Deprecated != "" {
			r.deprecated = fmt.Sprintf(" (DEPRECATED: %s)", f.Deprecated)
		}
		rows = append(rows, r)
	})

	var b strings.Builder
	for _, r := range rows {
		// Styled name column.
		nameCol := s.Flag.Render(r.flagToken)
		if r.typeToken != "" {
			nameCol += " " + s.Argument.Render(r.typeToken)
		}
		if r.noOptToken != "" {
			nameCol += s.Argument.Render(r.noOptToken)
		}
		// Padding so the description column lines up (uses plain widths).
		spacing := strings.Repeat(" ", maxlen-r.sidx)
		// Description column.
		desc := s.Description.Render(r.usage)
		if r.defValue != "" {
			desc += s.Default.Render(r.defValue)
		}
		if r.deprecated != "" {
			desc += s.Default.Render(r.deprecated)
		}
		// Match pflag's Fprintln(buf, nameCol, spacing, desc) layout: a
		// single space separates each argument, so the gap between the
		// name column and the description is maxlen-sidx+2 chars (>= 3).
		fmt.Fprintf(&b, "%s %s %s\n", nameCol, spacing, desc)
	}
	return b.String()
}

// defaultIsZeroValue mirrors pflag's private (*Flag).defaultIsZeroValue.
// Switching on the type *string* (rather than the private value types)
// covers every standard pflag type; unknown custom types fall through to
// pflag's catch-all comparison against the common zero strings.
func defaultIsZeroValue(f *pflag.Flag) bool {
	switch f.Value.Type() {
	case "bool", "boolfunc":
		return f.DefValue == "false" || f.DefValue == ""
	case "duration":
		return f.DefValue == "0" || f.DefValue == "0s"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"count", "float32", "float64":
		return f.DefValue == "0"
	case "string":
		return f.DefValue == ""
	case "ip", "ipMask", "ipNet":
		return f.DefValue == "<nil>"
	case "intSlice", "stringSlice", "stringArray":
		return f.DefValue == "[]"
	}
	switch f.DefValue {
	case "false", "<nil>", "", "0":
		return true
	}
	return false
}

func rpad(s string, n int) string {
	if pad := n - len(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func trimRightSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}
