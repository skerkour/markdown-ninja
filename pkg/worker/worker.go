package worker

import (
	"context"
	"errors"
	"log/slog"

	"markdown.ninja/pkg/services/contacts"
	"markdown.ninja/pkg/services/content"
	"markdown.ninja/pkg/services/emails"
	"markdown.ninja/pkg/services/events"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/pkg/services/organizations"
	"markdown.ninja/pkg/services/site"
	"markdown.ninja/pkg/services/store"
	"markdown.ninja/pkg/services/websites"
	"markdown.ninja/stdx/queue"
	"markdown.ninja/stdx/workerpool"
)

func Start(ctx context.Context, logger *slog.Logger, concurrencyMax uint32, queuee queue.Queue, kernelService kernel.PrivateService,
	websitesService websites.Service, emailsService emails.Service, storeService store.Service,
	contentService content.Service, siteService site.Service, contactsService contacts.Service,
	eventsService events.Service, organizationsService organizations.Service) (err error) {
	if logger == nil {
		return errors.New("worker: logger is null")
	}

	workerOpt := workerpool.Options{
		ConcurrencyMax: concurrencyMax,
		Logger:         logger,
	}
	workerPool, err := workerpool.NewPool(queuee, &workerOpt)
	if err != nil {
		return
	}

	// kernel

	// organizations
	workerpool.AddHandler(workerPool, organizationsService.JobSendStaffInvitations)
	workerpool.AddHandler(workerPool, organizationsService.JobInvoiceMonthlyUsage)
	workerpool.AddHandler(workerPool, organizationsService.JobDispatchInvoiceMonthlyUsage)

	// websites

	// emails
	workerpool.AddHandler(workerPool, emailsService.JobDeleteWebsiteConfigurationData)
	workerpool.AddHandler(workerPool, emailsService.JobSendNewsletter)
	workerpool.AddHandler(workerPool, emailsService.JobSendPostAsNewsletter)
	workerpool.AddHandler(workerPool, emailsService.JobSendEmail)

	// store
	workerpool.AddHandler(workerPool, storeService.JobSendOrderConfirmationEmail)
	workerpool.AddHandler(workerPool, storeService.JobCreateStripeRefund)
	workerpool.AddHandler(workerPool, storeService.JobSyncRefundWithStripe)

	// content
	workerpool.AddHandler(workerPool, contentService.JobDeleteAssetData)
	workerpool.AddHandler(workerPool, contentService.JobDeleteAssetsDataWithPrefix)
	workerpool.AddHandler(workerPool, contentService.JobPublishPages)

	// site
	workerpool.AddHandler(workerPool, siteService.JobSendLoginEmail)
	workerpool.AddHandler(workerPool, siteService.JobSendSubscribeEmail)

	// contacts
	workerpool.AddHandler(workerPool, contactsService.JobDeleteOldUnverifiedSessions)
	workerpool.AddHandler(workerPool, contactsService.JobUpdateStripeContact)
	workerpool.AddHandler(workerPool, contactsService.JobSendVerifyEmailEmail)
	workerpool.AddHandler(workerPool, contactsService.JobSyncUnsubscribedContacts)
	// workerpool.AddHandler(workerPool, contacts.JobDeleteOldUnverifiedContacts, contactsService.JobDeleteOldUnverifiedContacts)

	// events
	workerpool.AddHandler(workerPool, eventsService.JobDeleteWebsiteEvents)
	workerpool.AddHandler(workerPool, eventsService.JobDeleteOrganizationEvents)
	workerpool.AddHandler(workerPool, eventsService.JobRotateAnonymousIDSalt)

	workerPool.Start(ctx)

	return
}
