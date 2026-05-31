package cli

import (
	"flag"
	"fmt"
	"io"

	"ilonasin/internal/app"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "manage":
		return runManage(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		usage(stderr)
		return 2
	}
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to config.toml")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := app.Options{ConfigPath: *configPath, Stdout: stdout, Stderr: stderr}
	if err := app.Serve(opts); err != nil {
		fmt.Fprintf(stderr, "serve failed: %v\n", err)
		return 1
	}
	return 0
}

func runManage(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manage", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to config.toml")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := app.Options{ConfigPath: *configPath, Stdout: stdout, Stderr: stderr}
	if err := app.Manage(opts); err != nil {
		fmt.Fprintf(stderr, "manage failed: %v\n", err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: ilonasin <serve|manage> [--config path]")
}
