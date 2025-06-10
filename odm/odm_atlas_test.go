// Tests for odm Atlas integration.

package odm

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	"github.com/SaiNageswarS/go-api-boot/async"
	"github.com/SaiNageswarS/go-api-boot/dotenv"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/blake2s"
)

type EmbeddedMovies struct {
	ID            string    `bson:"_id"`
	Plot          string    `bson:"plot"`
	FullPlot      string    `bson:"fullPlot"`
	Year          int       `bson:"year"`
	Cast          []string  `bson:"cast"`
	Languages     []string  `bson:"languages"`
	Directors     []string  `bson:"directors"`
	Genres        []string  `bson:"genres"`
	IMDB          IMDB      `bson:"imdb"`
	PlotEmbedding []float64 `bson:"plotEmbedding"`
	Title         string    `bson:"title"`
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

func TestEmbeddedMoviesCollection(t *testing.T) {
	dotenv.LoadEnv("../.env")

	mongo, err := GetClient()
	assert.NoError(t, err, "Failed to connect to MongoDB")

	defer func() { mongo.Disconnect(context.Background()) }()

	collection := CollectionOf[EmbeddedMovies](mongo, "apiboot_test")
	assert.NotNil(t, collection, "Failed to get collection for EmbeddedMovies")

	t.Run("Save,Find,Delete", func(t *testing.T) {
		ctx := context.Background()

		// Save a movie
		movie := EmbeddedMovies{
			Plot:          "A thrilling adventure",
			FullPlot:      "A thrilling adventure with unexpected twists",
			Year:          2023,
			Cast:          []string{"Actor A", "Actor B"},
			Languages:     []string{"English"},
			Directors:     []string{"Director X"},
			Genres:        []string{"Adventure", "Thriller"},
			IMDB:          IMDB{Rating: 8.5, Votes: 1000, Id: "tt1234567"},
			PlotEmbedding: []float64{0.1, 0.2, 0.3},
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
				FullPlot:      "A thrilling adventure with unexpected twists",
				Year:          2023,
				Cast:          []string{"Actor A", "Actor B"},
				Languages:     []string{"English"},
				Directors:     []string{"Director X"},
				Genres:        []string{"Adventure", "Thriller"},
				IMDB:          IMDB{Rating: 8.5, Votes: 1000, Id: "tt1234567"},
				PlotEmbedding: []float64{0.1, 0.2, 0.3},
				Title:         "Adventure Movie",
			},
			{
				Plot:          "A romantic comedy",
				FullPlot:      "A romantic comedy with a twist",
				Year:          2022,
				Cast:          []string{"Actor C", "Actor D"},
				Languages:     []string{"English"},
				Directors:     []string{"Director Y"},
				Genres:        []string{"Romance", "Comedy"},
				IMDB:          IMDB{Rating: 7.5, Votes: 500, Id: "tt7654321"},
				PlotEmbedding: []float64{0.4, 0.5, 0.6},
				Title:         "Romantic Comedy",
			},
			{
				Plot:          "Action-packed thriller",
				FullPlot:      "An action-packed thriller with high stakes",
				Year:          2023,
				Cast:          []string{"Actor A", "Actor B"},
				Languages:     []string{"English"},
				Directors:     []string{"Director X"},
				Genres:        []string{"Action", "Thriller"},
				IMDB:          IMDB{Rating: 8.5, Votes: 1000, Id: "tt1234567"},
				PlotEmbedding: []float64{0.1, 0.2, 0.3},
				Title:         "Action Thriller",
			},
		}

		for _, m := range movies {
			_, err := async.Await(collection.Save(ctx, m))
			assert.NoError(t, err, "Failed to save movie for distinct test")
		}

		// Test distinct year
		distinctYears, err := async.Await(collection.Distinct(ctx, "year", nil))
		assert.NoError(t, err, "Failed to get distinct years")
		assert.NotEmpty(t, distinctYears, "Distinct years should not be empty")
		assert.Len(t, distinctYears, 2, "There should be 2 distinct years")
		assert.Contains(t, distinctYears, 2022, "Distinct years should contain 2022")

		// Test distinct genres
		distinctGenres, err := async.Await(collection.Distinct(ctx, "genres", nil))
		assert.NoError(t, err, "Failed to get distinct genres")
		assert.NotEmpty(t, distinctGenres, "Distinct genres should not be empty")
		assert.Len(t, distinctGenres, 3, "There should be 3 distinct genres")
		assert.Contains(t, distinctGenres, "Adventure", "Distinct genres should contain Adventure")
	})
}

func hash(s string) string {
	h, _ := blake2s.New256(nil)
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))[:10]
}
