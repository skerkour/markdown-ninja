package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/stdx/log/slogx"
)

func (service *EmailsService) UpdateWebsiteConfiguration(ctx context.Context, input emails.UpdateWebsiteConfigurationInput) (configuration emails.WebsiteConfiguration, err error) {
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}
	logger := slogx.FromCtx(ctx)

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return
	}

	website, err := service.websitesService.FindWebsiteByID(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}

	err = service.organizationsService.CheckBillingGatedAction(ctx, service.db, website.OrganizationID, organizations.BillingGatedActionSetupCustomEmailDomain{})
	if err != nil {
		return
	}

	configuration, err = service.repo.FindWebsiteConfiguration(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}

	// TODO: validate
	fromnName := strings.TrimSpace(input.FromName)

	fromAddress := strings.ToLower(strings.TrimSpace(input.FromAddress))

	configuration.FromName = fromnName

	if fromAddress == "" {
		if configuration.FromDomain != "" {
			err = service.mailer.RemoveDomain(ctx, configuration.FromDomain)
			if err != nil {
				errMessage := "emails.UpdateWebsiteConfiguration: Removing domain from email provider (empty fromAddress)"
				logger.Error(errMessage, slogx.Err(err))
				err = emails.ErrRemovingDomain(configuration.FromDomain)
				return
			}
		}
		configuration.FromDomain = ""
		configuration.DnsRecords = []mailer.DnsRecord{}
		configuration.DomainVerified = false
		configuration.FromAddress = ""
	} else if fromAddress == configuration.FromAddress {
		// Do nothing
	} else {
		err = service.validateSenderEmailAddress(ctx, fromAddress)
		if err != nil {
			return
		}
		fromDomain := strings.Split(fromAddress, "@")[1]

		if fromDomain != configuration.FromDomain {
			var existingSite emails.WebsiteConfiguration
			existingSite, err = service.repo.FindWebsiteConfigurationByDomain(ctx, service.db, fromDomain)
			if err == nil && !existingSite.WebsiteID.Equal(configuration.WebsiteID) {
				err = emails.ErrDomainAlreadyInUse
			} else {
				if errs.IsNotFound(err) {
					err = nil
				}
			}
			if err != nil {
				return
			}

			if configuration.FromDomain != "" {
				err = service.mailer.RemoveDomain(ctx, configuration.FromDomain)
				if err != nil {
					errMessage := "emails.UpdateWebsiteConfiguration: Removing domain from email provider"
					logger.Error(errMessage, slogx.Err(err), slog.String("domain", fromDomain))
					err = errs.InvalidArgument(fmt.Sprintf("error adding domain: %s", fromDomain))
					return
				}
			}
			var emailDomain mailer.Domain
			emailDomain, err = service.mailer.AddDomain(ctx, fromDomain)
			if err != nil {
				errMessage := "emails.UpdateWebsiteConfiguration: Adding domain to email provider"
				logger.Error(errMessage, slogx.Err(err))
				err = emails.ErrRemovingDomain(configuration.FromDomain)
				return
			}

			configuration.UpdatedAt = time.Now().UTC()
			configuration.FromDomain = fromDomain
			configuration.DnsRecords = emailDomain.DnsRecords
			configuration.FromAddress = fromAddress
		}
	}

	err = service.repo.UpdateWebsiteConfiguration(ctx, service.db, configuration)
	if err != nil {
		return
	}

	service.sendEmailCache.Delete(getWebsiteConfigurationCacheKey(input.WebsiteID))
	service.sendEmailCache.Delete(getSenderApiTokenCacheKey(input.WebsiteID))

	return
}
