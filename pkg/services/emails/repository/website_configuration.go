package repository

import (
	"context"
	"database/sql"
	"fmt"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

func (repo *EmailsRepository) CreateWebsiteConfiguration(ctx context.Context, db db.Queryer, config emails.WebsiteConfiguration) (err error) {
	const query = `INSERT INTO emails_website_configuration
			(created_at, updated_at, from_name, from_address, from_domain, domain_verified, dns_records, website_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = db.Exec(ctx, query, config.CreatedAt, config.UpdatedAt, config.FromName, config.FromAddress,
		config.FromDomain, config.DomainVerified, config.DnsRecords, config.WebsiteID)
	if err != nil {
		err = fmt.Errorf("emails.CreateWebsiteConfiguration: %w", err)
		return
	}

	return
}

func (repo *EmailsRepository) FindWebsiteConfiguration(ctx context.Context, db db.Queryer, websiteID guid.GUID) (configuration emails.WebsiteConfiguration, err error) {
	const query = "SELECT * FROM emails_website_configuration WHERE website_id = $1"

	err = db.Get(ctx, &configuration, query, websiteID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = emails.ErrWebsiteConfigurationNotFound
		} else {
			err = fmt.Errorf("emails.FindWebsiteConfiguration: %w", err)
		}
		return
	}

	return
}

func (repo *EmailsRepository) FindWebsiteConfigurationByDomain(ctx context.Context, db db.Queryer, domain string) (configuration emails.WebsiteConfiguration, err error) {
	const query = "SELECT * FROM emails_website_configuration WHERE from_domain = $1"

	err = db.Get(ctx, &configuration, query, domain)
	if err != nil {
		if err == sql.ErrNoRows {
			err = emails.ErrWebsiteConfigurationNotFound
		} else {
			err = fmt.Errorf("emails.FindWebsiteConfigurationByDomain: %w", err)
		}
		return
	}

	return
}

func (repo *EmailsRepository) UpdateWebsiteConfiguration(ctx context.Context, db db.Queryer, config emails.WebsiteConfiguration) (err error) {
	const query = `UPDATE emails_website_configuration
		SET from_address = $1, from_domain = $2, domain_verified = $3, dns_records = $4,
			updated_at = $5, from_name = $6
		WHERE website_id = $7`

	_, err = db.Exec(ctx, query, config.FromAddress, config.FromDomain, config.DomainVerified, config.DnsRecords,
		config.UpdatedAt, config.FromName,
		config.WebsiteID)
	if err != nil {
		err = fmt.Errorf("emails.UpdateWebsiteConfiguration: %w", err)
		return
	}

	return
}
