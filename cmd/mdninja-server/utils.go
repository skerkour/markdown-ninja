package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"markdown.ninja/cmd/mdninja-server/config"
	"markdown.ninja/pkg/buildinfo"
	"markdown.ninja/pkg/kms"
	"markdown.ninja/pkg/mailer"
	"markdown.ninja/pkg/mailer/console"
	"markdown.ninja/pkg/mailer/ses"
	"markdown.ninja/pkg/storage/s3"
	"markdown.ninja/stdx/log/loki"
)

func newLogger(ctx context.Context) (*slog.Logger, *slog.LevelVar, *loki.Writer) {
	logLevel := &slog.LevelVar{}
	logLevel.Set(slog.LevelInfo)
	lokiWriter := loki.NewWriter(ctx, "", map[string]string{"service": "markdown-ninja-server"}, nil)
	replaceAttrFunc := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.MessageKey {
			a.Key = "message"
		}
		return a
	}
	logger := slog.New(slog.NewJSONHandler(lokiWriter, &slog.HandlerOptions{Level: logLevel, ReplaceAttr: replaceAttrFunc})).
		With(slog.String("service", "markdown-ninja-server"), slog.String("version", buildinfo.Version))
	return logger, logLevel, lokiWriter
}

func loadMailer(conf config.Config) (mailer mailer.Mailer, err error) {
	switch conf.Emails.Provider {
	case config.EmailsProviderConsole:
		mailer = console.NewConsoleMailer()
	case config.EmailsProviderSes:
		if conf.Aws == nil {
			return nil, errors.New("mailer: config.aws is null")
		}
		sesConf := ses.Config{
			AccessKeyID:     conf.Aws.AccessKeyID,
			SecretAccessKey: conf.Aws.SecretAccessKey,
			Region:          conf.Aws.Region,
		}
		mailer, err = ses.NewSesMailer(sesConf)
	default:
		err = fmt.Errorf("mailer: %s is not a valid email provider. Valid values are: [%s, %s]",
			conf.Emails.Provider, config.EmailsProviderConsole, config.EmailsProviderSes)
	}

	return
}

func loadS3(conf config.Config) (s3Client *s3.Client, err error) {
	switch conf.S3.Provider {
	case config.S3ProviderScaleway:
		if conf.Scaleway == nil {
			return nil, errors.New("s3: config.scaleway is null")
		}
		s3Client, err = s3.NewClient(s3.ClientConfig{
			Bucket:          conf.S3.Bucket,
			Endpoint:        conf.S3.Endpoint,
			AccessKeyID:     conf.Scaleway.AccessKeyID,
			SecretAccessKey: conf.Scaleway.SecretAccessKey,
			Region:          conf.Scaleway.Region,
		})
	case config.S3ProviderAws:
		if conf.Aws == nil {
			return nil, errors.New("s3: config.aws is null")
		}
		s3Client, err = s3.NewClient(s3.ClientConfig{
			Bucket:          conf.S3.Bucket,
			AccessKeyID:     conf.Aws.AccessKeyID,
			SecretAccessKey: conf.Aws.SecretAccessKey,
			Region:          conf.Aws.Region,
		})
	default:
		err = fmt.Errorf("s3: %s is not a valid provider. Valid values are: [%s, %s]",
			conf.S3.Provider, config.S3ProviderAws, config.S3ProviderScaleway)
	}

	return
}

func loadKms(conf config.Config) (*kms.Kms, error) {
	var kmsService kms.KmsService

	switch conf.Kms.Provider {
	case kms.ProviderConsole:
		kmsService = kms.NewConsoleKms()
	case kms.ProviderScaleway:
		kmsService = kms.NewScalewayKms(nil, conf.Scaleway.SecretAccessKey, conf.Scaleway.Region, conf.Kms.MasterKeyID)
	default:
		return nil, fmt.Errorf("kms: %s is not a valid provider. Valid values are: [%s, %s]",
			conf.Kms.Provider, kms.ProviderConsole, kms.ProviderScaleway)
	}

	kms := kms.New(kmsService, conf.Kms.MasterKeyID)
	return kms, nil
}
