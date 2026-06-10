package main

import (
	"os"

	"camera-appliance/camera-manager/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
