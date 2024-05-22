package main

import (
	"fmt"
	"os"
)

func CheckErr(err error) {
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
