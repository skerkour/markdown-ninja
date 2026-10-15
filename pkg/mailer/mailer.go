package mailer

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"markdown.ninja/stdx/email"
)

type Mailer interface {
	SendTransactionnal(ctx context.Context, email email.Email) error
	SendBroadcast(ctx context.Context, email email.Email) error
	AddDomain(ctx context.Context, domain string) (ret Domain, err error)
	RemoveDomain(ctx context.Context, domain string) (err error)
	VerifyDomain(ctx context.Context, domain string) (verified bool, err error)
	GetSuppressions(ctx context.Context) (suppressions []Suppression, err error)
	DeleteSuppression(ctx context.Context, email string) (err error)
}

type Domain struct {
	Domain     string
	DnsRecords []DnsRecord
}

type DnsRecord struct {
	Host string `json:"host"`
	Type string `json:"type"`
	Val  string `json:"value"`
}

func (record *DnsRecord) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, record)
		return nil
	case string:
		json.Unmarshal([]byte(v), record)
		return nil
	default:
		return fmt.Errorf("DnsRecord.Scan: Unsupported type: %T", v)
	}
}

func (record *DnsRecord) Value() (driver.Value, error) {
	return json.Marshal(record)
}

type DnsRecords []DnsRecord

func (records *DnsRecords) Scan(val any) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, records)
		return nil
	case string:
		json.Unmarshal([]byte(v), records)
		return nil
	default:
		return fmt.Errorf("DnsRecords.Scan: Unsupported type: %T", v)
	}
}

func (records *DnsRecords) Value() (driver.Value, error) {
	return json.Marshal(records)
}

type Suppression struct {
	Email string
}
