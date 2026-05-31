package source

import "time"

type Source struct {
	Name    string
	URL     string
	Commit  string
	AddedAt time.Time
}
