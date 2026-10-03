package arxiv

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"arxiv-weekly/domain"
)

const arxivApi = "https://export.arxiv.org/api/query"
const dateFormat = "200601021504"
const pageSize = 1000
const maxAttempts = 4

// arXiv asks for one request every three seconds.
const pause = 3 * time.Second

type feed struct {
	Total   int     `xml:"totalResults"`
	Entries []entry `xml:"entry"`
}

type entry struct {
	Id        string    `xml:"id"`
	Title     string    `xml:"title"`
	Summary   string    `xml:"summary"`
	Published time.Time `xml:"published"`
	Category  struct {
		Term string `xml:"term,attr"`
	} `xml:"primary_category"`
}

// FetchPapers returns every paper listed in any of the categories that was
// submitted in [from, to).
func FetchPapers(categories []string, from, to time.Time) ([]domain.Paper, error) {
	terms := make([]string, len(categories))
	for i, category := range categories {
		terms[i] = "cat:" + category
	}
	query := fmt.Sprintf("(%s) AND submittedDate:[%s TO %s]",
		strings.Join(terms, " OR "),
		from.UTC().Format(dateFormat),
		to.UTC().Add(-time.Minute).Format(dateFormat))

	var papers []domain.Paper
	seen := map[string]bool{}

	for start, total := 0, 1; start < total; start += pageSize {
		if start > 0 {
			time.Sleep(pause)
		}

		page, err := fetchPage(query, start)
		if err != nil {
			return nil, err
		}
		total = page.Total

		for _, e := range page.Entries {
			if seen[e.Id] {
				continue
			}
			seen[e.Id] = true
			papers = append(papers, domain.Paper{
				Link:        strings.Replace(e.Id, "http://", "https://", 1),
				Title:       strings.Join(strings.Fields(e.Title), " "),
				Abstract:    strings.Join(strings.Fields(e.Summary), " "),
				Category:    e.Category.Term,
				PublishedAt: e.Published,
			})
		}
	}

	return papers, nil
}

// fetchPage retries because the API sometimes answers a valid page with an
// error or with no entries at all.
func fetchPage(query string, start int) (*feed, error) {
	params := url.Values{}
	params.Set("search_query", query)
	params.Set("sortBy", "submittedDate")
	params.Set("sortOrder", "descending")
	params.Set("start", strconv.Itoa(start))
	params.Set("max_results", strconv.Itoa(pageSize))

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			log.Printf("arXiv: retrying page at %d (%v)", start, err)
			time.Sleep(pause * time.Duration(attempt))
		}

		var page *feed
		page, err = requestPage(arxivApi + "?" + params.Encode())
		if err != nil {
			continue
		}
		if len(page.Entries) == 0 && start < page.Total {
			err = fmt.Errorf("empty page, %d results expected", page.Total)
			continue
		}
		return page, nil
	}

	return nil, fmt.Errorf("arXiv: page at %d failed: %w", start, err)
}

func requestPage(url string) (*feed, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	page := &feed{}
	if err := xml.Unmarshal(body, page); err != nil {
		return nil, err
	}
	return page, nil
}
