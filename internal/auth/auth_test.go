package auth

import (
    "context"
    "database/sql"
    "os"
    "testing"
    "time"

    _ "github.com/lib/pq"
)

func TestAuthLifecycleAndLockout(t *testing.T) {
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
    repo := NewRepository(db)

    username := "authtestuser"
    password := "testpassword123"

    // Cleanup first if left over
    db.ExecContext(ctx, "DELETE FROM users WHERE username = $1", username)
    defer func() {
        db.ExecContext(ctx, "DELETE FROM users WHERE username = $1", username)
    }()

    // 1. Register User
    err = repo.Register(ctx, username, password)
    if err != nil {
        t.Fatalf("Register failed: %v", err)
    }

    // 2. Register Duplicate User
    err = repo.Register(ctx, username, password)
    if err != ErrUserExists {
        t.Errorf("Expected ErrUserExists, got %v", err)
    }

    // 3. Successful Login
    user, err := repo.Login(ctx, username, password)
    if err != nil {
        t.Fatalf("Login failed: %v", err)
    }
    if user.Username != username {
        t.Errorf("Expected Username %s, got %s", username, user.Username)
    }

    // 4. Failed Login Increments Attempts
    os.Setenv("MAX_FAILED_ATTEMPTS", "3")
    os.Setenv("LOCKOUT_MINUTES", "5")
    defer func() {
        os.Unsetenv("MAX_FAILED_ATTEMPTS")
        os.Unsetenv("LOCKOUT_MINUTES")
    }()

    // Attempt 1 incorrect
    _, err = repo.Login(ctx, username, "wrongpassword")
    if err != ErrWrongPassword {
        t.Errorf("Expected ErrWrongPassword, got %v", err)
    }

    // Check failed attempts
    user, err = repo.GetByUsername(ctx, username)
    if err != nil {
        t.Fatalf("GetByUsername failed: %v", err)
    }
    if user.FailedAttempts != 1 {
        t.Errorf("Expected FailedAttempts = 1, got %d", user.FailedAttempts)
    }

    // Attempt 2 incorrect
    _, err = repo.Login(ctx, username, "wrongpassword")
    if err != ErrWrongPassword {
        t.Errorf("Expected ErrWrongPassword, got %v", err)
    }

    // Attempt 3 incorrect (causes lockout)
    _, err = repo.Login(ctx, username, "wrongpassword")
    if err != ErrWrongPassword {
        t.Errorf("Expected ErrWrongPassword, got %v", err)
    }

    // Attempt 4 (locked out)
    _, err = repo.Login(ctx, username, "wrongpassword")
    if err == nil {
        t.Fatal("Expected login to fail due to lockout, got nil error")
    }

    // Check if error contains account locked warning
    user, err = repo.GetByUsername(ctx, username)
    if err != nil {
        t.Fatalf("GetByUsername failed: %v", err)
    }
    if !user.LockedUntil.Valid || user.LockedUntil.Time.Before(time.Now()) {
        t.Error("Expected user to be locked out")
    }
}
