package totp

import (
    "testing"
    "time"
    "github.com/pquerna/otp/totp"
)

func TestTOTPGenerateAndValidate(t *testing.T) {
    svc := NewService()

    secret, qrURL, err := svc.Generate("testuser")
    if err != nil {
        t.Fatalf("Generate failed: %v", err)
    }

    if secret == "" {
        t.Error("Secret is empty")
    }

    if qrURL == "" {
        t.Error("QR URL is empty")
    }

    code, err := totp.GenerateCode(secret, time.Now().UTC())
    if err != nil {
        t.Fatalf("Failed to generate code for validation: %v", err)
    }

    if !svc.Validate(secret, code) {
        t.Error("Validation failed for correct code")
    }

    if svc.Validate(secret, "000000") {
        t.Error("Validation succeeded for incorrect code")
    }
}
