package receipt

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"budgets_be/internal/auth"
)

const maxReceiptImageSize = 8 << 20

func RegisterRoutes(mux *http.ServeMux, service Service, authMiddleware func(http.Handler) http.Handler) {
	mux.Handle("POST /api/receipts/parse", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserIDFromContext(r.Context()); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		dataURL, err := imageDataURLFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		response, err := service.ParseImage(r.Context(), dataURL)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, response)
	})))
}

func imageDataURLFromRequest(r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxReceiptImageSize)
	if err := r.ParseMultipartForm(maxReceiptImageSize); err != nil {
		return "", errors.New("receipt image is required")
	}

	file, header, err := r.FormFile("receipt")
	if err != nil {
		return "", errors.New("receipt image is required")
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", errors.New("receipt must be an image")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxReceiptImageSize+1))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("receipt image is empty")
	}
	if len(data) > maxReceiptImageSize {
		return "", errors.New("receipt image must be 8MB or smaller")
	}

	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
