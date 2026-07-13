// Package pipeline is the single place that wires the trust boundary
// together. Both the CLI demo and the MCP server call Run — there is
// exactly one code path from "a request came in" to "a real action
// happened", which is what makes this auditable.
package pipeline

import (
	"context"
	"fmt"

	"github.com/ValeryCherneykin/aegis/internal/audit"
	appctx "github.com/ValeryCherneykin/aegis/internal/context"
	"github.com/ValeryCherneykin/aegis/internal/decision"
	"github.com/ValeryCherneykin/aegis/internal/executor"
	"github.com/ValeryCherneykin/aegis/internal/policy"
	"github.com/ValeryCherneykin/aegis/internal/store"
)

// Pipeline wires together the policy, store, agent, and audit logger.
type Pipeline struct {
	Policy *policy.Policy
	Store  *store.Store
	Agent  decision.Agent
	Audit  *audit.Logger
}

// Trace captures every step so callers (CLI or MCP) can render it however
// they like, without re-deriving anything.
type Trace struct {
	RequestID       string
	Categories      map[string]string
	AIDecision      decision.Decision
	FinalDecision   decision.Decision
	Overridden      bool
	ExecutionResult executor.Result
}

// Run executes the full flow for one request_id. Note the shape: raw
// facts are read from Store and immediately turned into categories;
// nothing below the Categorize() call ever sees the raw values again
// except the final Run() call into executor, which reads real data
// straight from Store — never from anything the Agent returned.
func (p *Pipeline) Run(ctx context.Context, requestID string) (Trace, error) {
	req, cust, err := p.Store.Lookup(requestID)
	if err != nil {
		return Trace{}, err
	}

	facts := appctx.Facts{
		"amount":             req.Amount,
		"account_age_years":  cust.AccountAgeYears,
		"refund_count_month": cust.RefundCountMonth,
	}
	categories := appctx.Categorize(facts, p.Policy)

	aiDecision, err := p.Agent.Decide(ctx, categories, p.Policy.AllowedActions)
	if err != nil {
		return Trace{}, fmt.Errorf("agent decide: %w", err)
	}

	final, overridden := decision.Resolve(aiDecision, categories, p.Policy)

	result := executor.Run(final, executor.Request{
		SessionID:  req.SessionID,
		CustomerID: req.CustomerID,
		Amount:     req.Amount,
		Reason:     req.Reason,
	})

	if p.Audit != nil {
		_ = p.Audit.Write(audit.Entry{
			SessionID:            req.SessionID,
			CustomerID:           req.CustomerID,
			PolicyName:           p.Policy.Name,
			PolicyVersion:        p.Policy.Version,
			InputCategories:      categories,
			AIAction:             aiDecision.Action,
			AIReason:             aiDecision.Reason,
			FinalAction:          final.Action,
			OverriddenByHardRule: overridden,
		})
	}

	return Trace{
		RequestID:       requestID,
		Categories:      categories,
		AIDecision:      aiDecision,
		FinalDecision:   final,
		Overridden:      overridden,
		ExecutionResult: result,
	}, nil
}
