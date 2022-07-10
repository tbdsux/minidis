/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/TheBoringDude/minidis/cli/minidis/cmd/internal"
	"github.com/spf13/cobra"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a new boilerplate",
	Long: `Generate a new boilerplate
	
It generates / creates the needed boilerplate files to
start a new discord bot easily.`,
	Run: func(cmd *cobra.Command, args []string) {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}

		mod := getModInfo()
		project := &internal.InitProject{
			Path:    wd,
			PkgName: mod.Path,
		}

		if err := project.Writer(); err != nil {
			log.Fatal(err)
		}

		fmt.Println("Succesfully initialized a new project. Please run `go get` to install the required dependencies")
	},
}

type GoModInfo struct {
	Path      string
	Main      bool
	Dir       string
	GoMod     string
	GoVersion string
}

func getModInfo() GoModInfo {
	args := []string{"list", "-json", "-m"}
	output, err := exec.Command("go", args...).Output()
	if err != nil {
		// TODO: show error in getting go.mod
		log.Fatal(err)
	}

	var mod GoModInfo
	if err := json.Unmarshal([]byte(output), &mod); err != nil {
		// TODO: handle error in here
		log.Fatal(err)
	}

	if mod.Path == "command-line-arguments" {
		// go.mod doesn't exist in here
		fmt.Println("\nPlease run `go mod init <MODNAME>` before running the command")
		os.Exit(1)
	}

	return mod
}

func init() {
	rootCmd.AddCommand(initCmd)

}
