// Tests for odm Atlas integration.

package odm

import (
	"context"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/async"
	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/SaiNageswarS/go-api-boot/llm"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/blake2s"
)

type EmbeddedMovies struct {
	ID            string      `bson:"_id"`
	Plot          string      `bson:"plot"`
	Year          int         `bson:"year"`
	Cast          []string    `bson:"cast"`
	Languages     []string    `bson:"languages"`
	Directors     []string    `bson:"directors"`
	Genres        []string    `bson:"genres"`
	IMDB          IMDB        `bson:"imdb"`
	PlotEmbedding bson.Vector `bson:"plotEmbedding"`
	Title         string      `bson:"title"`
}

type IMDB struct {
	Rating float64 `bson:"rating"`
	Votes  int     `bson:"votes"`
	Id     string  `bson:"id"`
}

func (m EmbeddedMovies) Id() string {
	if len(m.ID) == 0 {
		m.ID = hash(strings.Join([]string{m.Title, strconv.Itoa(m.Year)}, ">"))
	}

	return m.ID
}

func (m EmbeddedMovies) CollectionName() string {
	return "embedded_movies"
}

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

func TestEmbeddedMoviesCollection(t *testing.T) {
	dotenv.LoadEnv("../.env")

	mongo, err := GetClient()
	assert.NoError(t, err, "Failed to connect to MongoDB")

	defer func() { mongo.Disconnect(context.Background()) }()

	tenant := "apiboot_test"
	collection := CollectionOf[EmbeddedMovies](mongo, tenant)
	assert.NotNil(t, collection, "Failed to get collection for EmbeddedMovies")

	// Create Vector Index for plot embeddings
	err = EnsureIndexes[EmbeddedMovies](context.Background(), mongo, tenant)
	assert.NoError(t, err, "Failed to ensure vector index for EmbeddedMovies")

	// create embedder
	embedder, err := llm.ProvideJinaAIEmbeddingClient()
	assert.NoError(t, err, "Failed to create JinaAIEmbeddingClient")

	t.Run("TestSaveFindDelete", func(t *testing.T) {
		ctx := context.Background()

		// Save a movie
		movie := EmbeddedMovies{
			Plot:          "A thrilling adventure",
			Year:          2023,
			Cast:          []string{"Actor A", "Actor B"},
			Languages:     []string{"English"},
			Directors:     []string{"Director X"},
			Genres:        []string{"Adventure", "Thriller"},
			IMDB:          IMDB{Rating: 8.5, Votes: 1000, Id: "tt1234567"},
			PlotEmbedding: bson.NewVector(getRandomEmbedding(1024)),
		}

		_, err := async.Await(collection.Save(ctx, movie))
		assert.NoError(t, err, "Failed to save movie")

		// Find the movie by ID
		movieById, err := async.Await(collection.FindOneByID(ctx, movie.Id()))
		assert.NoError(t, err, "Failed to find movie by ID")
		assert.NotNil(t, movieById, "Movie should not be nil")
		assert.Equal(t, movie.Plot, movieById.Plot, "Plot should match")

		// Delete the movie
		_, err = async.Await(collection.DeleteByID(ctx, movie.Id()))
		assert.NoError(t, err, "Failed to delete movie")
		// Verify deletion
		deletedMovie, err := async.Await(collection.FindOneByID(ctx, movie.Id()))
		assert.Error(t, err, "Expected error when finding deleted movie")
		assert.Nil(t, deletedMovie, "Deleted movie should be nil")
	})

	t.Run("TestDistinct", func(t *testing.T) {
		ctx := context.Background()
		// Save multiple movies for distinct test
		movies := []EmbeddedMovies{
			{
				Plot:          "A thrilling adventure",
				Year:          2023,
				Cast:          []string{"Actor A", "Actor B"},
				Languages:     []string{"English"},
				Directors:     []string{"Director X"},
				Genres:        []string{"Adventure", "Thriller"},
				IMDB:          IMDB{Rating: 8.5, Votes: 1000, Id: "tt1234567"},
				PlotEmbedding: bson.NewVector(getRandomEmbedding(1024)),
				Title:         "Adventure Movie",
			},
			{
				Plot:          "A romantic comedy",
				Year:          2022,
				Cast:          []string{"Actor C", "Actor D"},
				Languages:     []string{"English"},
				Directors:     []string{"Director Y"},
				Genres:        []string{"Romance", "Comedy"},
				IMDB:          IMDB{Rating: 7.5, Votes: 500, Id: "tt7654321"},
				PlotEmbedding: bson.NewVector(getRandomEmbedding(1024)),
				Title:         "Romantic Comedy",
			},
			{
				Plot:          "Action-packed thriller",
				Year:          2023,
				Cast:          []string{"Actor A", "Actor B"},
				Languages:     []string{"English"},
				Directors:     []string{"Director X"},
				Genres:        []string{"Action", "Thriller"},
				IMDB:          IMDB{Rating: 8.5, Votes: 1000, Id: "tt1234567"},
				PlotEmbedding: bson.NewVector(getRandomEmbedding(1024)),
				Title:         "Action Thriller",
			},
		}

		for _, m := range movies {
			_, err := async.Await(collection.Save(ctx, m))
			assert.NoError(t, err, "Failed to save movie for distinct test")
		}

		// cleanup after distinct test
		defer func() {
			for _, m := range movies {
				_, err := async.Await(collection.DeleteByID(ctx, m.Id()))
				assert.NoError(t, err, "Failed to delete movie after distinct test")
			}
		}()

		// Test distinct year
		distinctYears := make([]int, 0)
		err := collection.DistinctInto(ctx, "year", nil, &distinctYears)
		assert.NoError(t, err, "Failed to get distinct years")
		assert.NotEmpty(t, distinctYears, "Distinct years should not be empty")
		assert.Len(t, distinctYears, 2, "There should be 2 distinct years")
		assert.Contains(t, distinctYears, 2022, "Distinct years should contain 2022")

		// Test distinct genres
		distinctGenres := make([]string, 0)
		err = collection.DistinctInto(ctx, "genres", nil, &distinctGenres)
		assert.NoError(t, err, "Failed to get distinct genres")
		assert.NotEmpty(t, distinctGenres, "Distinct genres should not be empty")
		assert.Len(t, distinctGenres, 5, "There should be 5 distinct genres")
		assert.Contains(t, distinctGenres, "Adventure", "Distinct genres should contain Adventure")
	})

	t.Run("TestVectorIndex", func(t *testing.T) {
		fixtures, err := parseTestFixture("../fixtures/odm/embedded_movies_data.tsv", embedder)
		assert.NoError(t, err, "Failed to parse test fixture")
		ctx := context.Background()

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

		// Perform vector search
		query := "shaolin medieval kings war"
		embedding, err := getEmbedding(embedder, query, "retrieval.query")
		assert.NoError(t, err, "Failed to get embedding for query")
		assert.NotEmpty(t, embedding, "Embedding should not be empty")

		searchQuery := VectorQuery{
			IndexName:     "plotEmbeddingIndex",
			Path:          "plotEmbedding",
			K:             2,
			NumCandidates: 5,
		}

		results, err := async.Await(collection.VectorSearch(ctx, embedding, searchQuery))
		assert.NoError(t, err, "Failed to perform vector search")
		assert.NotEmpty(t, results, "Vector search results should not be empty")
	})
}

func hash(s string) string {
	h, _ := blake2s.New256(nil)
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:10]
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

func getRandomEmbedding(dim int) []float32 {
	embedding := make([]float32, dim)
	for i := 0; i < dim; i++ {
		embedding[i] = rand.Float32()
	}
	return embedding
}
