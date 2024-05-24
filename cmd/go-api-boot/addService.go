package main

import (
	"errors"
	"fmt"
	"strings"
)

func AddService(serviceName string) {
	if strings.Contains(serviceName, "Service") {
		CheckErr(errors.New("pass only root name of service"))
	}

	fmt.Printf("Adding service %sService\n", serviceName)

	appState, err := ReadAppState()
	CheckErr(err)

	data := map[string]string{
		"ServiceName": serviceName,
	}

	appState.Services = append(appState.Services, data)
	err = WriteAppState(appState)
	CheckErr(err)

	err = GenerateService(serviceName, appState.ProjectName)
	CheckErr(err)

	err = GenerateWire(appState.ProjectName, ".", appState.DbModels, appState.Services)
	CheckErr(err)
}
