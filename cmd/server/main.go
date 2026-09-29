package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Warning: .env file not found")
	}

	db, err := connectDB()
	if err != nil {
		fmt.Println("Database connection failed:", err)
		return
	}

	defer db.Close(context.Background())

	fmt.Println("Connected to Neon PostgreSQL!")
	queue := NewQueue(10)
	store := NewJobStore(db)

	err = store.LoadJobsFromDB()
	if err != nil {
		fmt.Println("Failed to load jobs from database:", err)
		return
	}

	fmt.Println("Jobs loaded from database")

	for i := 0; i < 3; i++ {
		worker := NewWorker(fmt.Sprintf("worker-%d", i+1), queue, store)
		go worker.Start()
	}

	go startRecovery(store, queue)

	handler := NewHandler(queue, store)

	http.HandleFunc("POST /jobs", handler.CreateJob)
	http.HandleFunc("GET /jobs/{id}", handler.GetJob)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Distributed Job System is running!")
	})

	fmt.Println("Server starting on port 8080...")

	err = http.ListenAndServe(":8080", nil)

	if err != nil {
		fmt.Println("Server failed:", err)
	}
}
