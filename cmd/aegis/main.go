// Command aegis is both a runnable demo and a real MCP server.
//
//	go run .            # pretty-printed demo of the whole pipeline
//	go run . -serve     # real MCP stdio server for Claude Desktop/Code
//
// See README.md for how to plug -serve into an actual agent config.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ValeryCherneykin/aegis/internal/audit"
	"github.com/ValeryCherneykin/aegis/internal/decision"
	"github.com/ValeryCherneykin/aegis/internal/mcpserver"
	"github.com/ValeryCherneykin/aegis/internal/pipeline"
	"github.com/ValeryCherneykin/aegis/internal/policy"
	"github.com/ValeryCherneykin/aegis/internal/store"
)

func main() {
	policyPath := flag.String("policy", "policies/refund_v1.yaml", "path to policy file")
	dbPath := flag.String("db", "examples/refund/mock_db.json", "path to mock data store")
	auditPath := flag.String("audit", "audit.log", "path to audit log (JSONL)")
	requestID := flag.String("request", "req_001", "request_id to run in demo mode")
	serve := flag.Bool("serve", false, "run as a real MCP stdio server instead of the demo")
	flag.Parse()

	pol, err := policy.Load(*policyPath)
	fatalOn(err, "loading policy")

	st, err := store.Load(*dbPath)
	fatalOn(err, "loading mock data store")

	agent, usingClaude := agentFromEnv()

	pl := &pipeline.Pipeline{
		Policy: pol,
		Store:  st,
		Agent:  agent,
		Audit:  audit.New(*auditPath),
	}

	if *serve {
		fmt.Fprintf(os.Stderr, "aegis: serving MCP over stdio (policy=%s v%s, agent=%s)\n",
			pol.Name, pol.Version, agentLabel(usingClaude))
		if err := mcpserver.Serve(pl, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aegis: server error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	runDemo(pl, *requestID, usingClaude)
}

func runDemo(pl *pipeline.Pipeline, requestID string, usingClaude bool) {
	line := strings.Repeat("─", 60)
	fmt.Printf("%s\nAegis — Categorical AI Gateway — demo (%s)\n%s\n\n", line, agentLabel(usingClaude), line)

	fmt.Printf("1. Incoming request: %q\n", requestID)
	fmt.Println("2. Context engine fetches REAL facts from the store (never shown below)")

	trace, err := pl.Run(context.Background(), requestID)
	fatalOn(err, "running pipeline")

	fmt.Println("3. Real facts converted to categories:")
	for _, l := range sortedLines(trace.Categories) {
		fmt.Printf("     %s\n", l)
	}

	fmt.Printf("\n4. Agent saw ONLY the categories above and decided:\n     action=%s\n     reason=%s\n",
		trace.AIDecision.Action, trace.AIDecision.Reason)

	if trace.Overridden {
		fmt.Printf("\n5. Hard rule OVERRODE the agent: forced action=%s (%s)\n",
			trace.FinalDecision.Action, trace.FinalDecision.Reason)
	} else {
		fmt.Println("\n5. No hard rule triggered — agent's decision stands.")
	}

	fmt.Printf("\n6. Executor applied the decision to REAL data:\n     %s\n\n", trace.ExecutionResult.Summary)
	fmt.Println("Audit entry appended to audit.log (includes policy_version — see README).")
	fmt.Println(line)
}

func agentFromEnv() (decision.Agent, bool) {
	if claude, ok := decision.NewClaudeAgentFromEnv(); ok {
		return claude, true
	}
	return decision.MockAgent{}, false
}

func agentLabel(usingClaude bool) string {
	if usingClaude {
		return "real Claude via ANTHROPIC_API_KEY"
	}
	return "MockAgent (set ANTHROPIC_API_KEY to use real Claude instead)"
}

func sortedLines(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s: %s", k, m[k]))
	}
	return out
}

func fatalOn(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "aegis: %s: %v\n", what, err)
		os.Exit(1)
	}
}
