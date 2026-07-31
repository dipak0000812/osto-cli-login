# Osto CLI Login

A CLI authentication system written in Go. It supports user registration, password-based login, optional TOTP-based two-factor authentication, and session management. Everything runs in Docker with a PostgreSQL backend.

## Features

- Login with bcrypt password hashing and optional TOTP 2FA (RFC 6238, Google Authenticator compatible)
- Database-backed session management with configurable session expiry (default: 30 minutes)
- Account lockout after N failed attempts (default: 5 attempts, 15 minute lockout)
- User registration with password confirmation and length validation
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

Request flow:

```
CLI (cli.go)
 │
 ├── auth.Repository    ← registration, login, lockout
 │
 ├── session.Repository ← create, validate, delete
 │
 └── totp.Service       ← generate secret, validate code
          │
          ▼
      PostgreSQL
```

`main.go` initializes the database connection, constructs the application components, and starts the CLI. Dependencies are injected explicitly; packages do not rely on global state.

## Engineering Decisions

**bcrypt cost 12**: Cost 10 is the common default; 12 slows each hash to roughly 300ms, which meaningfully increases the cost of offline brute-force without making login noticeably slow.

**bcrypt 72-byte limit**: bcrypt silently truncates input beyond 72 bytes. The application enforces this limit explicitly so two different passwords cannot produce the same hash.

**Timing attack mitigation**: Login performs a dummy bcrypt comparison for unknown users to reduce response-time differences that could otherwise reveal whether a username exists.

**Database sessions over JWTs**: Session tokens are UUID v4 strings stored in PostgreSQL. Logging out immediately invalidates the session by deleting its database record. JWTs cannot be revoked without additional server-side state.

**TIMESTAMPTZ**: All timestamp columns use `TIMESTAMPTZ`. Using `TIMESTAMP` without timezone ties lockout and expiry comparisons to server and database timezone configuration. `TIMESTAMPTZ` stores UTC and is unambiguous.

**TOCTOU-safe registration**: Registration relies on the database's unique constraint rather than performing a check-before-insert, avoiding race conditions during concurrent registration.

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

Tests cover:

- User registration and duplicate username handling
- Login, wrong password, and account lockout
- Session creation, validation, and expiry
- TOTP secret generation and code validation

The project passes the Go race detector. The `cli` and `db` packages contain orchestration code and integration points; their logic is exercised through the tested packages.

## Limitations

- The session token lives in process memory. If the container restarts while a user is logged in, the token is lost and the user must log in again. The session row remains in the database until it expires.
- There is no background cleanup for expired sessions. They accumulate until the user logs out. A `DELETE FROM sessions WHERE expires_at < NOW()` run periodically would handle this.
- The TOTP secret is stored in plaintext in the database. Encrypting it at rest would require a key management approach that is out of scope for this assignment.
- Account lockout resets on successful login. There is no admin interface to manually unlock an account.

## License

MIT License. See [LICENSE](LICENSE) for details.
