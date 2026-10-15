package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"markdown.ninja/stdx/cron"
	"markdown.ninja/stdx/xwing"
	"markdown.ninja/stdx/yaml"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	configPath := flag.String("config", "", "path to config file")
	decryptFile := flag.String("decrypt", "", "path to encrypted file to decrypt")
	outputFile := flag.String("out", "", "output file (decrypt mode)")
	genKey := flag.Bool("generate", false, "generate an X-Wing keypair")
	flag.Parse()

	if *genKey {
		generateKeypair()
		return
	}

	if *decryptFile != "" {
		runDecrypt(*decryptFile, *outputFile)
		return
	}

	if *configPath == "" {
		slog.Error("config flag is required")
		os.Exit(1)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	s3Client, err := createS3Client(cfg.S3)
	if err != nil {
		slog.Error("creating s3 client", "error", err)
		os.Exit(1)
	}

	scheduler := cron.New()

	for _, db := range cfg.Databases {
		_, err := scheduler.AddFunc(db.Cron, func() {
			slog.Info("running backup", "folder", db.Folder)
			if err := createBackup(context.Background(), s3Client, cfg.S3, db); err != nil {
				slog.Error("backup failed", "folder", db.Folder, "error", err)
			}
		})
		if err != nil {
			slog.Error("invalid cron expression", "folder", db.Folder, "cron", db.Cron, "error", err)
			os.Exit(1)
		}

		slog.Info("scheduled backup", "folder", db.Folder, "cron", db.Cron)
	}

	scheduler.Run()
}

func runDecrypt(inputPath, outputPath string) {
	keyBase64 := os.Getenv("KEY")
	if keyBase64 == "" {
		slog.Error("KEY environment variable is required")
		os.Exit(1)
	}

	if outputPath == "" {
		if strings.HasSuffix(inputPath, ".enc") {
			outputPath = strings.TrimSuffix(inputPath, ".enc")
		} else {
			slog.Error("-out is required when input file does not end with .enc")
			os.Exit(1)
		}
		outputPath = strings.TrimSuffix(outputPath, ".gz")
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		slog.Error("reading input file", "error", err)
		os.Exit(1)
	}

	plaintext, err := decrypt(data, keyBase64)
	if err != nil {
		slog.Error("decryption failed", "error", err)
		os.Exit(1)
	}

	decompressed, err := decompressGzip(plaintext)
	if err != nil {
		slog.Error("decompression failed", "error", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, decompressed, 0600); err != nil {
		slog.Error("writing output file", "error", err)
		os.Exit(1)
	}

	slog.Info("decryption completed", "output", outputPath)
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if config.S3.Endpoint == "" {
		return nil, errors.New("s3.endpoint is required")
	}
	if config.S3.Bucket == "" {
		return nil, errors.New("s3.bucket is required")
	}
	if config.S3.AccessKeyID == "" {
		return nil, errors.New("s3.access_key_id is required")
	}
	if config.S3.SecretAccessKey == "" {
		return nil, errors.New("s3.secret_access_key is required")
	}
	if config.S3.Region == "" {
		config.S3.Region = "auto"
	}

	for i := range config.Databases {
		db := &config.Databases[i]
		if db.URL == "" {
			return nil, errors.New("database url is required")
		}

		if db.PublicKey == "" {
			db.PublicKey = config.PublicKey
		}
		if db.PublicKey == "" {
			return nil, errors.New("database public_key is required (set per-database or globally)")
		}

		pubKeyBytes, err := base64.StdEncoding.DecodeString(db.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("database public_key: invalid base64: %w", err)
		}
		if len(pubKeyBytes) != xwing.EncapsulationKeySize {
			return nil, fmt.Errorf("database public_key: expected %d bytes, got %d", xwing.EncapsulationKeySize, len(pubKeyBytes))
		}

		if db.Cron == "" {
			return nil, errors.New("database cron is required")
		}
		if db.Folder == "" {
			return nil, errors.New("database folder is required")
		}
	}

	return &config, nil
}

func generateKeypair() {
	decapKey := xwing.GenerateDecapsulationKey()

	secretKeyBase64 := base64.StdEncoding.EncodeToString(decapKey.Bytes())
	publicKeyBase64 := base64.StdEncoding.EncodeToString(decapKey.EncapsulationKey().Bytes())

	fmt.Printf("secret key: %s\n", secretKeyBase64)
	fmt.Printf("public key: %s\n", publicKeyBase64)
}
