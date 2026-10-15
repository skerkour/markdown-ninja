package service

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"markdown.ninja/pingoo-go"
	"markdown.ninja/pkg/errs"
	"markdown.ninja/pkg/services/kernel"
	"markdown.ninja/stdx/retry"
	"markdown.ninja/stdx/stringsx"
)

// var paletteColorRegexp = regexp.MustCompile("var\\(--test-palette-[1-8]\\)")

func (service *KernelService) ValidateEmail(ctx context.Context, emailAddress string, rejectBlockedDomains bool) (err error) {
	if emailAddress == "" || len(emailAddress) > kernel.EmailMaxLength ||
		!stringsx.IsLower(emailAddress) {
		return kernel.ErrEmailIsNotValid
	}

	var pingooRes pingoo.EmailInfo
	err = retry.Do(func() (retryErr error) {
		pingooRes, retryErr = service.pingooClient.LookupEmail(ctx, emailAddress)
		return retryErr
	}, retry.Context(ctx), retry.Attempts(3), retry.Delay(100*time.Millisecond))
	if err != nil {
		return errs.Internal("error checking email address with Pingoo", err)
	} else {
		if !pingooRes.Valid || len(pingooRes.MxRecords) == 0 || (rejectBlockedDomains && pingooRes.Disposable) {
			return kernel.ErrEmailIsNotValid
		}
	}

	return
}

func (service *KernelService) ValidateColor(color string) (err error) {
	// if acceptPalette && paletteColorRegexp.Match([]byte(color)) {
	// 	return nil
	// }

	if (len(color) != 7 && len(color) != 9) || !strings.HasPrefix(color, "#") {
		err = kernel.ErrColorIsNotValid
		return err
	}

	_, err = hex.DecodeString(color[1:])
	if err != nil {
		err = kernel.ErrColorIsNotValid
		return err
	}

	return nil
}
