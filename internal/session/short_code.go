package session

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// charset sem caracteres ambíguos (sem 0/O/1/I/L)
const shortCodeCharset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const (
	shortCodePart1Len = 4
	shortCodePart2Len = 3

	legacyShortCodePart1Len = 3
	legacyShortCodePart2Len = 2
)

// generateShortCode gera um código curto no formato XXXX-XXX.
// Fácil de ditar por voz, case-insensitive
func generateShortCode() (string, error) {
	part1, err := randomString(shortCodePart1Len)
	if err != nil {
		return "", fmt.Errorf("generating short code part1: %w", err)
	}

	part2, err := randomString(shortCodePart2Len)
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

// validateCodeFormat verifica se o código tem formato suportado.
// Formato atual: XXXX-XXX
// Formato legado aceito: XXX-YY (evita quebra em sessões restauradas)
func validateCodeFormat(code string) bool {
	normalized := normalizeCode(code)

	if matchesCodeFormat(normalized, shortCodePart1Len, shortCodePart2Len) {
		return true
	}

	return matchesCodeFormat(normalized, legacyShortCodePart1Len, legacyShortCodePart2Len)
}

func matchesCodeFormat(code string, part1Len int, part2Len int) bool {
	expectedLen := part1Len + 1 + part2Len
	if len(code) != expectedLen {
		return false
	}
	if code[part1Len] != '-' {
		return false
	}

	// Verificar se cada caractere está no charset
	for i, c := range code {
		if i == part1Len {
			continue // pular o hífen
		}
		if !strings.ContainsRune(shortCodeCharset, c) {
			return false
		}
	}

	return true
}
