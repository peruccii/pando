package session

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// charset sem caracteres ambíguos (sem 0/O/1/I/L)
const shortCodeCharset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// generateShortCode gera um código curto no formato XXX-YY
// Fácil de ditar por voz, case-insensitive
func generateShortCode() (string, error) {
	part1, err := randomString(3)
	if err != nil {
		return "", fmt.Errorf("generating short code part1: %w", err)
	}

	part2, err := randomString(2)
	if err != nil {
		return "", fmt.Errorf("generating short code part2: %w", err)
	}

	return part1 + "-" + part2, nil
}

// randomString gera uma string aleatória de tamanho n usando o charset seguro
func randomString(n int) (string, error) {
	result := make([]byte, n)
	charsetLen := big.NewInt(int64(len(shortCodeCharset)))

	for i := range result {
		idx, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("crypto rand: %w", err)
		}
		result[i] = shortCodeCharset[idx.Int64()]
	}

	return string(result), nil
}

// normalizeCode normaliza o código para comparação (uppercase, trim)
func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// validateCodeFormat verifica se o código tem o formato XXX-YY
func validateCodeFormat(code string) bool {
	normalized := normalizeCode(code)
	if len(normalized) != 6 {
		return false
	}
	if normalized[3] != '-' {
		return false
	}

	// Verificar se cada caractere está no charset
	for i, c := range normalized {
		if i == 3 {
			continue // pular o hífen
		}
		if !strings.ContainsRune(shortCodeCharset, c) {
			return false
		}
	}

	return true
}
