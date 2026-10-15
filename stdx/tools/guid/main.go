package main

import (
	"fmt"
	"os"

	"markdown.ninja/stdx/guid"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: guid [GUID]")
		return
	}

	guid, err := guid.Parse(os.Args[1])
	if err != nil {
		fmt.Printf("error parsing guid: %s\n", err)
		os.Exit(1)
	}

	fmt.Println(guid.ToUuidString())
}
