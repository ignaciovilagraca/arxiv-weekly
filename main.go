package main

import (
	"errors"
	"log"
	"os"

	"arxiv-weekly/usecases"
)

var categories = []string{"cs.AI", "cs.LG", "cs.CL"}

func main() {
	// --no-alert is for the runs that a later one will retry: only the last
	// failure reaches Telegram.
	alert := true
	if len(os.Args) == 3 && os.Args[1] == "prepare" && os.Args[2] == "--no-alert" {
		alert = false
	} else if len(os.Args) != 2 {
		log.Fatal("usage: arxiv-weekly prepare [--no-alert] | send")
	}

	var err error
	switch os.Args[1] {
	case "prepare":
		err = usecases.PrepareRecommendation(categories)
	case "send":
		err = usecases.SendRecommendation()
	default:
		log.Fatal("usage: arxiv-weekly prepare [--no-alert] | send")
	}
	if err != nil {
		if alert && !errors.Is(err, usecases.ErrNothingToSend) {
			if reportErr := usecases.ReportFailure(os.Args[1], err); reportErr != nil {
				log.Print(reportErr)
			}
		}
		log.Fatal(err)
	}
}
