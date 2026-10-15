package autoupdate

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"os"
	"time"

	"markdown.ninja/stdx/log/slogx"
)

// /[project]/[channel].json
// /[project]/[project_version]/[project_version_os_architecture].zip
// /[project]/[project_version]/release.json

const (
	updatedExecutableOpenFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
)

func (updater *Updater) RunInBackground(ctx context.Context) {
	logger := slogx.FromCtx(ctx)
	var err error
	var manifest ChannelManifest

	for {
		// sleep for autoupdateInterval + 60 seconds jitter to avoid DDoSing the server
		waitFor := rand.Int64N(60) + updater.autoupdateInterval

		select {
		case <-ctx.Done():
			if updater.verbose {
				logger.Info("autoupdate: stopping")
			}

			return
		case <-time.After(time.Duration(waitFor) * time.Second):
			if updater.verbose {
				logger.Info("autoupdate: checking for update")
			}

			manifest, err = updater.CheckUpdate(ctx)
			if err != nil {
				logger.Warn("autoupdate: error while checking for update", slogx.Err(err))
				continue
			}

			if updater.UpdateAvailable(manifest) {
				logger = logger.With(slog.String("autoupdate_new_version", manifest.Version))
				if updater.verbose {
					logger.Info("autoupdate: a new update is available")
				}

				err = updater.Update(ctx, manifest)
				if err != nil {
					logger.Warn("autoupdate: error installing new version", slogx.Err(err))
					continue
				}

				if updater.verbose {
					logger.Info("autoupdate: new version successfully installed")
				}

				updater.Updated <- struct{}{}
			}
		}
	}
}
