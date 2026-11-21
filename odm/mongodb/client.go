package mongodb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client is a connection to MongoDB plus the ODM's settings.
type Client struct {
	mongo *mongo.Client
	timer odm.Timer
}

// Option configures a Client.
type Option func(*Client)

// WithTimer sets the clock Save stamps createdOn and updatedOn from.
func WithTimer(t odm.Timer) Option { return func(c *Client) { c.timer = t } }

// Connect connects to uri and pings the server.
func Connect(ctx context.Context, uri string, opts ...Option) (*Client, error) {
	if uri == "" {
		return nil, errors.New("odm/mongodb: Connect: empty connection uri")
	}
	mc, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("odm/mongodb: Connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := mc.Ping(pingCtx, nil); err != nil {
		_ = mc.Disconnect(context.Background())
		return nil, fmt.Errorf("odm/mongodb: Connect: %w", err)
	}
	return New(mc, opts...), nil
}

// ConnectFromEnv connects to the uri in MONGO_URI.
func ConnectFromEnv(ctx context.Context, opts ...Option) (*Client, error) {
	return Connect(ctx, os.Getenv("MONGO_URI"), opts...)
}

// New wraps an existing driver client.
func New(mc *mongo.Client, opts ...Option) *Client {
	c := &Client{mongo: mc, timer: odm.SystemTimer{}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Mongo returns the underlying driver client.
func (c *Client) Mongo() *mongo.Client { return c.mongo }

// Disconnect closes the connection.
func (c *Client) Disconnect(ctx context.Context) error { return c.mongo.Disconnect(ctx) }

func (c *Client) collection(tenant, name string) *mongo.Collection {
	return c.mongo.Database(tenant).Collection(name)
}
