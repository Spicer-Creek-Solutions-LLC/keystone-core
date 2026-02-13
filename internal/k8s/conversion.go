package k8s

// ConvertedStateFile is a package-local representation of a state file converted
// from a StateConfig CRD. It mirrors statemgmt.StateFile but avoids the import cycle
// between internal/k8s and internal/statemgmt. The actual statemgmt conversion happens
// in the wiring layer (e.g., kscore-server or a dedicated adapter package).
type ConvertedStateFile struct {
	Variables map[string]interface{}
	States    map[string][]ConvertedStateDeclaration
}

// ConvertedStateDeclaration mirrors statemgmt.StateDeclaration without importing it.
type ConvertedStateDeclaration struct {
	ID         string
	Module     string
	Parameters map[string]interface{}
	Requisites ConvertedRequisites
}

// ConvertedRequisites mirrors statemgmt.Requisites.
type ConvertedRequisites struct {
	Require     []string
	RequireIn   []string
	Watch       []string
	WatchIn     []string
	Prereq      []string
	PrereqIn    []string
	Onchanges   []string
	OnchangesIn []string
}

// StateConfigToConvertedStateFile converts a K8s StateConfig CRD to a ConvertedStateFile.
// The caller can then map ConvertedStateFile → statemgmt.StateFile without an import cycle.
func StateConfigToConvertedStateFile(sc *StateConfig) *ConvertedStateFile {
	sf := &ConvertedStateFile{
		Variables: sc.Spec.Vars,
		States:    make(map[string][]ConvertedStateDeclaration),
	}

	for _, decl := range sc.Spec.States {
		sd := ConvertedStateDeclaration{
			ID:         decl.Name,
			Module:     decl.Module,
			Parameters: decl.Parameters,
			Requisites: convertRequisites(decl.Requisites),
		}
		sf.States[decl.Module] = append(sf.States[decl.Module], sd)
	}

	return sf
}

// convertRequisites maps the CRD's flat requisite map to ConvertedRequisites.
// CRD format: map[string][]string where key is the requisite type ("require", "watch", etc.)
// and values are state names.
func convertRequisites(reqs map[string][]string) ConvertedRequisites {
	if len(reqs) == 0 {
		return ConvertedRequisites{}
	}

	result := ConvertedRequisites{}
	for typ, refs := range reqs {
		switch typ {
		case "require":
			result.Require = refs
		case "require_in":
			result.RequireIn = refs
		case "watch":
			result.Watch = refs
		case "watch_in":
			result.WatchIn = refs
		case "prereq":
			result.Prereq = refs
		case "prereq_in":
			result.PrereqIn = refs
		case "onchanges":
			result.Onchanges = refs
		case "onchanges_in":
			result.OnchangesIn = refs
		}
	}
	return result
}
