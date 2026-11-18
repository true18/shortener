package main

import "os"

func main() {
	first := func() {
		second := func() {
			os.Exit(1) // want "direct calls to os.Exit are not allowed"
		}
		second()
	}
	first()
}
