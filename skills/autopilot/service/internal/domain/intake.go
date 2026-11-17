package domain

// IntakeState says how far intake got with one owner message.
type IntakeState string

const (
	// IntakeClaimed: the message is taken, no issue is recorded yet.
	IntakeClaimed IntakeState = "claimed"
	// IntakeIssued: the issue exists and is recorded; the run and card are still to do.
	IntakeIssued IntakeState = "issued"
	// IntakeRefused: a repo has no usable profile; nothing more happens for this message.
	IntakeRefused IntakeState = "refused"
	// IntakeFailed: a step after the issue failed and the owner was told; nothing more happens.
	IntakeFailed IntakeState = "failed"
	// IntakeDone: the run is stored and its card is posted.
	IntakeDone IntakeState = "done"
)

// Terminal reports whether a redelivery of the message must do nothing.
func (s IntakeState) Terminal() bool {
	return s == IntakeRefused || s == IntakeFailed || s == IntakeDone
}

// IntakeIssue is the Linear issue that intake created for a message.
type IntakeIssue struct {
	ID    string
	Key   string
	Title string
	URL   string
}

// IntakeRequest is the durable record of one owner message.
type IntakeRequest struct {
	ChannelID string
	RequestTS string
	State     IntakeState
	Issue     IntakeIssue
}
