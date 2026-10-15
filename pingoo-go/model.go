package pingoo

import (
	"encoding/json"
	"net/netip"
	"time"

	"markdown.ninja/stdx/uuid"
)

type EmailInfo struct {
	Email      string   `json:"email"`
	Disposable bool     `json:"disposable"`
	MxRecords  []string `json:"mx_records"`
	Valid      bool     `json:"valid"`
}

type IpInfo struct {
	IP          netip.Addr `json:"ip"`
	TorExitNode bool       `json:"tor_exit_node"`
	Country     string     `json:"country"`
	CountryName string     `json:"country_name"`
	Asn         int64      `json:"asn"`
	Vpn         bool       `json:"vpn"`
	Bogon       bool       `json:"bogon"`
}

type PaginatedResult[T any] struct {
	Data  []T   `json:"data"`
	Total int64 `json:"total"`
}

type ListUsersInput struct {
	ProjectID uuid.UUID   `json:"project_id"`
	IDs       []uuid.UUID `json:"ids,omitempty"`
}

type GetUserInput struct {
	ID uuid.UUID `json:"id"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name    string `json:"name"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type LookupEmailsInput struct {
	Emails    []string `json:"emails"`
	MxRecords *bool    `json:"mx_records,omitempty"`
}
