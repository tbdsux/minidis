package main

import (
	"bytes"
	"cmp"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"text/template"
)

//go:embed templates
var templateFS embed.FS

// goFloor is the go directive of the minidis module. New projects declare the
// same, so they build with any toolchain that builds minidis.
const goFloor = "1.25.0"

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
	flags.BoolVar(&o.force, "force", false, "overwrite Go files that already exist")
	flags.BoolVar(&o.skipDeps, "skip-deps", false, "do not run go get, go mod tidy and go build")
	flags.Usage = func() {
		fmt.Fprint(stderr, "Usage: minidis init [flags] [directory]\n\n"+
			"Creates a bot project in directory, the current one by default. A go.mod is\n"+
			"created unless the directory is already in a module. Existing Go files are\n"+
			"never overwritten without -force; README.md and .gitignore never are.\n\nFlags:\n")
		flags.PrintDefaults()
	}

	// Accept flags after the directory too, as in "minidis init mybot -http".
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return err
			}
			return fmt.Errorf("%w: %w", errUsage, err) // flag has printed it
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
	Package string // the import path of the bot's main package
	Name    string // the name go build gives the binary
	HTTP    bool
}

// plan is what init will do, worked out before anything is written.
type plan struct {
	dir         string // the project directory, absolute
	modRoot     string // the directory of the go.mod the project belongs to
	modulePath  string
	packagePath string // the import path of the main package in dir
	newModule   bool   // whether go mod init runs in dir
	hasMinidis  bool   // whether the go.mod already requires minidis
}

func initProject(o initOptions, stdout, stderr io.Writer) error {
	p, err := makePlan(o)
	if err != nil {
		return err
	}

	files, err := render(projectData{Package: p.packagePath, Name: binaryName(p.packagePath), HTTP: o.http})
	if err != nil {
		return err
	}

	// Check everything before writing anything, so a refusal changes nothing.
	if err := checkConflicts(p.dir, files, o.force); err != nil {
		return err
	}

	dependency := libraryPath + "@" + dependencyVersion()
	finish := manualSteps(o, p, dependency) // shown if a go command fails

	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}

	if p.newModule {
		if err := goCommand(p.dir, stderr, "mod", "init", "--", p.modulePath); err != nil {
			return err
		}
		if err := goCommand(p.dir, stderr, "mod", "edit", "-go="+goFloor); err != nil {
			return err
		}
	}

	// go get runs before the files are written, so that if it fails, for
	// example offline, nothing but go.mod exists and init can simply be rerun.
	getDeps := !o.skipDeps && !p.hasMinidis
	if getDeps {
		if dependencyVersion() == "latest" {
			fmt.Fprintln(stderr, "warning: this minidis was built from a checkout, so the templates may not match the latest release of", libraryPath)
		}
		if err := goCommand(p.modRoot, stderr, "get", dependency); err != nil {
			return fmt.Errorf("%w\n\nFinish the setup by hand:\n%s", err, finish)
		}
	}

	if err := writeFiles(p.dir, o.dir, files, stdout); err != nil {
		return err
	}

	if !o.skipDeps {
		if err := goCommand(p.modRoot, stderr, "mod", "tidy"); err != nil {
			return fmt.Errorf("%w\n\nFinish the setup by hand:\n%s", err, finish)
		}
		if err := goCommand(p.dir, stderr, "build", "./..."); err != nil {
			return buildError(err, getDeps, relativeTo(o.dir, p))
		}
	}

	printNextSteps(stdout, o, p, dependency)
	return nil
}

// makePlan works out where the project goes and which module it belongs to.
func makePlan(o initOptions) (plan, error) {
	dir, err := filepath.Abs(o.dir)
	if err != nil {
		return plan{}, err
	}
	p := plan{dir: dir, modRoot: dir}

	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return plan{}, fmt.Errorf("%s is a file, not a directory", dir)
	}

	gomod, err := readGoMod(filepath.Join(dir, "go.mod"))
	switch {
	case err == nil:
		if o.module != "" && o.module != gomod.Module.Path {
			return plan{}, fmt.Errorf("%s already has module %q, not %q", filepath.Join(dir, "go.mod"), gomod.Module.Path, o.module)
		}
		p.modulePath = gomod.Module.Path
		p.packagePath = gomod.Module.Path
		p.hasMinidis = gomod.requires(libraryPath)
		return p, nil
	case !errors.Is(err, fs.ErrNotExist):
		return plan{}, err
	}

	// Without -module, a directory inside a module becomes a package of it.
	if o.module == "" {
		root, err := enclosingModule(dir)
		if err != nil {
			return plan{}, err
		}
		if root != "" {
			parent, err := readGoMod(filepath.Join(root, "go.mod"))
			if err != nil {
				return plan{}, err
			}
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return plan{}, err
			}
			p.modRoot = root
			p.modulePath = parent.Module.Path
			p.packagePath = path.Join(parent.Module.Path, filepath.ToSlash(rel))
			p.hasMinidis = parent.requires(libraryPath)
			if err := checkImportPath(p.packagePath); err != nil {
				return plan{}, fmt.Errorf("%s would be package %q of module %s, which is not a valid import path: %w", dir, p.packagePath, parent.Module.Path, err)
			}
			return p, nil
		}
	}

	p.newModule = true
	p.modulePath = cmp.Or(o.module, filepath.Base(dir))
	p.packagePath = p.modulePath
	if err := checkImportPath(p.modulePath); err != nil {
		return plan{}, fmt.Errorf("%q is not a valid module path (%w); choose one with -module, such as -module example.com/mybot", p.modulePath, err)
	}
	return p, nil
}

// goMod is the part of "go mod edit -json" that init uses.
type goMod struct {
	Module struct {
		Path string
	}
	Require []struct {
		Path string
	}
}

func (m goMod) requires(module string) bool {
	return slices.ContainsFunc(m.Require, func(r struct{ Path string }) bool { return r.Path == module })
}

// readGoMod parses a go.mod file with the go command, which understands every
// form it can take.
func readGoMod(file string) (goMod, error) {
	if _, err := os.Stat(file); err != nil {
		return goMod{}, err
	}

	var stderr bytes.Buffer
	cmd := exec.Command("go", "mod", "edit", "-json", file)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return goMod{}, fmt.Errorf("reading %s: %w\n%s", file, err, stderr.Bytes())
	}

	var m goMod
	if err := json.Unmarshal(out, &m); err != nil {
		return goMod{}, fmt.Errorf("reading %s: %w", file, err)
	}
	if m.Module.Path == "" {
		return goMod{}, fmt.Errorf("%s has no module path", file)
	}
	return m, nil
}

// enclosingModule returns the nearest directory above dir that has a go.mod,
// or "" if there is none.
func enclosingModule(dir string) (string, error) {
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		_, err := os.Stat(filepath.Join(d, "go.mod"))
		if err == nil {
			return d, nil
		}
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, fs.ErrPermission) {
			return "", err
		}
		if parent := filepath.Dir(d); parent == d {
			return "", nil
		}
	}
}

var pathElement = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// checkImportPath reports whether path is usable as a module or import path,
// following the rules of the go command closely enough to fail before it does.
func checkImportPath(path string) error {
	if path == "" {
		return errors.New("it is empty")
	}
	elements := strings.Split(path, "/")
	for _, e := range elements {
		switch {
		case e == "":
			return errors.New("it has an empty element")
		case !pathElement.MatchString(e):
			return fmt.Errorf("%q may only use letters, digits and -._~", e)
		case strings.HasPrefix(e, "-"), strings.HasPrefix(e, "."):
			return fmt.Errorf("%q starts with %q", e, e[:1])
		case strings.HasSuffix(e, "."):
			return fmt.Errorf("%q ends with a dot", e)
		}
	}
	switch elements[0] {
	case "std", "cmd", "go", "toolchain":
		return fmt.Errorf("%q is reserved by the go command", elements[0])
	}
	return nil
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// binaryName is the name go build gives the binary of the package at
// importPath: its last element, skipping a major version suffix.
func binaryName(importPath string) string {
	elements := strings.Split(importPath, "/")
	if n := len(elements); n > 1 && majorVersion.MatchString(elements[n-1]) {
		return elements[n-2]
	}
	return elements[len(elements)-1]
}

// keptFiles are never overwritten: README.md is left alone, and .gitignore
// only gets the lines it is missing.
var keptFiles = map[string]bool{"README.md": true, ".gitignore": true}

func checkConflicts(dir string, files map[string][]byte, force bool) error {
	var existing []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		info, err := os.Stat(filepath.Join(dir, name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			// Such as a file named like a directory init needs.
			return fmt.Errorf("cannot create %s: %w", filepath.Join(dir, name), err)
		case info.IsDir():
			return fmt.Errorf("cannot create %s: it is a directory", filepath.Join(dir, name))
		case !keptFiles[name] && !force:
			existing = append(existing, name)
		}
	}
	if len(existing) > 0 {
		return fmt.Errorf("%s already has %s; use -force to overwrite", dir, strings.Join(existing, ", "))
	}
	return nil
}

func writeFiles(dir, shown string, files map[string][]byte, stdout io.Writer) error {
	for _, name := range slices.Sorted(maps.Keys(files)) {
		file := filepath.Join(dir, name)
		content := files[name]
		verb := "created"

		if keptFiles[name] {
			existing, err := os.ReadFile(file)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				return err
			case name == "README.md":
				fmt.Fprintln(stdout, "kept", filepath.Join(shown, name))
				continue
			default:
				missing := missingLines(existing, content)
				if len(missing) == 0 {
					fmt.Fprintln(stdout, "kept", filepath.Join(shown, name))
					continue
				}
				content = append(bytes.TrimRight(existing, "\n"), '\n', '\n')
				content = append(content, missing...)
				verb = "updated"
			}
		}

		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(file, content, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(stdout, verb, filepath.Join(shown, name))
	}
	return nil
}

// missingLines returns the rule lines of want that have does not contain.
func missingLines(have, want []byte) []byte {
	present := map[string]bool{}
	for line := range strings.Lines(string(have)) {
		present[strings.TrimSpace(line)] = true
	}

	var out []byte
	for line := range strings.Lines(string(want)) {
		rule := strings.TrimSpace(line)
		if rule != "" && !strings.HasPrefix(rule, "#") && !present[rule] {
			out = append(out, rule...)
			out = append(out, '\n')
		}
	}
	return out
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

// dependencyVersion is the version of minidis that go get adds: the one this
// command was installed from, so that the templates match its API.
var dependencyVersion = func() string {
	if v := version(); v != "(devel)" {
		return v
	}
	return "latest"
}

func buildError(err error, fetched bool, dir string) error {
	if !fetched || dependencyVersion() != "latest" {
		return fmt.Errorf("the generated project does not build: %w", err)
	}

	checkout := "<path to your minidis checkout>"
	if c := sourceCheckout(); c != "" {
		checkout = shellQuote(runtime.GOOS, c)
	}
	cd := ""
	if dir != "." {
		cd = "\tcd " + shellQuote(runtime.GOOS, dir) + "\n"
	}
	return fmt.Errorf("the generated project does not build against the latest release of %s: %w\n\n"+
		"This minidis was built from a checkout, whose API may be newer. Build against it instead:\n"+
		"%s\tgo mod edit -replace %s=%s\n\tgo mod tidy", libraryPath, err, cd, libraryPath, checkout)
}

// sourceCheckout returns the minidis checkout this command was built from, if
// its sources are still where they were compiled.
func sourceCheckout() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return "" // built with -trimpath
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	if m, err := readGoMod(filepath.Join(root, "go.mod")); err != nil || m.Module.Path != libraryPath {
		return ""
	}
	return filepath.Clean(root)
}

// manualSteps are the commands that finish the setup after a go command
// failed, run from the current directory.
func manualSteps(o initOptions, p plan, dependency string) string {
	var b strings.Builder
	if rel := relativeTo(o.dir, p); rel != "." {
		fmt.Fprintf(&b, "\tcd %s\n", shellQuote(runtime.GOOS, rel))
	}
	if !p.hasMinidis {
		fmt.Fprintf(&b, "\tgo get %s\n", dependency)
	}
	b.WriteString("\tgo mod tidy\n")
	return b.String()
}

// relativeTo is the directory the go commands run in, as the user named it.
func relativeTo(shown string, p plan) string {
	if p.modRoot == p.dir {
		return shown
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p.modRoot); err == nil {
			return rel
		}
	}
	return p.modRoot
}

func printNextSteps(w io.Writer, o initOptions, p plan, dependency string) {
	fmt.Fprintln(w, "\nYour bot is ready. Next:")

	if p.newModule && goWork(p.dir) != "" {
		fmt.Fprintf(w, "\n\tgo work use %s", shellQuote(runtime.GOOS, o.dir))
	}
	if o.dir != "." {
		fmt.Fprintf(w, "\n\tcd %s", shellQuote(runtime.GOOS, o.dir))
	}
	if o.skipDeps {
		if !p.hasMinidis {
			fmt.Fprintf(w, "\n\tgo get %s", dependency)
		}
		fmt.Fprint(w, "\n\tgo mod tidy")
	}

	env := [][2]string{{"TOKEN", "<bot token>"}, {"GUILD", "<test server ID>"}}
	if o.http {
		env = append(env, [2]string{"PUBLIC_KEY", "<app public key>"})
	}
	fmt.Fprintf(w, "\n\t%s\n\nSee README.md in the project for more.\n", runCommand(runtime.GOOS, env))
}

// runCommand is "go run ." with environment variables, in the shell of goos.
func runCommand(goos string, env [][2]string) string {
	var b strings.Builder
	for _, kv := range env {
		if goos == "windows" { // PowerShell
			fmt.Fprintf(&b, "$env:%s=%q; ", kv[0], kv[1])
		} else {
			fmt.Fprintf(&b, "%s=%s ", kv[0], kv[1])
		}
	}
	b.WriteString("go run .")
	return b.String()
}

// goWork returns the go.work file dir is in, if any.
func goWork(dir string) string {
	cmd := exec.Command("go", "env", "GOWORK")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	if w := strings.TrimSpace(string(out)); w != "off" {
		return w
	}
	return ""
}

var (
	shellSafe   = regexp.MustCompile(`^[A-Za-z0-9@%_+=:,./-]+$`)
	windowsSafe = regexp.MustCompile(`^[A-Za-z0-9@%_+=:,./\\-]+$`) // backslashes are separators there
)

// shellQuote quotes s for the shell of goos when it needs it.
func shellQuote(goos, s string) string {
	if goos == "windows" && windowsSafe.MatchString(s) || goos != "windows" && shellSafe.MatchString(s) {
		return s
	}
	// Both PowerShell and POSIX shells take single quotes literally; they
	// differ in how a single quote inside is escaped.
	if goos == "windows" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
