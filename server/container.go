package server

import (
	"fmt"
	"reflect"
)

// Dependency injection container for Go API Boot.
type container struct {
	singletons map[reflect.Type]reflect.Value
	providers  map[reflect.Type]reflect.Value // func(...) T
}

func newContainer(
	singletons map[reflect.Type]reflect.Value,
	providers map[reflect.Type]reflect.Value,
) *container {
	return &container{singletons: singletons, providers: providers}
}

func (c *container) resolve(t reflect.Type) (reflect.Value, error) {
	if v, ok := c.singletons[t]; ok {
		return v, nil
	}
	if p, ok := c.providers[t]; ok {
		args := make([]reflect.Value, p.Type().NumIn())
		for i := range args {
			v, err := c.resolve(p.Type().In(i))
			if err != nil {
				return v, err
			}
			args[i] = v
		}
		v := p.Call(args)[0]
		c.singletons[t] = v // memoise
		return v, nil
	}
	return reflect.Value{}, fmt.Errorf("no provider for %v", t)
}
