package validator_test

import (
	"strings"
	"testing"

	"github.com/thesouldev/goboxd/internal/config"
	"github.com/thesouldev/goboxd/internal/validator"
)

func TestValidateFilename_PathTraversal(t *testing.T) {
	badNames := []string{
		"../etc/passwd",
		"/etc/passwd",
		"foo/bar.py",
		".hidden",
		strings.Repeat("a", 65),
	}
	for _, name := range badNames {
		if err := validator.ValidateFilename(name); err == nil {
			t.Errorf("ValidateFilename(%q): expected error, got nil", name)
		}
	}
}

func TestValidateFilename_Accepted(t *testing.T) {
	goodNames := []string{
		"solution.py", "solution.cpp", "Main.java", "solution.sh",
		strings.Repeat("a", 64),
	}
	for _, name := range goodNames {
		if err := validator.ValidateFilename(name); err != nil {
			t.Errorf("ValidateFilename(%q): unexpected error: %v", name, err)
		}
	}
}

func TestValidateFilename_Backslash(t *testing.T) {
	if err := validator.ValidateFilename(`foo\bar.py`); err == nil {
		t.Error("expected error for backslash in filename, got nil")
	}
}

func makeCPPLang() *config.Language {
	return &config.Language{
		ID:   "cpp",
		Name: "C++",
		Build: &config.BuildPhase{
			Cmd: "/usr/bin/g++",
			FlagAllowlist: []string{
				"-O0", "-O1", "-O2", "-O3",
				"-Wall", "-Wextra",
				"-std=c++17", "-std=c++14", "-std=c++11",
				"-std=c11", "-std=c99",
			},
		},
		Run: config.RunPhase{Cmd: "./solution"},
	}
}

func TestValidateFlags_Allowed(t *testing.T) {
	lang := makeCPPLang()
	if err := validator.ValidateFlags([]string{"-O2", "-Wall", "-std=c++17"}, lang); err != nil {
		t.Errorf("unexpected error for allowed flags: %v", err)
	}
}

func TestValidateFlags_Rejected(t *testing.T) {
	lang := makeCPPLang()
	for _, flag := range []string{"-fplugin=evil", "@response_file", "-Wl,evil", "--specs=evil"} {
		err := validator.ValidateFlags([]string{flag}, lang)
		if err == nil {
			t.Errorf("ValidateFlags(%q): expected rejection, got nil", flag)
		}
	}
}

func TestValidateFlags_NoFlagsOnInterpreted(t *testing.T) {
	py := &config.Language{
		ID: "py3", Name: "Python 3",
		Build: nil,
		Run:   config.RunPhase{Cmd: "/usr/bin/python3"},
	}
	if err := validator.ValidateFlags([]string{"-O2"}, py); err == nil {
		t.Error("expected error for flag on interpreted language, got nil")
	}
}

func TestValidateRunRequest_SourceTooLarge(t *testing.T) {
	if err := validator.ValidateRunRequest(262145, 1, []int{0}); err == nil {
		t.Error("expected source_too_large error")
	}
}

func TestValidateRunRequest_TooManyTests(t *testing.T) {
	if err := validator.ValidateRunRequest(100, 51, make([]int, 51)); err == nil {
		t.Error("expected too_many_tests error")
	}
}

func TestValidateRunRequest_StdinTooLarge(t *testing.T) {
	if err := validator.ValidateRunRequest(100, 1, []int{65537}); err == nil {
		t.Error("expected stdin_too_large error")
	}
}

func TestValidateRunRequest_Valid(t *testing.T) {
	if err := validator.ValidateRunRequest(262144, 50, []int{65536}); err != nil {
		t.Errorf("expected valid request, got error: %v", err)
	}
}
