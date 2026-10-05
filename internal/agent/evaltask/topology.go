package evaltask

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const kindTask = "task"

// Node is one row of the topology a run produced. Kind is the container
// kind (group, sequence) or "task"; State is the settled outcome.
type Node struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Children []Node `json:"children,omitempty"`
}

type runDocument struct {
	Data struct {
		Collections []runCollection `json:"collections"`
		Tasks       []runTask       `json:"tasks"`
	} `json:"data"`
}

type runCollection struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
}

type runTask struct {
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

// ParseTopology builds the node tree from an EVO_OUTPUT=json evo.run
// document. Siblings are sorted by name so concurrent completion order
// never shows up in the comparison.
func ParseTopology(document []byte) ([]Node, error) {
	var doc runDocument
	if err := json.Unmarshal(document, &doc); err != nil {
		return nil, fmt.Errorf("parse evo.run document: %w", err)
	}
	return childrenOf("", doc), nil
}

func childrenOf(parentID string, doc runDocument) []Node {
	var nodes []Node
	for _, task := range doc.Data.Tasks {
		if task.ParentID == parentID {
			nodes = append(nodes, Node{Name: task.Name, Kind: kindTask, State: task.State})
		}
	}
	for _, c := range doc.Data.Collections {
		if c.ParentID == parentID {
			nodes = append(nodes, Node{Name: c.Name, Kind: c.Kind, State: c.State, Children: childrenOf(c.ID, doc)})
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes
}

// DiffTopology returns an empty string when got equals want, otherwise a
// readable rendering of both trees.
func DiffTopology(got, want []Node) string {
	if reflect.DeepEqual(normalize(got), normalize(want)) {
		return ""
	}
	return "got:\n" + renderNodes(got) + "want:\n" + renderNodes(want)
}

func normalize(nodes []Node) []Node {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]Node, len(nodes))
	for i, n := range nodes {
		out[i] = Node{Name: n.Name, Kind: n.Kind, State: n.State, Children: normalize(n.Children)}
	}
	return out
}

func renderNodes(nodes []Node) string {
	var sb strings.Builder
	writeNodes(&sb, nodes, 0)
	return sb.String()
}

func writeNodes(sb *strings.Builder, nodes []Node, depth int) {
	for _, n := range nodes {
		fmt.Fprintf(sb, "%s%s %s [%s]\n", strings.Repeat("  ", depth), n.Kind, n.Name, n.State)
		writeNodes(sb, n.Children, depth+1)
	}
}
