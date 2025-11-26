package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
)

type event struct {
	Action  string `json:"Action"`
	Test    string `json:"Test"`
	Package string `json:"Package"`
}

func parent(s string) string {
	if i := strings.LastIndex(s, "/"); i != -1 {
		return s[:i]
	}
	return s
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "Usage: go test -json ./... | gotest-rerun-failed [args...]")
	_, _ = fmt.Fprintln(os.Stderr, "\nReads JSON output from 'go test -json' on stdin, identifies failed tests, and reruns them.")
	_, _ = fmt.Fprintln(os.Stderr, "Additional arguments are passed to 'go test' when rerunning failed tests.")
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		usage()
		os.Exit(0)
	}

	// If stdin is a terminal (no pipe/redirection), print usage and exit
	if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		usage()
		os.Exit(2)
	}

	scanner := bufio.NewScanner(os.Stdin)

	// Parse the JSON test output
	failed := map[string]map[string]bool{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()

		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Skipping invalid JSON on line %d: %v\n", lineNo, err)
			continue
		}

		if e.Action != "fail" || e.Test == "" {
			continue
		}

		if failed[e.Package] == nil {
			failed[e.Package] = map[string]bool{}
		}
		failed[e.Package][e.Test] = true

		if strings.Contains(e.Test, "/") {
			// It's a subtest, so we don't have to rerun its parent
			delete(failed[e.Package], parent(e.Test))
		}
	}

	if err := scanner.Err(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
		os.Exit(1)
	}

	if len(failed) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "No failed tests found")
		os.Exit(0)
	}
	_, _ = fmt.Fprintln(os.Stderr, "Rerunning failed tests:")

	// Sort packages for deterministic output
	pkgs := make([]string, 0, len(failed))
	for p := range failed {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)

	// Construct commands for each package
	var cmds []*exec.Cmd
	for _, pkg := range pkgs {
		testsMap := failed[pkg]
		if len(testsMap) == 0 {
			continue
		}

		// Build the -run regex with sorted entries for deterministic output
		tests := make([]string, 0, len(testsMap))
		for t := range testsMap {
			tests = append(tests, t)
		}
		sort.Strings(tests)

		escaped := make([]string, 0, len(tests))
		for _, t := range tests {
			escaped = append(escaped, "^"+regexp.QuoteMeta(t)+"$")
		}
		runArg := strings.Join(escaped, "|")

		// Construct command args: go test <package> -run <regex> [other args...]
		cmdArgs := append([]string{"test", pkg, "-run", runArg}, os.Args[1:]...)
		cmd := exec.Command("go", cmdArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		_, _ = fmt.Fprintln(os.Stderr, cmd.String())
		cmds = append(cmds, cmd)
	}

	// Run the commands in parallel
	errs := make(chan error, len(cmds))
	var wg sync.WaitGroup
	for _, c := range cmds {
		wg.Add(1)
		go func(cmd *exec.Cmd) {
			defer wg.Done()
			errs <- cmd.Run()
		}(c)
	}

	// Wait for all commands to finish
	wg.Wait()
	close(errs)

	// Exit with non-zero status if any command failed
	for err := range errs {
		if err != nil {
			os.Exit(1)
		}
	}
}
