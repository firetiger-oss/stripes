package cobra

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func BenchmarkRenderHelpSmall(b *testing.B) {
	benchRenderHelp(b, buildTree(2, 2))
}

func BenchmarkRenderHelpMedium(b *testing.B) {
	benchRenderHelp(b, buildTree(8, 6))
}

func BenchmarkRenderHelpLarge(b *testing.B) {
	benchRenderHelp(b, buildTree(32, 16))
}

func benchRenderHelp(b *testing.B, cmd *cobra.Command) {
	var buf bytes.Buffer
	buf.Grow(8 << 10)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		buf.Reset()
		renderHelp(cmd, &buf, DefaultStyles)
	}
	b.SetBytes(int64(buf.Len()))
}

// buildTree returns a root command with subcommands and flags sized for
// benchmarking. subcommands controls the number of immediate children;
// flags controls the local flags on the root.
func buildTree(subcommands, flags int) *cobra.Command {
	root := &cobra.Command{
		Use:     "tool [flags]",
		Short:   "A benchmark tool",
		Long:    "tool is a benchmark fixture that exercises the help renderer.",
		Example: "  tool subcmd0\n  tool subcmd1 --opt0 1",
		Run:     func(*cobra.Command, []string) {},
	}
	for i := 0; i < flags; i++ {
		switch i % 3 {
		case 0:
			root.Flags().Bool(fmt.Sprintf("flag%d", i), false, fmt.Sprintf("toggle %d", i))
		case 1:
			root.Flags().Int(fmt.Sprintf("opt%d", i), i, fmt.Sprintf("integer option %d", i))
		case 2:
			root.Flags().String(fmt.Sprintf("path%d", i), "/tmp", fmt.Sprintf("path option %d", i))
		}
	}
	for i := 0; i < subcommands; i++ {
		root.AddCommand(&cobra.Command{
			Use:   fmt.Sprintf("subcmd%d", i),
			Short: fmt.Sprintf("subcommand %d", i),
			Run:   func(*cobra.Command, []string) {},
		})
	}
	return root
}
