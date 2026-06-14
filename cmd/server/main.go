package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"budgets_be/internal/advisor"
	"budgets_be/internal/asset"
	"budgets_be/internal/auth"
	"budgets_be/internal/bill"
	"budgets_be/internal/budget"
	"budgets_be/internal/config"
	"budgets_be/internal/couple"
	"budgets_be/internal/dashboard"
	"budgets_be/internal/database"
	"budgets_be/internal/goal"
	"budgets_be/internal/openrouter"
	"budgets_be/internal/receipt"
	"budgets_be/internal/report"
	"budgets_be/internal/transaction"
	"budgets_be/internal/wallet"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil && !os.IsNotExist(err) {
		log.Fatalf("load env: %v", err)
	}

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(context.Background(), db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	openRouterClient := openrouter.NewClient(openrouter.Config{
		APIKey:        cfg.OpenRouterAPIKey,
		PrimaryModel:  cfg.OpenRouterPrimaryModel,
		FallbackModel: cfg.OpenRouterFallbackModel,
		CheapModel:    cfg.OpenRouterCheapModel,
		ReceiptModel:  cfg.OpenRouterReceiptModel,
		HTTPClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	})

	authService := auth.Service{
		Repo:      auth.PostgresRepository{DB: db},
		JWTSecret: cfg.JWTSecret,
	}

	mux := http.NewServeMux()
	advisor.RegisterRoutes(mux, advisor.Service{
		LLM: openRouterClient,
		Report: report.Service{
			Repo: report.PostgresRepository{DB: db},
		},
		Store: advisor.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	auth.RegisterRoutes(mux, authService)
	wallet.RegisterRoutes(mux, wallet.Service{
		Repo: wallet.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	transaction.RegisterRoutes(mux, transaction.Service{
		Repo: transaction.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	receipt.RegisterRoutes(mux, receipt.Service{
		Parser: openRouterClient,
	}, auth.AuthMiddleware(authService))
	dashboard.RegisterRoutes(mux, dashboard.Service{
		Repo: dashboard.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	budget.RegisterRoutes(mux, budget.Service{
		Repo: budget.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	bill.RegisterRoutes(mux, bill.Service{
		Repo: bill.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	couple.RegisterRoutes(mux, couple.Service{
		Repo: couple.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	goal.RegisterRoutes(mux, goal.Service{
		Repo: goal.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	report.RegisterRoutes(mux, report.Service{
		Repo: report.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	asset.RegisterRoutes(mux, asset.Service{
		Repo: asset.PostgresRepository{DB: db},
	}, auth.AuthMiddleware(authService))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			log.Printf("readiness check failed: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           http.MaxBytesHandler(recoverMiddleware(securityHeadersMiddleware(corsMiddleware(mux, cfg.AllowedOrigins))), cfg.MaxRequestBodyBytes),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("budgets_be listening on http://localhost:%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowedOriginSet := map[string]struct{}{}
	allowAnyOrigin := false
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAnyOrigin = true
			continue
		}
		allowedOriginSet[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowAnyOrigin {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowedOriginSet[origin]; origin != "" && ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			if !allowAnyOrigin && origin != "" {
				if _, ok := allowedOriginSet[origin]; !ok {
					w.WriteHeader(http.StatusForbidden)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic: %v", recovered)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
