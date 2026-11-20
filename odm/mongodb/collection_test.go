// Integration tests. They need MongoDB Atlas, or the local Atlas image for
// Atlas Search and Vector Search:
//
//	docker run -d -p 57017:27017 mongodb/mongodb-atlas-local
//	MONGO_URI='mongodb://localhost:57017/?directConnection=true' go test ./odm/mongodb
//
// They are skipped when MONGO_URI is not set.

package mongodb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"github.com/SaiNageswarS/go-api-boot/odm/internal/integtest"
	"github.com/SaiNageswarS/go-collection-boot/async"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func connectTest(t *testing.T, opts ...Option) *Client {
	t.Helper()
	if os.Getenv("MONGO_URI") == "" {
		t.Skip("MONGO_URI not set")
	}
	c, err := ConnectFromEnv(t.Context(), opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Disconnect(context.Background()) })
	return c
}

// testTenant returns a unique tenant name. It stays short: Atlas caps
// database names at 38 bytes.
func testTenant(name string) string {
	return fmt.Sprintf("%s_%s", name, strconv.FormatInt(time.Now().UnixNano(), 36))
}

func TestConformance(t *testing.T) {
	clock := &integtest.Clock{}
	c := connectTest(t, WithTimer(clock))
	integtest.Run(t, integtest.Backend{
		Clock: clock,
		Collection: func(tenant string) odm.Collection[integtest.Movie] {
			return CollectionOf[integtest.Movie](c, tenant)
		},
		Ensure: func(ctx context.Context, tenant string) error {
			if err := EnsureCollection[integtest.Movie](ctx, c, tenant); err != nil {
				return err
			}
			waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			return WaitForSearchIndexes[integtest.Movie](waitCtx, c, tenant)
		},
		Drop: func(ctx context.Context, tenant string) { _ = c.Mongo().Database(tenant).Drop(ctx) },
		// Atlas Search indexes documents asynchronously.
		SearchLag: 30 * time.Second,
	}, testTenant("conformance"))
}

func TestMongoSpecifics(t *testing.T) {
	c := connectTest(t)
	ctx := t.Context()
	tenant := testTenant("specifics")
	t.Cleanup(func() { _ = c.Mongo().Database(tenant).Drop(context.Background()) })

	require.NoError(t, EnsureCollection[integtest.Movie](ctx, c, tenant))
	col := CollectionOf[integtest.Movie](c, tenant)
	for _, m := range integtest.Fixtures() {
		_, err := async.Await(col.Save(ctx, m))
		require.NoError(t, err)
	}

	t.Run("Aggregate", func(t *testing.T) {
		res, err := async.Await(col.Aggregate(ctx, mongo.Pipeline{
			{{Key: "$match", Value: bson.M{"genres": "sci-fi"}}},
			{{Key: "$sort", Value: bson.D{{Key: "year", Value: -1}}}},
			{{Key: "$limit", Value: 1}},
		}))
		require.NoError(t, err)
		require.Len(t, res, 1)
		assert.Equal(t, "m2", res[0].ID)

		var portable odm.Collection[integtest.Movie] = col
		_, ok := portable.(Collection[integtest.Movie])
		assert.True(t, ok, "reachable by type assertion")
	})

	t.Run("Raw", func(t *testing.T) {
		n, err := col.Raw().EstimatedDocumentCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(4), n)
	})

	t.Run("EnsureCollection replaces a changed index", func(t *testing.T) {
		require.NoError(t, EnsureCollection[movieV2](ctx, c, tenant))
		dup := integtest.Fixtures()[2]
		dup.ID = "dup"
		_, err := async.Await(CollectionOf[movieV2](c, tenant).Save(ctx, movieV2{dup}))
		assert.NoError(t, err, "title is no longer unique")
	})
}

type movieV2 struct{ integtest.Movie }

func (m movieV2) Indexes() []odm.IndexSpec {
	return []odm.IndexSpec{{Keys: []odm.IndexKey{odm.Key("title")}}}
}
