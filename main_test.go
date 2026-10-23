package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if captureDir := os.Getenv("GOTEST_RERUN_FAILED_TEST_CAPTURE"); captureDir != "" {
		args, err := json.Marshal(os.Args[1:])
		if err == nil {
			err = os.WriteFile(filepath.Join(captureDir, filepath.Base(os.Args[2])+".json"), args, 0o600)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		exitCode, err := strconv.Atoi(os.Getenv("GOTEST_RERUN_FAILED_TEST_EXIT"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(exitCode)
	}
	os.Exit(m.Run())
}

func TestRerunFailed(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "gotest-rerun-failed")
	build := exec.Command("go", "build", "-o", tool, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build retry tool: %v\n%s", err, output)
	}

	fakeGoDir := t.TempDir()
	fakeGoName := "go"
	if runtime.GOOS == "windows" {
		fakeGoName += ".exe"
	}
	testBinary, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeGoDir, fakeGoName), testBinary, 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		input    string
		want     map[string][]string
		goExit   int
		wantExit int
	}{
		{
			name: "package timeout without individual failures",
			input: `{"Action":"run","Package":"example/timeout","Test":"TestLogin"}
{"Action":"fail","Package":"example/timeout"}`,
			want: map[string][]string{"timeout": {"test", "example/timeout"}},
		},
		{
			name: "package failure with empty test name",
			input: `{"Action":"fail","Package":"example/crash","Test":""}
{"Action":"fail","Package":"example/crash"}`,
			want: map[string][]string{"crash": {"test", "example/crash"}},
		},
		{
			name: "ordinary subtest failure",
			input: `{"Action":"fail","Package":"example/named","Test":"TestLogin/subtest"}
{"Action":"fail","Package":"example/named","Test":"TestLogin"}
{"Action":"fail","Package":"example/named"}`,
			want: map[string][]string{"named": {"test", "example/named", "-run", "^TestLogin/subtest$"}},
		},
		{
			name: "mixed package and subtest failures",
			input: `{"Action":"fail","Package":"example/timeout"}
{"Action":"fail","Package":"example/named"}
{"Action":"fail","Package":"example/named","Test":"TestLogin"}
{"Action":"pass","Package":"example/passed"}`,
			want: map[string][]string{
				"timeout": {"test", "example/timeout"},
				"named":   {"test", "example/named", "-run", "^TestLogin$"},
			},
		},
		{
			name:     "package retry failure propagates",
			input:    `{"Action":"fail","Package":"example/timeout"}`,
			want:     map[string][]string{"timeout": {"test", "example/timeout"}},
			goExit:   42,
			wantExit: 1,
		},
		{
			name:  "successful input does not rerun tests",
			input: `{"Action":"pass","Package":"example/passed","Test":"TestLogin"}`,
			want:  map[string][]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			captureDir := t.TempDir()
			flags := []string{"-json", "-timeout", "30m", "-cover"}
			cmd := exec.Command(tool, flags...)
			cmd.Stdin = strings.NewReader(tc.input)
			cmd.Env = append(os.Environ(),
				"PATH="+fakeGoDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"GOTEST_RERUN_FAILED_TEST_CAPTURE="+captureDir,
				"GOTEST_RERUN_FAILED_TEST_EXIT="+strconv.Itoa(tc.goExit),
			)
			output, err := cmd.CombinedOutput()
			var exitError *exec.ExitError
			if err != nil && !errors.As(err, &exitError) {
				t.Fatalf("run retry tool: %v", err)
			}
			if got := cmd.ProcessState.ExitCode(); got != tc.wantExit {
				t.Errorf("exit code = %d, want %d\n%s", got, tc.wantExit, output)
			}

			files, err := os.ReadDir(captureDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != len(tc.want) {
				t.Errorf("retried %d packages, want %d\n%s", len(files), len(tc.want), output)
			}
			for pkg, args := range tc.want {
				data, err := os.ReadFile(filepath.Join(captureDir, pkg+".json"))
				if err != nil {
					t.Errorf("missing retry for %s: %v\n%s", pkg, err, output)
					continue
				}
				var got []string
				if err := json.Unmarshal(data, &got); err != nil {
					t.Fatal(err)
				}
				want := append(append([]string{}, args...), flags...)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("retry arguments = %q, want %q", got, want)
				}
			}
		})
	}
}
