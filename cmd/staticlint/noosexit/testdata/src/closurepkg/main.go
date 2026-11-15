package main

import "os"

func main() {
	func() {
		os.Exit(1)
	}()
}
