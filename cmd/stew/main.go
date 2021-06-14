package main

import (
	"log"

	"github.com/RaaLabs/steward"
)

func main() {
	stew, err := steward.NewStew()
	if err != nil {
		log.Printf("%v\n", err)
		return
	}

	err = stew.Start()
	if err != nil {
		log.Printf("%v\n", err)
	}

}
