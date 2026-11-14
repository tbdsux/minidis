// Command minidis creates Discord bot projects that use the minidis framework.
//
//	go install github.com/tbdsux/minidis/cli/minidis@latest
//	minidis init mybot
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

const usage = `minidis creates Discord bot projects that use github.com/tbdsux/minidis.

Usage:

	minidis init [flags] [directory]   create a project (default: the current directory)
	minidis version                    print the version
	minidis help                       show this help

Run "minidis init -h" for the flags of init.
`

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
	case errors.Is(err, errUsage): // already explained
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "minidis:", err)
		os.Exit(1)
	}
}

// errUsage marks an error about how the command was called that has already
// been printed, such as an unknown flag.
var errUsage = errors.New("usage error")

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return fmt.Errorf("%w: no command given", errUsage)
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, "minidis", version())
		return nil
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return nil
	default:
		fmt.Fprintf(stderr, "minidis: unknown command %q\n\n%s", args[0], usage)
		return fmt.Errorf("%w: unknown command %q", errUsage, args[0])
	}
}

// libraryPath is the module of both the framework and this command.
const libraryPath = "github.com/tbdsux/minidis"

// version is the version of the minidis module this command was installed
// from, or "(devel)" when it was built from a checkout.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	return buildVersion(info)
}

// buildVersion is version for the given build information. "go install
// module@version" records no version control settings, while a build in a
// checkout does, along with a pseudo-version for its commit that may exist
// nowhere but in that checkout.
func buildVersion(info *debug.BuildInfo) string {
	if info.Main.Path != libraryPath {
		return "(devel)"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return "(devel)"
		}
	}
	return releasedVersion(info.Main.Version)
}

// releasedVersion returns v if go get can fetch it. Builds from a checkout are
// stamped with versions such as "v0.0.0-20260101000000-abcdef123456+dirty",
// whose build metadata go get rejects.
func releasedVersion(v string) string {
	if v == "" || v == "(devel)" || strings.Contains(v, "+") {
		return "(devel)"
	}
	return v
}
