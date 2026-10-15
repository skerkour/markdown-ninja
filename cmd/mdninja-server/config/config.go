package config

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"log/slog"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/kms"
	"markdown.ninja/stdx/jsonutil"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/yaml"
)

type Config struct {
	// True if running as Saas and billing is required
	BlockedCountries   []string `json:"blocked_countries" yaml:"blocked_countries"`
	BlockSubscriptions bool     `json:"block_subscriptions" yaml:"block_subscriptions"`
	Saas               bool     `json:"saas" yaml:"saas"`
	HTTP               Http     `json:"http" yaml:"http"`
	Database           Database `json:"database" yaml:"database"`
	Emails             Emails   `json:"emails" yaml:"emails"`
	Worker             Worker   `json:"worker" yaml:"worker"`
	Logs               Logs     `json:"logs" yaml:"logs"`
	S3                 S3       `json:"s3" yaml:"s3"`
	Jwt                Jwt      `json:"jwt" yaml:"jwt"`
	Kms                Kms      `json:"kms" yaml:"kms"`

	// 3rd party providers & services
	// pingoo.io
	Pingoo Pingoo `json:"pingoo" yaml:"pingoo"`
	// stripe.com
	Stripe   *Stripe   `json:"stripe" yaml:"stripe"`
	Aws      *Aws      `json:"aws" yaml:"aws"`
	Scaleway *Scaleway `json:"scaleway" yaml:"scaleway"`
}

type Http struct {
	// The port to listen to. default: 8080
	Port uint16 `json:"port" yaml:"port"`
	// The IP address to listen to. default: 0.0.0.0
	Ip string `json:"ip" yaml:"ip"`
	// AccessLogs   bool   `json:"access_logs" yaml:"access_logs"`
	Tls bool `json:"tls" yaml:"tls"`

	WebappBaseUrlStr string   `json:"webapp_base_url" yaml:"webapp_base_url"`
	WebappBaseUrl    *url.URL `json:"-" yaml:"-"`
	WebappDomain     string   `json:"-" yaml:"-"`
	// if not empty, WebappPort is prefixed with ":"
	WebappPort string `json:"-" yaml:"-"`

	WebsitesBaseUrlStr string   `json:"websites_base_url" yaml:"websites_base_url"`
	WebsitesBaseUrl    *url.URL `json:"-" yaml:"-"`
	// the root domain for websites. e.g. markdown.club
	WebsitesRootDomain string `json:"-" yaml:"-"`
	// if not empty, WebsitesPort is prefixed with ":"
	WebsitesPort string `json:"-" yaml:"-"`
	// enable PROXY protocol listeners
	ProxyProtocol bool `json:"proxy_protocol" yaml:"proxy_protocol"`
}

type Tls struct {
	// AutocertDomains  []string `json:"autocert_domains" yaml:"autocert_domains"`
	LetsEncryptEmail string `json:"lets_encrypt_email" yaml:"lets_encrypt_email"`
}

type Database struct {
	Url      string `json:"url" yaml:"url"`
	PoolSize int    `json:"pool_size" yaml:"pool_size"`
}

type Worker struct {
	Concurrency uint32 `json:"concurrency" yaml:"concurrency"`
}

type Emails struct {
	Provider          EmailsProvider `json:"provider" yaml:"provider"`
	NotifyAddressStr  string         `json:"notify_address" yaml:"notify_address"`
	NotifyAddress     mail.Address   `json:"-"`
	ContactAddressStr string         `json:"contact_address" yaml:"contact_address"`
	ContactAddress    mail.Address   `json:"-"`
}

type Kms struct {
	MasterKeyID string       `json:"master_key_id" yaml:"master_key_id"`
	Provider    kms.Provider `json:"provider" yaml:"provider"`
}

type S3 struct {
	Provider S3Provider `json:"provider" yaml:"provider"`
	Bucket   string     `json:"bucket" yaml:"bucket"`
	Endpoint string     `json:"endpoint" yaml:"endpoint"`
}

type Scaleway struct {
	Region          string `json:"region" yaml:"region"`
	AccessKeyID     string `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" yaml:"secret_access_key"`
}

type Aws struct {
	Region          string `json:"region" yaml:"region"`
	AccessKeyID     string `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" yaml:"secret_access_key"`
}

type Pingoo struct {
	ApiKey        string  `json:"api_key" yaml:"api_key"`
	ProjectID     string  `json:"project_id" yaml:"project_id"`
	Url           *string `json:"url" yaml:"url"`
	WebhookSecret string  `json:"webhook_secret" yaml:"webhook_secret"`
	Endpoint      string  `json:"endpoint" yaml:"endpoint"`
	AppID         string  `json:"app_id" yaml:"app_id"`
}

type Stripe struct {
	SecretKey     string `json:"secret_key" yaml:"secret_key"`
	PublicKey     string `json:"public_key" yaml:"public_key"`
	WebhookSecret string `json:"webhook_secret" yaml:"webhook_secret"`
	// OauthClientID string         `json:"oauth_client_id" yaml:"oauth_client_id"`
	Prices StripePrices `json:"prices" yaml:"prices"`
}

type StripePrices struct {
	Pro   string `json:"pro" yaml:"pro"`
	Slots string `json:"slots" yaml:"slots"`
}

type Logs struct {
	Level        slog.Level `json:"level" yaml:"level"`
	LokiEndpoint *string    `json:"loki_endpoint" yaml:"loki_endpoint"`
}

type Jwt struct {
	Issuer string `json:"issuer" yaml:"issuer"`
}

type YamlBytes []byte

func (buff *YamlBytes) UnmarshalText(input []byte) (err error) {
	data, err := base64.StdEncoding.DecodeString(string(input))
	if err != nil {
		return
	}

	*buff = data
	return nil
}

// Not directly related, for print function etc.
func (buff YamlBytes) String() string {
	return base64.StdEncoding.EncodeToString(buff)
}

// Load config from the given file or from env var: MARKDOWN_NINJA_CONFIG and validate it
// Also load some values from env if found:
// MARKDOWN_NINJA_CONFIG, DATABASE_URL, PORT, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_DEFAULT_REGION

// MARKDOWN_NINJA_CONFIG
//
// GOMAXPROCS
// GOMEMLIMIT
func Load(ctx context.Context, configPath string) (config Config, err error) {
	logger := slogx.FromCtx(ctx)
	var configData []byte
	markdownNinjaConfigEnvValue := os.Getenv(envMarkdownNinjaConfig)

	if markdownNinjaConfigEnvValue != "" {
		logger.Info("config.Load: Loading config from env", slog.String("env.var", envMarkdownNinjaConfig))
		configData = []byte(markdownNinjaConfigEnvValue)
	} else {
		logger.Info("config.Load: Loading config from file", slog.String("file", configPath))

		configData, err = os.ReadFile(configPath)
		if err != nil {
			err = errs.InvalidArgument(err.Error())
			return
		}
	}

	configData = bytes.TrimSpace(configData)
	if len(configData) == 0 {
		err = errs.InvalidArgument(fmt.Sprintf("config: configuration file is not valid: %s", configPath))
		return
	}

	if configData[0] == '{' {
		configData, err = jsonutil.StripComments(configData)
		if err != nil {
			err = errs.InvalidArgument(fmt.Sprintf("config: error stripping comments: %s", err))
			return
		}

		err = json.Unmarshal(configData, &config)
		if err != nil {
			err = errs.InvalidArgument(err.Error())
			return
		}
	} else {
		err = yaml.Unmarshal(configData, &config)
		if err != nil {
			err = errs.InvalidArgument(err.Error())
			return
		}
	}

	err = config.validateAndDefaultValues()
	if err != nil {
		return
	}

	return
}

// TODO
func (config *Config) validateAndDefaultValues() (err error) {
	if config.BlockedCountries == nil {
		config.BlockedCountries = []string{}
	}

	// Geoip Database

	// HTTP
	portFromEnvStr := strings.TrimSpace(os.Getenv(envPort))
	if portFromEnvStr != "" {
		portFromEnv, err := strconv.ParseUint(portFromEnvStr, 10, 16)
		if err != nil {
			err = errs.InvalidArgument(fmt.Sprintf("config: error parsing the PORT env variable: %s", err.Error()))
			return err
		}
		config.HTTP.Port = uint16(portFromEnv)
	}
	if config.HTTP.Port == 0 {
		config.HTTP.Port = defaultHttpPort
	}

	err = cleanAndValidateHttpConfig(config)
	if err != nil {
		return
	}

	// Databases
	if !strings.HasPrefix(config.Database.Url, postgresUrlScheme) {
		err = errs.InvalidArgument("config: database.url is not a valid postgres URL")
		return err
	}
	if config.Database.PoolSize < 0 {
		err = errs.InvalidArgument("config: database.pool_size can't be negative")
		return err
	}
	if config.Database.PoolSize == 0 {
		config.Database.PoolSize = defaultDatabasePoolSize
	}

	// if !strings.HasPrefix(config.Database.EventsUrl, postgresUrlScheme) {
	// 	err = errs.InvalidArgument("config: database.events_url is not a valid postgres URL")
	// 	return err
	// }
	// if config.Database.EventsPoolSize < 0 {
	// 	err = errs.InvalidArgument("config: database.events_pool_size can't be negative")
	// 	return err
	// }
	// if config.Database.EventsPoolSize == 0 {
	// 	config.Database.EventsPoolSize = defaultDatabasePoolSize
	// }

	// Storage
	// if config.Storage.Driver != StorageDriverFilesystem && config.Storage.Driver != StorageDriverS3 {
	// 	err = errs.InvalidArgument(fmt.Sprintf("config: storage.driver can't be empty. Valid values are [%s, %s]", StorageDriverFilesystem, StorageDriverS3))
	// 	return err
	// }
	// if config.Storage.Driver == StorageDriverS3 {

	// if config.S3 == nil {
	// 	err = errs.InvalidArgument("config: s3 can't be null")
	// 	return err
	// }

	// S3
	if config.S3.Bucket == "" {
		err = errs.InvalidArgument("config: s3.bucket is missing")
		return err
	}

	if config.S3.Provider != S3ProviderAws && config.S3.Provider != S3ProviderScaleway {
		return errs.InvalidArgument("config: s3.provider is not valid")
	}

	if config.S3.Provider == S3ProviderScaleway {
		if config.Scaleway == nil {
			return errs.InvalidArgument("config: scaleway is null while s3.provider == \"scaleway\"")
		}
		if config.S3.Endpoint == "" {
			return errs.InvalidArgument("config: s3.endpoint is empty while s3.provider == \"scaleway\"")
		}
	}

	if config.S3.Provider == S3ProviderAws {
		if config.Aws == nil {
			return errs.InvalidArgument("config: aws is null while s3.provider == \"aws\"")
		}
	}

	// KMS
	if config.Kms.Provider != kms.ProviderConsole && config.Kms.Provider != kms.ProviderScaleway {
		return fmt.Errorf("config: invalid kms.provider. Valdi values are: [%s, %s]", kms.ProviderConsole, kms.ProviderScaleway)
	}

	config.Kms.MasterKeyID = strings.TrimSpace(config.Kms.MasterKeyID)
	if config.Kms.Provider == kms.ProviderScaleway {
		if config.Kms.MasterKeyID == "" {
			return errors.New("config: kms.master_key_id is empty while kms.provider == \"scaleway\"")
		}
	}

	if config.Kms.Provider == kms.ProviderScaleway && config.Scaleway == nil {
		return errors.New("config: scaleway is null while kms.provider == \"scaleway\"")
	}

	// Scaleway
	if config.Scaleway != nil {
		if config.Scaleway.Region == "" {
			return errs.InvalidArgument("config: scaleway.region is empty")
		}

		if config.Scaleway.AccessKeyID == "" {
			return errs.InvalidArgument("config: scaleway.access_key_id is missing")
		}

		if config.Scaleway.SecretAccessKey == "" {
			return errs.InvalidArgument("config: scaleway.secret_access_key is missing")
		}
	}

	// Aws
	if config.Aws != nil {
		if config.Aws.Region == "" {
			return errs.InvalidArgument("config: aws.region is empty")
		}

		if config.Aws.AccessKeyID == "" {
			return errs.InvalidArgument("config: aws.access_key_id is missing")
		}

		if config.Aws.SecretAccessKey == "" {
			return errs.InvalidArgument("config: aws.secret_access_key is missing")
		}
	}

	// }

	// Emails
	if config.Emails.Provider != EmailsProviderConsole && config.Emails.Provider != EmailsProviderSes {
		return errs.InvalidArgument(fmt.Sprintf("config: emails.provider is not valid. Valid values are [%s, %s]", EmailsProviderConsole, EmailsProviderSes))
	}
	if config.Emails.Provider == EmailsProviderSes && config.Aws == nil {
		return errs.InvalidArgument("config: aws is null but emails.provider is \"ses\"")
	}

	if config.Emails.NotifyAddressStr == "" {
		err = errs.InvalidArgument("config: emails.notify_address is empty")
		return err
	}
	mailNotifyAddress, err := mail.ParseAddress(config.Emails.NotifyAddressStr)
	if err != nil {
		err = errs.InvalidArgument(fmt.Sprintf("config: error parsing emails.notify_address (%s): %s", config.Emails.NotifyAddressStr, err))
		return
	}
	config.Emails.NotifyAddress = *mailNotifyAddress

	if config.Emails.ContactAddressStr == "" {
		err = errs.InvalidArgument("config: emails.contact_address is empty")
		return err
	}
	mailContactAddress, err := mail.ParseAddress(config.Emails.ContactAddressStr)
	if err != nil {
		err = errs.InvalidArgument(fmt.Sprintf("config: error parsing emails.contact_address (%s): %s", config.Emails.ContactAddressStr, err))
		return
	}
	config.Emails.ContactAddress = *mailContactAddress

	// Worker
	if config.Worker.Concurrency > math.MaxInt32 {
		err = errs.InvalidArgument("config: worker.concurrency is too high")
		return err
	}

	// Logs
	if config.Logs.Level != slog.LevelDebug && config.Logs.Level != slog.LevelInfo &&
		config.Logs.Level != slog.LevelWarn && config.Logs.Level != slog.LevelError {
		err = errs.InvalidArgument("config: logs.level is not valid. Valid values are: [DEBUG, INFO, WARN, ERROR]")
		return err
	}

	if config.Logs.LokiEndpoint != nil {
		lokiEndpoint := strings.TrimSpace(*config.Logs.LokiEndpoint)
		if lokiEndpoint == "" {
			err = errs.InvalidArgument("config: logs.loki_endpoint is empty")
			return err
		}
		if !strings.HasPrefix(lokiEndpoint, "https://") {
			err = errs.InvalidArgument("config: logs.loki_endpoint is not a valid https URL")
			return err
		}
		config.Logs.LokiEndpoint = &lokiEndpoint
	}

	// Stripe
	if !config.Saas {
		err = cleanAndValdiateStripeConfig(config.Stripe)
		if err != nil {
			return err
		}
	}

	// Jwt
	err = cleanAndValidateJwt(&config.Jwt)
	if err != nil {
		return
	}

	return
}

func cleanAndValidateHttpConfig(config *Config) (err error) {
	if config.HTTP.Ip != "" {
		if _, err = netip.ParseAddr(config.HTTP.Ip); err != nil {
			return errs.InvalidArgument("config: http.ip is not valid")
		}
	}

	// webapp_base_url
	config.HTTP.WebappBaseUrlStr = strings.ToLower(strings.TrimSpace(config.HTTP.WebappBaseUrlStr))
	if config.HTTP.WebappBaseUrlStr == "" {
		return errs.InvalidArgument("config: http.webapp_base_url is empty")
	}
	config.HTTP.WebappBaseUrl, err = url.Parse(config.HTTP.WebappBaseUrlStr)
	if err != nil {
		return errs.InvalidArgument(fmt.Sprintf("config: error parsing http.webapp_base_url: %s", err))
	}

	if config.HTTP.WebappBaseUrl.Scheme != protocolHttp && config.HTTP.WebappBaseUrl.Scheme != protocolHttps {
		return errs.InvalidArgument(fmt.Sprintf("config: invalid protocol for http.webapp_base_url. Valid values are: [%s, %s]", protocolHttp, protocolHttps))
	}
	if config.HTTP.WebappBaseUrl.Path != "" {
		return errs.InvalidArgument("config: http.webapp_base_url path must be empty")
	}

	if strings.Contains(config.HTTP.WebappBaseUrl.Host, ":") {
		config.HTTP.WebappDomain, config.HTTP.WebappPort, err = net.SplitHostPort(config.HTTP.WebappBaseUrl.Host)
		if err != nil {
			return errs.InvalidArgument(fmt.Sprintf("config: error parsing http.webapp_base_url: %s", err))
		}
		if config.HTTP.WebappPort != "" {
			config.HTTP.WebappPort = ":" + config.HTTP.WebappPort
		}
	} else {
		config.HTTP.WebappDomain = config.HTTP.WebappBaseUrl.Host
	}
	if config.HTTP.WebappDomain == "" {
		return errors.New("config: domain is empty for http.webapp_base_url")
	}

	// websites_base_url
	config.HTTP.WebsitesBaseUrlStr = strings.ToLower(strings.TrimSpace(config.HTTP.WebsitesBaseUrlStr))
	if config.HTTP.WebsitesBaseUrlStr == "" {
		return errs.InvalidArgument("config: http.websites_base_url is empty")
	}
	config.HTTP.WebsitesBaseUrl, err = url.Parse(config.HTTP.WebsitesBaseUrlStr)
	if err != nil {
		return errs.InvalidArgument(fmt.Sprintf("config: error parsing http.websites_base_url: %s", err))
	}
	if config.HTTP.WebsitesBaseUrl.Scheme != protocolHttp && config.HTTP.WebsitesBaseUrl.Scheme != protocolHttps {
		return errs.InvalidArgument(fmt.Sprintf("config: invalid protocol for http.websites_base_url. Valid values are: [%s, %s]", protocolHttp, protocolHttps))
	}
	if config.HTTP.WebsitesBaseUrl.Path != "" {
		return errs.InvalidArgument("config: http.websites_base_url path must be empty")
	}

	if strings.Contains(config.HTTP.WebsitesBaseUrl.Host, ":") {
		config.HTTP.WebsitesRootDomain, config.HTTP.WebsitesPort, err = net.SplitHostPort(config.HTTP.WebsitesBaseUrl.Host)
		if err != nil {
			return errs.InvalidArgument(fmt.Sprintf("config: error parsing http.websites_base_url: %s", err))
		}
		if config.HTTP.WebsitesPort != "" {
			config.HTTP.WebsitesPort = ":" + config.HTTP.WebsitesPort
		}
	} else {
		config.HTTP.WebsitesRootDomain = config.HTTP.WebsitesBaseUrl.Host
	}
	if config.HTTP.WebsitesRootDomain == "" {
		return errors.New("config: domain is empty for http.websites_base_url")
	}

	// pingoo
	err = cleanAndValdiatePingooConfig(&config.Pingoo)
	if err != nil {
		return err
	}

	// if config.HTTP.Geoip == "" {
	// 	config.HTTP.Geoip = config.HTTP.Cdn
	// }
	// err = validateCdnProvider("http.geoip", config.HTTP.Geoip)
	// if err != nil {
	// 	return
	// }

	// if config.HTTP.ClientIp == "" {
	// 	config.HTTP.ClientIp = config.HTTP.Cdn
	// }
	// err = validateCdnProvider("http.client_ip", config.HTTP.ClientIp)
	// if err != nil {
	// 	return
	// }

	return nil
}

func cleanAndValidateJwt(jwtConfig *Jwt) (err error) {
	jwtConfig.Issuer = strings.TrimSpace(jwtConfig.Issuer)
	if jwtConfig.Issuer == "" {
		return errors.New("config: jwt.issuer is empty")
	}

	return nil
}

func cleanAndValdiateStripeConfig(stripeConfig *Stripe) (err error) {
	stripeConfig.SecretKey = strings.TrimSpace(stripeConfig.SecretKey)
	if !strings.HasPrefix(stripeConfig.SecretKey, stripeSecretKeyPrefix) {
		return errs.InvalidArgument("config: stripe.secret_key is not valid")
	}

	stripeConfig.PublicKey = strings.TrimSpace(stripeConfig.PublicKey)
	if !strings.HasPrefix(stripeConfig.PublicKey, stripePublicKeyPrefix) {
		return errs.InvalidArgument("config: stripe.public_key is not valid")
	}

	stripeConfig.WebhookSecret = strings.TrimSpace(stripeConfig.WebhookSecret)
	if !strings.HasPrefix(stripeConfig.WebhookSecret, stripeWebhookSecretPrefix) {
		return errs.InvalidArgument("config: stripe.webhook_secret is not valid")
	}

	stripeConfig.Prices.Pro = strings.TrimSpace(stripeConfig.Prices.Pro)
	if !strings.HasPrefix(stripeConfig.Prices.Pro, stripePrefixPrice) {
		return errs.InvalidArgument("config: stripe.prices.pro is not valid")
	}

	stripeConfig.Prices.Slots = strings.TrimSpace(stripeConfig.Prices.Slots)
	if !strings.HasPrefix(stripeConfig.Prices.Slots, stripePrefixPrice) {
		return errs.InvalidArgument("config: stripe.prices.slots is not valid")
	}

	return
}

func cleanAndValdiatePingooConfig(pingoo *Pingoo) error {
	if pingoo.ProjectID == "" {
		return errors.New("config: pingoo.project_id is empty")
	}

	if pingoo.ApiKey == "" {
		return errors.New("config: pingoo.api_key is empty")
	}
	if pingoo.WebhookSecret == "" {
		return errors.New("config: pingoo.webhook_secret is empty")
	}

	endpointUrl, err := url.Parse(pingoo.Endpoint)
	if err != nil {
		return errors.New("config: pingoo.endpoint is not valid")
	}

	if endpointUrl.Scheme != "http" && endpointUrl.Scheme != "https" {
		return errors.New("config: pingoo.endpoint must be http or https")
	}

	if len(pingoo.AppID) < 8 {
		return errors.New("config: pingoo.app_id is not valid")
	}

	return nil
}
