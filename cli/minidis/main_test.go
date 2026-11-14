package main

import (
	"bytes"
	"errors"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// minidisInit runs "minidis init" with args and returns its output.
func minidisInit(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	err := run(append([]string{"init"}, args...), &out, &out)
	return out.String(), err
}

func requireGo(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is not available")
	}
}

// offline keeps the go commands init runs from reaching the network.
func offline(t *testing.T) {
	t.Helper()

	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOWORK", "off")
}

// goIn runs the go command in dir without reaching the network.
func goIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=mod", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// checkout is the minidis module this test runs in.
func checkout(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// useCheckout points the module in dir at this checkout of minidis.
func useCheckout(t *testing.T, dir string) {
	t.Helper()

	goIn(t, dir, "mod", "edit", "-require="+libraryPath+"@v0.0.0", "-replace="+libraryPath+"="+checkout(t))
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGeneratedProjectsBuild generates each kind of project against this
// checkout of minidis and vets it, so the templates cannot fall behind the API.
func TestGeneratedProjectsBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds generated projects")
	}
	requireGo(t)

	for _, variant := range []string{"gateway", "http"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "botdir")
			args := []string{"-skip-deps", "-module", "example.com/discord-bot/v2", dir}
			if variant == "http" {
				args = append([]string{"-http"}, args...)
			}

			if out, err := minidisInit(t, args...); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}

			useCheckout(t, dir)
			goIn(t, dir, "mod", "tidy")
			goIn(t, dir, "vet", "./...")
			goIn(t, dir, "build")

			// go build names the binary after the module, not the directory.
			if _, err := os.Stat(filepath.Join(dir, "discord-bot")); err != nil {
				t.Fatalf("binary: %v", err)
			}
			if ignore := readFile(t, filepath.Join(dir, ".gitignore")); !strings.Contains(ignore, "/discord-bot\n") {
				t.Errorf(".gitignore does not ignore the binary:\n%s", ignore)
			}

			main := readFile(t, filepath.Join(dir, "main.go"))
			if !strings.HasPrefix(main, "// Command discord-bot ") {
				t.Errorf("main.go starts with %q", main[:30])
			}
			if got := strings.Contains(main, "HTTPHandler"); got != (variant == "http") {
				t.Errorf("main.go uses HTTPHandler = %v", got)
			}

			if goMod := readFile(t, filepath.Join(dir, "go.mod")); !strings.Contains(goMod, "\ngo "+goFloor+"\n") {
				t.Errorf("go.mod does not declare go %s:\n%s", goFloor, goMod)
			}
		})
	}
}

func TestGoFloorMatchesTheLibrary(t *testing.T) {
	requireGo(t)

	if got := goIn(t, ".", "mod", "edit", "-json", filepath.Join(checkout(t), "go.mod")); !strings.Contains(got, `"Go": "`+goFloor+`"`) {
		t.Errorf("goFloor = %s, but minidis declares:\n%s", goFloor, got)
	}
}

// TestInitInsideAModule generates a package of the enclosing module, which
// already requires minidis, so init must not pin it again.
func TestInitInsideAModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a generated project")
	}
	requireGo(t)
	offline(t)

	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, "go.mod"), "module example.com/parent\n\ngo 1.25.0\n")
	useCheckout(t, parent)

	// No -skip-deps: go get would fail offline, so this also shows it is skipped.
	out, err := minidisInit(t, filepath.Join(parent, "cmd", "bot"))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(parent, "cmd", "bot", "go.mod")); err == nil {
		t.Error("a nested module was created")
	}
	if main := readFile(t, filepath.Join(parent, "cmd", "bot", "main.go")); !strings.Contains(main, `"example.com/parent/cmd/bot/commands"`) {
		t.Errorf("main.go does not import the package in the parent module:\n%s", main)
	}
	if goMod := readFile(t, filepath.Join(parent, "go.mod")); !strings.Contains(goMod, libraryPath+" v0.0.0") {
		t.Errorf("minidis was re-pinned:\n%s", goMod)
	}
}

func TestInitCanBeRerunAfterGoGetFails(t *testing.T) {
	requireGo(t)
	offline(t)

	saved := dependencyVersion
	dependencyVersion = func() string { return "v0.0.0-20000101000000-000000000000" }
	t.Cleanup(func() { dependencyVersion = saved })

	dir := filepath.Join(t.TempDir(), "mybot")
	_, err := minidisInit(t, dir)
	if err == nil || !strings.Contains(err.Error(), "go get "+libraryPath+"@v0.0.0-20000101000000-000000000000") {
		t.Fatalf("err = %v, want the manual go get", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "go.mod" {
		t.Fatalf("after the failure the directory has %v, want only go.mod", entries)
	}

	if out, err := minidisInit(t, "-skip-deps", dir); err != nil {
		t.Fatalf("rerun: %v\n%s", err, out)
	}
}

func TestInitNeverOverwritesWithoutForce(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/existing\n\ngo 1.25\n")
	mine := "package main // mine\n"
	writeFile(t, filepath.Join(dir, "main.go"), mine)

	_, err := minidisInit(t, "-skip-deps", dir)
	if err == nil || !strings.Contains(err.Error(), "main.go") || !strings.Contains(err.Error(), "-force") {
		t.Fatalf("err = %v", err)
	}
	if got := readFile(t, filepath.Join(dir, "main.go")); got != mine {
		t.Fatal("main.go was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "commands")); err == nil {
		t.Fatal("files were written although init refused")
	}

	if out, err := minidisInit(t, "-skip-deps", "-force", dir); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if main := readFile(t, filepath.Join(dir, "main.go")); !strings.Contains(main, `"example.com/existing/commands"`) {
		t.Errorf("main.go does not import the existing module's commands:\n%s", main)
	}
}

func TestInitKeepsReadmeAndExtendsGitignore(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/repo\n")
	writeFile(t, filepath.Join(dir, "README.md"), "# My repo\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\nsecrets/\n.env\n")

	// Even -force leaves them alone.
	out, err := minidisInit(t, "-skip-deps", "-force", dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	if got := readFile(t, filepath.Join(dir, "README.md")); got != "# My repo\n" {
		t.Errorf("README.md = %q", got)
	}
	if got, want := readFile(t, filepath.Join(dir, ".gitignore")), "*.log\nsecrets/\n.env\n\n/repo\n/repo.exe\n"; got != want {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}
	if !strings.Contains(out, "kept "+filepath.Join(dir, "README.md")) || !strings.Contains(out, "updated "+filepath.Join(dir, ".gitignore")) {
		t.Errorf("output:\n%s", out)
	}
}

func TestInitRefusesWhereAFileBlocksADirectory(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/x\n")
	writeFile(t, filepath.Join(dir, "commands"), "not a directory\n")

	_, err := minidisInit(t, "-skip-deps", "-force", dir)
	if err == nil || !strings.Contains(err.Error(), "commands") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err == nil {
		t.Error("main.go was written although init failed")
	}
}

func TestInitReadsEveryGoModForm(t *testing.T) {
	requireGo(t)

	tests := map[string]string{
		"trailing comment": "module example.com/a // my bot\n",
		"quoted":           "module \"example.com/a\"\n",
		"block":            "module (\n\texample.com/a\n)\n",
	}

	for name, goMod := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "go.mod"), goMod)

			if out, err := minidisInit(t, "-skip-deps", dir); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if main := readFile(t, filepath.Join(dir, "main.go")); !strings.Contains(main, `"example.com/a/commands"`) {
				t.Errorf("main.go imports:\n%s", main)
			}
		})
	}
}

func TestInitModuleMustMatchAnExistingGoMod(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module \"example.com/a\"\n")

	if _, err := minidisInit(t, "-module", "example.com/b", dir); err == nil || !strings.Contains(err.Error(), "example.com/a") {
		t.Errorf("err = %v", err)
	}
}

func TestInitRejectsInvalidModulePathsBeforeWriting(t *testing.T) {
	for _, module := range []string{"bøt", "foo@v2", "a;b", "[x]", "-dash", "go", "std/x", "a//b", "a.", ".a", "has space"} {
		t.Run(module, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "new")

			_, err := minidisInit(t, "-module", module, dir)
			if err == nil || !strings.Contains(err.Error(), "-module") {
				t.Errorf("err = %v", err)
			}
			if _, err := os.Stat(dir); err == nil {
				t.Error("the directory was created")
			}
		})
	}

	// The default comes from the directory name, which is checked too.
	dir := filepath.Join(t.TempDir(), "my bot")
	if _, err := minidisInit(t, dir); err == nil || !strings.Contains(err.Error(), "-module") {
		t.Errorf("directory name: err = %v", err)
	}
}

func TestInitArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		usage   bool
	}{
		{"two directories", []string{"a", "b"}, "one directory", false},
		{"flags after the directory", []string{"a", "-module", "has space"}, "not a valid module path", false},
		{"unknown flag", []string{"-nope"}, "-nope", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			_, err := minidisInit(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to mention %q", err, tt.wantErr)
			}
			if got := errors.Is(err, errUsage); got != tt.usage {
				t.Errorf("errors.Is(err, errUsage) = %v", got)
			}
		})
	}
}

func TestRenderedGoFilesAreFormatted(t *testing.T) {
	for _, http := range []bool{false, true} {
		files, err := render(projectData{Package: "example.com/x", Name: "x", HTTP: http})
		if err != nil {
			t.Fatal(err)
		}

		for name, content := range files {
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			formatted, err := format.Source(content)
			if err != nil || !bytes.Equal(formatted, content) {
				t.Errorf("%s is not gofmt'd (err %v)", name, err)
			}
		}
		if !bytes.Contains(files[".gitignore"], []byte("/x\n")) {
			t.Errorf(".gitignore = %s", files[".gitignore"])
		}
	}
}

func TestBinaryName(t *testing.T) {
	tests := map[string]string{
		"mybot":                        "mybot",
		"example.com/me/bot":           "bot",
		"github.com/me/discord-bot/v2": "discord-bot",
		"example.com/parent/cmd/bot":   "bot",
		"v2":                           "v2",
	}
	for in, want := range tests {
		if got := binaryName(in); got != want {
			t.Errorf("binaryName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextStepsAreRunnable(t *testing.T) {
	quotes := []struct {
		goos, in, want string
	}{
		{"linux", "mybot", "mybot"},
		{"linux", "my bot", "'my bot'"},
		{"linux", "it's", `'it'\''s'`},
		{"linux", `a\b`, `'a\b'`},
		{"windows", `C:\bots\mybot`, `C:\bots\mybot`},
		{"windows", "my bot", "'my bot'"},
		{"windows", "it's", "'it''s'"},
	}
	for _, q := range quotes {
		if got := shellQuote(q.goos, q.in); got != q.want {
			t.Errorf("shellQuote(%s, %q) = %s, want %s", q.goos, q.in, got, q.want)
		}
	}

	env := [][2]string{{"TOKEN", "<bot token>"}}
	if got := runCommand("linux", env); got != "TOKEN=<bot token> go run ." {
		t.Errorf("linux: %s", got)
	}
	if got := runCommand("windows", env); got != `$env:TOKEN="<bot token>"; go run .` {
		t.Errorf("windows: %s", got)
	}

	// With -skip-deps the go get printed is the pinned one.
	var out bytes.Buffer
	printNextSteps(&out, initOptions{dir: "my bot", skipDeps: true}, plan{}, libraryPath+"@v1.2.3")
	if !strings.Contains(out.String(), "go get "+libraryPath+"@v1.2.3") || !strings.Contains(out.String(), "cd "+shellQuote(runtime.GOOS, "my bot")) {
		t.Errorf("next steps:\n%s", out.String())
	}
}

func TestReleasedVersion(t *testing.T) {
	tests := map[string]string{
		"v1.2.3":                                   "v1.2.3",
		"v0.0.0-20260101000000-abcdef123456":       "v0.0.0-20260101000000-abcdef123456",
		"v0.0.0-20260101000000-abcdef123456+dirty": "(devel)",
		"(devel)": "(devel)",
		"":        "(devel)",
	}
	for in, want := range tests {
		if got := releasedVersion(in); got != want {
			t.Errorf("releasedVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommands(t *testing.T) {
	var out bytes.Buffer

	if err := run([]string{"version"}, &out, &out); err != nil || !strings.HasPrefix(out.String(), "minidis ") {
		t.Errorf("version: %v %q", err, out.String())
	}

	out.Reset()
	if err := run([]string{"nope"}, &out, &out); err == nil || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("unknown command: %v %q", err, out.String())
	}

	out.Reset()
	if err := run(nil, &out, &out); err == nil {
		t.Error("no command was accepted")
	}
}
