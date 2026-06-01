package handler

import (
	"net/http"
	"runtime"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/thesouldev/goboxd/internal/config"
	"github.com/thesouldev/goboxd/internal/runner"
)

// InfoHandler handles GET /info.
type InfoHandler struct {
	Runner    *runner.Runner
	Config    *config.Config
	JailDir   string
	MaxJobs   int
	Version   string
	GitCommit string
	Logger    *zap.Logger
}

func (h *InfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	langs := h.Config.All()
	sort.Slice(langs, func(i, j int) bool { return langs[i].ID < langs[j].ID })

	langInfos := make([]map[string]any, 0, len(langs))
	for _, lang := range langs {
		version, _ := runner.LanguageVersion(lang.VersionCmd)
		langInfos = append(langInfos, map[string]any{
			"id":      lang.ID,
			"name":    lang.Name,
			"version": version,
			"default_run_limits": map[string]int{
				"wall_time_s":   lang.Run.Limits.WallTimeS,
				"memory_kb":     lang.Run.Limits.MemoryKB,
				"max_processes": lang.Run.Limits.MaxProcesses,
			},
		})
	}

	nsjailVer, _ := runner.NsjailVersion()

	var lastErrAt string
	if t := h.Runner.Stats.LastInternalError.Load(); t != nil {
		lastErrAt = t.UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"build_info": map[string]string{
			"version":    h.Version,
			"commit":     h.GitCommit,
			"go_version": runtime.Version(),
		},
		"nsjail": map[string]string{
			"path":    "/usr/sbin/nsjail",
			"version": nsjailVer,
		},
		"languages": langInfos,
		"limits": map[string]int{
			"max_source_bytes":    262144,
			"max_tests":           50,
			"max_concurrent_jobs": h.MaxJobs,
		},
		"stats": map[string]any{
			"in_flight_jobs":           h.Runner.Stats.InFlight.Load(),
			"jobs_total":               h.Runner.Stats.JobsTotal.Load(),
			"jobs_failed_internal":     h.Runner.Stats.JobsFailedInternal.Load(),
			"last_internal_error_at":   lastErrAt,
			"disk_free_bytes_jail_dir": runner.DiskFreeBytes(""),
		},
	})
}

// ReadyzHandler handles GET /readyz.
type ReadyzHandler struct {
	Config *config.Config
	Logger *zap.Logger
}

func (h *ReadyzHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	nsjailVer, nsjailErr := runner.NsjailVersion()
	nsjailStatus := map[string]any{"ok": nsjailErr == nil}
	if nsjailErr != nil {
		nsjailStatus["error"] = nsjailErr.Error()
	} else {
		nsjailStatus["version"] = nsjailVer
	}

	langStatuses := make(map[string]map[string]any)
	allOK := nsjailErr == nil

	for _, lang := range h.Config.All() {
		ver, err := runner.LanguageVersion(lang.VersionCmd)
		if err != nil {
			langStatuses[lang.ID] = map[string]any{"ok": false, "error": err.Error()}
			allOK = false
		} else {
			langStatuses[lang.ID] = map[string]any{"ok": true, "version": ver}
		}
	}

	h.Logger.Info("readyz check", zap.Bool("all_ok", allOK), zap.Bool("nsjail_ok", nsjailErr == nil))

	status := "ok"
	httpStatus := http.StatusOK
	if !allOK {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	writeJSON(w, httpStatus, map[string]any{
		"status":    status,
		"nsjail":    nsjailStatus,
		"languages": langStatuses,
	})
}
