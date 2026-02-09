package netconf

import (
	"strings"
)

// SubtreeFilter creates a subtree filter from an XML string.
func SubtreeFilter(xml string) *Filter {
	return &Filter{
		Type:    "subtree",
		Content: xml,
	}
}

// XPathFilter creates an XPath filter expression.
func XPathFilter(expr string) *Filter {
	return &Filter{
		Type:    "xpath",
		Content: expr,
	}
}

// PathToSubtree converts a simple slash-separated path to a nested XML subtree filter.
// For example, "interfaces/interface" becomes "<interfaces><interface/></interfaces>".
func PathToSubtree(path string) *Filter {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || (len(parts) == 1 && parts[0] == "") {
		return nil
	}

	// Build nested XML from innermost to outermost.
	inner := ""
	for i := len(parts) - 1; i >= 0; i-- {
		elem := strings.TrimSpace(parts[i])
		if elem == "" {
			continue
		}
		if inner == "" {
			inner = "<" + elem + "/>"
		} else {
			inner = "<" + elem + ">" + inner + "</" + elem + ">"
		}
	}

	return &Filter{
		Type:    "subtree",
		Content: inner,
	}
}
