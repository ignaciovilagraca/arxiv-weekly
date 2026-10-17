package domain

import "time"

type Paper struct {
	Link        string
	Title       string
	Abstract    string
	Category    string
	PublishedAt time.Time
}

// Recommendation is a paper picked for the reader and the reason to read it.
type Recommendation struct {
	Paper Paper
	Why   string
}
