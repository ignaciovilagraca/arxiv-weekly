package main

import (
	"flag"
	"log"

	"arxiv-weekly/usecases"
)

var categories = []string{"cs.AI", "cs.LG", "cs.CL"}

func main() {
	dryRun := flag.Bool("dry-run", false, "print the recommendation instead of sending it to Telegram")
	flag.Parse()

	if err := usecases.RecommendPapers(categories, *dryRun); err != nil {
		log.Fatal(err)
	}
}
