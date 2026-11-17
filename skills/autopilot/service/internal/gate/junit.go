package gate

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// outcome is what one JUnit testcase reports. A failure is an assertion; an error is a test that could not run,
// such as one that does not compile or import.
type outcome string

const (
	outcomePassed  outcome = "passed"
	outcomeFailed  outcome = "failed"
	outcomeErrored outcome = "errored"
	outcomeSkipped outcome = "skipped"
)

// rank orders outcomes so that the worst of several testcases with one id wins.
var rank = map[outcome]int{outcomePassed: 0, outcomeSkipped: 1, outcomeFailed: 2, outcomeErrored: 3}

type junitCase struct {
	profile.JUnitCase
	outcome  outcome
	failures []profile.Failure
}

// xmlSuite reads both a <testsuites> and a <testsuite> root; suites may nest.
type xmlSuite struct {
	Suites []xmlSuite `xml:"testsuite"`
	Cases  []xmlCase  `xml:"testcase"`
}

type xmlCase struct {
	Classname string     `xml:"classname,attr"`
	Name      string     `xml:"name,attr"`
	File      string     `xml:"file,attr"`
	Failures  []xmlNote  `xml:"failure"`
	Errors    []xmlNote  `xml:"error"`
	Skipped   []struct{} `xml:"skipped"`
}

type xmlNote struct {
	Type    string `xml:"type,attr"`
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func (n xmlNote) toFailure() profile.Failure {
	return profile.Failure{Type: n.Type, Message: n.Message, Body: n.Body}
}

func (c xmlCase) toCase() junitCase {
	result := junitCase{JUnitCase: profile.JUnitCase{Classname: c.Classname, Name: c.Name, File: c.File}, outcome: outcomePassed}
	switch {
	case len(c.Errors) > 0:
		result.outcome = outcomeErrored
	case len(c.Failures) > 0:
		result.outcome = outcomeFailed
		for _, f := range c.Failures {
			result.failures = append(result.failures, f.toFailure())
		}
	case len(c.Skipped) > 0:
		result.outcome = outcomeSkipped
	}
	return result
}

func readJUnit(path string) ([]junitCase, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var root xmlSuite
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return root.cases(nil), nil
}

func (s xmlSuite) cases(into []junitCase) []junitCase {
	for _, c := range s.Cases {
		into = append(into, c.toCase())
	}
	for _, child := range s.Suites {
		into = child.cases(into)
	}
	return into
}

// classify turns a failure that is not an assertion, such as a panic or a thrown TypeError, into an error.
func (c junitCase) classify(p IProfile) outcome {
	if c.outcome != outcomeFailed {
		return c.outcome
	}
	for _, f := range c.failures {
		if !p.AssertionFailure(f) {
			return outcomeErrored
		}
	}
	return outcomeFailed
}

// junitResults are the outcomes of one test run by test id.
type junitResults struct {
	byID map[string]outcome
	// ids keeps the report order, for stable reasons.
	ids []string
	ran int
}

// newJUnitResults takes each testcase's outcome; a subtest whose parent errored, such as a parent whose output
// holds the subtest's panic, errored too.
func newJUnitResults(cases []junitCase, p IProfile, repoDir string) junitResults {
	type key struct{ classname, name string }
	own := map[key]outcome{}
	for _, c := range cases {
		k := key{c.Classname, c.Name}
		if o := c.classify(p); rank[o] > rank[own[k]] || own[k] == "" {
			own[k] = o
		}
	}
	r := junitResults{byID: map[string]outcome{}}
	for _, c := range cases {
		id := p.TestID(repoDir, c.JUnitCase)
		o := own[key{c.Classname, c.Name}]
		for parent, ok := p.Parent(c.JUnitCase); ok; parent, ok = p.Parent(parent) {
			if own[key{parent.Classname, parent.Name}] == outcomeErrored {
				o = outcomeErrored
			}
		}
		prev, seen := r.byID[id]
		if !seen {
			r.ids = append(r.ids, id)
		}
		if !seen || rank[o] > rank[prev] {
			r.byID[id] = o
		}
		if o != outcomeSkipped {
			r.ran++
		}
	}
	return r
}

// outcomeOf is the outcome of id; ok is false when the report does not hold it.
func (r junitResults) outcomeOf(id string) (outcome, bool) {
	o, ok := r.byID[id]
	return o, ok
}

// with lists the ids whose outcome is one of outcomes, in report order.
func (r junitResults) with(outcomes ...outcome) []string {
	var ids []string
	for _, id := range r.ids {
		for _, o := range outcomes {
			if r.byID[id] == o {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// testsPassCheck is check (a): the test command exits 0 and no testcase failed or errored.
func testsPassCheck(exitCode int, results junitResults) domain.CheckResult {
	check := domain.CheckResult{Name: CheckTestsPass, Status: domain.CheckFail}
	failed, errored := results.with(outcomeFailed), results.with(outcomeErrored)
	switch {
	case len(failed) > 0 || len(errored) > 0:
		var reasons []string
		if len(failed) > 0 {
			reasons = append(reasons, fmt.Sprintf("failed: %s", strings.Join(failed, ", ")))
		}
		if len(errored) > 0 {
			reasons = append(reasons, fmt.Sprintf("errored: %s", strings.Join(errored, ", ")))
		}
		check.Detail = strings.Join(reasons, "; ")
	case exitCode != 0:
		check.Detail = fmt.Sprintf("the test command exited with code %d", exitCode)
	default:
		check.Status = domain.CheckPass
		check.Detail = fmt.Sprintf("%d tests ran, none failed", results.ran)
	}
	return check
}

// flakyCheck is check (a) after the second test run at the head: a test that failed or errored in it, although the
// first run passed, is flaky.
func flakyCheck(passed domain.CheckResult, exitCode int, again junitResults) domain.CheckResult {
	check := domain.CheckResult{Name: CheckTestsPass, Status: domain.CheckFail}
	if ids := again.with(outcomeFailed, outcomeErrored); len(ids) > 0 {
		check.Detail = fmt.Sprintf("flaky: %s failed in one of two runs", strings.Join(ids, ", "))
		return check
	}
	if exitCode != 0 {
		check.Detail = fmt.Sprintf("flaky: the second test command exited with code %d", exitCode)
		return check
	}
	return passed
}
