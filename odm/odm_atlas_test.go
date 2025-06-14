// Tests for odm Atlas integration.

package odm

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/SaiNageswarS/go-api-boot/llm"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type EmbeddedMovies struct {
	ID            string      `bson:"_id"`
	Plot          string      `bson:"plot"`
	Year          int         `bson:"year"`
	Cast          []string    `bson:"cast"`
	Languages     []string    `bson:"languages"`
	Directors     []string    `bson:"directors"`
	Genres        []string    `bson:"genres"`
	PlotEmbedding bson.Vector `bson:"plotEmbedding"`
	Title         string      `bson:"title"`
}

func (m EmbeddedMovies) Id() string {
	if len(m.ID) == 0 {
		m.ID, _ = HashedKey(m.Title, strconv.Itoa(m.Year))
	}

	return m.ID
}

func (m EmbeddedMovies) CollectionName() string {
	return "embedded_movies"
}

// Specify Vector Index for plot embeddings
func (m EmbeddedMovies) VectorIndexSpecs() []VectorIndexSpec {
	return []VectorIndexSpec{
		{
			Name:          "plotEmbeddingIndex",
			Path:          "plotEmbedding",
			Type:          "vector",
			NumDimensions: 1024,
			Similarity:    "cosine",
			Quantization:  "scalar",
		},
	}
}

// Specify Term Search Index for plot text
func (m EmbeddedMovies) TermSearchIndexSpecs() []TermSearchIndexSpec {
	return []TermSearchIndexSpec{
		{
			Name: "plotIndex",
			// building term index on both plot and title fields.
			// This allows searching by title as well as plot text.
			Paths: []string{"plot", "title"},
		},
	}
}

func TestEmbeddedMoviesCollection(t *testing.T) {
	dotenv.LoadEnv("../.env")
	ctx := context.Background()

	mongo, err := GetClient()
	assert.NoError(t, err, "Failed to connect to MongoDB")

	defer func() { mongo.Disconnect(ctx) }()

	tenant := "apiboot_test"
	collection := CollectionOf[EmbeddedMovies](mongo, tenant)
	assert.NotNil(t, collection, "Failed to get collection for EmbeddedMovies")

	// Create Vector Index for plot embeddings and term index for plot text.
	err = EnsureIndexes[EmbeddedMovies](ctx, mongo, tenant)
	assert.NoError(t, err, "Failed to ensure vector index for EmbeddedMovies")

	// create embedder
	embedder, err := llm.ProvideJinaAIEmbeddingClient()
	assert.NoError(t, err, "Failed to create JinaAIEmbeddingClient")

	// Save fixtures
	fixtures, err := parseTestFixture("../fixtures/odm/embedded_movies_data.tsv", embedder)
	assert.NoError(t, err, "Failed to parse test fixture")

	for _, movie := range fixtures {
		_, err := async.Await(collection.Save(ctx, movie))
		assert.NoError(t, err, "Failed to save movie from fixture")
	}

	// cleanup after vector index test
	defer func() {
		for _, movie := range fixtures {
			_, err := async.Await(collection.DeleteByID(ctx, movie.Id()))
			assert.NoError(t, err, "Failed to delete movie after vector index test")
		}
	}()

	t.Run("TestFind", func(t *testing.T) {
		// Find the movie by ID
		movieById, err := async.Await(collection.FindOneByID(ctx, fixtures[0].Id()))
		assert.NoError(t, err, "Failed to find movie by ID")
		assert.NotNil(t, movieById, "Movie should not be nil")
		assert.Equal(t, fixtures[0].Plot, movieById.Plot, "Plot should match")

		// Find movies of 1982
		movies1982, err := async.Await(collection.Find(ctx, bson.M{"year": 1982}, nil, 0, 0))
		assert.NoError(t, err, "Failed to find movies of 1982")
		assert.NotEmpty(t, movies1982, "Movies of 1982 should not be empty")
		assert.Len(t, movies1982, 2, "There should be 2 movies from 1982")
		assert.Equal(t, "The Shaolin Temple", movies1982[0].Title, "First movie should be The Shaolin Temple")
		assert.Equal(t, "Death Wish II", movies1982[1].Title, "Second movie should be Death Wish II")
	})

	t.Run("TestDistinct", func(t *testing.T) {
		// Test distinct year
		distinctYears := make([]int, 0)
		err := collection.DistinctInto(ctx, "year", nil, &distinctYears)
		assert.NoError(t, err, "Failed to get distinct years")
		assert.NotEmpty(t, distinctYears, "Distinct years should not be empty")
		assert.Len(t, distinctYears, 2, "There should be 2 distinct years")
		assert.Contains(t, distinctYears, 1981, "Distinct years should contain 1981")
		assert.Contains(t, distinctYears, 1982, "Distinct years should contain 1982")

		// Test distinct genres
		distinctGenres := make([]string, 0)
		err = collection.DistinctInto(ctx, "genres", nil, &distinctGenres)
		assert.NoError(t, err, "Failed to get distinct genres")
		assert.NotEmpty(t, distinctGenres, "Distinct genres should not be empty")
		assert.Len(t, distinctGenres, 9, "There should be 9 distinct genres")
		assert.Contains(t, distinctGenres, "Adventure", "Distinct genres should contain Adventure")
	})

	t.Run("TestVectorIndex", func(t *testing.T) {
		// Perform vector search
		query := "shaolin medieval kings war"
		embedding, err := getEmbedding(embedder, query, "retrieval.query")
		assert.NoError(t, err, "Failed to get embedding for query")
		assert.NotEmpty(t, embedding, "Embedding should not be empty")

		searchParams := VectorSearchParams{
			IndexName:     "plotEmbeddingIndex",
			Path:          "plotEmbedding",
			K:             2,
			NumCandidates: 5,
		}

		results, err := async.Await(collection.VectorSearch(ctx, embedding, searchParams))
		assert.NoError(t, err, "Failed to perform vector search")
		assert.NotEmpty(t, results, "Vector search results should not be empty")

		assert.Len(t, results, 2, "Expected 2 nearest neighbours")
		assert.Equal(t, results[0].Doc.Title, "The Shaolin Temple", "First result should be The Shaolin Temple")
		assert.Equal(t, results[1].Doc.Title, "Dragonslayer", "Second result should be The Dragonslayer")
	})

	t.Run("TestTextSearch", func(t *testing.T) {
		query := "shaolin medieval kings war"
		searchParams := TermSearchParams{
			IndexName: "plotIndex",
			Path:      []string{"plot", "title"},
			Limit:     2,
		}

		results, err := async.Await(collection.TermSearch(ctx, query, searchParams))
		assert.NoError(t, err, "Failed to perform text search")
		assert.NotEmpty(t, results, "Text search results should not be empty")

		assert.Len(t, results, 2, "Expected 2 search results")
		assert.Equal(t, results[0].Doc.Title, "The Shaolin Temple", "First result should be The Shaolin Temple")
		assert.Equal(t, results[1].Doc.Title, "Dragonslayer", "Second result should be The Dragonslayer")
	})
}

func parseTestFixture(fixturePath string, embedder *llm.JinaAIEmbeddingClient) ([]EmbeddedMovies, error) {
	fixture, err := os.Open(fixturePath)
	if err != nil {
		return nil, err
	}
	defer fixture.Close()

	reader := csv.NewReader(fixture)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1

	// Read first line - header
	_, err = reader.Read()
	if err != nil {
		return nil, err
	}

	var out []EmbeddedMovies
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if len(record) < 14 {
			continue // Skip records with insufficient fields
		}

		year, _ := strconv.Atoi(record[13])

		movie := EmbeddedMovies{
			Title:     record[3],
			Plot:      record[4],
			Year:      year,
			Genres:    collectNonEmpty(record[0:3]),
			Languages: collectNonEmpty(record[5:9]),
			Cast:      collectNonEmpty(record[9:13]),
		}

		embedding, err := getEmbedding(embedder, movie.Plot, "retrieval.passage")
		if err != nil {
			return nil, err
		}

		movie.PlotEmbedding = bson.NewVector(embedding)
		out = append(out, movie)
	}

	return out, nil
}

func collectNonEmpty(fields []string) []string {
	var result []string
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			result = append(result, field)
		}
	}
	return result
}

func getEmbedding(em *llm.JinaAIEmbeddingClient, text, task string) ([]float32, error) {
	if task == "" {
		task = "retrieval.passage" // Default task if not specified
	}

	req := llm.JinaAIEmbeddingRequest{
		Model: "jina-embeddings-v3",
		Task:  task,
		Input: []string{text},
	}

	result, err := async.Await(em.GetEmbedding(context.Background(), req))
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, errors.New("Embedding length is zero") // No embedding returned
	}

	return result, nil
}
