package main

import (
	"bytes"
	"errors"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
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

	dir := filepath.Join(t.TempDir(), "my bot")
	_, err := minidisInit(t, "-module", "example.com/mybot", dir)
	if err == nil || !strings.Contains(err.Error(), "run init again") ||
		!strings.Contains(err.Error(), "minidis init -module example.com/mybot "+shellQuote(runtime.GOOS, dir)) {
		t.Fatalf("err = %v, want the command to run init again", err)
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

	out, err := minidisInit(t, "-skip-deps", "-force", dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "overwrote "+filepath.Join(dir, "main.go")) || !strings.Contains(out, "created "+filepath.Join(dir, "commands", "hello.go")) {
		t.Errorf("output:\n%s", out)
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
	for _, module := range []string{"bøt", "foo@v2", "a;b", "[x]", "-dash", "go", "std", "toolchain", "a//b", "a.", ".a", "has space"} {
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
		{"two directories", []string{"a", "b"}, "one directory", true},
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
		"example.com/x/v10":            "x",
		"example.com/parent/v1":        "v1",
		"example.com/parent/v0":        "v0",
		"example.com/parent/v01":       "v01",
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
		{"windows", "a,b", "'a,b'"},
		{"windows", "@bot", "'@bot'"},
		{"linux", "a,b", "a,b"},
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
	if strings.Contains(out.String(), "ready") {
		t.Errorf("with -skip-deps the bot is not ready yet:\n%s", out.String())
	}

	if got := rerunCommand("linux", []string{"-http", "my bot"}); got != "minidis init -http 'my bot'" {
		t.Errorf("rerun: %s", got)
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

	// Usage errors exit 2, and main prints nothing more for them.
	out.Reset()
	if err := run([]string{"nope"}, &out, &out); !errors.Is(err, errUsage) || !strings.Contains(out.String(), `unknown command "nope"`) || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("unknown command: %v %q", err, out.String())
	}

	out.Reset()
	if err := run(nil, &out, &out); !errors.Is(err, errUsage) || !strings.Contains(out.String(), "Usage:") {
		t.Errorf("no command: %v %q", err, out.String())
	}
}

func TestBuildVersion(t *testing.T) {
	info := func(version string, settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: libraryPath, Version: version}, Settings: settings}
	}
	revision := debug.BuildSetting{Key: "vcs.revision", Value: "97e43a3dc727"}
	// tool is minidis built as a tool of another module, which is in version
	// control: the settings are that module's.
	tool := func(dep *debug.Module) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main:     debug.Module{Path: "example.com/app", Version: "(devel)"},
			Deps:     []*debug.Module{dep},
			Settings: []debug.BuildSetting{revision},
		}
	}

	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"go install module@version", info("v1.2.3"), "v1.2.3"},
		{"go install of a pseudo-version", info("v0.0.0-20260101000000-abcdef123456"), "v0.0.0-20260101000000-abcdef123456"},
		{"clean checkout, stamped by go build", info("v0.0.0-20261114124625-97e43a3dc727", revision), "(devel)"},
		{"dirty checkout", info("v0.0.0-20261114124625-97e43a3dc727+dirty", revision), "(devel)"},
		{"-buildvcs=false", info("(devel)"), "(devel)"},
		{"another main module", &debug.BuildInfo{Main: debug.Module{Path: "example.com/x", Version: "v1.0.0"}}, "(devel)"},
		{"a tool of another module", tool(&debug.Module{Path: libraryPath, Version: "v1.2.3"}), "v1.2.3"},
		{"a tool with minidis replaced", tool(&debug.Module{Path: libraryPath, Version: "v1.2.3", Replace: &debug.Module{Path: "../minidis"}}), "(devel)"},
		{"a tool at a dirty version", tool(&debug.Module{Path: libraryPath, Version: "v0.0.0-20260101000000-abcdef123456+dirty"}), "(devel)"},
	}
	for _, tt := range tests {
		if got := buildVersion(tt.info); got != tt.want {
			t.Errorf("%s: buildVersion = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"v1.2.3", "v1.2.3", 0, true},
		{"v1.2.3", "v1.10.0", -1, true},
		{"v2.0.0", "v1.99.99", 1, true},
		{"v1.0.0-rc.1", "v1.0.0", -1, true},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1, true},
		{"v1.0.0-alpha.2", "v1.0.0-alpha.10", -1, true},
		{"v1.0.0-1", "v1.0.0-alpha", -1, true},
		{"v2.0.0+incompatible", "v2.0.0", 0, true},
		// Pseudo-versions order by their time.
		{"v0.0.0-20261108111707-9fedc1855fae", "v0.0.0-20261114124625-97e43a3dc727", -1, true},
		{"v1.2.4-0.20260101000000-abcdef123456", "v1.2.3", 1, true},
		{"v1.2.4-0.20260101000000-abcdef123456", "v1.2.4", -1, true},
		{"latest", "v1.0.0", 0, false},
		{"v1.2", "v1.0.0", 0, false},
		{"v01.0.0", "v1.0.0", 0, false},
	}
	for _, tt := range tests {
		got, ok := compareVersions(tt.a, tt.b)
		if got != tt.want || ok != tt.ok {
			t.Errorf("compareVersions(%q, %q) = %d, %v; want %d, %v", tt.a, tt.b, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNeedsGoGet(t *testing.T) {
	const old, pinned, newer = "v0.0.0-20261108111707-9fedc1855fae", "v0.0.0-20261114124625-97e43a3dc727", "v1.0.0"

	tests := []struct {
		name     string
		skipDeps bool
		p        plan
		pinned   string
		want     bool
		from     string
	}{
		{"not required yet", false, plan{}, pinned, true, ""},
		{"skip-deps", true, plan{}, pinned, false, ""},
		{"older requirement is upgraded", false, plan{hasMinidis: true, minidisVersion: old}, pinned, true, old},
		{"newer requirement is kept", false, plan{hasMinidis: true, minidisVersion: newer}, pinned, false, ""},
		{"same requirement is kept", false, plan{hasMinidis: true, minidisVersion: pinned}, pinned, false, ""},
		{"replaced is kept", false, plan{hasMinidis: true, minidisVersion: old, minidisReplaced: true}, pinned, false, ""},
		{"a checkout build never changes it", false, plan{hasMinidis: true, minidisVersion: old}, "latest", false, ""},
	}
	for _, tt := range tests {
		get, from := needsGoGet(tt.skipDeps, tt.p, tt.pinned)
		if get != tt.want || from != tt.from {
			t.Errorf("%s: needsGoGet = %v, %q; want %v, %q", tt.name, get, from, tt.want, tt.from)
		}
	}
}

func TestCheckImportPath(t *testing.T) {
	tests := []struct {
		path   string
		module bool
		ok     bool
	}{
		{"example.com/bot", true, true},
		{"example.com/bot/v2", true, true},
		{"example.com/bot/v10", true, true},
		{"example.com/bot/v0", true, false},
		{"example.com/bot/v1", true, false},
		{"example.com/bot/v02", true, false},
		{"example.com/m/v1", false, true}, // just a directory name in a package path
		{"con", true, false},
		{"example.com/AUX.txt", true, false},
		{"example.com/lpt9", true, false},
		{"example.com/com0", true, true},
		{"example.com/x~1", true, false},
		{"example.com/x~1.go", true, false},
		{"example.com/x~y", true, true},
		{"a+b", true, false},
		{"example.com/m/a+b", false, true},
		{"C", true, false},
		{"example.com/vendor/x", true, false},
		{"example.com/m/vendor/bot", false, false},
		{"vendor", true, false},
		{"go", true, false},
		{"toolchain", true, false},
		{"std", true, false},
		{"cmd", true, false},
		{"go/bot", true, true},
		{"cmd/bot", true, true},
		{"toolchain/bot", true, true},
	}
	for _, tt := range tests {
		if err := checkImportPath(tt.path, tt.module); (err == nil) != tt.ok {
			t.Errorf("checkImportPath(%q, module %v) = %v, want ok %v", tt.path, tt.module, err, tt.ok)
		}
	}
}

func TestInitRejectsIgnoredPackageDirs(t *testing.T) {
	requireGo(t)

	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, "go.mod"), "module example.com/m\n")

	for _, rel := range []string{"_bot", "cmd/_bot", "testdata/bot", ".bot", "con", "vendor", "vendor/bot"} {
		t.Run(rel, func(t *testing.T) {
			dir := filepath.Join(parent, filepath.FromSlash(rel))
			if _, err := minidisInit(t, "-skip-deps", dir); err == nil {
				t.Error("accepted")
			}
			if _, err := os.Stat(dir); err == nil {
				t.Error("the directory was created")
			}
		})
	}
}

func TestInitRefusesADirectoryOfAnotherPackage(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/mod\n")
	writeFile(t, filepath.Join(dir, "mod.go"), "package mod\n")
	writeFile(t, filepath.Join(dir, "mod_test.go"), "package mod_test\n")

	_, err := minidisInit(t, "-skip-deps", dir)
	if err == nil || !strings.Contains(err.Error(), "package mod") || strings.Contains(err.Error(), "mod_test") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err == nil {
		t.Error("main.go was written")
	}

	if out, err := minidisInit(t, "-skip-deps", "-force", dir); err != nil {
		t.Fatalf("-force: %v\n%s", err, out)
	}
}

func TestInitInsideAWorkspace(t *testing.T) {
	requireGo(t)
	offline(t)
	t.Setenv("GOWORK", "") // find the go.work below

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.work"), "go "+goFloor+"\n")

	out, err := minidisInit(t, "-skip-deps", filepath.Join(ws, "bot"))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "go work use") {
		t.Errorf("no go work use hint:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(ws, "bot", "go.mod")); err != nil {
		t.Errorf("go.mod: %v", err)
	}
}

func TestInitNeedsTheGoCommand(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := filepath.Join(t.TempDir(), "bot")
	if _, err := minidisInit(t, dir); err == nil || !strings.Contains(err.Error(), "needs the go command") || strings.HasPrefix(err.Error(), "minidis") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("the directory was created")
	}
}

func TestInitRefusesStandardLibraryNames(t *testing.T) {
	requireGo(t)

	tests := []struct {
		name string
		args func(base string) []string
	}{
		{"directory named time", func(base string) []string { return []string{filepath.Join(base, "time")} }},
		{"directory named fmt", func(base string) []string { return []string{filepath.Join(base, "fmt")} }},
		{"-module net/http", func(base string) []string { return []string{"-module", "net/http", filepath.Join(base, "bot")} }},
		{"-module C", func(base string) []string { return []string{"-module", "C", filepath.Join(base, "bot")} }},
		{"-module with a vendor element", func(base string) []string {
			return []string{"-module", "example.com/vendor/x", filepath.Join(base, "bot")}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			args := append([]string{"-skip-deps"}, tt.args(base)...)
			if _, err := minidisInit(t, args...); err == nil {
				t.Fatal("accepted")
			}
			if entries, _ := os.ReadDir(base); len(entries) != 0 {
				t.Errorf("created %v", entries)
			}
		})
	}
}

func TestInitRefusesAnotherCommandsPackage(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/bot\n")
	writeFile(t, filepath.Join(dir, "commands", "x.go"), "package cmds\n")

	_, err := minidisInit(t, "-skip-deps", dir)
	if err == nil || !strings.Contains(err.Error(), "package cmds") || !strings.Contains(err.Error(), "package commands") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err == nil {
		t.Error("main.go was written")
	}

	if out, err := minidisInit(t, "-skip-deps", "-force", dir); err != nil {
		t.Fatalf("-force: %v\n%s", err, out)
	}
}

func TestInitMergesACRLFGitignore(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/repo\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\r\nsecrets/\r\n")

	if out, err := minidisInit(t, "-skip-deps", dir); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, want := readFile(t, filepath.Join(dir, ".gitignore")), "*.log\r\nsecrets/\r\n\r\n/repo\r\n/repo.exe\r\n.env\r\n"; got != want {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}
}

// TestInitBuildsInAWorkspaceThatDoesNotUseTheModule covers an existing module
// in a go.work tree that does not use it, such as on a rerun after go get
// failed: without GOWORK=off the build check fails.
func TestInitBuildsInAWorkspaceThatDoesNotUseTheModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a generated project")
	}
	requireGo(t)
	offline(t)
	t.Setenv("GOWORK", "") // find the go.work below

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.work"), "go "+goFloor+"\n")
	dir := filepath.Join(ws, "bot")
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/bot\n\ngo "+goFloor+"\n")
	useCheckout(t, dir)

	out, err := minidisInit(t, dir)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "go work use") {
		t.Errorf("no go work use hint:\n%s", out)
	}
}

func TestInitLeavesAWorkspaceModuleAlone(t *testing.T) {
	requireGo(t)
	offline(t)
	t.Setenv("GOWORK", "")

	ws := t.TempDir()
	writeFile(t, filepath.Join(ws, "go.work"), "go "+goFloor+"\n\nuse ./bot\n")
	writeFile(t, filepath.Join(ws, "bot", "go.mod"), "module example.com/bot\n\ngo "+goFloor+"\n")

	out, err := minidisInit(t, "-skip-deps", filepath.Join(ws, "bot"))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "go work use") {
		t.Errorf("hint for a module the workspace uses:\n%s", out)
	}
}

func TestBuildErrorAdvice(t *testing.T) {
	failed := errors.New("go build ./...: exit status 1")

	tests := []struct {
		name    string
		p       plan
		pinned  string
		replace bool // whether the replace advice is given
	}{
		{"devel build", plan{}, "latest", true},
		{"devel build, minidis replaced", plan{minidisReplaced: true}, "latest", false},
		{"released build", plan{}, "v1.2.3", false},
	}
	for _, tt := range tests {
		err := buildError(failed, tt.p, true, tt.pinned, ".")
		if got := strings.Contains(err.Error(), "-replace"); got != tt.replace {
			t.Errorf("%s: replace advice = %v:\n%v", tt.name, got, err)
		}
		if tt.replace && !strings.Contains(err.Error(), "possible cause") {
			t.Errorf("%s: the advice is not presented as a possible cause:\n%v", tt.name, err)
		}
	}
}
