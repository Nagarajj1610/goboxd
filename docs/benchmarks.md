# Benchmarks

Methodology: measured on Docker Desktop, Apple M1, 4 concurrent jobs limit
(`MAX_CONCURRENT_JOBS=4`), using a Hello World program in each language.
Each run averaged over 20 requests. Run `make load` to reproduce.

## Hello World latency (single client, sequential)

| Language | Median (ms) | p95 (ms) | p99 (ms) |
|---|---|---|---|
| Python 3 (`py3`) | — | — | — |
| C++ (`cpp`) | — | — | — |
| C (`c`) | — | — | — |
| Java (`java`) | — | — | — |
| JavaScript (`js`) | — | — | — |
| Bash (`bash`) | — | — | — |

_Fill after running `make load` inside the container._

## Throughput under concurrent load (C++, Hello World)

| Concurrent clients | Requests/s | Median latency (ms) | p99 latency (ms) |
|---|---|---|---|
| 1 | — | — | — |
| 10 | — | — | — |
| 50 | — | — | — |
| 100 | — | — | — |

_Fill after running `hey -n 500 -c N http://localhost:8080/run ...`_

## How to reproduce

```bash
# Start the container
docker build -t goboxd .
docker run -d -p 8080:8080 --privileged --name goboxd-bench goboxd

# Wait for ready
curl -f http://localhost:8080/healthz

# Single-language sequential load
make load

# Concurrent load with hey (install: go install github.com/rakyll/hey@latest)
hey -n 200 -c 10 -m POST \
  -H "Content-Type: application/json" \
  -d '{"language":"py3","source":"print(1)","tests":[{"stdin":"","expected_stdout":"1\n"}]}' \
  http://localhost:8080/run

# Clean up
docker stop goboxd-bench && docker rm goboxd-bench
```

## Notes

- nsjail namespace setup adds ~50-200 ms fixed overhead per request on Docker Desktop.
- C++ compilation is the main latency driver for compiled languages.
- Java startup JVM adds ~300-500 ms even for trivial programs.
- The semaphore means requests beyond `MAX_CONCURRENT_JOBS` queue rather than fail.
