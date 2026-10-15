package stringsx

import (
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func IsLower(input string) bool {
	for _, c := range input {
		if unicode.IsLetter(c) && !unicode.IsLower(c) {
			return false
		}
	}

	return true
}

func IsUpper(input string) bool {
	for _, c := range input {
		if unicode.IsLetter(c) && !unicode.IsUpper(c) {
			return false
		}
	}

	return true
}

func ToAscii(input string) (output string, err error) {
	output, _, err = transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), input)
	return output, err
}
