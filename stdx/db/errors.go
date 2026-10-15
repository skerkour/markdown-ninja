package db

import "strings"

const (
	ErrAlreadyExists = "duplicate key value violates unique constraint"
)

func IsErrAlreadyExists(err error) bool {
	return strings.Contains(err.Error(), ErrAlreadyExists)
}
