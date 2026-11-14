package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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

// goIn runs the go command in dir without reaching the network.
func goIn(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// TestGeneratedProjectsBuild generates each kind of project against this
// checkout of minidis and vets it, so the templates cannot fall behind the API.
func TestGeneratedProjectsBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds generated projects")
	}
	requireGo(t)

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	for _, variant := range []string{"gateway", "http"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), "mybot")
			args := []string{"-skip-deps", "-module", "example.com/mybot", dir}
			if variant == "http" {
				args = append([]string{"-http"}, args...)
			}

			if out, err := minidisInit(t, args...); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}

			goIn(t, dir, "mod", "edit", "-require="+libraryPath+"@v0.0.0", "-replace="+libraryPath+"="+root)
			goIn(t, dir, "mod", "tidy")
			goIn(t, dir, "vet", "./...")

			main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
			if got := strings.Contains(string(main), "HTTPHandler"); got != (variant == "http") {
				t.Errorf("main.go uses HTTPHandler = %v", got)
			}
		})
	}
}

func TestInitNeverOverwritesWithoutForce(t *testing.T) {
	requireGo(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/existing\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := []byte("package main // mine\n")
	if err := os.WriteFile(filepath.Join(dir, "main.go"), mine, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := minidisInit(t, "-skip-deps", dir)
	if err == nil || !strings.Contains(err.Error(), "main.go") || !strings.Contains(err.Error(), "-force") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "main.go")); !bytes.Equal(got, mine) {
		t.Fatal("main.go was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "commands")); err == nil {
		t.Fatal("files were written although init refused")
	}

	if out, err := minidisInit(t, "-skip-deps", "-force", dir); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(main), `"example.com/existing/commands"`) {
		t.Errorf("main.go does not import the existing module's commands:\n%s", main)
	}
}

func TestInitModuleMustMatchAnExistingGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module \"example.com/a\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := minidisInit(t, "-module", "example.com/b", dir); err == nil || !strings.Contains(err.Error(), "example.com/a") {
		t.Errorf("err = %v", err)
	}
}

func TestInitArguments(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"two directories", []string{"a", "b"}, "one directory"},
		{"flags after the directory", []string{"a", "-module", "has space"}, "not a valid module path"},
		{"unknown flag", []string{"-nope"}, "-nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if _, err := minidisInit(t, tt.args...); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestRenderedGoFilesAreFormatted(t *testing.T) {
	for _, http := range []bool{false, true} {
		files, err := render(projectData{Module: "example.com/x", Name: "x", HTTP: http})
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.HasPrefix(files["main.go"], []byte("// Command x")) {
			t.Errorf("main.go starts with %q", files["main.go"][:20])
		}
		if !bytes.Contains(files[".gitignore"], []byte("/x\n")) {
			t.Errorf(".gitignore = %s", files[".gitignore"])
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
