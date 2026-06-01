# Known Issues and Anticipated Problems

Pre-populated with issues that may arise during deployment and testing.

---

## Issue 1 — nsjail build dependencies on ubuntu:22.04

**Symptom:** `docker build` fails at the nsjail `make` step with missing headers
or linker errors.

**Cause:** nsjail requires `libprotobuf-dev`, `libnl-3-dev`, `libnl-route-3-dev`,
`libcap-dev`, `libseccomp-dev`, `bison`, `flex`, and `pkg-config`.
If any of these are missing from the apt-get install list, the build fails.

**Fix:** Ensure the Dockerfile Stage 1 `apt-get install` line includes all of:
```
build-essential pkg-config bison flex protobuf-compiler libprotobuf-dev
libnl-3-dev libnl-route-3-dev libcap-dev libseccomp-dev
```

**Status:** Included in Dockerfile. May still fail if upstream package names
change between ubuntu:22.04 minor releases.

---

## Issue 2 — Java: source_filename must match public class name

**Symptom:** Java compilation fails with:
`error: class Main is public, should be in a file named Main.java`

**Cause:** The Java compiler (`javac`) requires that a file containing a
`public class Foo` be named exactly `Foo.java`. If the caller sends
`source_filename: "solution.java"` but the class is named `Main`, javac
rejects the file.

**Fix:** Callers must set `source_filename` to match the public class name:
```json
{
  "language": "java",
  "source_filename": "Main.java",
  "artifact_filename": "Main",
  "source": "public class Main { ... }"
}
```

The YAML uses `source_filename_strategy: from_request` to give callers control.
This is documented in `docs/languages.md`.

**Status:** Known limitation of the Java toolchain. Cannot be fixed without
parsing the Java source to extract the class name (which would add complexity).

---

## Issue 3 — Verilog: iverilog not in default ubuntu repos on older releases

**Symptom:** `apt-get install iverilog` fails with `Package 'iverilog' has no
installation candidate` on ubuntu versions before 20.04.

**Cause:** `iverilog` (Icarus Verilog) is in the `universe` repository which
may not be enabled by default.

**Fix:** The Dockerfile uses ubuntu:22.04 where iverilog is available in the
default repos. If the base image is changed, add:
```dockerfile
RUN apt-get update && \
    apt-get install -y software-properties-common && \
    add-apt-repository universe && \
    apt-get update && \
    apt-get install -y iverilog
```

**Status:** Not an issue with the current ubuntu:22.04 base image.

---

## Issue 4 — Kotlin compiler takes very long to start

**Symptom:** Kotlin compilation exceeds the 30-second wall time limit set in
the YAML.

**Cause:** The Kotlin compiler (`kotlinc`) starts a JVM process which has
significant startup overhead (~1-3 seconds on fast hardware, longer in Docker).
Compilation itself adds to this.

**Fix:** The YAML sets `wall_time_s: 30` for Kotlin build, which is generous.
If issues persist, increase this value or consider using the Kotlin Daemon
(which keeps the JVM warm between compilations).

**Status:** Known Kotlin limitation. 30 seconds should be sufficient for
Hello World and small programs.

---

## Issue 5 — nsjail requires privileged Docker mode

**Symptom:** nsjail fails with `clone: Operation not permitted` when the
container runs without `--privileged`.

**Cause:** nsjail uses Linux namespaces (user, mount, PID, network) which
require `CAP_SYS_ADMIN` and other capabilities not available in unprivileged
containers.

**Fix:** Always run with `--privileged`:
```bash
docker run --privileged goboxd
```
Or in docker-compose.yml:
```yaml
privileged: true
```

**Status:** Known requirement, documented in README and docker-compose.yml.
For production, consider using rootless nsjail configurations or a dedicated
sandbox host.
