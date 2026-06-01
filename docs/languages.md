# Languages

goboxd supports multiple programming languages defined entirely in
`configs/languages.yaml`. Go code never hardcodes a language name.

## Supported Languages

| ID | Name | Type | Source File | Toolchain |
|---|---|---|---|---|
| `py3` | Python 3 | Interpreted | `solution.py` | `/usr/bin/python3` |
| `cpp` | C++ | Compiled | `solution.cpp` | `/usr/bin/g++` |
| `c` | C | Compiled | `solution.c` | `/usr/bin/gcc` |
| `java` | Java | Compiled | from request | `/usr/bin/javac` + `/usr/bin/java` |
| `bash` | Bash | Interpreted | `solution.sh` | `/bin/bash` |
| `js` | JavaScript (Node) | Interpreted | `solution.js` | `/usr/bin/node` |
| `verilog` | Verilog | Compiled | `solution.v` | `/usr/bin/iverilog` + `/usr/bin/vvp` |
| `rust` | Rust | Compiled | `solution.rs` | `/usr/local/bin/rustc` |
| `ruby` | Ruby | Interpreted | `solution.rb` | `/usr/bin/ruby` |
| `kotlin` | Kotlin | Compiled | `solution.kt` | `/usr/local/bin/kotlinc` |

## How to add a new language

Adding a new language requires **only a YAML block** in `configs/languages.yaml`.
No Go code changes are needed.

**Step 1:** Add a new entry to `configs/languages.yaml`:

```yaml
- id: mylang
  name: My Language
  source_filename: solution.ml
  version_cmd: ["/usr/bin/myinterp", "--version"]
  run:
    cmd: /usr/bin/myinterp
    args: ["{{source}}"]
    limits:
      wall_time_s: 10
      memory_kb: 131072
      max_processes: 50
```

For compiled languages, add a `build` section:

```yaml
- id: mylang
  name: My Language
  source_filename: solution.ml
  artifact: solution_ml
  version_cmd: ["/usr/bin/mycc", "--version"]
  build:
    cmd: /usr/bin/mycc
    args: ["{{flags}}", "-o", "{{artifact}}", "{{source}}"]
    limits:
      wall_time_s: 5
      memory_kb: 524288
      max_processes: 100
    flag_allowlist: ["-O0", "-O1", "-O2"]
  run:
    cmd: ./{{artifact}}
    limits:
      wall_time_s: 5
      memory_kb: 262144
      max_processes: 50
```

**Step 2:** Install the toolchain in the Dockerfile (Stage 2):

```dockerfile
RUN apt-get install -y mylang-compiler
```

**Step 3:** Rebuild the Docker image:

```bash
docker build -t goboxd .
```

That is all.

## Placeholder semantics

| Placeholder | Expanded to |
|---|---|
| `{{source}}` | Absolute path to the source file inside the jail directory |
| `{{artifact}}` | Absolute path to the compiled binary inside the jail directory |
| `{{flags}}` | All build flags from the request, spliced as separate arguments |

`{{flags}}` in the YAML `args` list is **spliced** (expanded to zero or more
arguments), not treated as a single argument. Example:

```yaml
args: ["{{flags}}", "-o", "{{artifact}}", "{{source}}"]
```

With `flags: ["-O2", "-Wall"]` becomes:

```
g++ -O2 -Wall -o /var/jails/jail-xyz/solution /var/jails/jail-xyz/solution.cpp
```

## flag_allowlist glob patterns

Each entry in `flag_allowlist` is either:
- An **exact match**: the flag must equal the entry exactly.
  Example: `"-O2"` matches only `"-O2"`.
- A **prefix glob**: entries ending in `*` match any flag that starts with the prefix.
  Example: `"-std=*"` matches `"-std=c++17"`, `"-std=c11"`, etc.

Any flag not matched by any allowlist entry is rejected with HTTP 400.
The choice of allowlist (not denylist) is intentional: a denylist cannot
enumerate all possible dangerous flags.

## Java special case

Java requires the source filename to match the public class name:

```json
{
  "language": "java",
  "source": "public class Main { public static void main(String[] args) { ... } }",
  "source_filename": "Main.java",
  "artifact_filename": "Main"
}
```

Both `source_filename` and `artifact_filename` come from the request
(`source_filename_strategy: from_request` and `artifact_filename_strategy: from_request`
in the YAML). Both are validated by `validator.ValidateFilename` before use.
