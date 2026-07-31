package session

import (
    "context"
    "database/sql"
    "testing"

    _ "github.com/lib/pq"
)

func TestSessionLifecycle(t *testing.T) {
    connStr := "host=localhost port=5432 user=osto password=ostopassword dbname=ostodb sslmode=disable"
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        t.Skip("Database not available: skipping integration test")
        return
    }
    if err := db.Ping(); err != nil {
        t.Skip("Database not running: skipping integration test")
        return
    }
    defer db.Close()

    ctx := context.Background()

    // Create a temporary user first for referencing foreign key
    var userID string
    err = db.QueryRowContext(ctx,
        "INSERT INTO users (username, password_hash) VALUES ($1, $2) RETURNING id",
        "sessiontestuser", "somehash",
    ).Scan(&userID)
    if err != nil {
        t.Fatalf("Failed to create test user: %v", err)
    }
    defer func() {
        db.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
    }()

    repo := NewRepository(db)

    // 1. Create session
    sess, err := repo.Create(ctx, userID)
    if err != nil {
        t.Fatalf("Create session failed: %v", err)
    }

    if sess.Token == "" {
        t.Error("Token is empty")
    }

    // 2. Get session
    retrieved, err := repo.Get(ctx, sess.Token)
    if err != nil {
        t.Fatalf("Get session failed: %v", err)
    }

    if retrieved.UserID != userID {
        t.Errorf("Expected UserID %s, got %s", userID, retrieved.UserID)
    }

    // 3. Delete session
    err = repo.Delete(ctx, sess.Token)
    if err != nil {
        t.Fatalf("Delete session failed: %v", err)
    }

    // 4. Verify session deleted
    _, err = repo.Get(ctx, sess.Token)
    if err != ErrSessionExpired {
        t.Errorf("Expected ErrSessionExpired, got %v", err)
    }
}
