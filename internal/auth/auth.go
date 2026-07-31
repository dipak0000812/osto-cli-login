package auth

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "os"
    "strconv"
    "time"

    "github.com/lib/pq"
    "golang.org/x/crypto/bcrypt"
)

var (
    ErrUserExists       = errors.New("username already exists")
    ErrUserNotFound     = errors.New("user not found")
    ErrWrongPassword    = errors.New("incorrect password")
    ErrAccountLocked    = errors.New("account is temporarily locked")
    ErrInvalid2FA       = errors.New("invalid 2FA code")
    ErrTOTPNotEnabled   = errors.New("2FA is not enabled")
    ErrTOTPAlreadyOn    = errors.New("2FA is already enabled")
)

type User struct {
    ID             string
    Username       string
    PasswordHash   string
    TOTPSecret     sql.NullString
    TOTPEnabled    bool
    FailedAttempts int
    LockedUntil    sql.NullTime
    LastLogin      sql.NullTime
    CreatedAt      time.Time
}

type Repository struct {
    db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
    return &Repository{db: db}
}

// Pre-computed dummy hash to prevent timing attacks on non-existent usernames
var dummyHash = "$2a$12$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (r *Repository) Register(ctx context.Context, username, password string) error {
    if len(password) > 72 {
        return errors.New("password cannot exceed 72 bytes")
    }

    hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
    if err != nil {
        return fmt.Errorf("Register: hash password: %w", err)
    }

    _, err = r.db.ExecContext(ctx,
        "INSERT INTO users (username, password_hash) VALUES ($1, $2)",
        username, string(hash),
    )
    if err != nil {
        var pqErr *pq.Error
        if errors.As(err, &pqErr) && pqErr.Code == "23505" {
            return ErrUserExists
        }
        return fmt.Errorf("Register: insert: %w", err)
    }

    return nil
}

func (r *Repository) Login(ctx context.Context, username, password string) (*User, error) {
    if len(password) > 72 {
        return nil, ErrWrongPassword
    }

    user, err := r.GetByUsername(ctx, username)
    if err != nil {
        // Run dummy hash check to ensure constant time execution and prevent username enumeration
        _ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
        return nil, ErrWrongPassword
    }

    // check lockout
    if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
        remaining := time.Until(user.LockedUntil.Time).Minutes()
        return nil, fmt.Errorf("%w: try again in %.0f minutes", ErrAccountLocked, remaining)
    }

    // verify password
    if err := bcrypt.CompareHashAndPassword(
        []byte(user.PasswordHash), []byte(password),
    ); err != nil {
        if incErr := r.incrementFailedAttempts(ctx, user); incErr != nil {
            return nil, fmt.Errorf("login: failed to increment attempts: %w", incErr)
        }
        return nil, ErrWrongPassword
    }

    // reset failed attempts and update last login
    if _, err := r.db.ExecContext(ctx,
        "UPDATE users SET failed_attempts = 0, locked_until = NULL, last_login = NOW() WHERE id = $1",
        user.ID,
    ); err != nil {
        return nil, fmt.Errorf("login: failed to reset attempts: %w", err)
    }

    return user, nil
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*User, error) {
    var u User
    err := r.db.QueryRowContext(ctx,
        `SELECT id, username, password_hash, totp_secret, totp_enabled,
         failed_attempts, locked_until, last_login, created_at
         FROM users WHERE username = $1`,
        username,
    ).Scan(
        &u.ID, &u.Username, &u.PasswordHash,
        &u.TOTPSecret, &u.TOTPEnabled,
        &u.FailedAttempts, &u.LockedUntil,
        &u.LastLogin, &u.CreatedAt,
    )
    if err == sql.ErrNoRows {
        return nil, ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("GetByUsername: %w", err)
    }
    return &u, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*User, error) {
    var u User
    err := r.db.QueryRowContext(ctx,
        `SELECT id, username, password_hash, totp_secret, totp_enabled,
         failed_attempts, locked_until, last_login, created_at
         FROM users WHERE id = $1`,
        id,
    ).Scan(
        &u.ID, &u.Username, &u.PasswordHash,
        &u.TOTPSecret, &u.TOTPEnabled,
        &u.FailedAttempts, &u.LockedUntil,
        &u.LastLogin, &u.CreatedAt,
    )
    if err == sql.ErrNoRows {
        return nil, ErrUserNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("GetByID: %w", err)
    }
    return &u, nil
}

func (r *Repository) UpdateTOTP(ctx context.Context, userID, secret string, enabled bool) error {
    _, err := r.db.ExecContext(ctx,
        "UPDATE users SET totp_secret = $1, totp_enabled = $2 WHERE id = $3",
        secret, enabled, userID,
    )
    return err
}

func (r *Repository) incrementFailedAttempts(ctx context.Context, user *User) error {
    maxAttempts, _ := strconv.Atoi(os.Getenv("MAX_FAILED_ATTEMPTS"))
    if maxAttempts == 0 {
        maxAttempts = 5
    }
    lockoutMinutes, _ := strconv.Atoi(os.Getenv("LOCKOUT_MINUTES"))
    if lockoutMinutes == 0 {
        lockoutMinutes = 15
    }

    newAttempts := user.FailedAttempts + 1

    var err error
    if newAttempts >= maxAttempts {
        lockUntil := time.Now().Add(time.Duration(lockoutMinutes) * time.Minute)
        _, err = r.db.ExecContext(ctx,
            "UPDATE users SET failed_attempts = $1, locked_until = $2 WHERE id = $3",
            newAttempts, lockUntil, user.ID,
        )
    } else {
        _, err = r.db.ExecContext(ctx,
            "UPDATE users SET failed_attempts = $1 WHERE id = $2",
            newAttempts, user.ID,
        )
    }
    return err
}