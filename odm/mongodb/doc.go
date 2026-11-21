// Package mongodb is go-api-boot's MongoDB ODM: typed collections queried
// with MongoDB filter documents, plus Atlas Search and Vector Search.
//
//	client, err := mongodb.Connect(ctx, os.Getenv("MONGO_URI"))
//	err = mongodb.EnsureCollection[Movie](ctx, client, tenant) // at deploy / startup
//	movies := mongodb.CollectionOf[Movie](client, tenant)
//	movie, err := async.Await(movies.FindByID(ctx, id))
//
// A tenant maps to a MongoDB database. Save stamps createdOn and updatedOn on
// every document; embed odm.Timestamps (with `bson:",inline"`) to read them.
package mongodb
