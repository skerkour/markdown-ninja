package websites

import (
	"context"
	"io"
	"time"

	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
	"markdown.ninja/stdx/uuid"
)

type Service interface {
	// Utils
	CheckUserIsStaff(ctx context.Context, db db.Queryer, userID uuid.UUID, websiteID guid.GUID) (err error)

	// Websites
	FindWebsiteByDomain(ctx context.Context, db db.Queryer, domain string) (website Website, err error)
	CreateWebsite(ctx context.Context, input CreateWebsiteInput) (Website, error)
	DeleteWebsite(ctx context.Context, input DeleteWebsiteInput) error
	GetWebsitesForOrganization(ctx context.Context, input GetWebsitesForOrganizationInput) (websites []Website, err error)
	GetWebsite(ctx context.Context, input GetWebsiteInput) (website Website, err error)
	UpdateWebsite(ctx context.Context, input UpdateWebsiteInput) (website Website, err error)
	FindWebsiteByID(ctx context.Context, db db.Queryer, websiteID guid.GUID) (website Website, err error)
	UpdateWebsiteModifiedAt(ctx context.Context, db db.Queryer, websiteID guid.GUID, modifiedAt time.Time) (err error)
	FindWebsitesForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (websites []Website, err error)
	ListWebsites(ctx context.Context, input ListWebsitesInput) (websites kernel.PaginatedResult[Website], err error)
	FindAllWebsites(ctx context.Context, db db.Queryer) (websites []Website, err error)
	GetWebsitesCountForOrganization(ctx context.Context, db db.Queryer, organizationID guid.GUID) (websitesCount int64, err error)
	UpdateWebsiteIcon(ctx context.Context, input UpdateWebsiteIconInput) (err error)
	GetWebsiteIcon(ctx context.Context, websiteID guid.GUID, size int) (icon io.ReadCloser, err error)
	GetAdminStatistics(ctx context.Context, _ kernel.EmptyInput) (stats AdminStatistics, err error)

	// Redirects
	SaveRedirects(ctx context.Context, input SaveRedirectsInput) (redirects []Redirect, err error)
	FindRedirects(ctx context.Context, db db.Queryer, websiteID guid.GUID) (redirects []Redirect, err error)
	MatchRedirect(ctx context.Context, domain, path string, redirects []Redirect) *Redirect

	// Domains
	AddDomain(ctx context.Context, input AddDomainInput) (domain Domain, err error)
	RemoveDomain(ctx context.Context, input RemoveDomainInput) (err error)
	SetDomainAsPrimary(ctx context.Context, input SetDomainAsPrimaryInput) (err error)
	CheckTlsCertificateForDomain(ctx context.Context, input CheckTlsCertificateForDomainInput) (err error)
	GetDomainsCountForWebsite(ctx context.Context, db db.Queryer, websiteID guid.GUID) (count int64, err error)

	// Jobs
	// JobDeleteCdnCustomHostname(ctx context.Context, input JobDeleteCdnCustomHostname) (err error)
}
