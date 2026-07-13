// Package executor performs the real-world side effect (or, in this demo,
// prints/writes what would happen) using the real facts that were never
// shown to the AI, plus the final action label it decided on.
package executor

import (
	"fmt"

	"github.com/ValeryCherneykin/aegis/internal/decision"
)

// Request is the real, sensitive record the executor is allowed to see.
// Nothing in this struct ever crosses into the decision.Agent.
type Request struct {
	SessionID  string
	CustomerID string
	Amount     float64
	Reason     string
}

// Result is a human-readable trace line plus the structured outcome,
// used both for the CLI demo and for what gets written to the audit log.
type Result struct {
	Summary string
	Action  string
}

// Run applies the final decision to the real request and returns what
// actually happened. ESCALATE hands the real numbers to a human queue;
// ALLOW/DENY act (or would act, in this demo) on the real balance.
func Run(final decision.Decision, req Request) Result {
	switch final.Action {
	case "ALLOW":
		return Result{
			Action: final.Action,
			Summary: fmt.Sprintf("✅ Refund executed: $%.2f to customer %s (session %s)",
				req.Amount, req.CustomerID, req.SessionID),
		}
	case "DENY":
		return Result{
			Action: final.Action,
			Summary: fmt.Sprintf("❌ Refund denied for customer %s: %s",
				req.CustomerID, final.Reason),
		}
	case "ESCALATE":
		return Result{
			Action: final.Action,
			Summary: fmt.Sprintf("🧑\u200d💼 Escalated to human reviewer — real amount $%.2f for customer %s is now visible ONLY in the manager queue, never to the AI",
				req.Amount, req.CustomerID),
		}
	default:
		return Result{Action: final.Action, Summary: fmt.Sprintf("⚠️ unrecognized action %q — refusing to execute", final.Action)}
	}
}
