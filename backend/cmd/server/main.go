package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const shutdownDeadline = 15 * time.Second

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; using environment variables")
	}
	db, err := connectDB()
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	log.Println("connected to PostgreSQL")

	store := NewJobStore(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var background sync.WaitGroup
	for i := 0; i < 10; i++ {
		worker := NewWorker(fmt.Sprintf("worker-%d", i+1), store)
		background.Add(1)
		go func() {
			defer background.Done()
			worker.Start(ctx)
		}()
	}
	background.Add(1)
	go func() {
		defer background.Done()
		startRecovery(ctx, store)
	}()

	mux := http.NewServeMux()
	handler := NewHandler(store, os.Getenv("ADMIN_API_KEY"))
	mux.HandleFunc("POST /admin/clients", handler.CreateAPIClient)
	mux.HandleFunc("POST /admin/users", handler.CreateUser)
	mux.HandleFunc("POST /login", handler.Login)
	mux.HandleFunc("POST /logout", handler.Logout)
	mux.HandleFunc("POST /jobs", handler.CreateJob)
	mux.HandleFunc("GET /jobs", handler.ListJobs)
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
	log.Printf("server listening on port %s", port)
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		// Stop accepting requests and drain handlers before canceling workers.
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownDeadline)
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown: %v", err)
			_ = server.Close()
		}
		shutdownCancel()
	case err := <-serveResult:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server failed: %v", err)
		}
	}
	stop()
	// If the listener failed independently, close any remaining HTTP handlers.
	_ = server.Close()
	backgroundCtx, backgroundCancel := context.WithTimeout(context.Background(), shutdownDeadline)
	if !waitForBackground(&background, backgroundCtx.Done()) {
		log.Printf("background shutdown exceeded %s; waiting for database users to exit before closing PostgreSQL", shutdownDeadline)
		background.Wait()
	}
	backgroundCancel()
	db.Close()
}

// waitForBackground waits for all registered background goroutines until the
// shutdown deadline. On timeout, callers must still ensure DB users have exited
// before closing their shared pool.
func waitForBackground(background *sync.WaitGroup, deadline <-chan struct{}) bool {
	done := make(chan struct{})
	go func() {
		background.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-deadline:
		return false
	}
}

func newDashboardHandler() (http.Handler, error) {
	paths := []string{os.Getenv("WEBSITE_DIR"), "../website", "website", "../../../website"}
	for _, path := range paths {
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			return http.FileServer(http.Dir(path)), nil
		}
	}
	return nil, fmt.Errorf("website directory not found; set WEBSITE_DIR")
}
