// Package context is the trust boundary of Aegis. Real facts (money
// amounts, account age, customer history) enter here and never leave —
// only the category labels computed from them are allowed to cross into
// the AI-facing side of the system.
package context

import "github.com/ValeryCherneykin/aegis/internal/policy"

// Facts holds raw, sensitive numeric values pulled from your real systems
// (a database, a payments API, a CRM — whatever mock_db.json stands in
// for). Nothing in this struct is ever serialized to the AI.
type Facts map[string]float64

// Categorize converts Facts into category labels using the tiers defined
// in the policy. It is pure, deterministic and has no knowledge of AI at
// all — you can unit test it without a model in sight.
func Categorize(facts Facts, p *policy.Policy) map[string]string {
	result := map[string]string{}
	for name, cat := range p.Categories {
		val, ok := facts[cat.Field]
		if !ok {
			continue
		}
		for _, tier := range cat.Tiers {
			if tier.Gte != nil && val >= *tier.Gte {
				result[name] = tier.Label
				break
			}
			if tier.Lte != nil && val <= *tier.Lte {
				result[name] = tier.Label
				break
			}
		}
	}
	return result
}
