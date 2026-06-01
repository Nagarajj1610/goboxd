# Security

This document lists all 7 security holes that goboxd closes, with the exact
file and line references where each fix is implemented.

---

## Hole 1 — Path traversal via source_filename

**Description:**
If a caller can set `source_filename` to `"../etc/passwd"` or `"/etc/passwd"`,
the server would write the source file outside the jail directory. This gives
an attacker read/write access to arbitrary host filesystem paths.

**Where fixed:**
`internal/validator/validator.go` — function `ValidateFilename` (line ~18)

**How fixed:**
- Reject any filename containing `/` or `\`
- Reject any filename starting with `.`
- Reject any filename containing `..`
- Reject absolute paths (starting with `/` or `\`)
- Enforce a maximum length of 64 characters
- Return HTTP 400 with error code `invalid_filename`

**Test cases covered:**
- `"../etc/passwd"` → rejected
- `"/etc/passwd"` → rejected
- `"foo/bar.py"` → rejected
- `".hidden"` → rejected
- 65-character string → rejected
- `"solution.py"` → accepted

---

## Hole 2 — Shell commands for directory operations

**Description:**
Using `exec.Command("sh", "-c", "mkdir ...")` to create jail directories is
dangerous: any shell metacharacter in a path component could lead to command
injection. It also makes the code harder to audit.

**Where fixed:**
`internal/sandbox/sandbox.go` — functions `New`, `Cleanup`, `WriteFile`, `SweepStale`

**How fixed:**
- All directory creation uses `os.MkdirAll()`
- All directory removal uses `os.RemoveAll()`
- All path construction uses `filepath.Join()`, never string concatenation
- No `exec.Command("sh", ...)` appears anywhere in the sandbox package

---

## Hole 3 — Compiler flag injection

**Description:**
If a caller can pass arbitrary compiler flags like `"-fplugin=evil.so"` or
`"@response_file"`, they can load arbitrary code into the compiler process or
redirect compiler input to host files.

**Where fixed:**
`internal/validator/validator.go` — function `ValidateFlags` (line ~60)

**How fixed:**
- Each flag is checked against the language's `flag_allowlist` from YAML
- Allowlist supports exact match and prefix glob (entries ending in `*`)
- Any flag not in the allowlist → HTTP 400, code `disallowed_flag`
- Interpreted languages (no `Build` section) reject all flags

**Test cases covered:**
- `"-fplugin=evil"` → rejected
- `"@response_file"` → rejected
- `"-Wl,evil"` → rejected
- `"--specs=evil"` → rejected
- `"-O2"` (in cpp allowlist) → accepted

---

## Hole 4 — Request size limits

**Description:**
Without size limits, a single request could exhaust server memory or disk:
a 100 MB source file, 10,000 test cases, or multi-GB stdin.

**Where fixed:**

1. `internal/handler/run.go` — `ServeHTTP` (line ~26):
   `http.MaxBytesReader(w, r.Body, 1<<20)` — 1 MiB hard cap on request body

2. `internal/validator/validator.go` — `ValidateRunRequest` (line ~85):
   - Source max: 262144 bytes (256 KiB)
   - Tests max: 50
   - Stdin per test max: 65536 bytes (64 KiB)

**Error codes:**
- Body > 1 MiB → `request_too_large` (HTTP 400)
- Source > 256 KiB → `source_too_large` (HTTP 400)
- Tests > 50 → `too_many_tests` (HTTP 400)
- Stdin > 64 KiB → `stdin_too_large` (HTTP 400)

---

## Hole 5 — UID collision in jail directory naming

**Description:**
If jail directories are named with a rolling counter or hand-rolled random
number, two concurrent requests could collide and share a jail directory.
One request's source code could then read or overwrite another's.

**Where fixed:**
`internal/sandbox/sandbox.go` — function `New` (line ~35)

**How fixed:**
- `os.MkdirTemp(baseDir, "jail-")` is called for every request
- The OS kernel guarantees the returned path is unique and did not exist before
- No custom UID, no atomic counter, no random number: the OS handles it

---

## Hole 6 — Unbounded child process output

**Description:**
A malicious program that prints an infinite loop would cause the server to
buffer unbounded output in memory until the process is killed or the server
runs out of RAM.

**Where fixed:**
`internal/runner/runner.go` — function `runPhase` (line ~155)

**How fixed:**
- `io.LimitReader(stdoutPipe, 65536)` caps stdout at 64 KiB
- `io.LimitReader(stderrPipe, 65536)` caps stderr at 64 KiB
- If exactly 65536 bytes were read (the limit was hit), the constant
  `"\n[output truncated]"` is appended to the buffer
- Output beyond the limit is discarded by the OS kernel pipe buffer

**Test cases covered (unit tests):**
- Output exactly 65536 bytes → no truncation marker
- Output 65537 bytes → truncation marker present

---

## Hole 7 — Stale jail directories

**Description:**
If the server crashes mid-request (panic, OOM kill, SIGKILL), the `defer
sb.Cleanup()` never runs and the jail directory is left on disk. Over time,
these orphans consume disk space and could contain sensitive source code.

**Where fixed (two places):**

1. `internal/runner/runner.go` — function `Run` (line ~105):
   `defer sb.Cleanup()` is called immediately after `sandbox.New` succeeds.
   Go's `defer` runs even if the function panics.

2. `internal/sandbox/sandbox.go` — function `SweepStale` (line ~80):
   Called at server startup. Reads the jail base directory, finds any
   subdirectory named `jail-*` with `ModTime` older than 10 minutes, and
   removes it with `os.RemoveAll`. Each removal is logged with the directory
   name and age.

3. `main.go` — `main()` (startup):
   `sandbox.SweepStale(jailDir, logger)` is called before the HTTP server
   starts accepting connections.
