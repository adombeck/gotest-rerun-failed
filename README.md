# gotest-rerun-failed

Rerun failed Go tests.

## Installation

```bash
go install github.com/adombeck/gotest-rerun-failed@latest
```

## Usage

Pipe the output of `go test -json` into `gotest-rerun-failed`:

```bash
go test -json ./... | gotest-rerun-failed
```

Additional arguments are passed to `go test` when rerunning failed tests.
