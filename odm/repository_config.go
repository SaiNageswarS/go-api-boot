package odm

import "go.mongodb.org/mongo-driver/mongo"

type Option func(*OdmSettings)

type OdmSettings struct {
	Database       string
	CollectionName string
	Client         MongoClient
}

func NewOdmSettings(options ...Option) *OdmSettings {
	odmSettings := &OdmSettings{Client: nil}
	for _, option := range options {
		option(odmSettings)
	}
	return odmSettings
}

func WithDatabase(database string) Option {
	return func(c *OdmSettings) {
		c.Database = database
	}
}

func WithCollectionName(collectionName string) Option {
	return func(c *OdmSettings) {
		c.CollectionName = collectionName
	}
}

func WithClient(client *mongo.Client) Option {
	return func(c *OdmSettings) {
		c.Client = client
	}
}
