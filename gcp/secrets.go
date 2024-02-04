package gcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// listSecrets lists all secrets in the given project.
func LoadGCPSecretsIntoEnv(projectID string) error {

	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create secretmanager client: %w", err)
	}
	defer client.Close()

	// Build the request.
	req := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", projectID),
	}

	// Call the API.
	it := client.ListSecrets(ctx, req)
	var secretList []string
	for {
		secret, err := it.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		req := &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("%s/versions/latest", secret.Name),
		}
		result, err := client.AccessSecretVersion(ctx, req)
		if err != nil {
			return fmt.Errorf("Failed to access secret version for %s: %v", secret.Name, err)
		}

		// Extract the secret name and value.
		secretValue := string(result.Payload.Data)
		secretName := secret.Name[strings.LastIndex(secret.Name, "/")+1:]

		os.Setenv(secretName, secretValue)
		fmt.Println("secretName", secretName, secretValue)
		secretList = append(secretList, secretName)
	}
	logger.Info("Successfully loaded GCP Keyvault secrets into environment variables.", zap.Any("secrets", secretList))
	return nil
}
