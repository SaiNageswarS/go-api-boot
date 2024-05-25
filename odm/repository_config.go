package odm

import "go.mongodb.org/mongo-driver/mongo"

type Option func(*Config)

type Config struct {
	Database       string
	CollectionName string
	Client         *mongo.Client
}

func NewConfig(options ...Option) *Config {
	config := &Config{Client: nil}
	for _, option := range options {
		option(config)
	}
	return config
}

func WithDatabase(database string) Option {
	return func(c *Config) {
		c.Database = database
	}
}

func WithCollectionName(collectionName string) Option {
	return func(c *Config) {
		c.CollectionName = collectionName
	}
}

func WithClient(client *mongo.Client) Option {
	return func(c *Config) {
		c.Client = client
	}
}
