package domain

import "time"

type AskID int64

type AskStatus string

const (
	AskOpen      AskStatus = "open"
	AskQueued    AskStatus = "queued"
	AskAnswered  AskStatus = "answered"
	AskCancelled AskStatus = "cancelled"
)

type AnsweredBy string

const (
	AnsweredByAgterm AnsweredBy = "agterm"
	AnsweredBySlack  AnsweredBy = "slack"
)

type AskOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AskAnswer struct {
	Text string
	By   AnsweredBy
}

type Ask struct {
	ID         AskID
	IssueKey   IssueKey
	Question   string
	Options    []AskOption
	Status     AskStatus
	Answer     string
	AnsweredBy AnsweredBy
	DialogID   string
	CreatedAt  time.Time
	ClosedAt   time.Time
}
