// Command kite-plugin provides developer tools for creating, building,
// validating, and packaging Kite plugins.
package main

import (
	"fmt"
	"os"
)

const usage = `kite-plugin — Kite Plugin Developer CLI

Usage:
  kite-plugin <command> [options]

Commands:
  init <name>  [--with-frontend]  Create a new plugin project
  build                           Build plugin binary (and frontend if present)
  validate                        Validate manifest.yaml and structure
  package                         Package plugin as .tar.gz for distribution

Options:
  -h, --help   Show this help message

Examples:
  kite-plugin init my-plugin --with-frontend
  kite-plugin build
  kite-plugin validate
  kite-plugin package
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		if err := runInit(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "build":
		if err := runBuild(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		if err := runValidate(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "package":
		if err := runPackage(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
}
