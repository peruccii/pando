package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// GeneratePKCE gera os componentes do PKCE flow (code_verifier + code_challenge)
func GeneratePKCE() (*PKCEChallenge, error) {
	// Gerar code_verifier (43-128 chars, random)
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Gerar code_challenge = BASE64URL(SHA256(code_verifier))
	h := sha256.New()
	h.Write([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	// Gerar state (CSRF protection) - Usando Hex para maior compatibilidade
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, err
	}
	state := hex.EncodeToString(stateBytes)

	return &PKCEChallenge{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
		State:         state,
	}, nil
}
