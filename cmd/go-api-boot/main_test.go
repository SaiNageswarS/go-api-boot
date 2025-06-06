package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// helper to execute command strings
func execute(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	_, err := root.ExecuteC()
	return buf.String(), err
}

func TestBootstrapInvoked(t *testing.T) {
	var gotProj, gotProto string
	bootstrapFn = func(proj, proto string) error {
		gotProj, gotProto = proj, proto
		return nil
	}

	defer func() { bootstrapFn = Bootstrap }()

	_, err := execute(t, NewRoot(), "bootstrap", "myproj", "api/v1.proto")
	assert.NoError(t, err)
	assert.Equal(t, "myproj", gotProj)
	assert.Equal(t, "api/v1.proto", gotProto)
}

func TestRepositoryInvoked(t *testing.T) {
	var got string
	addRepoFn = func(m string) error {
		got = m
		return nil
	}
	defer func() { addRepoFn = AddRepository }()

	_, err := execute(t, NewRoot(), "repository", "UserModel")
	assert.NoError(t, err)
	assert.Equal(t, "UserModel", got)
}

func TestServiceInvoked(t *testing.T) {
	var got string
	addServiceFn = func(s string) error {
		got = s
		return nil
	}
	defer func() { addServiceFn = AddService }()

	_, err := execute(t, NewRoot(), "service", "Auth")
	assert.NoError(t, err)
	assert.Equal(t, "Auth", got)
}

func TestBootstrap_ArgCountError(t *testing.T) {
	// no stubs needed: we want cobra validation to fail
	_, err := execute(t, NewRoot(), "bootstrap", "only-one-arg")
	assert.Error(t, err)
}
