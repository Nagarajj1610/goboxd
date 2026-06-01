# AI Usage Log

Every significant design decision made during the build of goboxd is logged here.
This file documents what was considered, what was chosen, and why.

---

## [2026-06-01] HTTP framework selection

**Prompt:** Which Go HTTP router to use: net/http (standard library), chi, gin, echo, or fiber?

**Response summary:** Chose `github.com/go-chi/chi/v5`.

**What we used / didn't use:**
- Used `chi`: lightweight, composable middleware, named route parameters, ~800 lines of source. Easy for a beginner to read through.
- Rejected `gin`: adds request binding, validation, rendering — framework-level abstractions that hide what the server is doing from a learner.
- Rejected `echo`, `fiber`: similar objection; more magic, larger surface area.
- Rejected pure `net/http`: no named route parameters, no middleware chain without boilerplate.
- chi was also chosen because it is idiomatic: it implements `http.Handler`, so any chi router can be dropped into a `http.Server` without framework lock-in.

---

## [2026-06-01] Concurrency model selection

**Prompt:** How to bound concurrency: semaphore channel, worker pool with goroutines, goroutine-per-request, or external queue?

**Response summary:** Used a buffered channel semaphore (`make(chan struct{}, maxJobs)`).

**What we used / didn't use:**
- Used buffered channel semaphore: idiomatic Go, ~3 lines of code, zero dependencies. Sending blocks when full (queuing without 429), receiving in defer releases.
- Rejected worker pool (goroutines waiting on a work channel): adds complexity without benefit here because each job's logic is already sequential (build then run).
- Rejected goroutine-per-request with no limit: would allow unbounded nsjail spawning, exhausting system resources under load.
- Rejected external queue (Redis, etc.): massively overengineered for a hackathon service; adds an infrastructure dependency.
- `sync/atomic.Int64` for counters avoids mutexes entirely — reads from `/info` never block writers.

---

## [2026-06-01] nsjail integration approach

**Prompt:** How to invoke nsjail: exec.Command with constructed args, a config file, or a gRPC API?

**Response summary:** `exec.Command(nsjailPath, args...)` with args constructed from the YAML config.

**What we used / didn't use:**
- Used direct `exec.Command`: straightforward, debuggable, no config file format to learn.
- Used `--mode o` (run once) so nsjail exits when the user program exits.
- Used `--time_limit`, `--rlimit_as`, `--rlimit_nproc`, `--rlimit_fsize` to map YAML limits to nsjail flags.
- Rejected nsjail config file (proto format): adds another file format and makes limits harder to derive from YAML.
- Rejected gRPC API: nsjail does have a mode for this but it is complex and unnecessary for our use case.
- Did NOT embed nsjail or use a Go binding — the binary is installed at `/usr/sbin/nsjail` and exec'd.

---

## [2026-06-01] Jail directory management

**Prompt:** How to create unique per-request jail directories: os.MkdirTemp, atomic counter, UUID library, or random hex?

**Response summary:** `os.MkdirTemp(baseDir, "jail-")`.

**What we used / didn't use:**
- Used `os.MkdirTemp`: the OS kernel guarantees uniqueness atomically. No two goroutines can get the same path even at high concurrency. This closes Hole 5.
- Rejected atomic counter: monotonically increasing numbers are predictable. A malicious client that knows the counter value could attempt timing attacks.
- Rejected UUID library: would add a dependency not in the allowed list. `os.MkdirTemp` is strictly better (OS-guaranteed, no library needed).
- Rejected random hex: still possible (though unlikely) to collide; requires crypto/rand for safety; `os.MkdirTemp` is simpler and correct.
- `defer sb.Cleanup()` called immediately after `New` succeeds — this fires even on panic, closing Hole 7.

---

## [2026-06-01] Output capture approach

**Prompt:** How to capture and limit stdout/stderr from nsjail: io.LimitReader, a goroutine that reads and discards after N bytes, pipe with a bytes.Buffer, or os.Pipe with select?

**Response summary:** `io.LimitReader` wrapping stdout/stderr pipes, read into `bytes.Buffer` in goroutines.

**What we used / didn't use:**
- Used `io.LimitReader(pipe, 65536)`: caps reads at exactly 64 KiB per stream. If `io.Copy` returns n == 65536, the limit was hit and we append `"\n[output truncated]"`.
- Rejected reading everything then truncating: would still buffer the full output in memory first, defeating the purpose.
- Rejected goroutine with manual discard loop: more code, same semantics as LimitReader.
- Used separate goroutines for stdout and stderr to avoid deadlock: if stderr fills and we're not reading it while waiting for stdout, the child process blocks on write and the parent deadlocks.
- `context.WithTimeout` wraps the entire phase with an extra 2-second grace period beyond `wall_time_s` to let nsjail's own timeout fire first.

---

## [2026-06-01] YAML structure design

**Prompt:** How to represent placeholder arguments in YAML: Go template syntax `{{.Source}}`, simple string markers `{{source}}`, or positional argument arrays?

**Response summary:** Simple double-brace markers `{{source}}`, `{{artifact}}`, `{{flags}}`.

**What we used / didn't use:**
- Used `{{source}}`, `{{artifact}}`, `{{flags}}`: readable in YAML without escaping. A language maintainer editing the YAML does not need to know Go template syntax.
- Rejected `{{.Source}}` (Go text/template): would require importing `text/template` and executing templates, adding complexity for a simple string replacement.
- Rejected positional arrays: would make the YAML harder to read and the position of each argument would need to be documented separately.
- `{{flags}}` is special: it is **spliced** (expands to zero or more separate arguments), not a single argument. This allows `["-O2", "-Wall"]` to become two separate args to the compiler.

---

## [2026-06-01] Flag validation approach

**Prompt:** Should compiler flags be validated with an allowlist (accept known-good) or a denylist (reject known-bad)?

**Response summary:** Allowlist with exact match + prefix glob, implemented in `validator.ValidateFlags`.

**What we used / didn't use:**
- Used allowlist: the set of dangerous flags is unbounded (every compiler adds new ones). An allowlist only admits flags that have been explicitly reviewed.
- Rejected denylist: `gcc` has thousands of flags. A denylist that misses one dangerous flag (`-fplugin`, `@response_file`, `-specs=`, etc.) is a security hole.
- Added prefix glob support (`"-std=*"` matches `"-std=c++17"`, `"-std=c11"`, etc.) to avoid enumerating every possible standard version.
- Allowlist is per-language (from YAML) so each language only admits its own relevant flags.
- The fix is in `internal/validator/validator.go` so it runs before any process is spawned.
