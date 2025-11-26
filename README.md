# gotest-rerun-failed

Rerun failed Go tests.

## Installation

```bash
go install github.com/adombeck/gotest-rerun-failed@latest
```

## Basic Usage

Pipe the output of `go test -json` into `gotest-rerun-failed`:

```bash
go test -json ./... | gotest-rerun-failed
```

Additional arguments are passed to `go test` when rerunning failed tests.

## Advanced Usage

Combine with [gotestfmt](https://github.com/ubuntu/gotestfmt) to print formatted
test output to stdout:

```bash
go test -json -v ./... 2>&1 | tee /tmp/gotest.log | gotestfmt
gotest-rerun-failed -json < /tmp/gotest.log | gotestfmt
```

Example CI script to retry failed tests up to three times:
```bash
outdir=$(mktemp -d gotest-XXXXXX)
test_output=$outdir/test.json

if ! go test -json ./... | tee "$test_output" | gotestfmt; then
    # Retry the failed tests up to three times
    for i in $(seq 1 3); do
        echo "Retrying failed tests (attempt $i)"
        next_output=$outdir/retry-$i.json

        if gotest-rerun-failed -json < "$test_output" | tee "$next_output" | gotestfmt; then
            break
        fi

        if [ "$i" -eq 3 ]; then
            echo "Tests failed 3 times, giving up"
            exit 1
        fi

        test_output=$next_output
    done
fi
```
