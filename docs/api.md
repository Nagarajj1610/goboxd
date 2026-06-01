# API Reference

## Endpoints

### POST /run

Execute source code against one or more test cases.

**Request body** (JSON):

```json
{
  "language": "cpp",
  "source": "#include<iostream>\nint main(){std::cout<<\"hi\"<<std::endl;}",
  "source_filename": "solution.cpp",
  "artifact_filename": "solution",
  "build": {
    "limits": { "wall_time_s": 5, "memory_kb": 1048576, "max_processes": 100 },
    "flags": ["-O2"]
  },
  "run": {
    "limits": { "wall_time_s": 3, "memory_kb": 524288, "max_processes": 64 },
    "flags": []
  },
  "tests": [
    { "stdin": "1\n", "expected_stdout": "hi\n" }
  ]
}
```

Field notes:
- `language` — required. One of the ids in `configs/languages.yaml`.
- `source` — required. Max 256 KiB.
- `source_filename` — optional for most languages. Required for Java (must match public class name).
- `artifact_filename` — optional for most languages. Required for Java (e.g. `"Main"`).
- `build.flags` — must all be in the language's `flag_allowlist`.
- `tests` — max 50 items. Each `stdin` max 64 KiB.

**Response** — always HTTP 200 if execution ran (even if code crashed):

```json
{
  "status": "wrong_output",
  "build": {
    "status": "ok",
    "stdout": "",
    "stderr": "",
    "duration_ms": 412
  },
  "tests": [
    {
      "status": "wrong_output",
      "stdout": "HI\n",
      "stderr": "",
      "duration_ms": 38,
      "memory_peak_kb": 8192
    }
  ]
}
```

**Error response** (HTTP 400 for bad input):

```json
{
  "error": {
    "code": "invalid_filename",
    "message": "source_filename must be a single path component"
  }
}
```

---

### GET /healthz

Always returns 200. Used by load balancers to check that the process is alive.
Does not check language toolchains.

```json
{ "status": "ok" }
```

---

### GET /readyz

Actively probes nsjail and every language toolchain. Returns 200 if all pass,
503 if any fail.

**200 response:**
```json
{
  "status": "ok",
  "nsjail": { "ok": true, "version": "3.4" },
  "languages": {
    "py3": { "ok": true, "version": "Python 3.11.2" },
    "cpp": { "ok": true, "version": "g++ (Ubuntu 11.4.0) 11.4.0" }
  }
}
```

**503 response (degraded):**
```json
{
  "status": "degraded",
  "nsjail": { "ok": true, "version": "3.4" },
  "languages": {
    "py3": { "ok": true, "version": "Python 3.11.2" },
    "cpp": { "ok": false, "error": "g++ not found at /usr/bin/g++" }
  }
}
```

---

### GET /info

Returns server metadata, language list with live version checks, and stats.
Always 200.

```json
{
  "build_info": {
    "version": "0.1.0",
    "commit": "abc1234",
    "go_version": "go1.22.3"
  },
  "nsjail": {
    "path": "/usr/sbin/nsjail",
    "version": "3.4"
  },
  "languages": [
    {
      "id": "py3",
      "name": "Python 3",
      "version": "Python 3.11.2",
      "default_run_limits": {
        "wall_time_s": 9,
        "memory_kb": 102400,
        "max_processes": 100
      }
    }
  ],
  "limits": {
    "max_source_bytes": 262144,
    "max_tests": 50,
    "max_concurrent_jobs": 4
  },
  "stats": {
    "in_flight_jobs": 0,
    "jobs_total": 41892,
    "jobs_failed_internal": 4,
    "last_internal_error_at": "2026-06-01T11:22:09Z",
    "disk_free_bytes_jail_dir": 53687091200
  }
}
```

---

## Status values

### Top-level `status`

| Value | Meaning |
|---|---|
| `accepted` | Build ok AND every test accepted |
| `build_failed` | Compilation error |
| `wrong_output` | At least one test had wrong output |
| `output_whitespace_mismatch` | Output matches after trimming whitespace |
| `time_exceeded` | At least one test hit the wall time limit |
| `memory_exceeded` | At least one test hit the memory limit |
| `runtime_error` | At least one test exited non-zero |
| `internal_error` | Server-side failure (nsjail not found, disk full, etc.) |

Top-level status = first non-`accepted` test status in array order.
If build fails, top-level = `build_failed`.

### `build.status`

| Value | Meaning |
|---|---|
| `ok` | Compilation succeeded (exit 0) |
| `failed` | Compilation failed (exit non-zero) |
| `internal_error` | Server could not run the compiler |

### `tests[i].status`

| Value | Meaning |
|---|---|
| `accepted` | stdout matches expected exactly |
| `wrong_output` | stdout does not match |
| `output_whitespace_mismatch` | matches after TrimSpace |
| `time_exceeded` | hit wall_time_s limit |
| `memory_exceeded` | hit memory_kb limit |
| `runtime_error` | process exited non-zero |
| `not_executed` | build failed; this test was skipped |
| `internal_error` | server-side failure on this test |

---

## Error codes

| Code | HTTP | Meaning |
|---|---|---|
| `invalid_json` | 400 | Request body is not valid JSON |
| `request_too_large` | 400 | Body exceeds 1 MiB |
| `unknown_language` | 400 | Language id not in registry |
| `invalid_filename` | 400 | Filename contains path separators, `..`, starts with `.`, or is too long |
| `disallowed_flag` | 400 | Compiler flag not in allowlist |
| `source_too_large` | 400 | Source exceeds 256 KiB |
| `too_many_tests` | 400 | More than 50 test cases |
| `stdin_too_large` | 400 | A test's stdin exceeds 64 KiB |
| `internal_error` | 500 | Server-side failure |
