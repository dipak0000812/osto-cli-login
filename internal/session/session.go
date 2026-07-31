package session

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "os"
    "strconv"
    "time"

    "github.com/google/uuid"
)

var ErrSessionExpired = errors.New("session expired or not found")

type Session struct {
    ID        string
    UserID    string
    Token     string
    ExpiresAt time.Time
}

type Repository struct {
    db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
    return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, userID string) (*Session, error) {
    token := uuid.New().String()

    timeoutMinutes, _ := strconv.Atoi(os.Getenv("SESSION_TIMEOUT_MINUTES"))
    if timeoutMinutes == 0 {
        timeoutMinutes = 30
    }

    expiresAt := time.Now().Add(time.Duration(timeoutMinutes) * time.Minute)

    var sessionID string
    err := r.db.QueryRowContext(ctx,
        `INSERT INTO sessions (user_id, token, expires_at)
         VALUES ($1, $2, $3) RETURNING id`,
        userID, token, expiresAt,
    ).Scan(&sessionID)
    if err != nil {
        return nil, fmt.Errorf("Create session: %w", err)
    }

    return &Session{
        ID:        sessionID,
        UserID:    userID,
        Token:     token,
        ExpiresAt: expiresAt,
    }, nil
}

func (r *Repository) Get(ctx context.Context, token string) (*Session, error) {
    var s Session
    err := r.db.QueryRowContext(ctx,
        "SELECT id, user_id, token, expires_at FROM sessions WHERE token = $1",
        token,
    ).Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt)
    if err == sql.ErrNoRows {
        return nil, ErrSessionExpired
    }
    if err != nil {
        return nil, fmt.Errorf("Get session: %w", err)
    }

    if time.Now().After(s.ExpiresAt) {
        r.Delete(ctx, token)
        return nil, ErrSessionExpired
    }

    return &s, nil
}

func (r *Repository) Delete(ctx context.Context, token string) error {
    _, err := r.db.ExecContext(ctx,
        "DELETE FROM sessions WHERE token = $1", token,
    )
    return err
}
