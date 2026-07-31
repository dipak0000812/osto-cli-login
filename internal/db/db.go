package db

import (
    "database/sql"
    "errors"
    "fmt"
    "os"
    _ "github.com/lib/pq"
)

func Connect() (*sql.DB, error) {
    password := os.Getenv("DB_PASSWORD")
    if password == "" {
        return nil, errors.New("DB_PASSWORD environment variable is required")
    }

    connStr := fmt.Sprintf(
        "host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
        getEnv("DB_HOST", "localhost"),
        getEnv("DB_PORT", "5432"),
        getEnv("DB_USER", "osto"),
        password,
        getEnv("DB_NAME", "ostodb"),
    )

    db, err := sql.Open("postgres", connStr)
    if err != nil {
        return nil, fmt.Errorf("db.Connect: open: %w", err)
    }

    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("db.Connect: ping: %w", err)
    }

    db.SetMaxOpenConns(10)
    db.SetMaxIdleConns(5)

    return db, nil
}

func getEnv(key, defaultVal string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return defaultVal
}