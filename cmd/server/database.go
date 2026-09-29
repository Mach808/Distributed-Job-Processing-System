package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func connectDB() (*pgx.Conn, error) {
	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	conn, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		return nil, err
	}

	return conn, nil
}
