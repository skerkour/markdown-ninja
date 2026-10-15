package emails

import (
	"context"
	"net/mail"

	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type Service interface {
	// Misc
	GetDefaultFromAddressForWebsite(website websites.Website) mail.Address

	// Configuration
	InitWebsiteConfiguration(ctx context.Context, db db.Queryer, websiteID guid.GUID, name string) (configuration WebsiteConfiguration, err error)
	RemoveWebsiteConfiguration(ctx context.Context, db db.Queryer, websiteID guid.GUID) (err error)
	GetWebsiteConfiguration(ctx context.Context, input GetWebsiteConfigurationInput) (configuration WebsiteConfiguration, err error)
	UpdateWebsiteConfiguration(ctx context.Context, input UpdateWebsiteConfigurationInput) (configuration WebsiteConfiguration, err error)
	VerifyDnsConfiguration(ctx context.Context, input VerifyDnsConfigurationInput) (configuration WebsiteConfiguration, err error)
	FindWebsiteConfiguration(ctx context.Context, db db.Queryer, websiteID guid.GUID) (configuration WebsiteConfiguration, err error)

	// Newsletter
	CreateNewsletter(ctx context.Context, input CreateNewsletterInput) (newsletter Newsletter, err error)
	GetNewsletter(ctx context.Context, input GetNewsletterInput) (newsletter Newsletter, err error)
	GetNewsletters(ctx context.Context, input GetNewslettersInput) (newsletters []NewsletterMetadata, err error)
	DeleteNewsletter(ctx context.Context, input DeleteNewsletterInput) (err error)
	UpdateNewsletter(ctx context.Context, input UpdateNewsletterInput) (newsletter Newsletter, err error)
	SendNewsletter(ctx context.Context, input SendNewsletterInput) (newsletter Newsletter, err error)

	// Jobs
	JobDeleteWebsiteConfigurationData(ctx context.Context, input JobDeleteWebsiteConfigurationData) (err error)
	JobSendNewsletter(ctx context.Context, input JobSendNewsletter) (err error)
	JobSendPostAsNewsletter(ctx context.Context, input JobSendPostAsNewsletter) (err error)
	JobSendEmail(ctx context.Context, input JobSendEmail) (err error)

	// Tasks
	TaskSendScheduledNewsletters(ctx context.Context)
}
