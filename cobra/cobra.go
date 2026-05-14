// Package cobra applies a restrained ANSI palette — aligned with
// [github.com/firetiger-oss/stripes] — to help, usage, and error output of
// commands built with [github.com/spf13/cobra].
//
// The integration mirrors [github.com/charmbracelet/fang]: help and usage
// are rendered from scratch (not by post-processing cobra's defaults),
// and ANSI is downgraded or stripped automatically when stdout/stderr is
// not a terminal.
//
// Typical usage:
//
//	root := &cobra.Command{Use: "mytool", Short: "..."}
//	// ... add subcommands and flags ...
//	if err := stripescobra.Execute(ctx, root); err != nil {
//		os.Exit(1)
//	}
package cobra

import (
	"context"
	"io"
	"os"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"
)

// ErrorHandler renders an error to w using styles s. Implementations should
// not return — any reporting must happen before returning.
type ErrorHandler func(w io.Writer, s *Styles, err error)

type config struct {
	styles  *Styles
	out     io.Writer
	errOut  io.Writer
	onError ErrorHandler
}

// Option configures Execute or Apply.
type Option func(*config)

// WithStyles overrides the default styles.
func WithStyles(s *Styles) Option {
	return func(c *config) { c.styles = s }
}

// WithOutput overrides the writer used for help and usage output.
// Defaults to os.Stdout.
func WithOutput(w io.Writer) Option {
	return func(c *config) { c.out = w }
}

// WithErrorOutput overrides the writer used for error output.
// Defaults to os.Stderr.
func WithErrorOutput(w io.Writer) Option {
	return func(c *config) { c.errOut = w }
}

// WithErrorHandler overrides the function used to render errors returned
// from cobra.Command.ExecuteContext.
func WithErrorHandler(fn ErrorHandler) Option {
	return func(c *config) { c.onError = fn }
}

func newConfig(opts []Option) *config {
	c := &config{
		styles:  DefaultStyles,
		out:     os.Stdout,
		errOut:  os.Stderr,
		onError: DefaultErrorHandler,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Apply installs styled help and usage rendering on root and every
// subcommand reachable from it. It does not run the command.
//
// Use Apply when the caller wants to own the Execute call; otherwise
// prefer Execute, which also routes errors through the configured handler.
func Apply(root *cobra.Command, opts ...Option) {
	c := newConfig(opts)
	apply(root, c)
}

func apply(cmd *cobra.Command, c *config) {
	out := wrapOutput(c.out, os.Environ())
	cmd.SetOut(out)
	cmd.SetErr(wrapOutput(c.errOut, os.Environ()))
	cmd.SetHelpFunc(func(cc *cobra.Command, _ []string) {
		renderHelp(cc, out, c.styles)
	})
	cmd.SetUsageFunc(func(cc *cobra.Command) error {
		renderUsage(cc, out, c.styles)
		return nil
	})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	for _, sub := range cmd.Commands() {
		apply(sub, c)
	}
}

// Execute installs styled help, usage, and error rendering on root and
// every subcommand, then calls root.ExecuteContext(ctx). When cobra
// returns an error it is routed through the configured handler before
// being returned to the caller.
func Execute(ctx context.Context, root *cobra.Command, opts ...Option) error {
	c := newConfig(opts)
	apply(root, c)
	err := root.ExecuteContext(ctx)
	if err != nil {
		c.onError(wrapOutput(c.errOut, os.Environ()), c.styles, err)
	}
	return err
}

// wrapOutput returns w wrapped in a colorprofile.Writer that downgrades or
// strips ANSI escapes based on the terminal and environment. If w is
// already a *colorprofile.Writer it is returned unchanged so callers can
// pre-configure a profile.
func wrapOutput(w io.Writer, env []string) io.Writer {
	if _, ok := w.(*colorprofile.Writer); ok {
		return w
	}
	return colorprofile.NewWriter(w, env)
}
