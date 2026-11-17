package tests

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/usecase"
)

type CommentTestSuite struct {
	statusBase
}

func TestCommentTestSuite(t *testing.T) {
	suite.Run(t, new(CommentTestSuite))
}

func (s *CommentTestSuite) SetupTest() {
	s.statusBase.SetupTest()
	s.linear.On("Comment", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"commentCreate": map[string]any{"success": true}}}
	})
}

func (s *CommentTestSuite) handler() socket.HandlerFunc {
	return socket.NewCommentHandler(usecase.NewCommenter(usecase.CommenterData{
		Linear: linear.NewClient(s.linear.URL(), "k", nil),
		Runs:   s.runs,
	}))
}

func (s *CommentTestSuite) comments() []LinearRequest {
	var out []LinearRequest
	for _, r := range s.linear.Requests() {
		if r.Operation == "Comment" {
			out = append(out, r)
		}
	}
	return out
}

func (s *CommentTestSuite) TestCommentPostsTheBodyOnTheIssueOfTheRun() {
	resp := s.handler()(s.Ctx, socket.Request{Verb: socket.VerbComment, Key: string(statusRunKey), Body: "## Report\n\nall good"})

	s.Equal(socket.CodeOK, resp.Code, resp.Error)
	got := s.comments()
	s.Require().Len(got, 1)
	s.Equal("iss-1", got[0].Variables["issueId"])
	s.Equal("## Report\n\nall good", got[0].Variables["body"])
}

func (s *CommentTestSuite) TestUnknownRunIsAClearError() {
	resp := s.handler()(s.Ctx, socket.Request{Verb: socket.VerbComment, Key: "ACM-404", Body: "x"})

	s.Equal(socket.CodeError, resp.Code)
	s.Contains(resp.Error, "ACM-404")
	s.Empty(s.comments())
}

func (s *CommentTestSuite) TestEndedRunIsAClearError() {
	s.Require().NoError(s.runs.End(s.Ctx, statusRunKey, s.Clock.Now()))

	resp := s.handler()(s.Ctx, socket.Request{Verb: socket.VerbComment, Key: string(statusRunKey), Body: "x"})

	s.Equal(socket.CodeError, resp.Code)
	s.Contains(resp.Error, "has ended")
	s.Empty(s.comments())
}

func (s *CommentTestSuite) TestEmptyBodyIsRefused() {
	resp := s.handler()(s.Ctx, socket.Request{Verb: socket.VerbComment, Key: string(statusRunKey), Body: "  \n"})

	s.Equal(socket.CodeError, resp.Code)
	s.Empty(s.comments())
}

func (s *CommentTestSuite) TestLinearFailureIsReturned() {
	s.linear.On("Comment", func(map[string]any) LinearReply { return LinearReply{Status: 500, Body: "boom"} })

	resp := s.handler()(s.Ctx, socket.Request{Verb: socket.VerbComment, Key: string(statusRunKey), Body: "x"})

	s.Equal(socket.CodeError, resp.Code)
	s.Contains(resp.Error, "cannot comment")
}
