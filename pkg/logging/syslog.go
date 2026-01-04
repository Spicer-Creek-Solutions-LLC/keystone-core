// Package logging provides structured logging for Keystone Core.
// Epic 15: Observability Enhancements - RFC 5424 syslog support.
package logging

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Syslog severity levels (RFC 5424)
type SyslogSeverity int

const (
	SeverityEmergency SyslogSeverity = iota // System is unusable
	SeverityAlert                           // Action must be taken immediately
	SeverityCritical                        // Critical conditions
	SeverityError                           // Error conditions
	SeverityWarning                         // Warning conditions
	SeverityNotice                          // Normal but significant condition
	SeverityInfo                            // Informational messages
	SeverityDebug                           // Debug-level messages
)

// Syslog facility codes (RFC 5424)
type SyslogFacility int

const (
	FacilityKern     SyslogFacility = iota // Kernel messages
	FacilityUser                           // User-level messages
	FacilityMail                           // Mail system
	FacilityDaemon                         // System daemons
	FacilityAuth                           // Security/authorization messages
	FacilitySyslog                         // Internal syslog messages
	FacilityLPR                            // Line printer subsystem
	FacilityNews                           // Network news subsystem
	FacilityUUCP                           // UUCP subsystem
	FacilityCron                           // Clock daemon
	FacilityAuthPriv                       // Security/authorization (private)
	FacilityFTP                            // FTP daemon
	FacilityNTP                            // NTP subsystem
	FacilityAudit                          // Log audit
	FacilityAlert                          // Log alert
	FacilityClock                          // Clock daemon (note 2)
	FacilityLocal0                         // Local use 0
	FacilityLocal1                         // Local use 1
	FacilityLocal2                         // Local use 2
	FacilityLocal3                         // Local use 3
	FacilityLocal4                         // Local use 4
	FacilityLocal5                         // Local use 5
	FacilityLocal6                         // Local use 6
	FacilityLocal7                         // Local use 7
)

// ParseFacility parses a facility name string
func ParseFacility(s string) (SyslogFacility, bool) {
	switch strings.ToLower(s) {
	case "kern":
		return FacilityKern, true
	case "user":
		return FacilityUser, true
	case "mail":
		return FacilityMail, true
	case "daemon":
		return FacilityDaemon, true
	case "auth":
		return FacilityAuth, true
	case "syslog":
		return FacilitySyslog, true
	case "lpr":
		return FacilityLPR, true
	case "news":
		return FacilityNews, true
	case "uucp":
		return FacilityUUCP, true
	case "cron":
		return FacilityCron, true
	case "authpriv":
		return FacilityAuthPriv, true
	case "ftp":
		return FacilityFTP, true
	case "local0":
		return FacilityLocal0, true
	case "local1":
		return FacilityLocal1, true
	case "local2":
		return FacilityLocal2, true
	case "local3":
		return FacilityLocal3, true
	case "local4":
		return FacilityLocal4, true
	case "local5":
		return FacilityLocal5, true
	case "local6":
		return FacilityLocal6, true
	case "local7":
		return FacilityLocal7, true
	default:
		return FacilityLocal0, false
	}
}

// LevelToSeverity converts a log level to syslog severity
func LevelToSeverity(level Level) SyslogSeverity {
	switch level {
	case LevelDebug:
		return SeverityDebug
	case LevelInfo:
		return SeverityInfo
	case LevelWarn:
		return SeverityWarning
	case LevelError:
		return SeverityError
	default:
		return SeverityInfo
	}
}

// SyslogConfig contains syslog output configuration
type SyslogConfig struct {
	// Network is the network type: unix, udp, tcp, tcp+tls
	Network string

	// Address is the syslog server address (e.g., localhost:514 or /dev/log)
	Address string

	// Facility is the syslog facility
	Facility SyslogFacility

	// AppName is the application name in syslog messages
	AppName string

	// Hostname is the hostname to use (empty for auto-detect)
	Hostname string

	// TLS configuration for tcp+tls network
	TLS *SyslogTLSConfig

	// DialTimeout is the connection timeout
	DialTimeout time.Duration

	// WriteTimeout is the write timeout per message
	WriteTimeout time.Duration

	// ReconnectInterval is the interval between reconnection attempts
	ReconnectInterval time.Duration

	// MaxReconnectAttempts is the max reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int
}

// SyslogTLSConfig contains TLS configuration for syslog
type SyslogTLSConfig struct {
	// Enabled enables TLS
	Enabled bool

	// CACert is the path to CA certificate
	CACert string

	// Cert is the path to client certificate
	Cert string

	// Key is the path to client key
	Key string

	// InsecureSkipVerify skips certificate verification (not recommended)
	InsecureSkipVerify bool

	// ServerName is the expected server name for verification
	ServerName string
}

// DefaultSyslogConfig returns default syslog configuration
func DefaultSyslogConfig() *SyslogConfig {
	return &SyslogConfig{
		Network:              "unix",
		Address:              "/dev/log",
		Facility:             FacilityLocal0,
		AppName:              "kscore",
		DialTimeout:          5 * time.Second,
		WriteTimeout:         2 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0, // Unlimited
	}
}

// SyslogOutput implements Output interface for syslog
type SyslogOutput struct {
	config   *SyslogConfig
	conn     net.Conn
	hostname string
	mu       sync.Mutex
	closed   bool
}

// NewSyslogOutput creates a new syslog output
func NewSyslogOutput(config *SyslogConfig) (*SyslogOutput, error) {
	if config == nil {
		config = DefaultSyslogConfig()
	}

	// Auto-detect hostname if not set
	hostname := config.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			hostname = "unknown"
		}
	}

	output := &SyslogOutput{
		config:   config,
		hostname: hostname,
	}

	// Initial connection
	if err := output.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to syslog: %w", err)
	}

	return output, nil
}

// connect establishes connection to syslog server
func (s *SyslogOutput) connect() error {
	var conn net.Conn
	var err error

	network := s.config.Network
	address := s.config.Address

	switch network {
	case "unix":
		// Try common Unix socket paths
		if address == "" {
			for _, path := range []string{"/dev/log", "/var/run/syslog", "/var/run/log"} {
				conn, err = net.DialTimeout("unix", path, s.config.DialTimeout)
				if err == nil {
					break
				}
			}
		} else {
			conn, err = net.DialTimeout("unix", address, s.config.DialTimeout)
		}

	case "udp":
		conn, err = net.DialTimeout("udp", address, s.config.DialTimeout)

	case "tcp":
		conn, err = net.DialTimeout("tcp", address, s.config.DialTimeout)

	case "tcp+tls":
		tlsConfig, tlsErr := s.buildTLSConfig()
		if tlsErr != nil {
			return tlsErr
		}
		dialer := &net.Dialer{Timeout: s.config.DialTimeout}
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)

	default:
		return fmt.Errorf("unsupported syslog network: %s", network)
	}

	if err != nil {
		return err
	}

	s.conn = conn
	return nil
}

// buildTLSConfig builds TLS configuration
func (s *SyslogOutput) buildTLSConfig() (*tls.Config, error) {
	if s.config.TLS == nil {
		return &tls.Config{}, nil
	}

	tlsConfig := &tls.Config{
		ServerName: s.config.TLS.ServerName,
	}

	// InsecureSkipVerify - blocked by default unless KSCORE_ALLOW_INSECURE_TLS=1 is set
	if s.config.TLS.InsecureSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("syslog: insecure_skip_verify is not allowed in production (allows MITM attacks). " +
				"Set KSCORE_ALLOW_INSECURE_TLS=1 to override for development/testing only")
		}
		log.Printf("WARNING: Syslog TLS InsecureSkipVerify is enabled - this allows man-in-the-middle attacks")
		tlsConfig.InsecureSkipVerify = true
	}

	// Load CA certificate
	if s.config.TLS.CACert != "" {
		caCert, err := os.ReadFile(s.config.TLS.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate
	if s.config.TLS.Cert != "" && s.config.TLS.Key != "" {
		cert, err := tls.LoadX509KeyPair(s.config.TLS.Cert, s.config.TLS.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// Write writes a log entry to syslog
func (s *SyslogOutput) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("syslog output is closed")
	}

	// Try to write, reconnect if needed
	for attempt := 0; attempt <= s.config.MaxReconnectAttempts || s.config.MaxReconnectAttempts == 0; attempt++ {
		if s.conn == nil {
			if err := s.connect(); err != nil {
				if s.config.MaxReconnectAttempts > 0 && attempt >= s.config.MaxReconnectAttempts {
					return fmt.Errorf("failed to reconnect after %d attempts: %w", attempt, err)
				}
				time.Sleep(s.config.ReconnectInterval)
				continue
			}
		}

		// Set write deadline
		if s.config.WriteTimeout > 0 {
			s.conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
		}

		_, err := s.conn.Write(data)
		if err == nil {
			return nil
		}

		// Connection failed, close and retry
		s.conn.Close()
		s.conn = nil

		if s.config.MaxReconnectAttempts > 0 && attempt >= s.config.MaxReconnectAttempts {
			return fmt.Errorf("write failed after %d attempts: %w", attempt, err)
		}
	}

	return fmt.Errorf("failed to write to syslog")
}

// Close closes the syslog connection
func (s *SyslogOutput) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// SyslogFormatter formats log entries as RFC 5424 syslog messages
type SyslogFormatter struct {
	// Facility is the syslog facility
	Facility SyslogFacility

	// AppName is the application name
	AppName string

	// Hostname is the hostname
	Hostname string

	// IncludeStructuredData includes SD-ELEMENT in messages
	IncludeStructuredData bool

	// StructuredDataID is the SD-ID for structured data
	StructuredDataID string
}

// NewSyslogFormatter creates a new syslog formatter
func NewSyslogFormatter(facility SyslogFacility, appName string) *SyslogFormatter {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "-"
	}

	return &SyslogFormatter{
		Facility:              facility,
		AppName:               appName,
		Hostname:              hostname,
		IncludeStructuredData: true,
		StructuredDataID:      "kscore@49152",
	}
}

// Format formats a log entry as RFC 5424 syslog message
func (f *SyslogFormatter) Format(entry *Entry) ([]byte, error) {
	// Calculate priority: facility * 8 + severity
	severity := LevelToSeverity(entry.Level)
	priority := int(f.Facility)*8 + int(severity)

	// Version is always 1 for RFC 5424
	version := 1

	// Timestamp in RFC 3339 format with microseconds
	timestamp := entry.Timestamp.Format("2006-01-02T15:04:05.000000Z07:00")

	// PROCID is the process ID
	procid := fmt.Sprintf("%d", os.Getpid())

	// MSGID - use correlation ID if available, otherwise "-"
	msgid := "-"
	if entry.CorrelationID != "" {
		msgid = entry.CorrelationID
	}

	// Build structured data
	structuredData := "-"
	if f.IncludeStructuredData && (len(entry.Fields) > 0 || entry.Metadata != nil || entry.Logger != "") {
		structuredData = f.buildStructuredData(entry)
	}

	// Build the message
	// Format: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
	msg := fmt.Sprintf("<%d>%d %s %s %s %s %s %s %s\n",
		priority,
		version,
		timestamp,
		f.Hostname,
		f.AppName,
		procid,
		msgid,
		structuredData,
		entry.Message,
	)

	return []byte(msg), nil
}

// buildStructuredData builds RFC 5424 structured data
func (f *SyslogFormatter) buildStructuredData(entry *Entry) string {
	// Format: [SD-ID param1="value1" param2="value2"]
	var parts []string

	// Add logger name
	if entry.Logger != "" {
		parts = append(parts, fmt.Sprintf("logger=\"%s\"", escapeSDValue(entry.Logger)))
	}

	// Add level
	parts = append(parts, fmt.Sprintf("level=\"%s\"", entry.Level.String()))

	// Add fields
	for k, v := range entry.Fields {
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", escapeSDKey(k), escapeSDValue(fmt.Sprintf("%v", v))))
	}

	// Add metadata if present
	if entry.Metadata != nil {
		if entry.Metadata.Host != "" {
			parts = append(parts, fmt.Sprintf("host=\"%s\"", escapeSDValue(entry.Metadata.Host)))
		}
		if entry.Metadata.Service != "" {
			parts = append(parts, fmt.Sprintf("service=\"%s\"", escapeSDValue(entry.Metadata.Service)))
		}
		if entry.Metadata.Version != "" {
			parts = append(parts, fmt.Sprintf("version=\"%s\"", escapeSDValue(entry.Metadata.Version)))
		}
	}

	if len(parts) == 0 {
		return "-"
	}

	return fmt.Sprintf("[%s %s]", f.StructuredDataID, strings.Join(parts, " "))
}

// escapeSDKey escapes a structured data key (RFC 5424)
func escapeSDKey(s string) string {
	// SD-NAME must be printable ASCII, no spaces, =, ], or "
	var result strings.Builder
	for _, c := range s {
		if c >= 33 && c <= 126 && c != '=' && c != ']' && c != '"' && c != ' ' {
			result.WriteRune(c)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// escapeSDValue escapes a structured data value (RFC 5424)
func escapeSDValue(s string) string {
	// Escape backslash, double quote, and closing bracket
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "]", "\\]")
	return s
}

// BSD Syslog Format (RFC 3164) for compatibility with older systems

// BSDSyslogFormatter formats log entries as RFC 3164 BSD syslog messages
type BSDSyslogFormatter struct {
	// Facility is the syslog facility
	Facility SyslogFacility

	// Tag is the program name/tag
	Tag string
}

// NewBSDSyslogFormatter creates a new BSD syslog formatter
func NewBSDSyslogFormatter(facility SyslogFacility, tag string) *BSDSyslogFormatter {
	return &BSDSyslogFormatter{
		Facility: facility,
		Tag:      tag,
	}
}

// Format formats a log entry as RFC 3164 BSD syslog message
func (f *BSDSyslogFormatter) Format(entry *Entry) ([]byte, error) {
	// Calculate priority
	severity := LevelToSeverity(entry.Level)
	priority := int(f.Facility)*8 + int(severity)

	// Timestamp in BSD format: "Jan  2 15:04:05"
	timestamp := entry.Timestamp.Format("Jan  2 15:04:05")

	// Hostname
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// PID
	pid := os.Getpid()

	// Format: <PRI>TIMESTAMP HOSTNAME TAG[PID]: MSG
	msg := fmt.Sprintf("<%d>%s %s %s[%d]: %s\n",
		priority,
		timestamp,
		hostname,
		f.Tag,
		pid,
		entry.Message,
	)

	return []byte(msg), nil
}
