# Osto CLI Login

A CLI authentication system written in Go. It supports user registration, password-based login, optional TOTP-based two-factor authentication, and session management. Everything runs in Docker with a PostgreSQL backend.

## Features

- User registration with bcrypt password hashing (cost 12)
- Login with optional TOTP 2FA (RFC 6238, Google Authenticator compatible)
- Account lockout after N failed attempts (default: 5 attempts, 15 minute lockout)
- Database-backed session management with configurable session expiry (default: 30 minutes)
- Tab completion and command history in the CLI
- Data persists across container restarts via a named Docker volume

## Getting Started

**Prerequisites**: Docker and Docker Compose.

```bash
git clone https://github.com/dipak0000812/osto-cli-login.git
cd osto-cli-login
docker compose up --build
```

The database schema is applied automatically on first start via `docker-entrypoint-initdb.d`.

**Environment variables** (configured in `docker-compose.yml`):

```
DB_HOST=postgres
DB_PORT=5432
DB_USER=osto
DB_PASSWORD=            # required, no default
DB_NAME=ostodb
SESSION_TIMEOUT_MINUTES=30
MAX_FAILED_ATTEMPTS=5
LOCKOUT_MINUTES=15
```

`DB_PASSWORD` has no default. The application exits on startup if it is not set.

**Running locally without Docker** (requires a running PostgreSQL instance):

```bash
export DB_HOST=localhost DB_PASSWORD=yourpassword DB_USER=osto DB_NAME=ostodb
go run .
```

## Available Commands

Before login:
```
register    Create a new account
login       Authenticate with username and password
help        List commands
exit        Quit
```

After login:
```
whoami        Show username, registration date, 2FA status, session expiry, last login
enable-2fa    Generate a TOTP secret, display the otpauth:// URL, confirm with a code
disable-2fa   Disable TOTP after confirming with the current code
logout        Invalidate the session token and return to the unauthenticated prompt
exit          Logout and quit
```

## Architecture

```
main.go                 Wires dependencies, starts the CLI loop
internal/db/db.go       Opens the database connection, sets pool limits
internal/auth/auth.go   Registration, login, lockout, TOTP update
internal/session/       Session create, lookup, delete
internal/totp/totp.go   TOTP key generation and validation
internal/cli/cli.go     Command routing and user interaction
migrations/001_init.sql users and sessions schema
```

`main.go` initializes the database connection, constructs the application components, and starts the CLI. Dependencies are injected explicitly; packages do not rely on global state.

## Engineering Decisions

**bcrypt cost 12**: Cost 10 is the common default; 12 makes each hash operation take roughly 300ms on typical hardware, which slows offline brute-force while staying acceptable for a login flow.

**bcrypt 72-byte limit**: bcrypt silently truncates input beyond 72 bytes. Rather than let two different passwords produce the same hash, the application rejects passwords over 72 bytes at registration and returns a wrong-password error at login.

**Timing attack mitigation**: When a username does not exist, the code still calls `bcrypt.CompareHashAndPassword` against a pre-computed dummy hash. This keeps the response time consistent and prevents enumerating valid usernames by measuring response time differences.

**Database sessions over JWTs**: Session tokens are UUID v4 strings stored in PostgreSQL. Logging out immediately invalidates the session by deleting its database record. JWTs cannot be revoked without additional server-side state; database sessions make revocation a single DELETE.

**TIMESTAMPTZ**: All timestamp columns use `TIMESTAMPTZ`. Using `TIMESTAMP` without timezone means lockout and expiry comparisons depend on the server and database being configured to the same timezone. `TIMESTAMPTZ` stores UTC, so the arithmetic is correct regardless of where the process runs.

**TOCTOU-safe registration**: Registration uses a single `INSERT` and catches the unique-constraint error (`pq` error code `23505`) rather than doing a SELECT-then-INSERT. A check-before-insert under concurrent requests can allow duplicate usernames in the window between the read and the write.

**PostgreSQL over SQLite**: PostgreSQL was chosen because the assignment requires a containerized database and it provides stronger transactional guarantees and concurrency semantics than SQLite while keeping the deployment straightforward with Docker Compose.

**Non-root container**: The Dockerfile creates a system user (`appuser`, uid 10001) and runs the binary as that user. A compromised process running as root in a container has a larger attack surface.

**Static binary**: Built with `CGO_ENABLED=0` and `-ldflags="-w -s"`. Producing a statically linked binary simplifies deployment and reduces the runtime image size.

## Security Notes

- Passwords are never stored in plaintext. Only the bcrypt hash is written to the database.
- All queries use parameterized arguments (`$1`, `$2`, ...). No string concatenation in SQL.
- Session tokens, TOTP secrets, and database connection strings are not logged.
- `.env` is listed in `.gitignore`.

## Testing

```bash
go test ./...
go test -race ./...
go vet ./...
```

The `auth`, `session`, and `totp` packages are covered by unit tests. The project passes the Go race detector (`go test -race ./...`). The `cli` and `db` packages contain orchestration code and integration points; their logic is exercised through the tested packages.

## Limitations

- The session token lives in process memory. If the container restarts while a user is logged in, the token is lost and the user must log in again. The session row remains in the database until it expires.
- There is no background cleanup for expired sessions. They accumulate until the user logs out. A `DELETE FROM sessions WHERE expires_at < NOW()` run periodically would handle this.
- The TOTP secret is stored in plaintext in the database. Encrypting it at rest would require a key management approach that is out of scope for this assignment.
- Account lockout resets on successful login. There is no admin interface to manually unlock an account.
