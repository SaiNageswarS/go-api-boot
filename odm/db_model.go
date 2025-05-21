package odm

import "github.com/jinzhu/copier"

type DbModel interface {
	Id() string
	CollectionName() string
}

func NewModelFrom[T any](proto interface{}) *T {
	model := new(T)
	_ = copier.Copy(model, proto)
	return model
}
