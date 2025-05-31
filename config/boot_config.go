package config

import (
	"errors"

	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/caarlos0/env/v11"
	"github.com/go-ini/ini"
)

// Note: go-api-boot holds clear distinction between config and secrets.
// Config is for application configuration that can be stored in version control.
// Secrets are sensitive information like API keys, passwords, etc. that should not be stored in version control.
// Secrets should be exclusively read from environment variables.
type BootConfig struct {
	// ssl
	SslBucket string `env:"SSL-BUCKET" ini:"ssl_bucket"`
	Domain    string `env:"DOMAIN" ini:"domain"`

	// Cloud
	AzureStorageAccount string `env:"AZURE-STORAGE-ACCOUNT" ini:"azure_storage_account"`

	GcpProjectId string `env:"GCP-PROJECT-ID" ini:"gcp_project_id"`
}

// Loads config into the target struct from the given path - an INI file.
// Can override config values with environment variables. Don't put secrets in the INI file.
// It first loads the INI file and then overrides the values with environment variables.
func LoadConfig[T any](path string, target *T) error {
	if target == nil {
		return errors.New("target cannot be nil")
	}

	file, err := ini.Load(path)
	if err != nil {
		return err
	}

	// Step 1: Load from INI
	if err := file.Section("").MapTo(target); err != nil {
		return err
	}

	// Step 2: Override from ENV
	err = dotenv.LoadEnv()
	if err != nil {
		return err
	}

	if err := env.Parse(target); err != nil {
		return err
	}

	return nil
}
