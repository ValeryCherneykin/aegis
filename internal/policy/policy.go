// Package policy loads and represents Aegis decision policies: the
// versioned, human-editable rules that decide (a) how raw facts get turned
// into categories, and (b) which categories force a specific action
// regardless of what the AI decides.
package policy

import (
	"fmt"
	"os"

	"github.com/ValeryCherneykin/aegis/internal/yamlite"
)

// Tier is one rung of a category ladder, e.g. {Label: "HIGH", Gte: 500}.
type Tier struct {
	Label string
	Gte   *float64
	Lte   *float64
}

// Category defines how a single raw numeric fact is translated into a
// label. Tiers are evaluated top to bottom; the first match wins, so order
// them from strictest to loosest.
type Category struct {
	Field string
	Tiers []Tier
}

// HardRule is a deterministic override: if the computed categories match
// When, ForceAction wins no matter what the AI answered. This is what
// keeps the actual thresholds out of the model's hands.
type HardRule struct {
	When        map[string]string
	ForceAction string
	Reason      string
}

// Policy is one versioned ruleset.
type Policy struct {
	Version        string
	Name           string
	Description    string
	Categories     map[string]Category
	HardRules      []HardRule
	AllowedActions []string
}

// Load reads and parses a policy file from disk.
func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}
	raw := yamlite.Parse(string(data))
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("policy file %s is not a mapping at top level", path)
	}

	p := &Policy{
		Version:     getString(m, "version"),
		Name:        getString(m, "name"),
		Description: getString(m, "description"),
		Categories:  map[string]Category{},
	}

	if catsRaw, ok := m["categories"].(map[string]interface{}); ok {
		for name, v := range catsRaw {
			cm, ok := v.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("invalid structure for category '%s': expected a map", name)
			}
			cat := Category{Field: getString(cm, "field")}
			if tiersRaw, ok := cm["tiers"].([]interface{}); ok {
				for i, tRaw := range tiersRaw {
					tm, ok := tRaw.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("invalid tier at index %d in category '%s'", i, name)
					}
					tier := Tier{Label: getString(tm, "label")}
					if g, ok := tm["gte"]; ok {
						f := toFloat(g)
						tier.Gte = &f
					}
					if l, ok := tm["lte"]; ok {
						f := toFloat(l)
						tier.Lte = &f
					}
					cat.Tiers = append(cat.Tiers, tier)
				}
			}
			p.Categories[name] = cat
		}
	}

	if hrRaw, ok := m["hard_rules"].([]interface{}); ok {
		for _, hRaw := range hrRaw {
			hm, ok := hRaw.(map[string]interface{})
			if !ok {
				continue
			}
			hr := HardRule{
				ForceAction: getString(hm, "force_action"),
				Reason:      getString(hm, "reason"),
				When:        map[string]string{},
			}
			if whenRaw, ok := hm["when"].(map[string]interface{}); ok {
				for k, v := range whenRaw {
					hr.When[k] = fmt.Sprintf("%v", v)
				}
			}
			p.HardRules = append(p.HardRules, hr)
		}
	}

	if aiRaw, ok := m["ai"].(map[string]interface{}); ok {
		if actionsRaw, ok := aiRaw["allowed_actions"].([]interface{}); ok {
			for _, a := range actionsRaw {
				p.AllowedActions = append(p.AllowedActions, fmt.Sprintf("%v", a))
			}
		}
	}

	return p, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}
