package restconf

// ContentType constants for RESTCONF media types (RFC 8040 Section 3.2).
type ContentType string

const (
	ContentTypeYANGJSON  ContentType = "application/yang-data+json"
	ContentTypeYANGXML   ContentType = "application/yang-data+xml"
	ContentTypePatchJSON ContentType = "application/yang-patch+json"
	ContentTypePatchXML  ContentType = "application/yang-patch+xml"
)

// String returns the string representation of the content type.
func (c ContentType) String() string {
	return string(c)
}

// ContentOption specifies which data to return in a GET request.
type ContentOption string

const (
	ContentAll       ContentOption = "all"
	ContentConfig    ContentOption = "config"
	ContentNonconfig ContentOption = "nonconfig"
)

// Valid returns true if the content option is valid.
func (c ContentOption) Valid() bool {
	switch c {
	case ContentAll, ContentConfig, ContentNonconfig:
		return true
	default:
		return false
	}
}

// WithDefaultsMode controls default value reporting (RFC 6243).
type WithDefaultsMode string

const (
	WithDefaultsReportAll       WithDefaultsMode = "report-all"
	WithDefaultsReportAllTagged WithDefaultsMode = "report-all-tagged"
	WithDefaultsTrim            WithDefaultsMode = "trim"
	WithDefaultsExplicit        WithDefaultsMode = "explicit"
)

// Valid returns true if the with-defaults mode is valid.
func (w WithDefaultsMode) Valid() bool {
	switch w {
	case WithDefaultsReportAll, WithDefaultsReportAllTagged,
		WithDefaultsTrim, WithDefaultsExplicit:
		return true
	default:
		return false
	}
}

// QueryOptions holds RESTCONF query parameters (RFC 8040 Section 4.8).
type QueryOptions struct {
	Depth        int              `json:"depth,omitempty"`
	Fields       string           `json:"fields,omitempty"`
	Content      ContentOption    `json:"content,omitempty"`
	WithDefaults WithDefaultsMode `json:"with_defaults,omitempty"`
	Filter       string           `json:"filter,omitempty"`
}

// Module represents a YANG module reported by a RESTCONF server.
type Module struct {
	Name      string `json:"name"`
	Revision  string `json:"revision"`
	Namespace string `json:"namespace"`
	Schema    string `json:"schema,omitempty"`
}

// DefaultPort is the standard RESTCONF HTTPS port.
const DefaultPort = 443

// DefaultRootPath is the default RESTCONF API root.
const DefaultRootPath = "/restconf"
