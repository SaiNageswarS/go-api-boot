package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type AppState struct {
	ProjectName string              `json:"projectName"`
	DbModels    []map[string]string `json:"dbModels"`
	Services    []map[string]string `json:"services"`
}

func CheckErr(err error) {
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func ReadAppState() (AppState, error) {
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

func WriteAppState(appState AppState) error {
	jsonData, err := json.Marshal(appState)
	if err != nil {
		return err
	}

	if err := os.WriteFile("appState.json", jsonData, 0644); err != nil {
		return err
	}

	return nil
}
