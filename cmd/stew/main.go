package main

import (
	"log"

	steward "github.com/RaaLabs/steward/stew"
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
