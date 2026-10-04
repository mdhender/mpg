// Copyright (c) 2026 Michael D Henderson. All rights reserved.

// Command mpg generates a playable Voronoi province map.
//
// Usage:
//
//	mpg [-version] <command> [flags]
//
// Commands are generate, sweep, render-stage, validate, and version.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mdhender/mpg"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// command is one mpg subcommand.
type command struct {
	name    string
	summary string
	run     func(args []string, stdout, stderr io.Writer) int
}

var commands = []command{
	{"generate", "generate a world from a config", stub("generate")},
	{"sweep", "build a contact sheet across seeds and stages", stub("sweep")},
	{"render-stage", "render one stage of a world", stub("render-stage")},
	{"validate", "validate a generated world", stub("validate")},
	{"version", "print the version", runVersion},
}

// run executes mpg with args (excluding the program name) and returns the
// process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mpg", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() { usage(stderr, fs) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, mpg.Version().String())
		return 0
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "mpg: missing command")
		fs.Usage()
		return 2
	}
	name, rest := fs.Arg(0), fs.Args()[1:]
	for _, c := range commands {
		if c.name == name {
			return c.run(rest, stdout, stderr)
		}
	}
	fmt.Fprintf(stderr, "mpg: unknown command %q\n", name)
	fs.Usage()
	return 2
}

func usage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintln(w, "usage: mpg [-version] <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, c := range commands {
		fmt.Fprintf(w, "  %-14s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "flags:")
	fs.PrintDefaults()
}

// newFlagSet returns a FlagSet for a subcommand that writes to stderr.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("mpg "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// parse parses a subcommand's flags. It reports the exit code and whether the
// caller should stop.
func parse(fs *flag.FlagSet, args []string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, true
		}
		return 2, true
	}
	return 0, false
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("version", stderr)
	if code, stop := parse(fs, args); stop {
		return code
	}
	fmt.Fprintln(stdout, mpg.Version().String())
	return 0
}

// stub returns a subcommand that parses its (as yet empty) flags and reports
// that it is not implemented.
func stub(name string) func(args []string, stdout, stderr io.Writer) int {
	return func(args []string, stdout, stderr io.Writer) int {
		fs := newFlagSet(name, stderr)
		if code, stop := parse(fs, args); stop {
			return code
		}
		fmt.Fprintf(stderr, "mpg %s: not implemented yet\n", name)
		return 1
	}
}
