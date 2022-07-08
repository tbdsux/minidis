package internal

import (
	"bytes"
	"os"
	"text/template"

	"github.com/TheBoringDude/minidis/cli/minidis/templ"
	simplefiletest "github.com/TheBoringDude/simple-filetest"
)

func (i *InitProject) Writer() error {

	mainGo := template.Must(template.New("main.go").Parse(templ.MainGoTemplate))
	rootGo := template.Must(template.New("root.go").Parse(templ.RootGoTemplate))

	// write main.go file
	var mainBuf bytes.Buffer
	mainGo.ExecuteTemplate(&mainBuf, "main.go", i)
	if err := os.WriteFile("main.go", mainBuf.Bytes(), 0644); err != nil {
		return err
	}

	// write commands/root.go file
	var rootBuf bytes.Buffer
	if !simplefiletest.DirExists("commands") {
		if err := os.Mkdir("commands", 0755); err != nil {
			return err
		}
	}
	rootGo.ExecuteTemplate(&rootBuf, "root.go", i)
	if err := os.WriteFile("commands/root.go", rootBuf.Bytes(), 0644); err != nil {
		return err
	}

	// write commands/hello.go file
	if err := os.WriteFile("commands/hello.go", []byte(templ.HelloCmdGoTemplate), 0644); err != nil {
		return err
	}

	// write lib/env.go file
	if !simplefiletest.DirExists("lib") {
		if err := os.Mkdir("lib", 0755); err != nil {
			return err
		}
	}
	if err := os.WriteFile("lib/env.go", []byte(templ.LibEnvGoTemplate), 0644); err != nil {
		return err
	}

	return nil
}
