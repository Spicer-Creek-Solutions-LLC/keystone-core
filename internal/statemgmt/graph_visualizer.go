package statemgmt

import (
	"fmt"
	"sort"
	"strings"
)

// GraphFormat represents the output format for graph visualization
type GraphFormat string

const (
	// GraphFormatDOT outputs in Graphviz DOT format
	GraphFormatDOT GraphFormat = "dot"
	// GraphFormatMermaid outputs in Mermaid format for documentation
	GraphFormatMermaid GraphFormat = "mermaid"
	// GraphFormatText outputs a simple text representation
	GraphFormatText GraphFormat = "text"
	// GraphFormatJSON outputs a JSON representation
	GraphFormatJSON GraphFormat = "json"
)

// GraphNode represents a node in the dependency graph
type GraphNode struct {
	ID       string `json:"id"`
	Module   string `json:"module"`
	StateID  string `json:"state_id"`
	State    string `json:"state"`
	Level    int    `json:"level"`
	InDegree int    `json:"in_degree"`
}

// GraphEdge represents an edge in the dependency graph
type GraphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	EdgeType string `json:"edge_type"` // require, watch, prereq, onchanges
}

// VisualGraph represents a complete dependency graph
type VisualGraph struct {
	Nodes      []GraphNode `json:"nodes"`
	Edges      []GraphEdge `json:"edges"`
	Levels     [][]string  `json:"levels"`
	HasCycle   bool        `json:"has_cycle"`
	CyclePath  []string    `json:"cycle_path,omitempty"`
	TotalNodes int         `json:"total_nodes"`
	TotalEdges int         `json:"total_edges"`
}

// GraphVisualizer creates visual representations of state dependencies
type GraphVisualizer struct {
	resolver *DependencyResolver
}

// NewGraphVisualizer creates a new graph visualizer
func NewGraphVisualizer() *GraphVisualizer {
	return &GraphVisualizer{
		resolver: NewDependencyResolver(),
	}
}

// BuildFromStateFile builds the dependency graph from a state file
func (v *GraphVisualizer) BuildFromStateFile(stateFile *StateFile) error {
	// Add all states to resolver
	for _, declarations := range stateFile.States {
		for i := range declarations {
			v.resolver.AddState(&declarations[i])
		}
	}

	// Build dependency graph
	return v.resolver.BuildGraph()
}

// GetGraph returns the complete dependency graph structure
func (v *GraphVisualizer) GetGraph() *VisualGraph {
	graph := &VisualGraph{
		Nodes:  make([]GraphNode, 0),
		Edges:  make([]GraphEdge, 0),
		Levels: make([][]string, 0),
	}

	// Calculate in-degrees for each node
	inDegree := make(map[string]int)
	for node := range v.resolver.graph {
		inDegree[node] = 0
	}
	for _, deps := range v.resolver.graph {
		for _, dep := range deps {
			inDegree[dep]++
		}
	}

	// Get execution order and levels
	_, err := v.resolver.TopologicalSort()
	hasCycle := err != nil
	graph.HasCycle = hasCycle

	if hasCycle {
		// Extract cycle info if available
		if strings.Contains(err.Error(), "circular dependency") {
			graph.CyclePath = extractCyclePath(err.Error())
		}
	}

	// Build node list
	levels := v.resolver.GetExecutionLevels()
	levelMap := make(map[string]int)
	for i, level := range levels {
		for _, nodeKey := range level {
			levelMap[nodeKey] = i
		}
		graph.Levels = append(graph.Levels, level)
	}

	// Add nodes
	for key, decl := range v.resolver.states {
		node := GraphNode{
			ID:       key,
			Module:   decl.Module,
			StateID:  decl.ID,
			State:    decl.State,
			Level:    levelMap[key],
			InDegree: inDegree[key],
		}
		graph.Nodes = append(graph.Nodes, node)
	}

	// Sort nodes for deterministic output
	sort.Slice(graph.Nodes, func(i, j int) bool {
		if graph.Nodes[i].Level != graph.Nodes[j].Level {
			return graph.Nodes[i].Level < graph.Nodes[j].Level
		}
		return graph.Nodes[i].ID < graph.Nodes[j].ID
	})

	// Build edges with edge types
	v.buildEdges(graph)

	graph.TotalNodes = len(graph.Nodes)
	graph.TotalEdges = len(graph.Edges)

	return graph
}

// buildEdges constructs the edge list with edge types
func (v *GraphVisualizer) buildEdges(graph *VisualGraph) {
	edgeSet := make(map[string]GraphEdge)

	for key, decl := range v.resolver.states {
		// Process require edges
		for _, ref := range decl.Requisites.Require {
			depKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := depKey + "->" + key
			edgeSet[edgeID] = GraphEdge{From: depKey, To: key, EdgeType: "require"}
		}

		// Process require_in edges
		for _, ref := range decl.Requisites.RequireIn {
			targetKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := key + "->" + targetKey
			edgeSet[edgeID] = GraphEdge{From: key, To: targetKey, EdgeType: "require_in"}
		}

		// Process watch edges
		for _, ref := range decl.Requisites.Watch {
			depKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := depKey + "->" + key
			if _, exists := edgeSet[edgeID]; !exists {
				edgeSet[edgeID] = GraphEdge{From: depKey, To: key, EdgeType: "watch"}
			}
		}

		// Process watch_in edges
		for _, ref := range decl.Requisites.WatchIn {
			targetKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := key + "->" + targetKey
			if _, exists := edgeSet[edgeID]; !exists {
				edgeSet[edgeID] = GraphEdge{From: key, To: targetKey, EdgeType: "watch_in"}
			}
		}

		// Process prereq edges
		for _, ref := range decl.Requisites.Prereq {
			depKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := depKey + "->" + key
			if _, exists := edgeSet[edgeID]; !exists {
				edgeSet[edgeID] = GraphEdge{From: depKey, To: key, EdgeType: "prereq"}
			}
		}

		// Process prereq_in edges
		for _, ref := range decl.Requisites.PrereqIn {
			targetKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := key + "->" + targetKey
			if _, exists := edgeSet[edgeID]; !exists {
				edgeSet[edgeID] = GraphEdge{From: key, To: targetKey, EdgeType: "prereq_in"}
			}
		}

		// Process onchanges edges
		for _, ref := range decl.Requisites.Onchanges {
			depKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := depKey + "->" + key
			if _, exists := edgeSet[edgeID]; !exists {
				edgeSet[edgeID] = GraphEdge{From: depKey, To: key, EdgeType: "onchanges"}
			}
		}

		// Process onchanges_in edges
		for _, ref := range decl.Requisites.OnchangesIn {
			targetKey := v.resolver.makeStateKey(ref.Module, ref.ID)
			edgeID := key + "->" + targetKey
			if _, exists := edgeSet[edgeID]; !exists {
				edgeSet[edgeID] = GraphEdge{From: key, To: targetKey, EdgeType: "onchanges_in"}
			}
		}
	}

	// Convert to slice and sort
	for _, edge := range edgeSet {
		graph.Edges = append(graph.Edges, edge)
	}

	sort.Slice(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].From != graph.Edges[j].From {
			return graph.Edges[i].From < graph.Edges[j].From
		}
		return graph.Edges[i].To < graph.Edges[j].To
	})
}

// Render renders the graph in the specified format
func (v *GraphVisualizer) Render(format GraphFormat) string {
	graph := v.GetGraph()

	switch format {
	case GraphFormatDOT:
		return v.renderDOT(graph)
	case GraphFormatMermaid:
		return v.renderMermaid(graph)
	case GraphFormatText:
		return v.renderText(graph)
	case GraphFormatJSON:
		return v.renderJSON(graph)
	default:
		return v.renderText(graph)
	}
}

// renderDOT renders in Graphviz DOT format
func (v *GraphVisualizer) renderDOT(graph *VisualGraph) string {
	var sb strings.Builder

	sb.WriteString("digraph StateDependencies {\n")
	sb.WriteString("  rankdir=TB;\n")
	sb.WriteString("  node [shape=box, style=filled];\n")
	sb.WriteString("  \n")

	// Define module colors
	moduleColors := map[string]string{
		"file":    "#a8d5ba",
		"package": "#f9c74f",
		"service": "#90be6d",
		"cmd":     "#f8961e",
		"git":     "#577590",
		"user":    "#f94144",
		"group":   "#43aa8b",
	}

	// Create subgraphs for execution levels
	for i, level := range graph.Levels {
		sb.WriteString(fmt.Sprintf("  subgraph cluster_level%d {\n", i))
		sb.WriteString(fmt.Sprintf("    label=\"Level %d\";\n", i))
		sb.WriteString("    style=dashed;\n")
		sb.WriteString("    color=gray;\n")

		for _, nodeKey := range level {
			// Find node details
			var node *GraphNode
			for _, n := range graph.Nodes {
				if n.ID == nodeKey {
					node = &n
					break
				}
			}

			if node != nil {
				color := moduleColors[node.Module]
				if color == "" {
					color = "#ffffff"
				}
				label := fmt.Sprintf("%s\\n%s", node.Module, truncateID(node.StateID, 30))
				sb.WriteString(fmt.Sprintf("    %q [label=%q, fillcolor=%q];\n",
					escapeQuotes(node.ID), label, color))
			}
		}
		sb.WriteString("  }\n")
	}

	sb.WriteString("  \n")

	// Define edge styles by type
	edgeStyles := map[string]string{
		"require":      "color=black, penwidth=2",
		"require_in":   "color=black, penwidth=2",
		"watch":        "color=blue, style=dashed",
		"watch_in":     "color=blue, style=dashed",
		"prereq":       "color=green, style=dotted",
		"prereq_in":    "color=green, style=dotted",
		"onchanges":    "color=orange, style=bold",
		"onchanges_in": "color=orange, style=bold",
	}

	// Add edges
	for _, edge := range graph.Edges {
		style := edgeStyles[edge.EdgeType]
		if style == "" {
			style = "color=black"
		}
		sb.WriteString(fmt.Sprintf("  %q -> %q [%s, label=%q];\n",
			escapeQuotes(edge.From), escapeQuotes(edge.To), style, edge.EdgeType))
	}

	// Add legend
	sb.WriteString("  \n")
	sb.WriteString("  // Legend\n")
	sb.WriteString("  subgraph cluster_legend {\n")
	sb.WriteString("    label=\"Legend\";\n")
	sb.WriteString("    style=filled;\n")
	sb.WriteString("    fillcolor=lightyellow;\n")
	sb.WriteString("    node [shape=plaintext];\n")
	sb.WriteString("    legend [label=<\n")
	sb.WriteString("      <table border=\"0\" cellborder=\"0\" cellspacing=\"1\">\n")
	sb.WriteString("        <tr><td align=\"left\"><b>Edge Types:</b></td></tr>\n")
	sb.WriteString("        <tr><td align=\"left\">━━ require (hard dependency)</td></tr>\n")
	sb.WriteString("        <tr><td align=\"left\">- - watch (trigger on change)</td></tr>\n")
	sb.WriteString("        <tr><td align=\"left\">··· prereq (soft dependency)</td></tr>\n")
	sb.WriteString("        <tr><td align=\"left\">▬▬ onchanges (conditional)</td></tr>\n")
	sb.WriteString("      </table>\n")
	sb.WriteString("    >];\n")
	sb.WriteString("  }\n")

	sb.WriteString("}\n")

	return sb.String()
}

// renderMermaid renders in Mermaid format
func (v *GraphVisualizer) renderMermaid(graph *VisualGraph) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("flowchart TD\n")

	// Create subgraphs for execution levels
	for i, level := range graph.Levels {
		sb.WriteString(fmt.Sprintf("    subgraph Level%d[\"Level %d\"]\n", i, i))

		for _, nodeKey := range level {
			// Find node details
			var node *GraphNode
			for _, n := range graph.Nodes {
				if n.ID == nodeKey {
					node = &n
					break
				}
			}

			if node != nil {
				nodeID := sanitizeMermaidID(node.ID)
				label := fmt.Sprintf("%s: %s", node.Module, truncateID(node.StateID, 25))
				sb.WriteString(fmt.Sprintf("        %s[%q]\n", nodeID, label))
			}
		}
		sb.WriteString("    end\n")
	}

	sb.WriteString("\n")

	// Add edges with styling
	for _, edge := range graph.Edges {
		fromID := sanitizeMermaidID(edge.From)
		toID := sanitizeMermaidID(edge.To)

		// Different arrow styles for edge types
		switch edge.EdgeType {
		case "require", "require_in":
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, toID))
		case "watch", "watch_in":
			sb.WriteString(fmt.Sprintf("    %s -.-> %s\n", fromID, toID))
		case "prereq", "prereq_in":
			sb.WriteString(fmt.Sprintf("    %s -.- %s\n", fromID, toID))
		case "onchanges", "onchanges_in":
			sb.WriteString(fmt.Sprintf("    %s ==> %s\n", fromID, toID))
		default:
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", fromID, toID))
		}
	}

	// Add styling
	sb.WriteString("\n")
	sb.WriteString("    %% Styling\n")

	// Group nodes by module for styling
	moduleNodes := make(map[string][]string)
	for _, node := range graph.Nodes {
		nodeID := sanitizeMermaidID(node.ID)
		moduleNodes[node.Module] = append(moduleNodes[node.Module], nodeID)
	}

	moduleStyles := map[string]string{
		"file":    "fill:#a8d5ba",
		"package": "fill:#f9c74f",
		"service": "fill:#90be6d",
		"cmd":     "fill:#f8961e",
		"git":     "fill:#577590",
		"user":    "fill:#f94144",
		"group":   "fill:#43aa8b",
	}

	for module, nodes := range moduleNodes {
		if style, ok := moduleStyles[module]; ok {
			for _, nodeID := range nodes {
				sb.WriteString(fmt.Sprintf("    style %s %s\n", nodeID, style))
			}
		}
	}

	sb.WriteString("```\n")

	return sb.String()
}

// renderText renders a simple text representation
func (v *GraphVisualizer) renderText(graph *VisualGraph) string {
	var sb strings.Builder

	sb.WriteString("State Dependency Graph\n")
	sb.WriteString("======================\n\n")

	if graph.HasCycle {
		sb.WriteString("⚠️  WARNING: Circular dependency detected!\n")
		if len(graph.CyclePath) > 0 {
			sb.WriteString("   Cycle: ")
			sb.WriteString(strings.Join(graph.CyclePath, " → "))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Total States: %d\n", graph.TotalNodes))
	sb.WriteString(fmt.Sprintf("Total Dependencies: %d\n\n", graph.TotalEdges))

	// Show execution levels
	sb.WriteString("Execution Order (by level):\n")
	sb.WriteString("---------------------------\n")

	for i, level := range graph.Levels {
		sb.WriteString(fmt.Sprintf("\nLevel %d (can execute in parallel):\n", i))
		for _, nodeKey := range level {
			// Find node details
			for _, n := range graph.Nodes {
				if n.ID == nodeKey {
					sb.WriteString(fmt.Sprintf("  • %s (%s)\n", n.ID, n.State))
					break
				}
			}
		}
	}

	// Show dependencies
	sb.WriteString("\n\nDependency Details:\n")
	sb.WriteString("-------------------\n")

	// Group edges by from node
	fromEdges := make(map[string][]GraphEdge)
	for _, edge := range graph.Edges {
		fromEdges[edge.From] = append(fromEdges[edge.From], edge)
	}

	// Get sorted keys
	sortedFroms := make([]string, 0, len(fromEdges))
	for from := range fromEdges {
		sortedFroms = append(sortedFroms, from)
	}
	sort.Strings(sortedFroms)

	for _, from := range sortedFroms {
		edges := fromEdges[from]
		sb.WriteString(fmt.Sprintf("\n%s\n", from))
		for _, edge := range edges {
			arrow := "→"
			switch edge.EdgeType {
			case "watch", "watch_in":
				arrow = "⟿"
			case "onchanges", "onchanges_in":
				arrow = "⇒"
			}
			sb.WriteString(fmt.Sprintf("  %s %s (%s)\n", arrow, edge.To, edge.EdgeType))
		}
	}

	return sb.String()
}

// renderJSON renders a JSON representation (simple format)
func (v *GraphVisualizer) renderJSON(graph *VisualGraph) string {
	var sb strings.Builder

	sb.WriteString("{\n")
	sb.WriteString(fmt.Sprintf("  \"total_nodes\": %d,\n", graph.TotalNodes))
	sb.WriteString(fmt.Sprintf("  \"total_edges\": %d,\n", graph.TotalEdges))
	sb.WriteString(fmt.Sprintf("  \"has_cycle\": %v,\n", graph.HasCycle))

	// Nodes
	sb.WriteString("  \"nodes\": [\n")
	for i, node := range graph.Nodes {
		comma := ","
		if i == len(graph.Nodes)-1 {
			comma = ""
		}
		sb.WriteString(fmt.Sprintf("    {\"id\": %q, \"module\": %q, \"state_id\": %q, \"state\": %q, \"level\": %d}%s\n",
			escapeJSON(node.ID), escapeJSON(node.Module), escapeJSON(node.StateID), escapeJSON(node.State), node.Level, comma))
	}
	sb.WriteString("  ],\n")

	// Edges
	sb.WriteString("  \"edges\": [\n")
	for i, edge := range graph.Edges {
		comma := ","
		if i == len(graph.Edges)-1 {
			comma = ""
		}
		sb.WriteString(fmt.Sprintf("    {\"from\": %q, \"to\": %q, \"type\": %q}%s\n",
			escapeJSON(edge.From), escapeJSON(edge.To), escapeJSON(edge.EdgeType), comma))
	}
	sb.WriteString("  ],\n")

	// Levels
	sb.WriteString("  \"levels\": [\n")
	for i, level := range graph.Levels {
		comma := ","
		if i == len(graph.Levels)-1 {
			comma = ""
		}
		levelStr := make([]string, len(level))
		for j, l := range level {
			levelStr[j] = fmt.Sprintf("%q", escapeJSON(l))
		}
		sb.WriteString(fmt.Sprintf("    [%s]%s\n", strings.Join(levelStr, ", "), comma))
	}
	sb.WriteString("  ]\n")

	sb.WriteString("}\n")

	return sb.String()
}

// VisualizeDependencies creates a visualization from a state file
func VisualizeDependencies(stateFile *StateFile, format GraphFormat) (string, error) {
	viz := NewGraphVisualizer()

	if err := viz.BuildFromStateFile(stateFile); err != nil {
		return "", fmt.Errorf("failed to build dependency graph: %w", err)
	}

	return viz.Render(format), nil
}

// Helper functions

func extractCyclePath(errMsg string) []string {
	// Extract cycle path from error message like "circular dependency detected: [A, B, C, A]"
	start := strings.Index(errMsg, "[")
	end := strings.Index(errMsg, "]")
	if start == -1 || end == -1 || end <= start {
		return nil
	}

	pathStr := errMsg[start+1 : end]
	parts := strings.Split(pathStr, ", ")

	// Clean up parts
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func sanitizeMermaidID(s string) string {
	// Mermaid IDs need to be alphanumeric with underscores
	result := strings.Builder{}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result.WriteRune(c)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

func truncateID(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
