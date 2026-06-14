package advisor

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"budgets_be/internal/auth"
)

func RegisterRoutes(mux *http.ServeMux, service Service, authMiddleware func(http.Handler) http.Handler) {
	askHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}

		var req AdviceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body",
			})
			return
		}

		resp, err := service.GetAdvice(r.Context(), userID, req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"error": err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, resp)
	})

	if authMiddleware != nil {
		mux.Handle("POST /api/ai/advisor", authMiddleware(askHandler))
		mux.Handle("GET /api/ai/advisor/threads", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			threads, err := service.Threads(r.Context(), userID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, threads)
		})))
		mux.Handle("GET /api/ai/advisor/threads/{id}/messages", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}

			messages, err := service.Messages(r.Context(), userID, r.PathValue("id"))
			if err == sql.ErrNoRows {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "thread not found"})
				return
			}
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}

			writeJSON(w, http.StatusOK, messages)
		})))
		return
	}

	mux.Handle("POST /api/ai/advisor", askHandler)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
