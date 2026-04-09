// Package viz generates visual representations of a flow.
package viz

import (
	"fmt"
	"strings"

	"github.com/samleeney/flows/pkg/model"
)

// Mermaid generates a Mermaid flowchart diagram from a flow.
func Mermaid(flow *model.Flow) string {
	var b strings.Builder

	b.WriteString("graph LR\n")

	// Define nodes
	for _, agent := range flow.Agents {
		shape := nodeShape(agent.NodeType)
		label := agent.Name
		if agent.NodeType == model.FunctionNode {
			label += " [" + agent.Language + "]"
		}
		fmt.Fprintf(&b, "    %s%s%s%s\n", agent.Name, shape.open, label, shape.close)
	}

	b.WriteString("\n")

	// Build edges from inputs and start conditions
	edges := buildEdges(flow)
	for _, edge := range edges {
		if edge.label != "" {
			fmt.Fprintf(&b, "    %s -->|%s| %s\n", edge.from, edge.label, edge.to)
		} else {
			fmt.Fprintf(&b, "    %s --> %s\n", edge.from, edge.to)
		}
	}

	return b.String()
}

type shape struct {
	open  string
	close string
}

func nodeShape(nt model.NodeType) shape {
	switch nt {
	case model.FunctionNode:
		return shape{"[[", "]]"} // stadium shape for function nodes
	default:
		return shape{"[", "]"} // rectangle for prompt nodes
	}
}

type edge struct {
	from  string
	to    string
	label string
}

func buildEdges(flow *model.Flow) []edge {
	var edges []edge
	seen := make(map[string]bool)

	for _, agent := range flow.Agents {
		// Edges from start condition dependencies
		for _, cond := range agent.Start {
			for _, dep := range cond.When {
				key := dep + "->" + agent.Name
				label := ""
				if cond.Contains != "" {
					label = fmt.Sprintf("contains %q", cond.Contains)
				}
				if cond.MaxRuns > 0 {
					if label != "" {
						label += fmt.Sprintf(", max %d", cond.MaxRuns)
					} else {
						label = fmt.Sprintf("max %d", cond.MaxRuns)
					}
				}
				// Avoid duplicate edges with same key
				edgeKey := key + "|" + label
				if !seen[edgeKey] {
					seen[edgeKey] = true
					edges = append(edges, edge{from: dep, to: agent.Name, label: label})
				}
			}
		}

		// Edges from data inputs (only if not already covered by start conditions)
		for _, input := range agent.Inputs {
			if input.From == "external" || input.From == "" {
				continue
			}
			key := input.From + "->" + agent.Name
			if !seen[key] && !seen[key+"|"] {
				// Check if any edge from this source already exists
				hasEdge := false
				for k := range seen {
					if strings.HasPrefix(k, key) {
						hasEdge = true
						break
					}
				}
				if !hasEdge {
					seen[key] = true
					edges = append(edges, edge{from: input.From, to: agent.Name})
				}
			}
		}
	}

	return edges
}
