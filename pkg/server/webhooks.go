package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stripe/stripe-go/v81/webhook"
	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/server/apiutil"
	"markdown.ninja/stdx/crypto"
	"markdown.ninja/stdx/log/slogx"
)

func (server *server) stripeWebhook(res http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	logger := slogx.FromCtx(ctx)

	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(res, req.Body, MaxBodyBytes)
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		err = fmt.Errorf("server.stripeWebhook: reading body: %w", err)
		apiutil.SendError(ctx, res, err)
		return
	}

	stripeEvent, err := webhook.ConstructEvent(payload, req.Header.Get("Stripe-Signature"), server.stripeWebhookSecret)
	if err != nil {
		logger.Warn("server.stripeWebhook: error verifying webhook signature", slogx.Err(err))
		apiutil.SendError(ctx, res, errs.InvalidArgument(fmt.Sprintf("Error verifying webhook signature: %v", err)))
		return
	}

	// if the event comes from a connected account (via Stripe Connect), then Account will not be empty
	if stripeEvent.Account != "" {
		err = server.storeService.HandleStripeEvent(ctx, stripeEvent)
	} else {
		err = server.organizationsService.HandleStripeEvent(ctx, stripeEvent)
	}
	if err != nil {
		err = fmt.Errorf("server.stripeWebhook: processing event [%s - %s]: %w", stripeEvent.ID, stripeEvent.Type, err)
		apiutil.SendError(ctx, res, err)
		return
	}

	res.WriteHeader(http.StatusOK)
}

func (server *server) pingooWebhookHandler(res http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	webhookSecret := chi.URLParam(req, "secret")

	if crypto.ConstantTimeCompare([]byte(webhookSecret), []byte(server.pingooConfig.WebhookSecret)) {
		server.kernelService.SleepAuthFailure()
		err := errors.New("server.pingooWebhookHandler: webhook secret is not valid")
		apiutil.SendError(ctx, res, err)
		return
	}

	const MaxBodyBytes = int64(500_000)
	req.Body = http.MaxBytesReader(res, req.Body, MaxBodyBytes)
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		err = fmt.Errorf("server.pingooWebhookHandler: error reading body: %w", err)
		apiutil.SendError(ctx, res, err)
		return
	}

	var pingooEvent pingoo.Event
	err = json.Unmarshal(payload, &pingooEvent)
	if err != nil {
		err = errs.Internal(fmt.Sprintf("server.pingooWebhookHandler: error parsing event to JSON: %v", err), err)
		apiutil.SendError(ctx, res, err)
		return
	}

	err = server.kernelService.HandlePingooWebhook(ctx, pingooEvent)
	if err != nil {
		err = fmt.Errorf("server.pingooWebhookHandler: error processing event [%s]: %w", pingooEvent.Type, err)
		apiutil.SendError(ctx, res, err)
		return
	}

	res.WriteHeader(http.StatusOK)
}
