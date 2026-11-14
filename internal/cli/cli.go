// Package cli: dispatch subcommand ccwatch dan parsing flag.
package cli

import (
	"fmt"
	"os"

	"ccwatch/internal/demo"
	"ccwatch/internal/hook"
	"ccwatch/internal/statusline"
)

const Version = "0.1.0"

type command func(args []string) error

var commands = map[string]command{
	"watch":      runWatch,
	"hook":       func([]string) error { hook.Run(os.Stdin); return nil },
	"statusline": func([]string) error { statusline.Run(os.Stdin, os.Stdout); return nil },
	"install":    runInstall,
	"uninstall":  runUninstall,
	"rules":      runRules,
	"demo":       demo.Run,
	"version":    func([]string) error { fmt.Println("ccwatch", Version); return nil },
	"help":       func([]string) error { fmt.Println(usage); return nil },
}

// splitCommand memisahkan subcommand dari argumen. Tanpa subcommand (kosong atau
// diawali "-") artinya `watch`; --version/-v adalah jalan pintas ke `version`.
func splitCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "watch", nil
	}
	switch first := args[0]; {
	case first == "--version" || first == "-v":
		return "version", args[1:]
	case first != "" && first[0] != '-':
		return first, args[1:]
	}
	return "watch", args
}

// Run menjalankan ccwatch dan mengembalikan exit code: 0 sukses, 1 gagal, 2 perintah tidak dikenal.
func Run(args []string) int {
	name, rest := splitCommand(args)
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintln(os.Stderr, "perintah tidak dikenal:", name)
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	if err := cmd(rest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
