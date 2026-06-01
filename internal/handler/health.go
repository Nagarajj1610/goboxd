package handler

import (
	"net/http"
)

// HealthHandler handles GET /healthz — always returns 200.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
