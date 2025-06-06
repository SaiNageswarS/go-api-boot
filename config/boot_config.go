package config

import (
	"errors"
	"os"

	"github.com/go-ini/ini"
)

// Note: go-api-boot holds clear distinction between config and secrets.
// Config is for application configuration that can be stored in version control.
// Secrets are sensitive information like API keys, passwords, etc. that should not be stored in version control.
// Secrets should be exclusively read from environment variables.
type BootConfig struct {
	// ssl
	SslBucket string `ini:"ssl_bucket"`
	Domain    string `ini:"domain"`

	// Cloud
	AzureStorageAccount string `ini:"azure_storage_account"`
	AzureKeyVaultName   string `ini:"azure_key_vault_name"`

	GcpProjectId string `ini:"gcp_project_id"`
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

	runMode := os.Getenv("ENV")

	// Step 1: Load from INI
	if err := file.Section(runMode).MapTo(target); err != nil {
		return err
	}

	return nil
}
