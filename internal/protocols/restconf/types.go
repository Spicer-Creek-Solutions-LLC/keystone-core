package restconf

// ContentType constants for RESTCONF media types (RFC 8040 Section 3.2).
type ContentType string

const (
	// ContentTypeYANGJSON is the JSON YANG data content type.
	ContentTypeYANGJSON ContentType = "application/yang-data+json"
	// ContentTypeYANGXML is the XML YANG data content type.
	ContentTypeYANGXML ContentType = "application/yang-data+xml"
	// ContentTypePatchJSON is the JSON YANG patch content type.
	ContentTypePatchJSON ContentType = "application/yang-patch+json"
	// ContentTypePatchXML is the XML YANG patch content type.
	ContentTypePatchXML ContentType = "application/yang-patch+xml"
)

// String returns the string representation of the content type.
func (c ContentType) String() string {
	return string(c)
}

// ContentOption specifies which data to return in a GET request.
type ContentOption string

const (
	// ContentAll returns all data (config and state).
	ContentAll ContentOption = "all"
	// ContentConfig returns only configuration data.
	ContentConfig ContentOption = "config"
	// ContentNonconfig returns only non-configuration (state) data.
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
	// WithDefaultsReportAll reports all default values.
	WithDefaultsReportAll WithDefaultsMode = "report-all"
	// WithDefaultsReportAllTagged reports all with tagged defaults.
	WithDefaultsReportAllTagged WithDefaultsMode = "report-all-tagged"
	// WithDefaultsTrim omits default values from the response.
	WithDefaultsTrim WithDefaultsMode = "trim"
	// WithDefaultsExplicit only reports explicitly set values.
	WithDefaultsExplicit WithDefaultsMode = "explicit"
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
