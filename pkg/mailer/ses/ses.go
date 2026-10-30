package ses

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/stdx/email"
)

// SesMailer implements the `Mailer` interface to send emails using AWS' SES
// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/sesv2
type SesMailer struct {
	sesClient *sesv2.Client
}

type Config struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	HttpClient      *http.Client
}

// ensure that sesMailer satisfies the Storage interface
var _ mailer.Mailer = (*SesMailer)(nil)

// NewMailer returns a new console Mailer
func NewSesMailer(config Config) (*SesMailer, error) {
	var options []func(*awsconfig.LoadOptions) error = make([]func(*awsconfig.LoadOptions) error, 0)

	options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")))
	options = append(options, awsconfig.WithRegion(config.Region))
	if config.HttpClient != nil {
		options = append(options, awsconfig.WithHTTPClient(config.HttpClient))
	} else {
		// optimized HTTP client for SES.
		// https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/custom-http/
		dialer := net.Dialer{
			Timeout: 10 * time.Second,
			// KeepAlive: 15 * time.Second,
			// KeepAliveConfig: net.KeepAliveConfig{
			// 	Enable: true,
			// },
		}
		dialer.SetMultipathTCP(false)
		transport := &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			// ResponseHeaderTimeout: 10,
			ExpectContinueTimeout: 5 * time.Second,
			// IdleConnTimeout:       300 * time.Second,
			// maybe we should use something like number of CPU * 64
			MaxIdleConnsPerHost: 128,

			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		}

		httpClient := &http.Client{
			Transport: transport,
		}
		options = append(options, awsconfig.WithHTTPClient(httpClient))
	}

	sesConfig, err := awsconfig.LoadDefaultConfig(context.Background(), options...)
	if err != nil {
		err = fmt.Errorf("ses: error loading config: %w", err)
		return nil, err
	}

	sesClient := sesv2.NewFromConfig(sesConfig)

	return &SesMailer{
		sesClient,
	}, nil
}

func (sesMailer *SesMailer) SendTransactionnal(ctx context.Context, email email.Email) error {
	rawEmail, err := email.Bytes()
	if err != nil {
		return fmt.Errorf("ses: error getting raw email: %w", err)
	}
	request := sesv2.SendEmailInput{
		Content: &types.EmailContent{
			Raw: &types.RawMessage{
				Data: rawEmail,
			},
		},
	}

	_, err = sesMailer.sesClient.SendEmail(ctx, &request)
	if err != nil {
		return fmt.Errorf("ses: error sending email: %w", err)
	}

	return nil
}

func (sesMailer *SesMailer) SendBroadcast(ctx context.Context, email email.Email) error {
	return sesMailer.SendTransactionnal(ctx, email)
}

func (sesMailer *SesMailer) AddDomain(ctx context.Context, domain string) (ret mailer.Domain, err error) {
	request := sesv2.CreateEmailIdentityInput{
		EmailIdentity: aws.String(domain),
	}

	res, err := sesMailer.sesClient.CreateEmailIdentity(ctx, &request)
	if err != nil {
		return ret, fmt.Errorf("ses: error adding domain: %w", err)
	}

	dnsRecords := make([]mailer.DnsRecord, 0, 3)
	if res.DkimAttributes != nil {
		for _, token := range res.DkimAttributes.Tokens {
			dnsRecord := mailer.DnsRecord{
				Host: fmt.Sprintf("%s._domainkey.%s", token, domain),
				Type: "CNAME",
				Val:  fmt.Sprintf("%s.dkim.amazonses.com", token),
			}
			dnsRecords = append(dnsRecords, dnsRecord)
		}
	}

	ret = mailer.Domain{
		Domain:     domain,
		DnsRecords: dnsRecords,
	}

	return ret, nil
}

func (sesMailer *SesMailer) RemoveDomain(ctx context.Context, domain string) error {
	request := sesv2.DeleteEmailIdentityInput{
		EmailIdentity: aws.String(domain),
	}

	_, err := sesMailer.sesClient.DeleteEmailIdentity(ctx, &request)
	if err != nil {
		return fmt.Errorf("ses: error deleting domain: %w", err)
	}

	return nil
}

func (sesMailer *SesMailer) VerifyDomain(ctx context.Context, domain string) (verified bool, err error) {
	request := sesv2.GetEmailIdentityInput{
		EmailIdentity: aws.String(domain),
	}

	res, err := sesMailer.sesClient.GetEmailIdentity(ctx, &request)
	if err != nil {
		return false, fmt.Errorf("ses: error getting domain verification status: %w", err)
	}

	return res.VerifiedForSendingStatus, nil
}

func (sesMailer *SesMailer) GetSuppressions(ctx context.Context) (suppressions []mailer.Suppression, err error) {
	suppressions = make([]mailer.Suppression, 0)

	var nextToken *string
	for {
		request := sesv2.ListSuppressedDestinationsInput{
			NextToken: nextToken,
			PageSize:  aws.Int32(100),
		}
		res, err := sesMailer.sesClient.ListSuppressedDestinations(ctx, &request)
		if err != nil {
			err = fmt.Errorf("ses: error listing suppressions: %w", err)
			return []mailer.Suppression{}, err
		}

		for _, suppressedDestination := range res.SuppressedDestinationSummaries {
			if suppressedDestination.EmailAddress != nil {
				suppression := mailer.Suppression{
					Email: *suppressedDestination.EmailAddress,
				}
				suppressions = append(suppressions, suppression)
			}
		}

		nextToken = res.NextToken
		if nextToken == nil {
			break
		}
	}

	return suppressions, nil
}

func (sesMailer *SesMailer) DeleteSuppression(ctx context.Context, email string) (err error) {
	request := sesv2.DeleteSuppressedDestinationInput{
		EmailAddress: aws.String(email),
	}
	_, err = sesMailer.sesClient.DeleteSuppressedDestination(ctx, &request)
	if err != nil {
		return fmt.Errorf("ses: error deleting suppression (%s): %w", email, err)
	}

	return nil
}
