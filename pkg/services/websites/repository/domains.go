package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *WebsitesRepository) CreateDomain(ctx context.Context, db db.Queryer, domain websites.Domain) (err error) {
	const query = `INSERT INTO domains
			(id, created_at, updated_at, hostname, tls_active, website_id)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = db.Exec(ctx, query, domain.ID, domain.CreatedAt, domain.UpdatedAt, domain.Hostname, domain.TlsActive,
		domain.WebsiteID)
	if err != nil {
		err = fmt.Errorf("websites.CreateDomain: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) UpdateDomain(ctx context.Context, db db.Queryer, domain websites.Domain) (err error) {
	const query = `UPDATE domains
		SET updated_at = $1, tls_active = $2
		WHERE id = $3`

	_, err = db.Exec(ctx, query, domain.UpdatedAt, domain.TlsActive,
		domain.ID)
	if err != nil {
		err = fmt.Errorf("websites.UpdateDomain: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) FindDomainByID(ctx context.Context, db db.Queryer, domainID guid.GUID) (domain websites.Domain, err error) {
	const query = "SELECT * FROM domains WHERE id = $1"

	err = db.Get(ctx, &domain, query, domainID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = websites.ErrDomainNotFound
		} else {
			err = fmt.Errorf("websites.FindDomainByID: %w", err)
		}
		return
	}

	return
}

func (repo *WebsitesRepository) FindDomainByHostname(ctx context.Context, db db.Queryer, domain string) (ret websites.Domain, err error) {
	const query = "SELECT * FROM domains WHERE hostname = $1"

	err = db.Get(ctx, &ret, query, domain)
	if err != nil {
		if err == sql.ErrNoRows {
			err = websites.ErrDomainNotFound
		} else {
			err = fmt.Errorf("websites.FindDomainByHostname: %w", err)
		}
		return ret, err
	}

	return ret, nil
}

func (repo *WebsitesRepository) DeleteDomain(ctx context.Context, db db.Queryer, domainID guid.GUID) (err error) {
	const query = `DELETE FROM domains WHERE id = $1`

	_, err = db.Exec(ctx, query, domainID)
	if err != nil {
		err = fmt.Errorf("websites.DeleteDomain: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) FindDomainsForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (domains []websites.Domain, err error) {
	domains = []websites.Domain{}
	const query = `SELECT * FROM domains
		WHERE website_id = $1
		ORDER BY hostname
`

	err = db.Select(ctx, &domains, query, websiteID)
	if err != nil {
		err = fmt.Errorf("websites.FindDomainsForWebsite: %w", err)
		return
	}

	return
}

func (repo *WebsitesRepository) GetDomainsCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error) {
	const query = `SELECT COUNT(*) FROM domains WHERE website_id = $1`

	err = db.Get(ctx, &count, query, websiteID)
	if err != nil {
		err = fmt.Errorf("websites.GetDomainsCountForWebsite: %w", err)
		return
	}

	return
}
