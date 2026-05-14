package cobra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// TestMain forces a TrueColor profile so styled output is deterministic
// regardless of whether the test process has a TTY. Matches the pattern
// used in code_test.go and line_numbers_test.go.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

func TestRenderHelp(t *testing.T) {
	tests := []struct {
		name string
		make func() *cobra.Command
		want string
	}{
		{
			name: "minimal",
			make: func() *cobra.Command {
				return &cobra.Command{Use: "tool", Short: "A small tool"}
			},
			want: "A small tool\n\nUsage:\n\n",
		},
		{
			name: "long description and a flag",
			make: func() *cobra.Command {
				c := &cobra.Command{
					Use:   "tool [flags]",
					Short: "A small tool",
					Long:  "tool does a thing.\nIt does it well.",
					Run:   func(*cobra.Command, []string) {},
				}
				c.Flags().BoolP("verbose", "v", false, "verbose output")
				c.Flags().StringP("config", "c", "/etc/cfg", "config `file` path")
				return c
			},
			want: "tool does a thing.\nIt does it well.\n\n" +
				"Usage:\n" +
				"  tool [flags]\n\n" +
				"Flags:\n" +
				"  -c, --config <file>   config file path (default \"/etc/cfg\")\n" +
				"  -v, --verbose         verbose output\n\n",
		},
		{
			name: "subcommands grouped and ungrouped",
			make: func() *cobra.Command {
				root := &cobra.Command{Use: "tool", Short: "root"}
				root.AddGroup(&cobra.Group{ID: "core", Title: "Core Commands:"})
				root.AddCommand(&cobra.Command{
					Use: "serve", Short: "Start the server", GroupID: "core",
					Run: func(*cobra.Command, []string) {},
				})
				root.AddCommand(&cobra.Command{
					Use: "version", Short: "Print version",
					Run: func(*cobra.Command, []string) {},
				})
				root.SetHelpCommand(&cobra.Command{Use: "help", Hidden: true})
				return root
			},
			want: "root\n\n" +
				"Usage:\n" +
				"  tool [command]\n\n" +
				"Core Commands:\n" +
				"  serve   Start the server\n\n" +
				"Available Commands:\n" +
				"  version   Print version\n\n" +
				`Use "tool [command] --help" for more information about a command.` + "\n",
		},
		{
			name: "examples and aliases",
			make: func() *cobra.Command {
				return &cobra.Command{
					Use:     "tool",
					Short:   "root",
					Aliases: []string{"t", "tl"},
					Example: "  tool run\n  tool serve --port 8080",
					Run:     func(*cobra.Command, []string) {},
				}
			},
			want: "root\n\n" +
				"Usage:\n" +
				"  tool\n\n" +
				"Aliases:\n" +
				"  t, tl\n\n" +
				"Examples:\n" +
				"    tool run\n" +
				"    tool serve --port 8080\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.make()
			var buf bytes.Buffer
			renderHelp(cmd, &buf, DefaultStyles)
			got := ansi.Strip(buf.String())
			if got != tt.want {
				t.Errorf("renderHelp mismatch\nwant:\n%q\n got:\n%q\n with ANSI:\n%s", tt.want, got, buf.String())
			}
		})
	}
}

func TestRenderUsage(t *testing.T) {
	cmd := &cobra.Command{Use: "tool [flags]", Run: func(*cobra.Command, []string) {}}
	var buf bytes.Buffer
	renderUsage(cmd, &buf, DefaultStyles)
	got := ansi.Strip(buf.String())
	want := "Usage:\n  tool [flags]\n\n" + `Run "tool --help" for usage.` + "\n"
	if got != want {
		t.Errorf("renderUsage mismatch\nwant:\n%q\n got:\n%q", want, got)
	}
}

func TestDefaultErrorHandler(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "plain error",
			err:  errors.New("something went wrong"),
			want: "Error: something went wrong\n",
		},
		{
			name: "unknown flag prints hint",
			err:  errors.New("unknown flag: --bogus"),
			want: "Error: unknown flag: --bogus\nTry --help for usage.\n",
		},
		{
			name: "invalid argument prints hint",
			err:  fmt.Errorf("invalid argument %q for flag --port", "foo"),
			want: "Error: invalid argument \"foo\" for flag --port\nTry --help for usage.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			DefaultErrorHandler(&buf, DefaultStyles, tt.err)
			got := ansi.Strip(buf.String())
			if got != tt.want {
				t.Errorf("DefaultErrorHandler mismatch\nwant:\n%q\n got:\n%q", tt.want, got)
			}
		})
	}
}

func TestHelpContainsANSI(t *testing.T) {
	cmd := &cobra.Command{Use: "tool", Short: "root", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().BoolP("verbose", "v", false, "verbose output")

	var buf bytes.Buffer
	renderHelp(cmd, &buf, DefaultStyles)
	out := buf.String()

	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI escape sequences in styled output, got: %q", out)
	}
	if !strings.Contains(out, "--verbose") {
		t.Errorf("expected flag long name in output: %q", out)
	}
}

func TestApplyInstallsOnSubcommands(t *testing.T) {
	root := &cobra.Command{Use: "tool", Run: func(*cobra.Command, []string) {}}
	sub := &cobra.Command{Use: "sub", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(sub)

	var buf bytes.Buffer
	Apply(root, WithOutput(&buf), WithErrorOutput(&buf))

	// Trigger help on the subcommand; should hit our renderer (no "Usage:" with
	// cobra's default template header order — ours starts with description or
	// "Usage:" depending on fields set).
	root.SetArgs([]string{"sub", "--help"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := ansi.Strip(buf.String())
	// UseLine appends "[flags]" when there are flags (cobra adds the implicit
	// --help), so we anchor on the program path only.
	if !strings.HasPrefix(got, "Usage:\n  tool sub") {
		t.Errorf("expected our renderer's Usage header on subcommand, got: %q", got)
	}
}

func TestExecuteRoutesErrorThroughHandler(t *testing.T) {
	root := &cobra.Command{
		Use:  "tool",
		RunE: func(*cobra.Command, []string) error { return errors.New("boom") },
	}
	root.SetArgs(nil)

	var errBuf bytes.Buffer
	err := Execute(context.Background(), root,
		WithOutput(&bytes.Buffer{}),
		WithErrorOutput(&errBuf),
	)
	if err == nil {
		t.Fatal("expected error to be returned")
	}
	got := ansi.Strip(errBuf.String())
	if got != "Error: boom\n" {
		t.Errorf("expected styled error in stderr, got %q", got)
	}
}
