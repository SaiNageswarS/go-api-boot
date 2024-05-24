package main

import (
	"errors"
	"fmt"
	"strings"
)

func AddRepository(modelName string) {
	if strings.Contains(modelName, "Model") || strings.Contains(modelName, "Repo") {
		CheckErr(errors.New("pass only root name of db model/repo"))
	}

	fmt.Printf("Adding repository %s Repository\n", modelName)

	appState, err := ReadAppState()
	CheckErr(err)

	data := map[string]string{
		"ModelName": modelName,
	}
	appState.DbModels = append(appState.DbModels, data)
	err = WriteAppState(appState)
	CheckErr(err)

	err = GenerateRepo(modelName)
	CheckErr(err)

	err = GenerateDbApi(".", appState.DbModels)
	CheckErr(err)

	err = GenerateWire(appState.ProjectName, ".", appState.DbModels, appState.Services)
	CheckErr(err)
}
