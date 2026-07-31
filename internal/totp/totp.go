package totp

import (
    "fmt"
    "github.com/pquerna/otp/totp"
    "github.com/pquerna/otp"
)

type Service struct{}

func NewService() *Service {
    return &Service{}
}

func (s *Service) Generate(username string) (secret, qrURL string, err error) {
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      "Osto Auth",
        AccountName: username,
        Algorithm:   otp.AlgorithmSHA1,
    })
    if err != nil {
        return "", "", fmt.Errorf("Generate: %w", err)
    }
    return key.Secret(), key.URL(), nil
}

func (s *Service) Validate(secret, code string) bool {
    return totp.Validate(code, secret)
}