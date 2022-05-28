package server

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func LoadSecretsIntoEnv(useAzureKeyvault bool) {
	godotenv.Load()

	if useAzureKeyvault {
		loadAzureKeyvaultSecretsIntoEnv()
	}
}

func loadAzureKeyvaultSecretsIntoEnv() {
	logger.Info("Loading Azure Keyvault secrets into environment variables.")
	client := getKeyvaultClient()

	//List secrets
	var secretList []string
	pager := client.ListPropertiesOfSecrets(nil)

	for pager.More() {
		page, err := pager.NextPage(context.TODO())
		if err != nil {
			panic(err)
		}

		for _, v := range page.Secrets {
			resp, err := client.GetSecret(context.TODO(), *v.Name, nil)
			if err != nil {
				panic(err)
			}

			os.Setenv(*v.Name, *resp.Value)
			secretList = append(secretList, *v.Name)
		}
	}

	logger.Info("Successfully loaded Azure Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
}

func getKeyvaultClient() *azsecrets.Client {
	keyVaultName := os.Getenv("AZURE-KEYVAULT-NAME")
	keyVaultUrl := fmt.Sprintf("https://%s.vault.azure.net/", keyVaultName)

	//Create a credential using the NewDefaultAzureCredential type.
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		log.Fatalf("failed to obtain a credential: %v", err)
	}

	//Establish a connection to the Key Vault client
	client, err := azsecrets.NewClient(keyVaultUrl, cred, nil)
	if err != nil {
		log.Fatalf("failed to connect to client: %v", err)
	}

	return client
}
