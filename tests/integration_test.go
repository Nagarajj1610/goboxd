//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

type TestCase struct {
	Stdin          string `json:"stdin"`
	ExpectedStdout string `json:"expected_stdout"`
}

type PhaseReq struct {
	Flags []string `json:"flags,omitempty"`
}

type RunRequest struct {
	Language         string     `json:"language"`
	Source           string     `json:"source"`
	SourceFilename   string     `json:"source_filename,omitempty"`
	ArtifactFilename string     `json:"artifact_filename,omitempty"`
	Build            *PhaseReq  `json:"build,omitempty"`
	Tests            []TestCase `json:"tests"`
}

type PhaseResult struct {
	Status string `json:"status"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type TestResult struct {
	Status string `json:"status"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type RunResponse struct {
	Status string       `json:"status"`
	Build  *PhaseResult `json:"build,omitempty"`
	Tests  []TestResult `json:"tests,omitempty"`
}

func getAPIURL() string {
	if u := os.Getenv("API_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func TestHealthCheck(t *testing.T) {
	resp, err := http.Get(getAPIURL() + "/healthz")
	if err != nil {
		t.Fatalf("Failed to call healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestReadyCheck(t *testing.T) {
	resp, err := http.Get(getAPIURL() + "/readyz")
	if err != nil {
		t.Fatalf("Failed to call readyz: %v", err)
	}
	defer resp.Body.Close()

	// Readyz can return 200 OK or 503 Service Unavailable depending on whether all language toolchains are fully ready.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 200 or 503, got %d", resp.StatusCode)
	}
}

func TestPythonHelloWorld(t *testing.T) {
	reqBody := RunRequest{
		Language: "py3",
		Source:   "print(\"Hello, World!\")",
		Tests: []TestCase{
			{Stdin: "", ExpectedStdout: "Hello, World!\n"},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post(getAPIURL() + "/run", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to call run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var runResp RunResponse
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if runResp.Status != "accepted" {
		t.Errorf("Expected status accepted, got %s", runResp.Status)
	}

	if len(runResp.Tests) != 1 {
		t.Fatalf("Expected 1 test result, got %d", len(runResp.Tests))
	}

	if runResp.Tests[0].Status != "accepted" {
		t.Errorf("Expected test status accepted, got %s", runResp.Tests[0].Status)
	}

	if runResp.Tests[0].Stdout != "Hello, World!\n" {
		t.Errorf("Expected stdout 'Hello, World!\n', got %q", runResp.Tests[0].Stdout)
	}
}

func TestPathTraversalValidation(t *testing.T) {
	reqBody := RunRequest{
		Language:       "py3",
		Source:         "print(1)",
		SourceFilename: "../etc/passwd",
		Tests:          []TestCase{},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post(getAPIURL() + "/run", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to call run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for path traversal, got %d", resp.StatusCode)
	}
}

func TestDisallowedFlagValidation(t *testing.T) {
	reqBody := RunRequest{
		Language: "cpp",
		Source:   "int main(){}",
		Build: &PhaseReq{
			Flags: []string{"-fplugin=evil"},
		},
		Tests: []TestCase{},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	resp, err := http.Post(getAPIURL() + "/run", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("Failed to call run: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for disallowed flag, got %d", resp.StatusCode)
	}
}
