// Package runner orchestrates the build and run phases for a single submission.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/thesouldev/goboxd/internal/config"
	"github.com/thesouldev/goboxd/internal/sandbox"
)

const (
	// maxOutputBytes caps stdout and stderr per execution phase.
	// HOLE 6 FIX: io.LimitReader at this boundary.
	// Security: file:internal/runner/runner.go (runPhase)
	maxOutputBytes = 65536

	truncationMarker = "\n[output truncated]"
	nsjailPath       = "/usr/sbin/nsjail"
)

// Stats holds server-wide atomic counters exposed by GET /info.
type Stats struct {
	InFlight           atomic.Int64
	JobsTotal          atomic.Int64
	JobsFailedInternal atomic.Int64
	LastInternalError  atomic.Pointer[time.Time]
}

// Runner holds shared state for all requests.
type Runner struct {
	cfg     *config.Config
	jailDir string
	sem     chan struct{}
	logger  *zap.Logger
	Stats   *Stats
}

// New creates a Runner with a semaphore of capacity maxJobs.
func New(cfg *config.Config, jailDir string, maxJobs int, logger *zap.Logger) *Runner {
	return &Runner{
		cfg:     cfg,
		jailDir: jailDir,
		sem:     make(chan struct{}, maxJobs),
		logger:  logger,
		Stats:   &Stats{},
	}
}

// --- Request / Response types ---

// Limits from the caller (may override YAML defaults).
type Limits struct {
	WallTimeS    int `json:"wall_time_s"`
	MemoryKB     int `json:"memory_kb"`
	MaxProcesses int `json:"max_processes"`
}

// TestCase is one stdin/expected-stdout pair.
type TestCase struct {
	Stdin          string `json:"stdin"`
	ExpectedStdout string `json:"expected_stdout"`
}

// Request is the parsed body of POST /run.
type Request struct {
	Language         string     `json:"language"`
	Source           string     `json:"source"`
	SourceFilename   string     `json:"source_filename"`
	ArtifactFilename string     `json:"artifact_filename"`
	Build            *PhaseReq  `json:"build"`
	Run              *PhaseReq  `json:"run"`
	Tests            []TestCase `json:"tests"`
}

// PhaseReq contains caller-supplied overrides for one phase.
type PhaseReq struct {
	Limits *Limits  `json:"limits"`
	Flags  []string `json:"flags"`
}

// PhaseResult is the outcome of one execution phase.
type PhaseResult struct {
	Status     string `json:"status"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
}

// TestResult is the outcome of running one test case.
type TestResult struct {
	Status       string `json:"status"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	DurationMS   int64  `json:"duration_ms"`
	MemoryPeakKB int64  `json:"memory_peak_kb"`
}

// Response is the full JSON response body for POST /run.
type Response struct {
	Status string       `json:"status"`
	Build  *PhaseResult `json:"build,omitempty"`
	Tests  []TestResult `json:"tests,omitempty"`
}

// Run executes the full pipeline: acquire semaphore → build → run tests → release.
func (r *Runner) Run(req *Request) (*Response, error) {
	r.Stats.JobsTotal.Add(1)

	lang, ok := r.cfg.Get(req.Language)
	if !ok {
		return nil, fmt.Errorf("unknown language: %s", req.Language)
	}

	sourceFilename := lang.SourceFilename
	if lang.SourceFilenameStrategy == "from_request" {
		sourceFilename = req.SourceFilename
	}
	artifactName := lang.Artifact
	if lang.ArtifactFilenameStrategy == "from_request" {
		artifactName = req.ArtifactFilename
	}

	sb, err := sandbox.New(r.jailDir, r.logger)
	if err != nil {
		r.recordInternalError()
		return nil, fmt.Errorf("runner: cannot create sandbox: %w", err)
	}
	// HOLE 7 FIX: defer cleanup immediately — fires even on panic.
	// Security: file:internal/runner/runner.go (Run)
	defer sb.Cleanup()

	sourcePath, err := sb.WriteFile(sourceFilename, req.Source)
	if err != nil {
		r.recordInternalError()
		return nil, fmt.Errorf("runner: cannot write source: %w", err)
	}

	// Section 6: Acquire semaphore — blocks when MAX_CONCURRENT_JOBS are running.
	r.sem <- struct{}{}
	r.Stats.InFlight.Add(1)
	defer func() {
		<-r.sem
		r.Stats.InFlight.Add(-1)
	}()

	resp := &Response{}

	// Build phase (compiled languages only).
	if lang.Build != nil {
		buildFlags := []string{}
		if req.Build != nil {
			buildFlags = req.Build.Flags
		}
		buildLimits := lang.Build.Limits
		if req.Build != nil && req.Build.Limits != nil {
			buildLimits = mergedLimits(buildLimits, req.Build.Limits)
		}
		artifactPath := artifactFilePath(sb.Dir, artifactName)
		buildArgs := expandArgs(lang.Build.Args, sourcePath, artifactPath, buildFlags)
		buildResult, internalErr := r.runPhase(lang.Build.Cmd, buildArgs, sb.Dir, "", buildLimits)
		if internalErr != nil {
			r.recordInternalError()
			resp.Status = "internal_error"
			resp.Build = &PhaseResult{Status: "internal_error"}
			return resp, nil
		}
		resp.Build = buildResult
		if buildResult.Status != "ok" {
			resp.Status = "build_failed"
			resp.Tests = make([]TestResult, len(req.Tests))
			for i := range resp.Tests {
				resp.Tests[i] = TestResult{Status: "not_executed"}
			}
			return resp, nil
		}
	}

	// Run phase (one execution per test case).
	runLimits := lang.Run.Limits
	if req.Run != nil && req.Run.Limits != nil {
		runLimits = mergedLimits(runLimits, req.Run.Limits)
	}
	artifactPath := artifactFilePath(sb.Dir, artifactName)
	runCmd := expandRunCmd(lang.Run.Cmd, artifactPath)
	runArgs := expandArgs(lang.Run.Args, sourcePath, artifactPath, nil)

	resp.Tests = make([]TestResult, len(req.Tests))
	overallStatus := "accepted"

	for i, tc := range req.Tests {
		testResult, internalErr := r.runPhase(runCmd, runArgs, sb.Dir, tc.Stdin, runLimits)
		if internalErr != nil {
			r.recordInternalError()
			resp.Tests[i] = TestResult{Status: "internal_error"}
			if overallStatus == "accepted" {
				overallStatus = "internal_error"
			}
			continue
		}
		tr := TestResult{
			Stdout:     testResult.Stdout,
			Stderr:     testResult.Stderr,
			DurationMS: testResult.DurationMS,
		}
		switch testResult.Status {
		case "ok":
			tr.Status = classifyOutput(testResult.Stdout, tc.ExpectedStdout)
		default:
			tr.Status = testResult.Status
		}
		resp.Tests[i] = tr
		if overallStatus == "accepted" && tr.Status != "accepted" {
			overallStatus = tr.Status
		}
	}

	resp.Status = overallStatus
	return resp, nil
}

// classifyOutput compares actual vs expected stdout.
func classifyOutput(actual, expected string) string {
	if actual == expected {
		return "accepted"
	}
	if strings.TrimSpace(actual) == strings.TrimSpace(expected) {
		return "output_whitespace_mismatch"
	}
	return "wrong_output"
}

// runPhase runs a single command inside nsjail and collects the result.
// HOLE 6 FIX: io.LimitReader caps stdout and stderr at 64 KiB each.
// Security: file:internal/runner/runner.go (runPhase)
func (r *Runner) runPhase(cmd string, args []string, cwd, stdin string, limits config.Limits) (*PhaseResult, error) {
	wallTimeout := time.Duration(limits.WallTimeS)*time.Second + 2*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), wallTimeout)
	defer cancel()

	nsjailArgs := buildNsjailArgs(cmd, args, cwd, limits)
	proc := exec.CommandContext(ctx, nsjailPath, nsjailArgs...)
	proc.Dir = cwd
	if stdin != "" {
		proc.Stdin = strings.NewReader(stdin)
	}

	stdoutPipe, err := proc.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("runPhase: stdout pipe: %w", err)
	}
	stderrPipe, err := proc.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("runPhase: stderr pipe: %w", err)
	}

	start := time.Now()
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("runPhase: cannot start nsjail: %w", err)
	}

	var rawStdout, rawStderr bytes.Buffer
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan error, 1)

	go func() {
		n, err := io.Copy(&rawStdout, io.LimitReader(stdoutPipe, maxOutputBytes))
		if n == maxOutputBytes {
			rawStdout.WriteString(truncationMarker)
		}
		stdoutDone <- err
	}()
	go func() {
		n, err := io.Copy(&rawStderr, io.LimitReader(stderrPipe, maxOutputBytes))
		if n == maxOutputBytes {
			rawStderr.WriteString(truncationMarker)
		}
		stderrDone <- err
	}()

	<-stdoutDone
	<-stderrDone
	waitErr := proc.Wait()
	elapsed := time.Since(start)

	result := &PhaseResult{
		Stdout:     rawStdout.String(),
		Stderr:     rawStderr.String(),
		DurationMS: elapsed.Milliseconds(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.Status = "time_exceeded"
		return result, nil
	}
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 && strings.Contains(rawStderr.String(), "memory") {
				result.Status = "memory_exceeded"
			} else {
				result.Status = "runtime_error"
			}
		} else {
			result.Status = "time_exceeded"
		}
		return result, nil
	}
	result.Status = "ok"
	return result, nil
}

func buildNsjailArgs(cmd string, args []string, cwd string, limits config.Limits) []string {
	nargs := []string{
		"--mode", "o",
		"--chroot", "/",
		"--time_limit", fmt.Sprintf("%d", limits.WallTimeS),
		"--rlimit_as", fmt.Sprintf("%d", limits.MemoryKB/1024),
		"--rlimit_nproc", fmt.Sprintf("%d", limits.MaxProcesses),
		"--rlimit_fsize", "64",
		"--cwd", cwd,
		"--",
		cmd,
	}
	return append(nargs, args...)
}

func expandArgs(args []string, sourcePath, artifactPath string, flags []string) []string {
	result := make([]string, 0, len(args)+len(flags))
	for _, arg := range args {
		switch arg {
		case "{{source}}":
			result = append(result, sourcePath)
		case "{{artifact}}":
			result = append(result, artifactPath)
		case "{{flags}}":
			result = append(result, flags...)
		default:
			result = append(result, arg)
		}
	}
	return result
}

func expandRunCmd(cmd, artifactPath string) string {
	return strings.ReplaceAll(cmd, "{{artifact}}", artifactPath)
}

func artifactFilePath(jailDir, artifactName string) string {
	if artifactName == "" {
		return ""
	}
	return filepath.Join(jailDir, artifactName)
}

func mergedLimits(defaults config.Limits, override *Limits) config.Limits {
	merged := defaults
	if override.WallTimeS > 0 {
		merged.WallTimeS = override.WallTimeS
	}
	if override.MemoryKB > 0 {
		merged.MemoryKB = override.MemoryKB
	}
	if override.MaxProcesses > 0 {
		merged.MaxProcesses = override.MaxProcesses
	}
	return merged
}

func (r *Runner) recordInternalError() {
	r.Stats.JobsFailedInternal.Add(1)
	now := time.Now()
	r.Stats.LastInternalError.Store(&now)
}

// NsjailVersion probes the nsjail binary and returns its version string.
func NsjailVersion() (string, error) {
	out, err := exec.Command(nsjailPath, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nsjail not found at %s: %w", nsjailPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// LanguageVersion runs a language's version_cmd and returns the first line of output.
func LanguageVersion(versionCmd []string) (string, error) {
	if len(versionCmd) == 0 {
		return "", fmt.Errorf("no version_cmd configured")
	}
	out, err := exec.Command(versionCmd[0], versionCmd[1:]...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	lines := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)
	return lines[0], nil
}

// DiskFreeBytes returns available bytes on the filesystem containing path.
// Returns -1 on platforms without syscall.Statfs (e.g. Windows dev).
func DiskFreeBytes(_ string) int64 {
	return -1
}
