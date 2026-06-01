# Plan Evolution

Documents how the implementation plan changed as we built goboxd.

---

## [2026-06-01] Initial approach → final architecture

**What we thought we'd do:**
Start with a simple `net/http` server that shells out to compilers directly,
with hardcoded language support for Python and C++.

**What we actually did:**
Used chi for routing, designed a YAML-driven language registry with 10 languages
(7 required + 3 bonus), and integrated nsjail for proper sandboxing. All language
logic is in YAML, not Go.

**Why it changed:**
The hackathon spec explicitly required chi, nsjail, and YAML. Once we committed
to a data-driven language registry, the architecture became cleaner: Go code
only reads config, never branches on language names.

---

## [2026-06-01] Language additions beyond py3 + cpp

**What we thought we'd do:**
Implement the 7 required languages (py3, cpp, java, bash, js, c, verilog) and
stop there.

**What we actually did:**
Added 3 bonus languages (rust, ruby, kotlin) in the same YAML file. Because
the registry is data-driven, adding a language is a single YAML block plus a
Dockerfile RUN line.

**Why it changed:**
The spec noted bonus languages as extra scoring points. Since the architecture
already supported arbitrary languages with zero Go code changes, the cost of
adding them was minimal (write YAML + install toolchain in Dockerfile).

---

## [2026-06-01] Security approach changes

**What we thought we'd do:**
Address security as a post-implementation pass, retrofitting checks into handlers.

**What we actually did:**
Built a dedicated `internal/validator` package first and made it the mandatory
gatekeeper before any file write or process spawn. Security is enforced in the
request pipeline, not as afterthoughts.

**Why it changed:**
When writing the handler, it became clear that security checks mixed into handler
code would be hard to audit. A separate validator package with its own tests
makes each check independently verifiable. The spec required unit tests for the
validator, which reinforced this decision.
