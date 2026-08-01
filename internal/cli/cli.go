package cli

import (
    "context"
    "fmt"
    "strings"

    "github.com/chzyer/readline"
    "github.com/dipak/osto-auth/internal/auth"
    "github.com/dipak/osto-auth/internal/session"
    "github.com/dipak/osto-auth/internal/totp"
)

type CLI struct {
    authRepo    *auth.Repository
    sessionRepo *session.Repository
    totpSvc     *totp.Service
    rl          *readline.Instance

    currentToken  string
    currentUserID string
}

func New(
    authRepo *auth.Repository,
    sessionRepo *session.Repository,
    totpSvc *totp.Service,
) *CLI {
    return &CLI{
        authRepo:    authRepo,
        sessionRepo: sessionRepo,
        totpSvc:     totpSvc,
    }
}

func (c *CLI) Run() error {
    rl, err := readline.NewEx(&readline.Config{
        Prompt:          "osto> ",
        HistoryFile:     "/tmp/osto_history",
        AutoComplete:    c.completer(),
        InterruptPrompt: "^C",
        EOFPrompt:       "exit",
    })
    if err != nil {
        return err
    }
    defer rl.Close()
    c.rl = rl

    fmt.Println("╔══════════════════════════════════╗")
    fmt.Println("║      Osto Auth CLI v1.0          ║")
    fmt.Println("║  Type 'help' to see commands     ║")
    fmt.Println("╚══════════════════════════════════╝")
    fmt.Println()

    for {
        line, err := rl.Readline()
        if err != nil {
            break
        }

        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }

        parts := strings.Fields(line)
        cmd := parts[0]

        ctx := context.Background()

        if c.currentToken == "" {
            if done := c.handleUnauthenticated(ctx, cmd); done {
                break
            }
        } else {
            if done := c.handleAuthenticated(ctx, cmd); done {
                break
            }
        }
    }

    fmt.Println("\nGoodbye!")
    return nil
}

func (c *CLI) handleUnauthenticated(ctx context.Context, cmd string) bool {
    switch cmd {
    case "register":
        c.cmdRegister(ctx)
    case "login":
        c.cmdLogin(ctx)
    case "help":
        c.printUnauthHelp()
    case "exit":
        return true
    default:
        fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
    }
    return false
}

func (c *CLI) handleAuthenticated(ctx context.Context, cmd string) bool {
    switch cmd {
    case "whoami":
        c.cmdWhoami(ctx)
    case "enable-2fa":
        c.cmdEnable2FA(ctx)
    case "disable-2fa":
        c.cmdDisable2FA(ctx)
    case "logout":
        c.cmdLogout(ctx)
        return false
    case "help":
        c.printAuthHelp()
    case "exit":
        c.cmdLogout(ctx)
        return true
    default:
        fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
    }
    return false
}

func (c *CLI) cmdRegister(ctx context.Context) {
    fmt.Print("Enter username: ")
    username := c.readLine()
    if username == "" {
        fmt.Println("Username cannot be empty.")
        return
    }

    password := c.readPassword("Enter password: ")
    if len(password) < 8 {
        fmt.Println("Password must be at least 8 characters.")
        return
    }

    confirm := c.readPassword("Confirm password: ")
    if password != confirm {
        fmt.Println("Passwords do not match.")
        return
    }

    if err := c.authRepo.Register(ctx, username, password); err != nil {
        if err == auth.ErrUserExists {
            fmt.Println("Username already taken.")
            return
        }
        fmt.Printf("Registration failed: %v\n", err)
        return
    }

    fmt.Printf("Account created. Welcome, %s.\n", username)
}

func (c *CLI) cmdLogin(ctx context.Context) {
    fmt.Print("Enter username: ")
    username := c.readLine()

    password := c.readPassword("Enter password: ")

    user, err := c.authRepo.Login(ctx, username, password)
    if err != nil {
        fmt.Printf("%v\n", err)
        return
    }

    if user.TOTPEnabled && user.TOTPSecret.Valid {
        fmt.Print("Enter 2FA code: ")
        code := c.readLine()
        if !c.totpSvc.Validate(user.TOTPSecret.String, code) {
            fmt.Println("Invalid 2FA code.")
            return
        }
    }

    sess, err := c.sessionRepo.Create(ctx, user.ID)
    if err != nil {
        fmt.Printf("Failed to create session: %v\n", err)
        return
    }

    c.currentToken = sess.Token
    c.currentUserID = user.ID
    c.rl.SetPrompt(fmt.Sprintf("osto [%s]> ", username))

    fmt.Printf("\nWelcome back, %s.\n", user.Username)
    fmt.Printf("   Registered: %s\n", user.CreatedAt.Format("02 Jan 2006"))
    fmt.Printf("   2FA Status: %s\n", boolToStatus(user.TOTPEnabled))
    fmt.Printf("   Session expires: %s\n", sess.ExpiresAt.Format("15:04:05"))
    if user.LastLogin.Valid {
        fmt.Printf("   Last login: %s\n", user.LastLogin.Time.Format("02 Jan 2006 15:04:05"))
    }
    fmt.Println()
}

func (c *CLI) cmdWhoami(ctx context.Context) {
    sess, err := c.sessionRepo.Get(ctx, c.currentToken)
    if err != nil {
        fmt.Println("Session expired. Please login again.")
        c.currentToken = ""
        c.currentUserID = ""
        c.rl.SetPrompt("osto> ")
        return
    }

    user, err := c.authRepo.GetByID(ctx, c.currentUserID)
    if err != nil {
        fmt.Printf("Failed to get user details: %v\n", err)
        return
    }

    fmt.Println("┌─────────────────────────────────┐")
    fmt.Printf("│ Username:    %-20s│\n", user.Username)
    fmt.Printf("│ Registered:  %-20s│\n", user.CreatedAt.Format("02 Jan 2006"))
    fmt.Printf("│ 2FA Status:  %-20s│\n", boolToStatus(user.TOTPEnabled))
    fmt.Printf("│ Session exp: %-20s│\n", sess.ExpiresAt.Format("15:04:05"))
    if user.LastLogin.Valid {
        fmt.Printf("│ Last login:  %-20s│\n", user.LastLogin.Time.Format("02 Jan 06 15:04"))
    }
    fmt.Println("└─────────────────────────────────┘")
}

func (c *CLI) cmdEnable2FA(ctx context.Context) {
    user, err := c.authRepo.GetByID(ctx, c.currentUserID)
    if err != nil {
        fmt.Printf("%v\n", err)
        return
    }

    if user.TOTPEnabled {
        fmt.Println("2FA is already enabled.")
        return
    }

    secret, qrURL, err := c.totpSvc.Generate(user.Username)
    if err != nil {
        fmt.Printf("Failed to generate 2FA secret: %v\n", err)
        return
    }

    fmt.Println("\nScan this URL in Google Authenticator or Authy:")
    fmt.Println(qrURL)
    fmt.Printf("\nOr enter this secret manually: %s\n\n", secret)
    fmt.Print("Enter the 6-digit code from your app to confirm: ")
    code := c.readLine()

    if !c.totpSvc.Validate(secret, code) {
        fmt.Println("Invalid code. 2FA not enabled.")
        return
    }

    if err := c.authRepo.UpdateTOTP(ctx, c.currentUserID, secret, true); err != nil {
        fmt.Printf("Failed to enable 2FA: %v\n", err)
        return
    }

    fmt.Println("2FA enabled. Your account now requires a code on login.")
}

func (c *CLI) cmdDisable2FA(ctx context.Context) {
    user, err := c.authRepo.GetByID(ctx, c.currentUserID)
    if err != nil {
        fmt.Printf("%v\n", err)
        return
    }

    if !user.TOTPEnabled {
        fmt.Println("2FA is not enabled.")
        return
    }

    fmt.Print("Enter your 2FA code to confirm disabling: ")
    code := c.readLine()

    if !c.totpSvc.Validate(user.TOTPSecret.String, code) {
        fmt.Println("Invalid code. 2FA not disabled.")
        return
    }

    if err := c.authRepo.UpdateTOTP(ctx, c.currentUserID, "", false); err != nil {
        fmt.Printf("Failed to disable 2FA: %v\n", err)
        return
    }

    fmt.Println("2FA disabled.")
}

func (c *CLI) cmdLogout(ctx context.Context) {
    c.sessionRepo.Delete(ctx, c.currentToken)
    c.currentToken = ""
    c.currentUserID = ""
    c.rl.SetPrompt("osto> ")
    fmt.Println("Logged out.")
}

func (c *CLI) readLine() string {
    line, _ := c.rl.Readline()
    return strings.TrimSpace(line)
}

func (c *CLI) readPassword(prompt string) string {
    pwd, _ := c.rl.ReadPassword(prompt)
    return strings.TrimSpace(string(pwd))
}

func (c *CLI) completer() *readline.PrefixCompleter {
    return readline.NewPrefixCompleter(
        readline.PcItem("register"),
        readline.PcItem("login"),
        readline.PcItem("whoami"),
        readline.PcItem("enable-2fa"),
        readline.PcItem("disable-2fa"),
        readline.PcItem("logout"),
        readline.PcItem("help"),
        readline.PcItem("exit"),
    )
}

func (c *CLI) printUnauthHelp() {
    fmt.Println("\nAvailable commands:")
    fmt.Println("  register   Create a new account")
    fmt.Println("  login      Login to your account")
    fmt.Println("  help       Show this help message")
    fmt.Println("  exit       Quit the program")
    fmt.Println()
}

func (c *CLI) printAuthHelp() {
    fmt.Println("\nAvailable commands:")
    fmt.Println("  whoami       Show current user details")
    fmt.Println("  enable-2fa   Enable TOTP two-factor authentication")
    fmt.Println("  disable-2fa  Disable two-factor authentication")
    fmt.Println("  logout       End your session")
    fmt.Println("  help         Show this help message")
    fmt.Println("  exit         Logout and quit")
    fmt.Println()
}

func boolToStatus(b bool) string {
    if b {
        return "enabled"
    }
    return "disabled"
}