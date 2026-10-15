package console

import (
	"context"
	"fmt"

	"markdown.ninja/pkg/mailer"
	"markdown.ninja/stdx/email"
)

// consoleMailer implements the `Mailer` interface to print emails to console
type ConsoleMailer struct {
}

// ensure that consoleMailer satisfies the Storage interface
var _ mailer.Mailer = (*ConsoleMailer)(nil)

// NewMailer returns a new console Mailer
func NewConsoleMailer() *ConsoleMailer {
	return &ConsoleMailer{}
}

// Send an email using the console mailer
func (consoleMailer *ConsoleMailer) SendTransactionnal(ctx context.Context, email email.Email) error {
	data, err := email.Bytes()
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	return nil
}

func (consoleMailer *ConsoleMailer) SendBroadcast(ctx context.Context, email email.Email) error {
	return consoleMailer.SendTransactionnal(ctx, email)
}

func (consoleMailer *ConsoleMailer) AddDomain(ctx context.Context, domain string) (ret mailer.Domain, err error) {
	fmt.Printf("mailer: Adding domain: %s\n", domain)
	ret = mailer.Domain{
		Domain:     domain,
		DnsRecords: []mailer.DnsRecord{},
	}
	return ret, nil
}

func (consoleMailer *ConsoleMailer) RemoveDomain(ctx context.Context, domain string) error {
	fmt.Printf("mailer: Removing domain: %s\n", domain)
	return nil
}

func (consoleMailer *ConsoleMailer) VerifyDomain(ctx context.Context, domain string) (bool, error) {
	fmt.Printf("mailer: Verifying Domain: %s\n", domain)
	return true, nil
}

func (consoleMailer *ConsoleMailer) GetSuppressions(ctx context.Context) (suppressions []mailer.Suppression, err error) {
	return []mailer.Suppression{}, nil
}

func (consoleMailer *ConsoleMailer) DeleteSuppression(ctx context.Context, email string) (err error) {
	return nil
}
