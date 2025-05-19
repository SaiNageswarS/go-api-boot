package config

import (
	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/caarlos0/env/v11"
	"github.com/go-ini/ini"
)

type BootConfig struct {
	MongoUri string `env:"MONGO-URI" ini:"mongo_uri"`

	// jwt
	AccessSecret string `env:"ACCESS-SECRET" ini:"access_secret"`

	// ssl
	SslBucket string `env:"SSL-BUCKET" ini:"ssl_bucket"`
	Domain    string `env:"DOMAIN" ini:"domain"`

	// Cloud
	AzureStorageAccount string `env:"AZURE-STORAGE-ACCOUNT" ini:"azure_storage_account"`

	GcpProjectId string `env:"GCP-PROJECT-ID" ini:"gcp_project_id"`
}

// Loads config into the target struct from the given path - an INI file.
// It first loads the INI file and then overrides the values with environment variables.
// If loading secrets from cloud like Azure Keyvault or GCP Secret Manager, first load the secrets into
// environment variables and then load the config struct.
func LoadConfig[T any](path string, target *T) error {
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
