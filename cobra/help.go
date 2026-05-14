package cobra

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// renderHelp writes a fully styled help page for cmd to w.
func renderHelp(cmd *cobra.Command, w io.Writer, s *Styles) {
	b := &strings.Builder{}

	if d := description(cmd); d != "" {
		fmt.Fprintln(b, s.Description.Render(d))
		b.WriteByte('\n')
	}

	writeUsage(b, cmd, s)

	if len(cmd.Aliases) > 0 {
		fmt.Fprintln(b, s.Title.Render("Aliases:"))
		fmt.Fprintf(b, "%s%s\n\n", s.Indent, strings.Join(cmd.Aliases, ", "))
	}

	if cmd.HasExample() {
		fmt.Fprintln(b, s.Title.Render("Examples:"))
		for _, line := range strings.Split(strings.TrimRight(cmd.Example, "\n"), "\n") {
			fmt.Fprintf(b, "%s%s\n", s.Indent, s.Example.Render(line))
		}
		b.WriteByte('\n')
	}

	writeCommands(b, cmd, s)
	writeFlags(b, cmd, s)

	if cmd.HasAvailableSubCommands() {
		hint := fmt.Sprintf(`Use "%s [command] --help" for more information about a command.`, cmd.CommandPath())
		fmt.Fprintln(b, s.Hint.Render(hint))
	}

	io.WriteString(w, b.String())
}

// renderUsage writes a compact usage block to w. Cobra calls SetUsageFunc
// after a flag-parse error; we print just the usage line and a hint.
func renderUsage(cmd *cobra.Command, w io.Writer, s *Styles) {
	b := &strings.Builder{}
	writeUsage(b, cmd, s)
	hint := fmt.Sprintf(`Run "%s --help" for usage.`, cmd.CommandPath())
	fmt.Fprintln(b, s.Hint.Render(hint))
	io.WriteString(w, b.String())
}

// description returns Long if set, falling back to Short.
func description(cmd *cobra.Command) string {
	if cmd.Long != "" {
		return strings.TrimSpace(cmd.Long)
	}
	return strings.TrimSpace(cmd.Short)
}

func writeUsage(b *strings.Builder, cmd *cobra.Command, s *Styles) {
	fmt.Fprintln(b, s.Title.Render("Usage:"))
	if cmd.Runnable() {
		fmt.Fprintf(b, "%s%s\n", s.Indent, styleUseLine(cmd, s))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(b, "%s%s %s\n",
			s.Indent,
			s.Program.Render(cmd.CommandPath()),
			s.Argument.Render("[command]"),
		)
	}
	b.WriteByte('\n')
}

// styleUseLine restyles the result of cmd.UseLine: program name in Program
// style, anything that looks like a flag or placeholder in Argument style.
func styleUseLine(cmd *cobra.Command, s *Styles) string {
	use := cmd.UseLine()
	path := cmd.CommandPath()
	rest := strings.TrimPrefix(use, path)
	rest = strings.TrimPrefix(rest, " ")

	out := s.Program.Render(path)
	if rest == "" {
		return out
	}
	// Style placeholders ([flags], <arg>, etc.) and any flag tokens.
	parts := strings.Fields(rest)
	styled := make([]string, len(parts))
	for i, p := range parts {
		switch {
		case strings.HasPrefix(p, "-"):
			styled[i] = s.Flag.Render(p)
		default:
			styled[i] = s.Argument.Render(p)
		}
	}
	return out + " " + strings.Join(styled, " ")
}

func writeCommands(b *strings.Builder, cmd *cobra.Command, s *Styles) {
	if !cmd.HasAvailableSubCommands() {
		return
	}

	groups := cmd.Groups()
	type bucket struct {
		title string
		cmds  []*cobra.Command
	}
	buckets := make([]*bucket, 0, len(groups)+1)
	byID := make(map[string]*bucket, len(groups))
	for _, g := range groups {
		bk := &bucket{title: g.Title}
		buckets = append(buckets, bk)
		byID[g.ID] = bk
	}
	// Subcommands without a group fall into a default bucket appended last.
	var ungrouped *bucket
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() && sub.Name() != "help" {
			continue
		}
		if bk, ok := byID[sub.GroupID]; ok {
			bk.cmds = append(bk.cmds, sub)
			continue
		}
		if ungrouped == nil {
			ungrouped = &bucket{title: "Available Commands:"}
			buckets = append(buckets, ungrouped)
		}
		ungrouped.cmds = append(ungrouped.cmds, sub)
	}

	for _, bk := range buckets {
		if len(bk.cmds) == 0 {
			continue
		}
		fmt.Fprintln(b, s.Title.Render(bk.title))
		width := 0
		for _, c := range bk.cmds {
			if n := len(c.Name()); n > width {
				width = n
			}
		}
		for _, c := range bk.cmds {
			pad := strings.Repeat(" ", width-len(c.Name()))
			fmt.Fprintf(b, "%s%s%s   %s\n",
				s.Indent,
				s.Command.Render(c.Name()),
				pad,
				s.Description.Render(c.Short),
			)
		}
		b.WriteByte('\n')
	}
}

func writeFlags(b *strings.Builder, cmd *cobra.Command, s *Styles) {
	local := collectFlags(cmd.LocalFlags())
	inherited := collectFlags(cmd.InheritedFlags())

	if len(local) > 0 {
		fmt.Fprintln(b, s.Title.Render("Flags:"))
		writeFlagList(b, local, s)
		b.WriteByte('\n')
	}
	if len(inherited) > 0 {
		fmt.Fprintln(b, s.Title.Render("Global Flags:"))
		writeFlagList(b, inherited, s)
		b.WriteByte('\n')
	}
}

func collectFlags(set *pflag.FlagSet) []*pflag.Flag {
	var out []*pflag.Flag
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		out = append(out, f)
	})
	return out
}

func writeFlagList(b *strings.Builder, flags []*pflag.Flag, s *Styles) {
	// Pre-render the "name column" (shorthand + long + type) for each flag
	// without styling, so we can pad to the longest plain width. Styles are
	// re-applied below over the same tokens.
	type row struct {
		shortToken string // "-s," or "   "
		longToken  string // "--name"
		typeToken  string // "<int>" or ""
		usage      string
		defValue   string
	}
	rows := make([]row, len(flags))
	plainWidths := make([]int, len(flags))
	maxWidth := 0
	for i, f := range flags {
		typeName, usage := pflag.UnquoteUsage(f)
		r := row{
			longToken: "--" + f.Name,
			usage:     usage,
		}
		if f.Shorthand != "" && f.ShorthandDeprecated == "" {
			r.shortToken = "-" + f.Shorthand + ","
		} else {
			r.shortToken = "   "
		}
		if typeName != "" {
			r.typeToken = "<" + typeName + ">"
		}
		if showDefault(f) {
			r.defValue = fmt.Sprintf("(default %s)", quoteDefault(f))
		}
		rows[i] = r

		w := len(r.shortToken) + 1 + len(r.longToken)
		if r.typeToken != "" {
			w += 1 + len(r.typeToken)
		}
		plainWidths[i] = w
		if w > maxWidth {
			maxWidth = w
		}
	}

	for i, r := range rows {
		pad := strings.Repeat(" ", maxWidth-plainWidths[i])
		nameCol := r.shortToken
		if strings.HasPrefix(nameCol, "-") {
			nameCol = s.Flag.Render(r.shortToken)
		}
		nameCol += " " + s.Flag.Render(r.longToken)
		if r.typeToken != "" {
			nameCol += " " + s.Argument.Render(r.typeToken)
		}
		line := s.Indent + nameCol + pad + "   " + s.Description.Render(r.usage)
		if r.defValue != "" {
			line += " " + s.Default.Render(r.defValue)
		}
		fmt.Fprintln(b, line)
	}
}

// showDefault returns true when a flag has a meaningful default worth printing.
func showDefault(f *pflag.Flag) bool {
	switch f.DefValue {
	case "", "false", "[]", "0", "0s":
		return false
	}
	return true
}

// quoteDefault wraps string-valued defaults in quotes; numeric/bool defaults
// are left bare to match cobra's own help formatting.
func quoteDefault(f *pflag.Flag) string {
	if f.Value.Type() == "string" {
		return fmt.Sprintf("%q", f.DefValue)
	}
	return f.DefValue
}
