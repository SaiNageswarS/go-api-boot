package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type AppState struct {
	DbModels []map[string]string `json:"dbModels"`
	Services []map[string]string `json:"services"`
}

func AddRepository(modelName string) {
	if strings.Contains(modelName, "Model") || strings.Contains(modelName, "Repo") {
		CheckErr(errors.New("pass only root name of db model/repo"))
	}

	fmt.Printf("Adding repository %s Repository\n", modelName)

	appState, err := readAppState()
	CheckErr(err)

	data := map[string]string{
		"ModelName": modelName,
	}
	appState.DbModels = append(appState.DbModels, data)
	err = writeAppState(appState)
	CheckErr(err)

	err = GenerateRepo(modelName, data)
	CheckErr(err)

	err = GenerateDbApi(".", appState.DbModels)
	CheckErr(err)

	err = GenerateWire(".", appState.DbModels)
	CheckErr(err)
}

func readAppState() (AppState, error) {
	byteValue, err := os.ReadFile("appState.json")
	if err != nil {
		return AppState{}, err
	}

	var appState AppState
	if err := json.Unmarshal(byteValue, &appState); err != nil {
		return AppState{}, err
	}

	return appState, nil
}

func writeAppState(appState AppState) error {
	jsonData, err := json.Marshal(appState)
	if err != nil {
		return err
	}

	if err := os.WriteFile("appState.json", jsonData, 0644); err != nil {
		return err
	}

	return nil
}
