package notification

import (
	"encoding/json"
	"net/http"

	"budgets_be/internal/auth"
)

func RegisterRoutes(mux *http.ServeMux, service Service, authMiddleware func(http.Handler) http.Handler) {
	mux.Handle("GET /api/push/public-key", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !service.Enabled() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "push notifications are not configured"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"publicKey": service.Config.PublicKey})
	})))

	mux.Handle("POST /api/push/subscriptions", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		var subscription Subscription
		if err := json.NewDecoder(r.Body).Decode(&subscription); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if err := service.SaveSubscription(r.Context(), userID, subscription); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	mux.Handle("POST /api/push/subscriptions/unsubscribe", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		var subscription Subscription
		if err := json.NewDecoder(r.Body).Decode(&subscription); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			return
		}
		if err := service.RemoveSubscription(r.Context(), userID, subscription.Endpoint); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
