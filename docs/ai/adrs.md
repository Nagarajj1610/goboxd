# Architectural Decision Records

---

## HTTP framework selection

**Context:**
goboxd needs an HTTP server with named route parameters (`/run`, `/healthz`, etc.)
and request middleware (body size limiting, panic recovery). The standard library
`net/http` alone lacks these.

**Options considered:**
1. `net/http` — no route parameters, no middleware chain without boilerplate
2. `chi` — lightweight, idiomatic, implements `http.Handler`, ~800 lines of source
3. `gin` — full framework with binding, validation, rendering
4. `echo` / `fiber` — similar to gin in scope

**Decision:** `github.com/go-chi/chi/v5`

**Rationale:**
chi adds only what is missing from `net/http` (route parameters and composable
middleware) without adding framework-level abstractions. A beginner reading the
code can trace a request through chi into the handler without needing to understand
gin's context or binding internals. chi also implements the standard `http.Handler`
interface, so there is no framework lock-in.

---

## Concurrency model: semaphore vs worker pool vs goroutine-per-request

**Context:**
nsjail processes are expensive to spawn. Allowing unbounded concurrent nsjail
processes would exhaust system resources. The spec requires queuing (block, not
return 429) when at the concurrency limit.

**Options considered:**
1. Goroutine-per-request with no limit — simple, but unbounded
2. Worker pool — a fixed set of goroutines each pulling from a work channel
3. Buffered channel semaphore — `make(chan struct{}, N)`; send to acquire, receive to release
4. External queue (Redis, etc.) — persistent, distributable, complex

**Decision:** Buffered channel semaphore

**Rationale:**
The semaphore pattern is the canonical Go idiom for this problem. It requires
~3 lines of code, no external dependencies, and correctly queues callers when
full. A worker pool adds complexity (work channel, result channel, multiple
goroutine lifetimes) with no benefit here since each job is a sequential
build-then-run pipeline, not a parallel task farm. External queues are appropriate
for distributed systems, not a single-binary service.

---

## Jail directory naming: os.MkdirTemp vs atomic counter vs UUID

**Context:**
Each request needs an isolated directory. Two concurrent requests must never get
the same path.

**Options considered:**
1. Atomic counter: `fmt.Sprintf("jail-%d", counter.Add(1))`
2. UUID library (e.g. `github.com/google/uuid`)
3. `os.MkdirTemp(baseDir, "jail-")` — OS-provided unique temp directory
4. Random hex from `crypto/rand`

**Decision:** `os.MkdirTemp`

**Rationale:**
`os.MkdirTemp` calls the kernel's tempfile mechanism, which guarantees atomicity:
the directory is created and named in a single syscall, so no two callers can
race to the same name. An atomic counter is predictable (a client could guess
the next name). UUID requires an external library (not in the allowed dependency
list). Random hex requires `crypto/rand` and manual uniqueness verification.
`os.MkdirTemp` is strictly correct, requires no dependencies, and is the
idiomatic Go solution.

---

## Flag validation strategy: allowlist vs denylist

**Context:**
Compilers accept hundreds of flags. Some flags are dangerous: `-fplugin=evil.so`
loads a shared library into the compiler, `@file` redirects input, `-specs=file`
overrides the compiler's built-in specs.

**Options considered:**
1. Allowlist: explicitly list every flag that is permitted
2. Denylist: explicitly list every flag that is forbidden
3. Regex pattern matching on flag prefixes

**Decision:** Allowlist with exact match and prefix glob (entries ending in `*`)

**Rationale:**
A denylist is fundamentally incomplete: each compiler release may add new flags,
and an attacker who knows a flag is missing from the denylist can exploit it
immediately. An allowlist inverts this: the attacker must find a dangerous flag
that is explicitly on the permitted list, which is much harder. We add prefix
globs (e.g. `-std=*`) to avoid enumerating every possible standard version while
keeping the allowlist tight.

---

## Output capture strategy: LimitReader vs pipe with goroutine

**Context:**
A malicious program could print infinite output, exhausting server memory.
We need to cap stdout and stderr at 64 KiB each.

**Options considered:**
1. Read all output then truncate: still buffers everything first
2. `io.LimitReader` wrapping the pipe: stops reading after N bytes
3. A goroutine that reads N bytes then discards the rest
4. `os.Pipe` with a polling `select` loop

**Decision:** `io.LimitReader` wrapping `proc.StdoutPipe()` and `proc.StderrPipe()`

**Rationale:**
`io.LimitReader` is the standard library's purpose-built solution. It stops
reading after N bytes — any further output goes into the OS pipe buffer and is
discarded when the process exits. The implementation is 1 line per stream. We
use separate goroutines for stdout and stderr to prevent deadlock: if stderr
fills its kernel buffer and we aren't reading it while reading stdout, the child
process blocks on `write(2)` and the parent waits forever for the process to exit.
