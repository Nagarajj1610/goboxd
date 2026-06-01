package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/thesouldev/goboxd/internal/config"
	"github.com/thesouldev/goboxd/internal/runner"
	"github.com/thesouldev/goboxd/internal/validator"
)

// RunHandler handles POST /run.
type RunHandler struct {
	Runner *runner.Runner
	Config *config.Config
	Logger *zap.Logger
}

func newRequestID() string {
	return fmt.Sprintf("%016x", rand.Int63()) //nolint:gosec
}

func (h *RunHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := newRequestID()

	// HOLE 4 FIX: Hard 1 MiB cap on request body.
	// Security: file:internal/handler/run.go (ServeHTTP)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req runner.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err.Error() == "http: request body too large" {
			writeError(w, http.StatusBadRequest, "request_too_large", "request body exceeds 1 MiB limit")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	lang, ok := h.Config.Get(req.Language)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown_language", "language "+req.Language+" is not supported")
		return
	}

	sourceFilename := lang.SourceFilename
	if lang.SourceFilenameStrategy == "from_request" {
		sourceFilename = req.SourceFilename
	}

	// HOLE 1 FIX: Validate source_filename.
	// Security: file:internal/handler/run.go (ServeHTTP) → validator.ValidateFilename
	if err := validator.ValidateFilename(sourceFilename); err != nil {
		var ve *validator.ValidationError
		if errors.As(err, &ve) {
			h.Logger.Info("filename rejected", zap.String("request_id", requestID),
				zap.String("filename", sourceFilename), zap.String("code", ve.Code))
			writeError(w, http.StatusBadRequest, ve.Code, ve.Message)
		} else {
			writeError(w, http.StatusBadRequest, "invalid_filename", err.Error())
		}
		return
	}

	if req.ArtifactFilename != "" {
		if err := validator.ValidateFilename(req.ArtifactFilename); err != nil {
			var ve *validator.ValidationError
			if errors.As(err, &ve) {
				writeError(w, http.StatusBadRequest, ve.Code, ve.Message)
			} else {
				writeError(w, http.StatusBadRequest, "invalid_filename", err.Error())
			}
			return
		}
	}

	// HOLE 3 FIX: Validate build flags.
	// Security: file:internal/handler/run.go (ServeHTTP) → validator.ValidateFlags
	var buildFlags []string
	if req.Build != nil {
		buildFlags = req.Build.Flags
	}
	if err := validator.ValidateFlags(buildFlags, lang); err != nil {
		var ve *validator.ValidationError
		if errors.As(err, &ve) {
			h.Logger.Info("flag rejected", zap.String("request_id", requestID),
				zap.Strings("flags", buildFlags), zap.String("code", ve.Code))
			writeError(w, http.StatusBadRequest, ve.Code, ve.Message)
		} else {
			writeError(w, http.StatusBadRequest, "disallowed_flag", err.Error())
		}
		return
	}

	// HOLE 4 FIX: Validate source size, test count, stdin sizes.
	// Security: file:internal/handler/run.go (ServeHTTP) → validator.ValidateRunRequest
	stdinLengths := make([]int, len(req.Tests))
	for i, tc := range req.Tests {
		stdinLengths[i] = len(tc.Stdin)
	}
	if err := validator.ValidateRunRequest(len(req.Source), len(req.Tests), stdinLengths); err != nil {
		var ve *validator.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Code, ve.Message)
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}

	resp, err := h.Runner.Run(&req)
	if err != nil {
		h.Logger.Error("runner internal error", zap.String("request_id", requestID),
			zap.String("language", req.Language), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	h.Logger.Info("request completed",
		zap.String("request_id", requestID),
		zap.String("language", req.Language),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
		zap.String("status", resp.Status),
		zap.Int64("in_flight_jobs", h.Runner.Stats.InFlight.Load()),
		zap.Int("test_count", len(req.Tests)),
	)

	writeJSON(w, http.StatusOK, resp)
}
