package jwt

import (
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"markdown.ninja/pkg/kms"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/memorycache"
	"markdown.ninja/stdx/uuid"
)

type Provider struct {
	issuer *string
	store  KeyStore
	kms    *kms.Kms

	signingKey         atomic.Pointer[HmacSha512Key]
	verifyingKeysCache *memorycache.Cache[string, HmacSha512Key]
}

type NewProviderOptions struct {
	Issuer string
}

func NewProvider(ctx context.Context, db db.DB, kms *kms.Kms, options *NewProviderOptions) (*Provider, error) {
	if options == nil {
		options = defaultProviderOptions()
	}
	var issuer *string
	if options.Issuer != "" {
		issuer = &options.Issuer
	}

	keyStore := newPostgresKeyStore(db)

	var signingKey HmacSha512Key
	signingJwtKey, err := keyStore.GetLatestKey(ctx)
	if err == nil {
		signingKey, err = decryptJwtSecretKey(ctx, kms, signingJwtKey)
		if err != nil {
			return nil, fmt.Errorf("jwt.NewProvider: error decrypting JWT key: %w", err)
		}
	} else {
		if !errors.Is(err, ErrKeyNotFound) {
			return nil, fmt.Errorf("jwt.NewProvider: error finding latest key: %w", err)
		}
		newSigningKey, err := generateAndSaveNewJwtKey(ctx, keyStore, kms)
		if err != nil {
			return nil, fmt.Errorf("jwt.NewProvider: error generating new JWT key: %w", err)
		}
		signingKey = newSigningKey.hmacKey
	}

	verifyingKeysCache := memorycache.New(
		memorycache.WithCapacity[string, HmacSha512Key](100),
		memorycache.WithTTL[string, HmacSha512Key](12*time.Hour),
	)
	verifyingKeysCache.Set(signingKey.id, signingKey, memorycache.DefaultTTL)

	provider := &Provider{
		issuer:             issuer,
		store:              keyStore,
		kms:                kms,
		signingKey:         atomic.Pointer[HmacSha512Key]{},
		verifyingKeysCache: verifyingKeysCache,
	}
	provider.signingKey.Store(&signingKey)
	return provider, nil
}

func defaultProviderOptions() *NewProviderOptions {
	return &NewProviderOptions{
		Issuer: "",
	}
}

func (provider *Provider) NewSignedToken(data any, options *TokenOptions) (token string, err error) {
	tokenBuffer := bytes.NewBuffer(make([]byte, 0, 100))
	signingKey := provider.signingKey.Load()

	// Header
	header := header{Algorithm: AlgorithmHS512, Type: TypeJWT, KeyID: signingKey.id}
	headerJson, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("jwt: encoding the header to JSON: %w", err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJson)
	tokenBuffer.WriteString(encodedHeader)
	tokenBuffer.WriteString(".")

	// Claims
	var claimsJson []byte
	var dataJson []byte
	var reservedClaims = reservedClaims{}

	if provider.issuer != nil {
		reservedClaims.Issuer = *provider.issuer
	}

	if options.ExpirationTime != nil {
		reservedClaims.ExpirationTime = options.ExpirationTime.Unix()
		if reservedClaims.ExpirationTime < 1 {
			return "", errors.New("jwt: ExpirationTime should not be < 1")
		}
	}
	if options.NotBefore != nil {
		reservedClaims.NotBefore = options.NotBefore.Unix()
		if reservedClaims.NotBefore < 1 {
			return "", errors.New("jwt: NotBefore should not be < 1")
		}
	}
	reservedClaims.JwtID = uuid.NewV7().String()

	claimsJson, err = json.Marshal(reservedClaims)
	if err != nil {
		return "", fmt.Errorf("jwt: encoding claims to JSON: %w", err)
	}
	dataJson, err = json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("jwt: encoding claims to JSON: %w", err)
	}

	// merge claims and reserverClaims
	if string(dataJson) != "{}" {
		dataJson[0] = ','
		claimsJson = append(claimsJson[:len(claimsJson)-1], dataJson...)
	}

	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJson)
	tokenBuffer.WriteString(encodedClaims)

	// Signature
	rawSignature := signTokenHMAC(sha512.New, signingKey.secret[:], tokenBuffer.Bytes())

	encodedSignature := base64.RawURLEncoding.EncodeToString(rawSignature)
	tokenBuffer.WriteString(".")
	tokenBuffer.WriteString(encodedSignature)

	token = tokenBuffer.String()

	return token, nil
}

// ParseAndVerifyToken parses a JWS token and verify its signature.
func (provider *Provider) ParseAndVerifyToken(token string, claimsPointer any) (err error) {
	if strings.Count(token, ".") != 2 {
		return ErrTokenIsNotValid
	}

	ctx := context.Background()

	// Header
	var header header
	headerEnd := strings.IndexByte(token, '.')
	encodedHeader := token[:headerEnd]
	headerJson, err := base64.RawURLEncoding.DecodeString(encodedHeader)
	if err != nil {
		return ErrTokenIsNotValid
	}
	err = json.Unmarshal(headerJson, &header)
	if err != nil {
		return ErrTokenIsNotValid
	}

	var verifyingKey HmacSha512Key
	cachedVerifyingKey := provider.verifyingKeysCache.Get(header.KeyID)
	if cachedVerifyingKey != nil {
		verifyingKey = cachedVerifyingKey.Value()
	} else {
		storedVerifyingKey, err := provider.store.GetKey(ctx, header.KeyID)
		if err != nil {
			return err
		}
		verifyingKey, err = decryptJwtSecretKey(ctx, provider.kms, storedVerifyingKey)
		if err != nil {
			return err
		}
		provider.verifyingKeysCache.Set(verifyingKey.id, verifyingKey, memorycache.DefaultTTL)
	}

	if header.Algorithm != verifyingKey.algorithm || header.Type != TypeJWT || header.KeyID == "" {
		return ErrTokenIsNotValid
	}

	// Signature
	signatureStart := strings.LastIndexByte(token, '.')
	encodedSignature := token[signatureStart+1:]
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil {
		return ErrTokenIsNotValid
	}

	encodedHeaderAndClaims := token[:signatureStart]

	err = verifyTokenHMAC(sha512.New, verifyingKey.secret[:], signature, []byte(encodedHeaderAndClaims))
	if err != nil {
		return err
	}

	// Reserved Claims
	encodedClaims := token[headerEnd+1 : signatureStart]
	claimsJson, err := base64.RawURLEncoding.DecodeString(encodedClaims)
	if err != nil {
		return ErrTokenIsNotValid
	}

	var reservedClaims reservedClaims
	err = json.Unmarshal(claimsJson, &reservedClaims)
	if err != nil {
		return ErrTokenIsNotValid
	}

	now := time.Now().Unix()
	if reservedClaims.ExpirationTime != 0 {
		if now > reservedClaims.ExpirationTime {
			return ErrTokenHasExpired
		}
	}
	if reservedClaims.NotBefore != 0 {
		if now < reservedClaims.NotBefore {
			return ErrTokenIsNotValid
		}
	}
	if provider.issuer != nil {
		if subtle.ConstantTimeCompare([]byte(reservedClaims.Issuer), []byte(*provider.issuer)) != 1 {
			return ErrIssuerIsNotValid
		}
	}

	err = json.Unmarshal(claimsJson, claimsPointer)
	if err != nil {
		return ErrTokenIsNotValid
	}

	return nil
}

func (provider *Provider) RotateKeys(ctx context.Context) {
	logger := slogx.FromCtx(ctx)

	now := time.Now().UTC()
	// We add a few minutes to account for clock shift and make sure we create a new key
	twentyFourHoursAgo := now.Add(-24 * time.Hour).Add(10 * time.Minute)
	thirtyFiveDaysAgo := now.Add(-35 * 24 * time.Hour)

	latestKey, err := provider.store.GetLatestKey(ctx)
	if err != nil {
		logger.Error(fmt.Sprintf("jwt.RotateKeys: error finding latest key: %s", err))
		return
	}
	if latestKey.CreatedAt.Before(twentyFourHoursAgo) {
		newSigningKey, err := generateAndSaveNewJwtKey(ctx, provider.store, provider.kms)
		if err != nil {
			logger.Error(fmt.Sprintf("jwt.RotateKeys: error generating new signing key: %s", err))
			return
		}
		provider.signingKey.Store(&newSigningKey.hmacKey)
		logger.Info("jwt.RotateKeys: new signing key created")
	}

	err = provider.store.DeleteOlderKeys(ctx, thirtyFiveDaysAgo)
	if err != nil {
		logger.Error(fmt.Sprintf("jwt.RotateKeys: error deleting older keys: %s", err))
	}
}
