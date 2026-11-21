package main

import operatingSystem "os"

func main() {
	operatingSystem.Exit(1) // want "direct calls to os.Exit are not allowed"
}
