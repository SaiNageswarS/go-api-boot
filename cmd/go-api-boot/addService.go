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

	projectName, err := GetProjectName()
	CheckErr(err)

	err = GenerateService(serviceName, projectName)
	CheckErr(err)
}
