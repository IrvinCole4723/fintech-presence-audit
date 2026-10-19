package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	presenceaudit "github.com/example/fintech-presence-audit"
)

func main() {
	client := presenceaudit.NewClient(os.Getenv("INFRAI_API_KEY"))
	reviewer := presenceaudit.Reviewer{Realtime: client, Channel: envOr("WORKSPACE_CHANNEL", "fintech-ops")}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments/review", reviewHandler(reviewer))
	server := &http.Server{Addr: envOr("LISTEN_ADDR", ":8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("payment presence service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func reviewHandler(reviewer presenceaudit.Reviewer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payment presenceaudit.PaymentEvent
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payment); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payment event"})
			return
		}
		result, err := reviewer.Review(r.Context(), payment)
		if err != nil {
			var apiErr *presenceaudit.APIError
			if errors.As(err, &apiErr) {
				writeRawJSON(w, apiErr.HTTPStatus, apiErr.Envelope)
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "realtime request failed"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeRawJSON(w http.ResponseWriter, status int, value json.RawMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(value, '\n'))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
