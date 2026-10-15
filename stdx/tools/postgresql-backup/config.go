package main

type Config struct {
	S3        S3Config         `yaml:"s3"`
	PublicKey string           `yaml:"public_key"`
	Databases []DatabaseConfig `yaml:"databases"`
}

type S3Config struct {
	Endpoint        string `yaml:"endpoint"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	Region          string `yaml:"region"`
}

type DatabaseConfig struct {
	URL       string `yaml:"url"`
	PublicKey string `yaml:"public_key"`
	Cron      string `yaml:"cron"`
	Folder    string `yaml:"folder"`
}
