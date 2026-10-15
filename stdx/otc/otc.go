// package otc provides alphanumeric One Time Codes that can be used for email-based 2FA,
// account verification and more.
package otc

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"strings"
	"unicode"

	"markdown.ninja/stdx/crypto"
)

const (
	otcPrefix   = "otc"
	version1    = "v1"
	tokenPrefix = otcPrefix + "." + version1 + "."
)

var (
	ErrTokenIsNotValid = errors.New("otc: token is not valid")
)

type Code struct {
	code  string
	token string
}

type NewCodeOptions struct {
}

func New(length uint16) (code Code, err error) {
	randomBytes := make([]byte, length)

	_, err = rand.Read(randomBytes)
	if err != nil {
		return
	}

	codeText := base32.StdEncoding.EncodeToString(randomBytes)
	// format code (with '-')

	hash := crypto.HashPassword([]byte(codeText), crypto.DefaultHashPasswordParams)
	encodedHash := base64.RawURLEncoding.EncodeToString([]byte(hash))

	code = Code{
		code:  codeText,
		token: tokenPrefix + encodedHash,
	}

	return
}

func (code *Code) Code() string {
	return code.code
}

// CodeHTML returns the code wrapped in a <span> and with numbers wrapped in <span style="color: red">
func (code *Code) CodeHTML() (ret string) {
	ret = "<span>"
	for _, c := range code.code {
		if unicode.IsLetter(c) || c == '-' {
			ret += string(c)
		} else {
			ret += `<span style="color: red">`
			ret += string(c)
			ret += `</span>`
		}
	}

	ret += "</span>"
	return
}

// Token returns a token of the form otc.v[N].[XXXX]
// where [N] is the version number of the token
// and [XXXX] is Base64URL encoded data
// The token should be stored in a database or a similar secure place
// and use it later to verify that a code is valid
func (code *Code) Token() string {
	return code.token
}

func Verify(code, token string) bool {
	if strings.Count(token, ".") != 2 {
		return false
	}
	if !strings.HasPrefix(token, tokenPrefix) {
		return false
	}

	encodedHashStart := strings.LastIndexByte(token, '.')
	encodedHash := token[encodedHashStart+1:]
	hash, err := base64.RawURLEncoding.DecodeString(encodedHash)
	if err != nil {
		return false
	}

	// TODO: cleanup code from '_'

	return crypto.VerifyPasswordHash([]byte(code), string(hash))
}
