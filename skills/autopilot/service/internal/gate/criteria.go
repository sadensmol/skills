package gate

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

const criteriaFile = "criteria.json"

// loadRunTests reads run/criteria.json and run/qa-tests.json; both must exist.
func loadRunTests(base string) (engine.Criteria, []string, error) {
	criteria, err := engine.LoadCriteria(filepath.Join(engine.RunFolder(base), criteriaFile))
	if err != nil {
		return engine.Criteria{}, nil, err
	}
	qaFiles, err := engine.LoadQATests(base)
	if err != nil {
		return engine.Criteria{}, nil, err
	}
	return criteria, qaFiles, nil
}

// manualChecks lists what a human must check at the PR, one line per manual criterion.
func manualChecks(criteria engine.Criteria) []string {
	var manual []string
	for _, cr := range criteria.Criteria {
		if cr.Manual != "" {
			manual = append(manual, fmt.Sprintf("%s: %s", cr.ID, cr.Manual))
		}
	}
	return manual
}

// criteriaCheck is check (c): the criteria table is valid and QA-owned, and every criterion test whose file is in
// this repo passed on the head. A test whose file is in no repo of the run fails, so no test escapes every gate.
// It also returns the manual checks.
func (r *gateRun) criteriaCheck(ctx context.Context, language profile.Language, results junitResults) (domain.CheckResult, []string) {
	check := domain.CheckResult{Name: CheckCriteria, Status: domain.CheckFail}
	criteria, qaFiles, err := loadRunTests(r.d.RunBase)
	if err != nil {
		check.Detail = err.Error()
		return check, nil
	}
	manual := manualChecks(criteria)
	problems := append(engine.ValidateCriteria(criteria, language), engine.QAOwnershipProblems(criteria, qaFiles)...)
	problems = append(problems, frozenTestProblems(criteria, r.d.Red.CriteriaTests)...)
	here, err := trackedFiles(ctx, r.d.Checkout, r.sha)
	if err != nil {
		return stepResult(ctx, CheckCriteria, err), manual
	}
	var elsewhere []string
	for _, other := range r.d.OtherCheckouts {
		files, err := trackedFiles(ctx, other, "HEAD")
		if err != nil {
			return stepResult(ctx, CheckCriteria, err), manual
		}
		elsewhere = append(elsewhere, files...)
	}
	checked := 0
	for _, cr := range criteria.Criteria {
		for _, t := range cr.Tests {
			if _, ok := trackedPath(here, t.File); !ok {
				if _, ok := trackedPath(elsewhere, t.File); !ok {
					problems = append(problems, fmt.Sprintf("%s: test %s file %s is in no repo of the run", cr.ID, t.ID, t.File))
				}
				continue
			}
			checked++
			o, ok := results.outcomeOf(t.JUnitName)
			switch {
			case !ok:
				problems = append(problems, fmt.Sprintf("%s: test %s %s is missing from the JUnit report", cr.ID, t.ID, t.JUnitName))
			case o != outcomePassed:
				problems = append(problems, fmt.Sprintf("%s: test %s %s %s", cr.ID, t.ID, t.JUnitName, o))
			}
		}
	}
	if len(problems) > 0 {
		check.Detail = strings.Join(problems, "; ")
		return check, manual
	}
	check.Status = domain.CheckPass
	check.Detail = fmt.Sprintf("%d criterion tests in this repo passed", checked)
	return check, manual
}

// frozenTestProblems lists the criterion tests of the red proof that run/criteria.json dropped or swapped.
func frozenTestProblems(criteria engine.Criteria, frozen []domain.CriterionTest) []string {
	var problems []string
	for _, old := range frozen {
		kept := false
		for _, cr := range criteria.Criteria {
			for _, t := range cr.Tests {
				kept = kept || sameCriterionTest(domain.CriterionTest{ID: t.ID, JUnitName: t.JUnitName, File: t.File}, old)
			}
		}
		if !kept {
			problems = append(problems, fmt.Sprintf("criterion test %s (%s in %s) of the red proof is dropped or changed in run/criteria.json", old.ID, old.JUnitName, old.File))
		}
	}
	return problems
}
