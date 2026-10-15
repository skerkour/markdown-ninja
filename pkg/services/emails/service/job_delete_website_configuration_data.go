package service

import (
	"context"
	"time"

	"log/slog"

	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/log/slogx"
	"markdown.ninja/stdx/retry"
)

func (service *EmailsService) JobDeleteWebsiteConfigurationData(ctx context.Context, input emails.JobDeleteWebsiteConfigurationData) (err error) {
	logger := slogx.FromCtx(ctx)

	err = retry.Do(func() error {
		return service.mailer.RemoveDomain(ctx, input.Domain)
	}, retry.Context(ctx), retry.Attempts(5), retry.Delay(time.Second*10))
	if err != nil {
		errMessage := "emails.JobDeleteWebsiteConfigurationData: Removing email domain from provider"
		logger.Error(errMessage, slogx.Err(err), slog.String("domain", input.Domain))
		err = websites.ErrRemovingEmailDomain(input.Domain)
		return
	}

	return
}
