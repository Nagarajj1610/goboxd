# Architecture

## What goboxd does

goboxd receives source code over HTTP, writes it to an isolated directory, runs
it inside an nsjail Linux namespace, and returns the output. It is designed as
a single stateless binary that can be scaled horizontally behind a load balancer.

## Request flow (step by step)

```
Client
  │
  │  POST /run (JSON body)
  ▼
chi router                    ← selects ServeHTTP on RunHandler
  │
  ▼
handler/run.go
  ├─ http.MaxBytesReader       ← hard 1 MiB cap on body (Hole 4)
  ├─ json.Decode               ← parse request
  ├─ config.Get(language)      ← resolve language config from YAML
  ├─ validator.ValidateFilename ← path traversal check (Hole 1)
  ├─ validator.ValidateFlags    ← flag injection check (Hole 3)
  └─ validator.ValidateRunRequest ← source/test/stdin size checks (Hole 4)
       │
       ▼
runner.Run(req)
  ├─ sandbox.New(jailDir)      ← os.MkdirTemp (Hole 5), Chmod 1777
  ├─ defer sb.Cleanup()        ← deferred os.RemoveAll (Hole 7)
  ├─ sb.WriteFile(source)      ← filepath.Join only (Hole 2)
  │
  ├─ sem <- struct{}{}         ← acquire semaphore slot (Section 6)
  ├─ Stats.InFlight.Add(1)
  │
  ├─ [Build phase — compiled langs only]
  │    nsjail <build cmd> <args>
  │    io.LimitReader(stdout, 65536)  ← (Hole 6)
  │    io.LimitReader(stderr, 65536)  ← (Hole 6)
  │    → PhaseResult{status, stdout, stderr, duration_ms}
  │
  └─ [Run phase — one per test case]
       nsjail <run cmd> <args>
       io.LimitReader per stream
       classifyOutput(actual, expected)
       → TestResult{status, stdout, stderr, duration_ms}
       │
  ◄────┘
  │
  ├─ <-sem                     ← release semaphore
  └─ Stats.InFlight.Add(-1)

handler/run.go
  └─ writeJSON(200, Response)
```

## Language registry

At server startup, `config.Load(path)` reads `configs/languages.yaml` and
builds a `map[string]*Language` keyed by language id. Every handler call uses
`cfg.Get(language)` for O(1) lookup. Adding a new language requires only a
new YAML block — no Go code changes.

## Concurrency model (semaphore)

```
Requests ──► [chi goroutines] ──► sem channel (capacity = MAX_CONCURRENT_JOBS)
                                         │
              blocked (queued) ◄──────── │ ──────────────► nsjail process
                                         │
                   release ◄─────────────┘  (when nsjail exits)
```

The semaphore is a buffered channel of `struct{}{}`. Sending blocks when full.
Receiving (defer) frees a slot. This is the simplest possible bounded-concurrency
pattern in Go, with no external queue library needed.

`Stats.InFlight` and `Stats.JobsTotal` are `sync/atomic.Int64` values that are
safe to read from any goroutine at any time (no mutex required).

## Jail directory lifecycle

```
startup:        sandbox.SweepStale(baseDir)    ← remove orphans >10 min old
                  └─ os.ReadDir → filter "jail-" prefix → os.RemoveAll

per request:    sandbox.New(baseDir)
                  └─ os.MkdirAll(baseDir)
                  └─ os.MkdirTemp(baseDir, "jail-")   ← e.g. /var/jails/jail-123456789
                  └─ os.Chmod(dir, 1777)
                defer sb.Cleanup()
                  └─ os.RemoveAll(dir)          ← fires even on panic
```

## Why chi over net/http

`net/http` alone has no named route parameters and no middleware chain.
`gin` adds request binding, validation, and rendering that we don't need and
that obscure what the server is doing. `chi` adds only routing and middleware
chaining, and its source is ~800 lines — readable enough for a beginner to
follow a request through.

## Directory structure

```
goboxd/
├── main.go                  ← wire everything, start HTTP server
├── go.mod / go.sum          ← module definition and dependency checksums
├── configs/
│   └── languages.yaml       ← THE ONLY place languages are defined
├── internal/
│   ├── config/              ← load + validate the YAML at startup
│   ├── validator/           ← all input validation (Holes 1, 3, 4)
│   ├── sandbox/             ← jail directory lifecycle (Holes 2, 5, 7)
│   ├── runner/              ← nsjail orchestration, concurrency (Hole 6)
│   └── handler/             ← HTTP layer: parse → validate → run → respond
├── Dockerfile               ← multi-stage: builder + runtime
├── docker-compose.yml       ← local dev
├── Makefile                 ← standard targets
└── docs/                    ← full documentation
```
