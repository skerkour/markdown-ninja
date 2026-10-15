package auth

import (
	"github.com/fxamacker/cbor/v2"
	"markdown.ninja/pkg/jwt"
	"markdown.ninja/stdx/uuid"
)

type AccessToken struct {
	UserID  uuid.UUID `json:"sub"`
	Name    string    `json:"name"`
	Email   string    `json:"email"`
	IsAdmin bool      `json:"is_admin"`
	jwt.RegisteredClaims
}

func (token *AccessToken) UnmarshalCBOR(data []byte) error {
	type cborAccessToken struct {
		UserID  string `cbor:"sub"`
		Name    string `cbor:"name"`
		Email   string `cbor:"email"`
		IsAdmin bool   `cbor:"is_admin"`
		jwt.RegisteredClaims
	}
	var cborToken cborAccessToken

	if err := cbor.Unmarshal(data, &cborToken); err != nil {
		return err
	}

	subUuid, err := uuid.Parse(cborToken.UserID)
	if err != nil {
		return err
	}

	*token = AccessToken{
		UserID:           subUuid,
		Name:             cborToken.Name,
		Email:            cborToken.Email,
		IsAdmin:          cborToken.IsAdmin,
		RegisteredClaims: cborToken.RegisteredClaims,
	}
	return nil
}
