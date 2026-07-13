package decision

import "github.com/ValeryCherneykin/aegis/internal/policy"

// Resolve asks the agent for a decision, then checks it against the
// policy's hard rules. If a hard rule matches the current categories, its
// ForceAction wins no matter what the agent said — the $500 threshold
// itself always lives in this deterministic code path, never in the model.
//
// overridden is true when the agent's own answer disagreed with the hard
// rule; this is exactly the signal a compliance dashboard wants to see.
func Resolve(agentDecision Decision, categories map[string]string, p *policy.Policy) (final Decision, overridden bool) {
	for _, hr := range p.HardRules {
		if matches(categories, hr.When) {
			forced := Decision{Action: hr.ForceAction, Reason: hr.Reason}
			return forced, agentDecision.Action != hr.ForceAction
		}
	}
	return agentDecision, false
}

func matches(categories, when map[string]string) bool {
	for k, v := range when {
		if categories[k] != v {
			return false
		}
	}
	return true
}
