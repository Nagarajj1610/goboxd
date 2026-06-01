package runner_test

import (
	"strings"
	"testing"

	"github.com/thesouldev/goboxd/internal/runner"
)

func TestOutputAtLimit(t *testing.T) {
	output := strings.Repeat("x", 65536)
	truncated := applyOutputLimit(output, 65536)
	if strings.Contains(truncated, "[output truncated]") {
		t.Error("output at exactly the limit should not have truncation marker")
	}
}

func TestOutputOverLimit(t *testing.T) {
	output := strings.Repeat("x", 65537)
	truncated := applyOutputLimit(output, 65536)
	if !strings.Contains(truncated, "[output truncated]") {
		t.Error("output over the limit should have truncation marker")
	}
}

func applyOutputLimit(output string, limit int) string {
	if len(output) > limit {
		return output[:limit] + "\n[output truncated]"
	}
	return output
}

func TestClassifyOutput_Accepted(t *testing.T) {
	if s := classifyOutput("hello\n", "hello\n"); s != "accepted" {
		t.Errorf("expected accepted, got %q", s)
	}
}

func TestClassifyOutput_WhitespaceMismatch(t *testing.T) {
	if s := classifyOutput("hello\n", "hello"); s != "output_whitespace_mismatch" {
		t.Errorf("expected output_whitespace_mismatch, got %q", s)
	}
}

func TestClassifyOutput_WrongOutput(t *testing.T) {
	if s := classifyOutput("HI\n", "hi\n"); s != "wrong_output" {
		t.Errorf("expected wrong_output, got %q", s)
	}
}

func TestRunnerResponseType(_ *testing.T) {
	_ = runner.Response{}
}

func classifyOutput(actual, expected string) string {
	if actual == expected {
		return "accepted"
	}
	if strings.TrimSpace(actual) == strings.TrimSpace(expected) {
		return "output_whitespace_mismatch"
	}
	return "wrong_output"
}
