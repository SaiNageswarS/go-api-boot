package embed

import (
	"context"
	"errors"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/testutil"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"
)

// mockGenaiClient implements genaiClientInterface for testing
type mockGenaiClient struct {
	embedContentResponse *genai.EmbedContentResponse
	embedContentError    error
	capturedModel        string
	capturedContents     []*genai.Content
	capturedConfig       *genai.EmbedContentConfig
}

func (m *mockGenaiClient) EmbedContent(ctx context.Context, model string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	m.capturedModel = model
	m.capturedContents = contents
	m.capturedConfig = config
	return m.embedContentResponse, m.embedContentError
}

func TestProvideGeminiEmbeddingClient_Success(t *testing.T) {
	testutil.WithEnv("GEMINI_API_KEY", "dummy-key", func(logger *testutil.MockLogger) {
		// Note: This will fail in real tests since genai.NewClient requires a valid API key
		// In a real scenario, you'd need to mock the genai.NewClient function
		// For now, we'll test the structure
		client := ProvideGeminiEmbeddingClient()
		assert.NotNil(t, client)
	})
}

func TestProvideGeminiEmbeddingClient_MissingAPIKey(t *testing.T) {
	testutil.WithEnv("GEMINI_API_KEY", "", func(logger *testutil.MockLogger) {
		ProvideGeminiEmbeddingClient()
		assert.True(t, logger.IsFatalCalled)
		assert.Equal(t, logger.FatalMsg, "GEMINI_API_KEY environment variable is not set")
	})
}

func TestGeminiGetEmbedding_Success(t *testing.T) {
	mockClient := &mockGenaiClient{
		embedContentResponse: &genai.EmbedContentResponse{
			Embeddings: []*genai.ContentEmbedding{
				{
					Values: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
				},
			},
		},
		embedContentError: nil,
	}

	client := &GeminiEmbeddingClient{
		client: mockClient,
	}

	ctx := context.Background()
	result, err := async.Await(client.GetEmbedding(ctx, "test text"))

	assert.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3, 0.4, 0.5}, result)
	assert.Equal(t, "text-embedding-004", mockClient.capturedModel)
	assert.Equal(t, 1, len(mockClient.capturedContents))
	assert.Equal(t, "SEMANTIC_SIMILARITY", mockClient.capturedConfig.TaskType)
}

func TestGeminiGetEmbedding_WithCustomModel(t *testing.T) {
	mockClient := &mockGenaiClient{
		embedContentResponse: &genai.EmbedContentResponse{
			Embeddings: []*genai.ContentEmbedding{
				{
					Values: []float32{0.1, 0.2, 0.3},
				},
			},
		},
		embedContentError: nil,
	}

	client := &GeminiEmbeddingClient{
		client: mockClient,
	}

	ctx := context.Background()
	result, err := async.Await(client.GetEmbedding(ctx, "test text", WithModel("custom-model")))

	assert.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, result)
	assert.Equal(t, "custom-model", mockClient.capturedModel)
}

func TestGeminiGetEmbedding_WithTaskType(t *testing.T) {
	mockClient := &mockGenaiClient{
		embedContentResponse: &genai.EmbedContentResponse{
			Embeddings: []*genai.ContentEmbedding{
				{
					Values: []float32{0.1, 0.2, 0.3},
				},
			},
		},
		embedContentError: nil,
	}

	client := &GeminiEmbeddingClient{
		client: mockClient,
	}

	ctx := context.Background()
	result, err := async.Await(client.GetEmbedding(ctx, "test text", WithRetrievalQuery()))

	assert.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, result)
	assert.Equal(t, "RETRIEVAL_QUERY", mockClient.capturedConfig.TaskType)
}

func TestGeminiGetEmbedding_APIError(t *testing.T) {
	mockClient := &mockGenaiClient{
		embedContentResponse: nil,
		embedContentError:    errors.New("API error occurred"),
	}

	client := &GeminiEmbeddingClient{
		client: mockClient,
	}

	ctx := context.Background()
	result := <-client.GetEmbedding(ctx, "test text")

	assert.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "API error occurred")
}

func TestGeminiGetEmbedding_NoEmbeddingData(t *testing.T) {
	mockClient := &mockGenaiClient{
		embedContentResponse: &genai.EmbedContentResponse{
			Embeddings: []*genai.ContentEmbedding{}, // Empty embeddings
		},
		embedContentError: nil,
	}

	client := &GeminiEmbeddingClient{
		client: mockClient,
	}

	ctx := context.Background()
	result := <-client.GetEmbedding(ctx, "test text")

	assert.Error(t, result.Err)
	assert.Equal(t, "no embedding data found", result.Err.Error())
}

func TestGeminiGetEmbedding_EmptyEmbeddingValues(t *testing.T) {
	mockClient := &mockGenaiClient{
		embedContentResponse: &genai.EmbedContentResponse{
			Embeddings: []*genai.ContentEmbedding{
				{
					Values: []float32{}, // Empty values
				},
			},
		},
		embedContentError: nil,
	}

	client := &GeminiEmbeddingClient{
		client: mockClient,
	}

	ctx := context.Background()
	result := <-client.GetEmbedding(ctx, "test text")

	assert.Error(t, result.Err)
	assert.Equal(t, "no embedding data found", result.Err.Error())
}

func TestGeminiTaskMapping(t *testing.T) {
	// Test task mappings using helper functions
	helperFunctionTests := []struct {
		name       string
		option     EmbedOption
		geminiTask string
	}{
		{"WithRetrievalQuery", WithRetrievalQuery(), "RETRIEVAL_QUERY"},
		{"WithRetrievalPassage", WithRetrievalPassage(), "RETRIEVAL_DOCUMENT"},
		{"WithCodeQuery", WithCodeQuery(), "CODE_RETRIEVAL_QUERY"},
		{"WithCodePassage", WithCodePassage(), "RETRIEVAL_DOCUMENT"},
		{"WithTextMatching", WithTextMatching(), "SEMANTIC_SIMILARITY"},
		{"WithClassification", WithClassification(), "CLASSIFICATION"},
		{"WithClustering", WithClustering(), "CLUSTERING"},
		{"WithQuestionAnswering", WithQuestionAnswering(), "QUESTION_ANSWERING"},
		{"WithFactVerification", WithFactVerification(), "FACT_VERIFICATION"},
	}

	for _, tc := range helperFunctionTests {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &mockGenaiClient{
				embedContentResponse: &genai.EmbedContentResponse{
					Embeddings: []*genai.ContentEmbedding{
						{Values: []float32{0.1, 0.2}},
					},
				},
			}

			client := &GeminiEmbeddingClient{client: mockClient}
			ctx := context.Background()

			_, err := async.Await(client.GetEmbedding(ctx, "test", tc.option))

			assert.NoError(t, err)
			assert.Equal(t, tc.geminiTask, mockClient.capturedConfig.TaskType)
		})
	}
}
