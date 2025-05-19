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

	err := GenerateRepo(modelName)
	CheckErr(err)
}
