package config

import (
	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/caarlos0/env/v11"
	"github.com/go-ini/ini"
)

type BaseConfig struct {
	MongoUri string `env:"MONGO-URI"`

	// jwt
	AccessSecret string `env:"ACCESS-SECRET"`

	// ssl
	SslBucket string `env:"SSL-BUCKET"`
	Domain    string `env:"DOMAIN"`

	// Cloud
	AzureStorageAccount string `env:"AZURE-STORAGE-ACCOUNT"`

	GcpProjectId string `env:"GCP-PROJECT-ID"`
}

// Loads config into the target struct from the given path - an INI file.
// It first loads the INI file and then overrides the values with environment variables.
// If loading secrets from cloud like Azure Keyvault or GCP Secret Manager, first load the secrets into
// environment variables and then load the config struct.
func LoadConfig[T any](path string, target *T) error {
	cfg, err := ini.Load(path)
	if err != nil {
		return err
	}

	// Step 1: Load from INI
	if err := cfg.MapTo(target); err != nil {
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
