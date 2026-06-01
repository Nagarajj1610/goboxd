# Go Patterns Used in goboxd

---

## Deferred cleanup with stale-orphan sweep

**Context:**
A per-request temporary directory must be removed when the request finishes,
even if the handler panics. Orphans from crashed servers must also be cleaned.

**Pattern:**

```go
// Per-request cleanup via defer:
sb, err := sandbox.New(baseDir, logger)
if err != nil { return err }
defer sb.Cleanup()  // ← called even on panic

// At startup, sweep orphans:
sandbox.SweepStale(baseDir, logger)
```

`Cleanup` calls `os.RemoveAll`. `defer` in Go runs when the enclosing function
returns for any reason, including panics (as long as a `recover` is somewhere
in the call stack, which `chi.Recoverer` provides).

**Where we used it:**
`internal/sandbox/sandbox.go` — `Cleanup`, `SweepStale`
`internal/runner/runner.go` — `Run` (line with `defer sb.Cleanup()`)

---

## Semaphore with atomic counter for /info stats

**Context:**
Bound the number of concurrent nsjail processes and expose the live count in
`GET /info` without locking.

**Pattern:**

```go
// Setup:
sem := make(chan struct{}, maxJobs)
var inFlight atomic.Int64

// Per-request acquire/release:
sem <- struct{}{}         // blocks when full
inFlight.Add(1)
defer func() {
    <-sem
    inFlight.Add(-1)
}()
```

The buffered channel acts as the semaphore. `atomic.Int64` is a lock-free
counter safe for concurrent reads (by `/info`) and writes (by running jobs).

**Where we used it:**
`internal/runner/runner.go` — `Runner`, `Run`
`internal/runner/runner.go` — `Stats` struct

---

## io.LimitReader with truncation marker

**Context:**
Cap child process output at 64 KiB per stream. If the limit is hit, append a
marker so the caller knows the output was cut.

**Pattern:**

```go
const maxOutput = 65536
const marker = "\n[output truncated]"

var buf bytes.Buffer
n, _ := io.Copy(&buf, io.LimitReader(pipe, maxOutput))
if n == maxOutput {
    buf.WriteString(marker)
}
```

`io.LimitReader` returns an `io.Reader` that returns `io.EOF` after N bytes.
`io.Copy` returns the number of bytes copied; if it equals the limit, the
source had more data (the limit was hit).

**Where we used it:**
`internal/runner/runner.go` — `runPhase` (stdout and stderr goroutines)

---

## YAML template placeholder substitution

**Context:**
The YAML `args` list contains markers like `{{source}}` and `{{flags}}`.
These must be replaced with actual paths and flags at request time.

**Pattern:**

```go
func expandArgs(args []string, sourcePath, artifactPath string, flags []string) []string {
    result := make([]string, 0, len(args)+len(flags))
    for _, arg := range args {
        switch arg {
        case "{{source}}":
            result = append(result, sourcePath)
        case "{{artifact}}":
            result = append(result, artifactPath)
        case "{{flags}}":
            result = append(result, flags...)  // splice: one marker → N args
        default:
            result = append(result, arg)
        }
    }
    return result
}
```

The key insight: `{{flags}}` is **spliced** (variadic expansion), not substituted
as a single string. This preserves proper argument separation for the exec call.

**Where we used it:**
`internal/runner/runner.go` — `expandArgs`, `expandRunCmd`
