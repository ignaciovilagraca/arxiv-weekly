package main

import (
	"log"
	"os"

	"arxiv-weekly/usecases"
)

var categories = []string{"cs.AI", "cs.LG", "cs.CL"}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: arxiv-weekly prepare|send")
	}

	var err error
	switch os.Args[1] {
	case "prepare":
		err = usecases.PrepareRecommendation(categories)
	case "send":
		err = usecases.SendRecommendation()
	default:
		log.Fatal("usage: arxiv-weekly prepare|send")
	}
	if err != nil {
		log.Fatal(err)
	}
}
