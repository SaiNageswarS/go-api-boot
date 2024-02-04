package server

import (
	"os"

	"github.com/SaiNageswarS/go-api-boot/azure"
	"github.com/SaiNageswarS/go-api-boot/gcp"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/joho/godotenv"
)

func LoadSecretsIntoEnv(useAzureKeyvault bool, useGCPSecrets bool) {
	godotenv.Load()

	if useAzureKeyvault {
		azure.LoadAzureKeyvaultSecretsIntoEnv()
	}

	if useGCPSecrets {
		projectID := os.Getenv("GCP-PROJECT-ID")
		if len(projectID) == 0 {
			logger.Error("GCP-PROJECT-ID environment variable is not set")
			return
		}

		gcp.LoadGCPSecretsIntoEnv(projectID)
	}
}
