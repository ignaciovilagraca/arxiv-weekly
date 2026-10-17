package usecases

import (
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"strings"
	"syscall"
	"time"

	"arxiv-weekly/domain"
	"arxiv-weekly/frameworks/arxiv"
	"arxiv-weekly/frameworks/claude"
	"arxiv-weekly/frameworks/telegram"
)

const interestsFile = "intereses.md"
const pendingFile = "pending-message.html"
const lockFile = "prepare.lock"
const shortlistSize = 150
const picks = 5

// ErrNothingToSend means prepare left no message, which it already reported.
var ErrNothingToSend = errors.New("no recommendation is ready to send")

// PrepareRecommendation picks the papers to read from the last closed week
// and leaves the message ready for SendRecommendation. It can take hours:
// Claude answers through the Batches API.
func PrepareRecommendation(categories []string) error {
	// Cron runs prepare several times on Sunday in case arXiv is down, and
	// the Batches API can keep one run going for hours: the others wait out.
	lock, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Print("prepare: another run is in progress")
		return nil
	}

	from, to := lastClosedWeek(time.Now())

	// A message written after the week closed is this week's, from an
	// earlier run. One written before belongs to a week that is already over.
	if info, err := os.Stat(pendingFile); err == nil && info.ModTime().After(to) {
		log.Print("prepare: this week's message is already prepared")
		return nil
	}
	if err := os.Remove(pendingFile); err != nil && !os.IsNotExist(err) {
		return err
	}

	interests, err := os.ReadFile(interestsFile)
	if err != nil {
		return err
	}

	papers, err := arxiv.FetchPapers(categories, from, to)
	if err != nil {
		return err
	}
	log.Printf("arXiv: %d papers from %s to %s", len(papers), from.Format(time.DateOnly), to.Format(time.DateOnly))
	if len(papers) == 0 {
		return fmt.Errorf("arXiv returned no papers for the week")
	}

	shortlist, err := claude.Shortlist(papers, string(interests), shortlistSize)
	if err != nil {
		return err
	}
	log.Printf("shortlist: %d papers", len(shortlist))

	recommendations, err := claude.Pick(shortlist, string(interests), picks)
	if err != nil {
		return err
	}
	if len(recommendations) == 0 {
		return fmt.Errorf("no papers were picked")
	}

	message := buildMessage(recommendations, len(papers), from, to)
	fmt.Println(message)
	return os.WriteFile(pendingFile, []byte(message), 0o644)
}

// SendRecommendation sends the prepared message to Telegram.
func SendRecommendation() error {
	message, err := os.ReadFile(pendingFile)
	if os.IsNotExist(err) {
		return ErrNothingToSend
	}
	if err != nil {
		return err
	}

	if err := telegram.SendMessage(string(message)); err != nil {
		return err
	}
	return os.Remove(pendingFile)
}

// lastClosedWeek returns the latest Thursday-to-Thursday week that arXiv has
// fully announced. Submissions close at 18:00 UTC at the earliest and what
// arrives after Thursday's cutoff isn't listed until the following week, so a
// plain "last seven days" on a weekend would never see those papers.
func lastClosedWeek(now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	end := time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, time.UTC)
	for end.Weekday() != time.Thursday || now.Sub(end) < 24*time.Hour {
		end = end.AddDate(0, 0, -1)
	}
	return end.AddDate(0, 0, -7), end
}

// ReportFailure tells the chat that a step failed, since cron mails nobody.
func ReportFailure(step string, failure error) error {
	return telegram.SendMessage(fmt.Sprintf("<b>arxiv-weekly: falló %s</b>\n%s\nLog: ~/Library/Logs/arxiv-weekly.log",
		step, html.EscapeString(failure.Error())))
}

func buildMessage(recommendations []domain.Recommendation, reviewed int, from, to time.Time) string {
	var message strings.Builder
	fmt.Fprintf(&message, "<b>Papers de la semana</b> (%s al %s)\n%d revisados en arXiv\n",
		from.Format("02/01"), to.Format("02/01"), reviewed)

	for i, recommendation := range recommendations {
		fmt.Fprintf(&message, "\n%d. <b>%s</b>\n%s\n%s\n",
			i+1,
			html.EscapeString(recommendation.Paper.Title),
			html.EscapeString(recommendation.Why),
			recommendation.Paper.Link)
	}
	return message.String()
}
