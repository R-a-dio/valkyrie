// +build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:generate go run generate.go

const (
	// extension used for output files
	beckyExt = ".gen.go"
)

func main() {
	files, err := filepath.Glob("*.sql")
	if err != nil {
		fmt.Println("glob errored:", err)
		os.Exit(1)
	}

	for _, filename := range files {
		fmt.Printf("embedding %s\n", filename)

		err = execBecky(filename)
		if err != nil {
			fmt.Println("becky errored:", err)
			os.Exit(1)
		}

		// output filename of becky
		filename = filename + beckyExt
		// move the output file to a subdirectory
		err = os.Rename(filename, filepath.Join("embed", filename))
		if err != nil {
			fmt.Println("rename errored:", err)
			os.Exit(1)
		}
	}
}

func execBecky(filename string) error {
	// remove sql extension, keep .down and .up
	variable := strings.TrimSuffix(filename, ".sql")
	// replace .down and .up with _down and _up
	variable = strings.ReplaceAll(variable, ".", "_")
	// add an underscore because Go variables have to start with a letter
	variable = "_" + variable

	cmd := exec.Command("go", "run", "asset.go",
		"-lib=false",
		"-var", variable,
		"-wrap", "migration",
		"--", filename)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
