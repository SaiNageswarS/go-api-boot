package odm

type DbModel interface {
	Id() string
	CollectionName() string
}
