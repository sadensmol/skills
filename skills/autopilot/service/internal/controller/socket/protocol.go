package socket

// one request and one response per connection, each a single JSON value.

// FileName is the socket file inside <workspace folder>/.workspace/autopilot/.
const FileName = "autopilot.sock"

const (
	VerbPing    = "ping"
	VerbPost    = "post"
	VerbStatus  = "status"
	VerbAsk     = "ask"
	VerbAskWait = "ask-wait"
	VerbRed     = "red"
	VerbGate    = "gate"
	VerbComment = "comment"
	VerbPR      = "pr"
	VerbManual  = "manual"
	VerbRuns    = "runs"
	VerbCancel  = "cancel"
	VerbResume  = "resume"
	VerbFix     = "merge-fix"
)

// Verbs lists every verb the service knows; a verb without a handler answers "not implemented yet".
var Verbs = []string{
	VerbPing, VerbPost, VerbStatus, VerbAsk, VerbAskWait, VerbRed, VerbGate,
	VerbComment, VerbPR, VerbManual, VerbRuns, VerbCancel, VerbResume, VerbFix,
}

// response codes; the CLI uses them as exit codes.
const (
	CodeOK      = 0
	CodeError   = 1
	CodePending = 75
)

type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Request carries the arguments of every verb; each verb reads only its own fields.
type Request struct {
	Verb    string   `json:"verb"`
	Key     string   `json:"key,omitempty"`
	Text    string   `json:"text,omitempty"`
	Phase   string   `json:"phase,omitempty"`
	ID      int64    `json:"id,omitempty"`
	Repo    string   `json:"repo,omitempty"`
	SHA     string   `json:"sha,omitempty"`
	URL     string   `json:"url,omitempty"`
	Body    string   `json:"body,omitempty"`
	Options []Option `json:"options,omitempty"`
}

type Response struct {
	Code   int    `json:"code"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
