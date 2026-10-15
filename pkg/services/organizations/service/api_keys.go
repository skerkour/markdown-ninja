package service

import (
	"context"
	"crypto/rand"
	base32std "encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/zeebo/blake3"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/base32"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/guid"
)

const (
	apiKeySecretSize = 32
	apiKeyHashSize   = 32
	apiKeyIdSize     = 16
)

var (
	apiKeyRawTokenSize     = apiKeyIdSize + apiKeySecretSize
	apiKeyEncodedTokenSize = base32std.StdEncoding.WithPadding(base32std.NoPadding).EncodedLen(apiKeyRawTokenSize)
)

type parsedApiKey struct {
	Id      guid.GUID
	version int16
	secret  []byte
}

func (service *OrganizationsService) CheckCurrentApiKey(ctx context.Context, organizationID guid.GUID) (apiKey organizations.ApiKey, err error) {
	httpCtx := httpctx.FromCtx(ctx)
	if httpCtx == nil || httpCtx.ApiKey == nil {
		err = organizations.ErrApiKeyIsMissing
		return
	}

	apiKey = *httpCtx.ApiKey

	if !crypto.ConstantTimeCompare(apiKey.OrganizationID.Bytes(), organizationID.Bytes()) {
		err = kernel.ErrPermissionDenied
		return
	}

	return
}

func (service *OrganizationsService) generateApiKey(organizationID guid.GUID, name string) (ret organizations.ApiKeyWithToken, err error) {
	var secret [apiKeySecretSize]byte
	var tokenData [apiKeyIdSize + apiKeySecretSize]byte
	// we use time-based UUIDs because we need as much performance as possible
	apiKeyID := guid.NewTimeBased()
	now := time.Now().UTC()
	apiKeyVersion := int16(1)

	_, err = rand.Read(secret[:])
	if err != nil {
		err = fmt.Errorf("organizations: error generating new api key secret: %w", err)
		return
	}

	hash := service.generateApiKeyHash(apiKeyID, apiKeyVersion, secret[:], organizationID)

	copy(tokenData[0:apiKeyIdSize], apiKeyID.Bytes())
	copy(tokenData[apiKeyIdSize:], secret[:])

	token := base32.EncodeToString(tokenData[:])
	token = organizations.ApiKeyPrefix + token

	ret = organizations.ApiKeyWithToken{
		ApiKey: organizations.ApiKey{
			ID:             apiKeyID,
			CreatedAt:      now,
			UpdatedAt:      now,
			Name:           name,
			Version:        1,
			Hash:           hash[:],
			OrganizationID: organizationID,
		},
		Token: token,
	}

	return
}

func (service *OrganizationsService) verifyApiKey(storedApiKey organizations.ApiKey, parsedApiKey parsedApiKey) (err error) {
	hash := service.generateApiKeyHash(storedApiKey.ID, parsedApiKey.version, parsedApiKey.secret, storedApiKey.OrganizationID)

	if !crypto.ConstantTimeCompare(storedApiKey.Hash, hash[:]) {
		return organizations.ErrApiKeyIsNotValid
	}

	return nil
}

func (service *OrganizationsService) generateApiKeyHash(apiKeyID guid.GUID, version int16, secret []byte, organizationID guid.GUID) (hash [apiKeyHashSize]byte) {
	hasher := blake3.New()

	hasher.Write(apiKeyID.Bytes())
	binary.Write(hasher, binary.LittleEndian, version)
	hasher.Write(organizationID.Bytes())
	hasher.Write(secret)

	hasher.Sum(hash[:0])
	return hash
}

func (service *OrganizationsService) parseApiKey(token string) (parsedApiKey, error) {
	var ret parsedApiKey

	if len(token) != (len(organizations.ApiKeyPrefix)+apiKeyEncodedTokenSize) ||
		!strings.HasPrefix(token, organizations.ApiKeyPrefix) {
		return ret, organizations.ErrApiKeyIsNotValid
	}

	ret.version = 1

	rawToken, err := base32.DecodeString(token[len(organizations.ApiKeyPrefix):])
	if err != nil {
		return ret, organizations.ErrApiKeyIsNotValid
	}

	if len(rawToken) != apiKeyRawTokenSize {
		return ret, organizations.ErrApiKeyIsNotValid
	}

	sessionIdBytes := rawToken[:apiKeyIdSize]
	secret := rawToken[apiKeyIdSize:]

	ret.Id, err = guid.FromBytes(sessionIdBytes)
	if err != nil {
		return ret, organizations.ErrApiKeyIsNotValid
	}

	ret.secret = secret

	return ret, nil
}
