package service

import (
	"context"
	"time"

	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/stdx/log/slogx"
)

func (service *EmailsService) VerifyDnsConfiguration(ctx context.Context, input emails.VerifyDnsConfigurationInput) (configuration emails.WebsiteConfiguration, err error) {
	logger := slogx.FromCtx(ctx)
	actorID, err := service.kernel.CurrentUserID(ctx)
	if err != nil {
		return
	}

	err = service.websitesService.CheckUserIsStaff(ctx, service.db, actorID, input.WebsiteID)
	if err != nil {
		return
	}

	configuration, err = service.repo.FindWebsiteConfiguration(ctx, service.db, input.WebsiteID)
	if err != nil {
		return
	}

	if configuration.FromDomain == "" {
		err = emails.ErrNoCustomEmailDomainConfigured
		return
	}

	configuration.DomainVerified, err = service.mailer.VerifyDomain(ctx, configuration.FromDomain)
	if err != nil {
		errMessage := "emails.VerifyDnsConfiguration: Verifying VerifyDnsConfiguration"
		logger.Error(errMessage, slogx.Err(err))
		err = errs.Internal(errMessage, err)
		return
	}

	configuration.UpdatedAt = time.Now().UTC()
	err = service.repo.UpdateWebsiteConfiguration(ctx, service.db, configuration)
	if err != nil {
		return
	}

	service.sendEmailCache.Delete(getWebsiteConfigurationCacheKey(input.WebsiteID))
	service.sendEmailCache.Delete(getSenderApiTokenCacheKey(input.WebsiteID))

	return
}
