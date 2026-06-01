// Package validator contains all input validation logic.
// Security holes 1, 3, and 4 are closed here.
package validator

import (
	"fmt"
	"strings"

	"github.com/thesouldev/goboxd/internal/config"
)

const (
	MaxSourceBytes    = 262144
	MaxTests          = 50
	MaxStdinPerTest   = 65536
	MaxFilenameLength = 64
)

// ValidateFilename rejects filenames that could escape the jail directory.
// HOLE 1 FIX: file:internal/validator/validator.go (ValidateFilename)
func ValidateFilename(name string) error {
	if name == "" {
		return &ValidationError{Code: "invalid_filename", Message: "filename must not be empty"}
	}
	if len(name) > MaxFilenameLength {
		return &ValidationError{
			Code:    "invalid_filename",
			Message: fmt.Sprintf("filename exceeds %d character limit", MaxFilenameLength),
		}
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return &ValidationError{Code: "invalid_filename", Message: "filename must not be an absolute path"}
	}
	if strings.HasPrefix(name, ".") {
		return &ValidationError{Code: "invalid_filename", Message: "filename must not start with '.'"}
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return &ValidationError{Code: "invalid_filename", Message: "source_filename must be a single path component"}
	}
	if strings.Contains(name, "..") {
		return &ValidationError{Code: "invalid_filename", Message: "filename must not contain '..'"}
	}
	return nil
}

// ValidateFlags checks every flag against the language's allowlist.
// HOLE 3 FIX: file:internal/validator/validator.go (ValidateFlags)
func ValidateFlags(flags []string, lang *config.Language) error {
	if lang.Build == nil {
		if len(flags) > 0 {
			return &ValidationError{
				Code:    "disallowed_flag",
				Message: fmt.Sprintf("language %q does not support build flags", lang.ID),
			}
		}
		return nil
	}
	for _, flag := range flags {
		if !flagAllowed(flag, lang.Build.FlagAllowlist) {
			return &ValidationError{
				Code:    "disallowed_flag",
				Message: fmt.Sprintf("flag %q is not in the allowlist for language %q", flag, lang.ID),
			}
		}
	}
	return nil
}

// flagAllowed returns true if flag matches any allowlist entry.
// Entries ending in "*" are treated as prefix globs.
func flagAllowed(flag string, allowlist []string) bool {
	for _, allowed := range allowlist {
		if strings.HasSuffix(allowed, "*") {
			if strings.HasPrefix(flag, allowed[:len(allowed)-1]) {
				return true
			}
		} else {
			if flag == allowed {
				return true
			}
		}
	}
	return false
}

// ValidateRunRequest validates size-related fields in a run request.
// HOLE 4 FIX: file:internal/validator/validator.go (ValidateRunRequest)
func ValidateRunRequest(sourceLen int, tests int, stdinLengths []int) error {
	if sourceLen > MaxSourceBytes {
		return &ValidationError{
			Code:    "source_too_large",
			Message: fmt.Sprintf("source exceeds %d byte limit (%d bytes submitted)", MaxSourceBytes, sourceLen),
		}
	}
	if tests > MaxTests {
		return &ValidationError{
			Code:    "too_many_tests",
			Message: fmt.Sprintf("request has %d tests but maximum is %d", tests, MaxTests),
		}
	}
	for i, stdinLen := range stdinLengths {
		if stdinLen > MaxStdinPerTest {
			return &ValidationError{
				Code:    "stdin_too_large",
				Message: fmt.Sprintf("tests[%d].stdin exceeds %d byte limit", i, MaxStdinPerTest),
			}
		}
	}
	return nil
}

// ValidationError carries a machine-readable code and a human-readable message.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
