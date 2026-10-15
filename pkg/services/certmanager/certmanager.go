package certmanager

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pkg/kms"
	"markdown.ninja/pkg/server/httpctx"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/set"
)

type CertManager struct {
	db              db.DB
	autocertDomains set.Set[string]
	// the self-signed certificate used by default when client hello server name doesn't match any allowed domain
	defaultCertificate *tls.Certificate
	websitesService    websites.Service
	httpConfig         config.Http
	kms                *kms.Kms
	kernel             kernel.Service

	// cache           *memorycache.Cache[string, *tls.Certificate]
	autocertManager *autocert.Manager
}

type TlsCertificate struct {
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
	Key            string    `db:"key" json:"key"`
	EncryptedValue []byte    `db:"encrypted_value" json:"-"`
}

type DeleteTlsCertificateInput struct {
	Key string `json:"key"`
}

// Note that all hosts will be converted to Punycode via idna.Lookup.ToASCII so that
// Manager.GetCertificate can handle the Unicode IDN and mixedcase hosts correctly.
// Invalid hosts will be silently ignored.
func NewCertManager(ctx context.Context, db db.DB, kms *kms.Kms,
	autocertManager *autocert.Manager, websitesService websites.Service, httpConfig config.Http, kernel kernel.Service) (certManager *CertManager, err error) {

	selfSignedTlsCertificate, err := generateSelfSignedCert()
	if err != nil {
		return
	}

	autocertDomains := set.New[string]()
	autocertDomains.Insert(httpConfig.WebappDomain)
	autocertDomains.Insert(fmt.Sprintf("www.%s", httpConfig.WebappDomain))
	autocertDomains.Insert(httpConfig.WebsitesRootDomain)

	// certsCache := memorycache.New(
	// 	memorycache.WithCapacity[string, *tls.Certificate](1_000),
	// 	memorycache.WithTTL[string, *tls.Certificate](1*time.Hour),
	// )

	certManager = &CertManager{
		db:                 db,
		kms:                kms,
		autocertDomains:    autocertDomains,
		defaultCertificate: selfSignedTlsCertificate,
		autocertManager:    autocertManager,
		websitesService:    websitesService,
		httpConfig:         httpConfig,
		// cache:              certsCache,
		kernel: kernel,
	}

	// go func() {
	// 	for {
	// 		// delete older certificates every 12 hours
	// 		certManager.deleteOlderCertificates(ctx)
	// 		select {
	// 		case <-ctx.Done():
	// 			return
	// 		case <-time.After(12 * time.Hour):
	// 		}
	// 	}
	// }()

	return certManager, nil
}

func (certManager *CertManager) isAllowedDomain(ctx context.Context, host string) bool {
	if certManager.autocertDomains.Contains(host) ||
		// allow subdomains for WebsitesRootDomain only 1 level deep
		(strings.HasSuffix(host, certManager.httpConfig.WebsitesRootDomain) &&
			strings.Count(host, ".") == (strings.Count(certManager.httpConfig.WebsitesRootDomain, ".")+1)) {
		return true
	}

	_, err := certManager.websitesService.FindWebsiteByDomain(ctx, certManager.db, host)
	if err == nil {
		return true
	}

	return false
}

func (certManager *CertManager) DefaultCertificate() *tls.Certificate {
	return certManager.defaultCertificate
}

func (certManager *CertManager) GetCertificate(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if certManager.isAllowedDomain(context.Background(), clientHello.ServerName) {
		// if cachedCert := certManager.cache.Get(clientHello.ServerName); cachedCert != nil {
		// 	return cachedCert.Value(), nil
		// }

		cert, err := certManager.autocertManager.GetCertificate(clientHello)
		if err != nil {
			return cert, err
		}

		// certManager.cache.Set(clientHello.ServerName, cert, memorycache.DefaultTTL)
		return cert, nil
	}

	return certManager.DefaultCertificate(), nil
}

func (certManager *CertManager) Get(ctx context.Context, key string) ([]byte, error) {
	var cert TlsCertificate
	logger := slogx.FromCtx(ctx)

	err := certManager.db.Get(ctx, &cert, "SELECT * FROM tls_certificates WHERE key = $1", key)
	if err != nil {
		if err == sql.ErrNoRows {
			err = autocert.ErrCacheMiss
		} else {
			err = fmt.Errorf("certmanager.Get: error getting cert from db: %w", err)
			logger.Error(err.Error())
		}
		return nil, err
	}

	data, err := certManager.kms.Decrypt(ctx, cert.EncryptedValue, []byte(cert.Key))
	if err != nil {
		err = fmt.Errorf("certmanager.Get: error decrypting value: %w", err)
		logger.Error(err.Error())
		return nil, err
	}

	return data, nil
}

func (certManager *CertManager) Put(ctx context.Context, key string, data []byte) error {
	logger := slogx.FromCtx(ctx)

	encryptedValue, err := certManager.kms.Encrypt(ctx, data, []byte(key))
	if err != nil {
		err = fmt.Errorf("certmanager.Put: error encrypting cert: %w", err)
		logger.Error(err.Error())
		return err
	}

	const query = `INSERT INTO tls_certificates (created_at, updated_at, key, encrypted_value) VALUES ($1, $1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET updated_at = $1, encrypted_value = $3`

	now := time.Now().UTC()
	_, err = certManager.db.Exec(ctx, query, now, key, encryptedValue)
	if err != nil {
		err = fmt.Errorf("certmanager.Put: error inserting tls_certificate in DB [%s]: %w", key, err)
		logger.Error(err.Error())
		return err
	}

	return nil
}

func (certManager *CertManager) Delete(ctx context.Context, key string) error {
	logger := slogx.FromCtx(ctx)

	// certManager.cache.Delete(key)
	_, err := certManager.db.Exec(ctx, "DELETE FROM tls_certificates WHERE key = $1", key)
	if err != nil {
		err = fmt.Errorf("certmanager.Delete: error deleting tls_certificate: %w", err)
		logger.Error(err.Error())
		return err
	}

	return nil
}

func (certManager *CertManager) deleteOlderCertificates(ctx context.Context) {
	// if a certificate hasn't been renewed in the last 80 days it may means that there is a problem
	// as certificates should be renewed 30 days before expiration.
	// Deleting older certificates ensures that autocert will try to get a new certificate instead of
	// serving an expired certificate.

	logger := slogx.FromCtx(ctx)

	// delete certificates that haven't been updated in the last 100 days
	olderThan := time.Now().UTC().Add(-100 * 24 * time.Hour)
	_, err := certManager.db.Exec(ctx, "DELETE FROM tls_certificates WHERE updated_at < $1", olderThan)
	if err != nil {
		err = fmt.Errorf("certmanager.deleteOlderCertificates: error deleting tls_certificates: %w", err)
		logger.Error(err.Error())
		return
	}

	logger.Debug("certmanager: older certificates successfully deleted")
}

// API endpoints

func (certManager *CertManager) DeleteTlsCertificate(ctx context.Context, input DeleteTlsCertificateInput) error {
	httpCtx := httpctx.FromCtx(ctx)

	accessToken := httpCtx.AccessToken
	if accessToken == nil || !accessToken.IsAdmin {
		return kernel.ErrPermissionDenied
	}

	return certManager.Delete(ctx, input.Key)
}

func (certManager *CertManager) ListCertificates(ctx context.Context, _ kernel.EmptyInput) (ret kernel.PaginatedResult[TlsCertificate], err error) {
	httpCtx := httpctx.FromCtx(ctx)

	accessToken := httpCtx.AccessToken
	if accessToken == nil || !accessToken.IsAdmin {
		return ret, kernel.ErrPermissionDenied
	}

	limit := math.MaxInt64
	certs := make([]TlsCertificate, 0, 10)

	err = certManager.db.Select(ctx, &certs, "SELECT * FROM tls_certificates ORDER BY updated_at DESC LIMIT $1", limit)
	if err != nil {
		return ret, fmt.Errorf("certmanager: error listing certificates: %w", err)
	}

	ret.Data = certs
	return ret, nil
}
