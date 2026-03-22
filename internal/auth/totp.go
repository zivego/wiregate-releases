package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	totpDigits = 6
	totpStep   = 30 * time.Second
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret creates a new RFC 6238 compatible base32 secret.
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return totpEncoding.EncodeToString(buf), nil
}

// TOTPProvisioningURI builds an otpauth URI for authenticator apps.
func TOTPProvisioningURI(issuer, accountName, secret string) string {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = "WireGate"
	}
	label := url.QueryEscape(issuer + ":" + strings.TrimSpace(accountName))
	values := url.Values{}
	values.Set("secret", strings.TrimSpace(secret))
	values.Set("issuer", issuer)
	values.Set("digits", strconv.Itoa(totpDigits))
	values.Set("period", strconv.Itoa(int(totpStep/time.Second)))
	values.Set("algorithm", "SHA1")
	return "otpauth://totp/" + label + "?" + values.Encode()
}

// ValidateTOTP checks a 6-digit TOTP code with a +/- 1 time-step drift window.
func ValidateTOTP(secret, code string, now time.Time) bool {
	secret = strings.TrimSpace(secret)
	code = normalizeOTPCode(code)
	if secret == "" || len(code) != totpDigits {
		return false
	}
	key, err := totpEncoding.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	counter := now.UTC().Unix() / int64(totpStep/time.Second)
	for _, delta := range []int64{-1, 0, 1} {
		candidate := totpCode(key, counter+delta)
		if subtleConstantTimeEqual(candidate, code) {
			return true
		}
	}
	return false
}

// GenerateTOTPCode returns the current 6-digit code for a valid secret.
func GenerateTOTPCode(secret string, at time.Time) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", fmt.Errorf("empty secret")
	}
	key, err := totpEncoding.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	counter := at.UTC().Unix() / int64(totpStep/time.Second)
	return totpCode(key, counter), nil
}

func totpCode(key []byte, counter int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	binaryCode := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	code := binaryCode % 1000000
	return fmt.Sprintf("%06d", code)
}

func normalizeOTPCode(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, " ", ""))
	if len(value) != totpDigits {
		return ""
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return ""
		}
	}
	return value
}

func subtleConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
