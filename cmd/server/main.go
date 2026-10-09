package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

//go:embed dashboard
var dashboardFiles embed.FS

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; using environment variables")
	}
	db, err := connectDB()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer db.Close()
	log.Println("connected to PostgreSQL")

	store := NewJobStore(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for i := 0; i < 3; i++ {
		worker := NewWorker(fmt.Sprintf("worker-%d", i+1), store)
		go worker.Start(ctx)
	}
	go startRecovery(ctx, store)

	mux := http.NewServeMux()
	handler := NewHandler(store)
	mux.HandleFunc("POST /jobs", handler.CreateJob)
	mux.HandleFunc("GET /jobs/{id}", handler.GetJob)
	dashboard, err := newDashboardHandler()
	if err != nil {
		log.Fatalf("load dashboard files: %v", err)
	}
	mux.Handle("GET /", dashboard)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
		}
	}()
	log.Printf("server listening on port %s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

func newDashboardHandler() (http.Handler, error) {
	dashboard, err := fs.Sub(dashboardFiles, "dashboard")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(dashboard)), nil
}
