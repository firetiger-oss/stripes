package stripes_test

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var update = flag.Bool("update", false, "update golden files inside testscript archives")

func TestMain(m *testing.M) {
	flag.Parse()

	binDir, err := os.MkdirTemp("", "stripes-bin-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(binDir)

	if err := buildBinary(binDir, "stripes", "./cmd/stripes"); err != nil {
		panic(err)
	}
	if err := buildBinary(binDir, "stub-pager", "./testdata/stubcmd/stub-pager"); err != nil {
		panic(err)
	}

	os.Setenv("STRIPES_TEST_BIN", binDir)
	os.Exit(m.Run())
}

func buildBinary(dir, name, pkg string) error {
	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestCLI(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:           "testdata/script",
		UpdateScripts: *update,
		Setup: func(env *testscript.Env) error {
			binDir := os.Getenv("STRIPES_TEST_BIN")
			env.Setenv("PATH", binDir+string(os.PathListSeparator)+env.Getenv("PATH"))
			env.Setenv("NO_COLOR", "")
			env.Setenv("PAGER", "")
			env.Setenv("STRIPES_PAGER", "")
			return nil
		},
	})
}
