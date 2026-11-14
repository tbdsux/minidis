package main

import (
	"bufio"
	"bytes"
	"cmp"
	"embed"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

//go:embed templates
var templateFS embed.FS

type initOptions struct {
	dir      string
	module   string
	http     bool
	force    bool
	skipDeps bool
}

func runInit(args []string, stdout, stderr io.Writer) error {
	var o initOptions

	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.module, "module", "", "module path of a new go.mod (default: the directory name)")
	flags.BoolVar(&o.http, "http", false, "receive interactions over HTTP instead of the gateway")
	flags.BoolVar(&o.force, "force", false, "overwrite files that already exist")
	flags.BoolVar(&o.skipDeps, "skip-deps", false, "do not run go get and go mod tidy")
	flags.Usage = func() {
		fmt.Fprint(stderr, "Usage: minidis init [flags] [directory]\n\n"+
			"Creates a bot project in directory, the current one by default. A go.mod is\n"+
			"created unless one is there, and existing files are never overwritten\n"+
			"without -force.\n\nFlags:\n")
		flags.PrintDefaults()
	}

	// Accept flags after the directory too, as in "minidis init mybot -http".
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		args = flags.Args()[1:]
	}
	if len(positional) > 1 {
		flags.Usage()
		return fmt.Errorf("init takes one directory, got %d", len(positional))
	}
	o.dir = cmp.Or(strings.Join(positional, ""), ".")

	return initProject(o, stdout, stderr)
}

// projectData is what the templates are rendered with.
type projectData struct {
	Module string // the project's module path
	Name   string // the directory name, which go build names the binary after
	HTTP   bool
}

func initProject(o initOptions, stdout, stderr io.Writer) error {
	dir, err := filepath.Abs(o.dir)
	if err != nil {
		return err
	}

	module, hasGoMod, err := readModulePath(dir)
	if err != nil {
		return err
	}
	switch {
	case hasGoMod && o.module != "" && o.module != module:
		return fmt.Errorf("%s already has module %q, not %q", filepath.Join(dir, "go.mod"), module, o.module)
	case !hasGoMod:
		module = cmp.Or(o.module, filepath.Base(dir))
		if strings.ContainsAny(module, " \t\n\"'`") {
			return fmt.Errorf("%q is not a valid module path; choose one with -module", module)
		}
	}

	files, err := render(projectData{Module: module, Name: filepath.Base(dir), HTTP: o.http})
	if err != nil {
		return err
	}

	// Check everything before writing anything, so a refusal changes nothing.
	if !o.force {
		var existing []string
		for _, name := range slices.Sorted(maps.Keys(files)) {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				existing = append(existing, name)
			}
		}
		if len(existing) > 0 {
			return fmt.Errorf("%s already has %s; use -force to overwrite", dir, strings.Join(existing, ", "))
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if !hasGoMod {
		if err := goCommand(dir, stderr, "mod", "init", module); err != nil {
			return err
		}
	}

	for _, name := range slices.Sorted(maps.Keys(files)) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "created", filepath.Join(o.dir, name))
	}

	if !o.skipDeps {
		// The templates are written for the API of the version this command
		// was built from, so pin the framework to it.
		dependency := libraryPath + "@latest"
		if v := version(); v != "(devel)" {
			dependency = libraryPath + "@" + v
		}
		if err := goCommand(dir, stderr, "get", dependency); err != nil {
			return err
		}
		if err := goCommand(dir, stderr, "mod", "tidy"); err != nil {
			return err
		}
	}

	printNextSteps(stdout, o)
	return nil
}

// readModulePath returns the module path declared in dir/go.mod, if any.
func readModulePath(dir string) (string, bool, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			return strings.Trim(strings.TrimSpace(rest), `"`), true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}
	return "", false, fmt.Errorf("%s has no module line", f.Name())
}

// render executes the templates for the project, keyed by the path of the
// file each one becomes. Go files are formatted.
func render(data projectData) (map[string][]byte, error) {
	main := "main_gateway.go.tmpl"
	if data.HTTP {
		main = "main_http.go.tmpl"
	}

	outputs := map[string]string{
		main:                        "main.go",
		"commands/commands.go.tmpl": "commands/commands.go",
		"commands/hello.go.tmpl":    "commands/hello.go",
		"README.md.tmpl":            "README.md",
		"gitignore.tmpl":            ".gitignore", // embed skips dotfiles
	}

	files := make(map[string][]byte, len(outputs))
	for src, dst := range outputs {
		tmpl, err := template.ParseFS(templateFS, "templates/"+src)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("rendering %s: %w", dst, err)
		}

		out := buf.Bytes()
		if strings.HasSuffix(dst, ".go") {
			if out, err = format.Source(out); err != nil {
				return nil, fmt.Errorf("formatting %s: %w", dst, err)
			}
		}
		files[dst] = out
	}
	return files, nil
}

// goCommand runs the go command in dir, showing its output.
func goCommand(dir string, stderr io.Writer, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Stdout = stderr
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func printNextSteps(w io.Writer, o initOptions) {
	fmt.Fprintln(w, "\nYour bot is ready. Next:")
	if o.dir != "." {
		fmt.Fprintf(w, "\n\tcd %s", o.dir)
	}
	if o.skipDeps {
		fmt.Fprintf(w, "\n\tgo get %s && go mod tidy", libraryPath)
	}
	fmt.Fprint(w, "\n\tTOKEN=<bot token> GUILD=<test server ID> ")
	if o.http {
		fmt.Fprint(w, "PUBLIC_KEY=<app public key> ")
	}
	fmt.Fprint(w, "go run .\n\nSee README.md in the project for more.\n")
}
