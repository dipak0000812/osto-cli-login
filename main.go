package main

import (
    "log"

    "github.com/dipak/osto-auth/internal/auth"
    "github.com/dipak/osto-auth/internal/cli"
    "github.com/dipak/osto-auth/internal/db"
    "github.com/dipak/osto-auth/internal/session"
    "github.com/dipak/osto-auth/internal/totp"
)

func main() {
    database, err := db.Connect()
    if err != nil {
        log.Fatalf("failed to connect to database: %v", err)
    }
    defer database.Close()

    authRepo := auth.NewRepository(database)
    sessionRepo := session.NewRepository(database)
    totpSvc := totp.NewService()

    app := cli.New(authRepo, sessionRepo, totpSvc)

    if err := app.Run(); err != nil {
        log.Fatal(err)
    }
}