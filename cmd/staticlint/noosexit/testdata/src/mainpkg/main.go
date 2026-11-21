package main

import "os"

func main() {
	os.Exit(1) // want "direct calls to os.Exit are not allowed"
}
