// Package wizard provides an interactive TUI for trust federation setup.
package wizard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/shawnbutts/keystone-core/internal/identity/federation"
)

// Step represents a step in the wizard.
type Step int

// StepTrustDomain and related constants.
const (
	StepTrustDomain Step = iota
	StepEndpointChoice
	StepEndpointDiscovery
	StepEndpointManual
	StepFederationType
	StepPolicyTemplate
	StepPolicyCustomAllowed
	StepPolicyCustomDenied
	StepRefreshInterval
	StepRequireMTLS
	StepPolicyTest
	StepConfirm
	StepExecuting
	StepDone
)

// Config holds the configuration collected by the wizard.
type Config struct {
	// TrustDomain is the partner trust domain.
	TrustDomain string

	// BundleEndpoint is the URL to fetch the trust bundle.
	BundleEndpoint string

	// EndpointProfile is the bundle endpoint profile.
	EndpointProfile string

	// FederationType is the type of federation.
	FederationType federation.Type

	// Policy is the trust policy configuration.
	Policy *federation.TrustPolicy

	// PolicyTemplateName is the name of the selected policy template.
	PolicyTemplateName string

	// RefreshInterval is how often to refresh the trust bundle.
	RefreshInterval time.Duration

	// RequireMTLS requires mutual TLS for connections.
	RequireMTLS bool

	// DiscoveryResult holds the endpoint discovery results.
	DiscoveryResult *EndpointDiscoveryResult
}

// Result contains the result of running the wizard.
type Result struct {
	// Config is the collected configuration.
	Config *Config

	// Domain is the constructed FederatedDomain ready for use.
	Domain *federation.FederatedDomain

	// Cancelled indicates if the wizard was cancelled.
	Cancelled bool

	// Error contains any error that occurred.
	Error error
}

// listItem is a generic list item adapter.
type listItem struct {
	title       string
	description string
	value       string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.description }
func (i listItem) FilterValue() string { return i.title }

// Model is the BubbleTea model for the federation wizard.
type Model struct {
	width  int
	height int
	step   Step

	// Input components
	trustDomainInput     textinput.Model
	endpointInput        textinput.Model
	refreshIntervalInput textinput.Model
	allowedPathsInput    textinput.Model
	deniedPathsInput     textinput.Model
	testSpiffeIDInput    textinput.Model

	// List components
	endpointChoiceList list.Model
	federationTypeList list.Model
	policyTemplateList list.Model
	mtlsList           list.Model

	// Spinner for async operations
	spinner spinner.Model

	// State
	config            *Config
	discoveryResult   *EndpointDiscoveryResult
	policyTestResults []PolicyTestResult
	isDiscovering     bool
	done              bool
	cancelled         bool
	err               error
}

// New creates a new wizard model.
func New() *Model {
	// Trust domain input
	trustDomainInput := textinput.New()
	trustDomainInput.Placeholder = "partner.example.org"
	trustDomainInput.Prompt = "Trust domain: "
	trustDomainInput.CharLimit = 253

	// Endpoint input
	endpointInput := textinput.New()
	endpointInput.Placeholder = "https://partner.example.org/.well-known/spiffe-bundle"
	endpointInput.Prompt = "Endpoint URL: "
	endpointInput.CharLimit = 512

	// Refresh interval input
	refreshIntervalInput := textinput.New()
	refreshIntervalInput.Placeholder = "5m"
	refreshIntervalInput.Prompt = "Refresh interval: "
	refreshIntervalInput.CharLimit = 20

	// Allowed paths input
	allowedPathsInput := textinput.New()
	allowedPathsInput.Placeholder = "/service/**, /agent/**"
	allowedPathsInput.Prompt = "Allowed paths: "
	allowedPathsInput.CharLimit = 512

	// Denied paths input
	deniedPathsInput := textinput.New()
	deniedPathsInput.Placeholder = "/admin/**, /internal/**"
	deniedPathsInput.Prompt = "Denied paths: "
	deniedPathsInput.CharLimit = 512

	// Test SPIFFE ID input
	testSpiffeIDInput := textinput.New()
	testSpiffeIDInput.Placeholder = "spiffe://partner.example.org/service/api"
	testSpiffeIDInput.Prompt = "> "
	testSpiffeIDInput.CharLimit = 512

	// Endpoint choice list
	endpointChoiceItems := []list.Item{
		listItem{
			title:       "Auto-discover",
			description: "Try well-known URLs to find the bundle endpoint",
			value:       "auto",
		},
		listItem{
			title:       "Enter manually",
			description: "Provide the bundle endpoint URL directly",
			value:       "manual",
		},
	}
	endpointChoiceList := list.New(endpointChoiceItems, list.NewDefaultDelegate(), 0, 0)
	endpointChoiceList.Title = "How would you like to configure the bundle endpoint?"
	endpointChoiceList.SetShowHelp(true)
	endpointChoiceList.DisableQuitKeybindings()

	// Federation type list
	fedTypeItems := []list.Item{
		listItem{
			title:       "Bidirectional (recommended)",
			description: "Both domains trust each other's identities",
			value:       string(federation.TypeBidirectional),
		},
		listItem{
			title:       "Unidirectional",
			description: "Only we trust them (they don't need to trust us)",
			value:       string(federation.TypeUnidirectional),
		},
	}
	federationTypeList := list.New(fedTypeItems, list.NewDefaultDelegate(), 0, 0)
	federationTypeList.Title = "Select federation type"
	federationTypeList.SetShowHelp(true)
	federationTypeList.DisableQuitKeybindings()

	// Policy template list
	policyItems := make([]list.Item, 0, len(PolicyTemplates))
	for _, tmpl := range PolicyTemplates {
		title := tmpl.DisplayName
		if tmpl.Recommended {
			title += " (recommended)"
		}
		policyItems = append(policyItems, listItem{
			title:       title,
			description: tmpl.Description,
			value:       tmpl.Name,
		})
	}
	policyTemplateList := list.New(policyItems, list.NewDefaultDelegate(), 0, 0)
	policyTemplateList.Title = "Select a policy template"
	policyTemplateList.SetShowHelp(true)
	policyTemplateList.DisableQuitKeybindings()

	// mTLS list
	mtlsItems := []list.Item{
		listItem{
			title:       "Yes (recommended)",
			description: "Require mutual TLS for all connections",
			value:       "yes",
		},
		listItem{
			title:       "No",
			description: "Do not require mutual TLS",
			value:       "no",
		},
	}
	mtlsList := list.New(mtlsItems, list.NewDefaultDelegate(), 0, 0)
	mtlsList.Title = "Require mutual TLS?"
	mtlsList.SetShowHelp(true)
	mtlsList.DisableQuitKeybindings()

	// Spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return &Model{
		step:                 StepTrustDomain,
		trustDomainInput:     trustDomainInput,
		endpointInput:        endpointInput,
		refreshIntervalInput: refreshIntervalInput,
		allowedPathsInput:    allowedPathsInput,
		deniedPathsInput:     deniedPathsInput,
		testSpiffeIDInput:    testSpiffeIDInput,
		endpointChoiceList:   endpointChoiceList,
		federationTypeList:   federationTypeList,
		policyTemplateList:   policyTemplateList,
		mtlsList:             mtlsList,
		spinner:              s,
		config: &Config{
			RefreshInterval: 5 * time.Minute,
			FederationType:  federation.TypeBidirectional,
		},
		policyTestResults: make([]PolicyTestResult, 0),
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

// discoveryMsg is sent when endpoint discovery completes.
type discoveryMsg struct {
	result *EndpointDiscoveryResult
	err    error
}

// Update handles messages and updates the model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listHeight := msg.Height - 6
		m.endpointChoiceList.SetSize(msg.Width, listHeight)
		m.federationTypeList.SetSize(msg.Width, listHeight)
		m.policyTemplateList.SetSize(msg.Width, listHeight)
		m.mtlsList.SetSize(msg.Width, listHeight)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		case "esc":
			if m.step == StepTrustDomain {
				m.cancelled = true
				m.done = true
				return m, tea.Quit
			}
			return m.back()
		case "enter":
			if m.step == StepPolicyTest {
				// Handle test input
				return m.handlePolicyTest()
			}
			return m.advance()
		case "backspace":
			if m.step == StepPolicyTest && m.testSpiffeIDInput.Value() == "" {
				return m.back()
			}
		}

	case discoveryMsg:
		m.isDiscovering = false
		if msg.err != nil {
			m.err = msg.err
			// Fall back to manual entry
			m.step = StepEndpointManual
			m.endpointInput.Focus()
			return m, nil
		}
		m.discoveryResult = msg.result
		m.config.DiscoveryResult = msg.result
		if msg.result.BestEndpoint != nil {
			m.config.BundleEndpoint = msg.result.BestEndpoint.URL
			m.config.EndpointProfile = msg.result.BestEndpoint.Profile
		}
		m.step = StepEndpointDiscovery
		return m, nil

	case spinner.TickMsg:
		if m.isDiscovering {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	// Update current component
	return m.updateCurrentStep(msg)
}

func (m *Model) updateCurrentStep(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch m.step {
	case StepTrustDomain:
		m.trustDomainInput, cmd = m.trustDomainInput.Update(msg)
	case StepEndpointChoice:
		m.endpointChoiceList, cmd = m.endpointChoiceList.Update(msg)
	case StepEndpointManual:
		m.endpointInput, cmd = m.endpointInput.Update(msg)
	case StepFederationType:
		m.federationTypeList, cmd = m.federationTypeList.Update(msg)
	case StepPolicyTemplate:
		m.policyTemplateList, cmd = m.policyTemplateList.Update(msg)
	case StepPolicyCustomAllowed:
		m.allowedPathsInput, cmd = m.allowedPathsInput.Update(msg)
	case StepPolicyCustomDenied:
		m.deniedPathsInput, cmd = m.deniedPathsInput.Update(msg)
	case StepRefreshInterval:
		m.refreshIntervalInput, cmd = m.refreshIntervalInput.Update(msg)
	case StepRequireMTLS:
		m.mtlsList, cmd = m.mtlsList.Update(msg)
	case StepPolicyTest:
		m.testSpiffeIDInput, cmd = m.testSpiffeIDInput.Update(msg)
	default:
		// StepEndpointDiscovery, StepConfirm, StepExecuting, StepDone - no input components
	}

	return m, cmd
}

func (m *Model) advance() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepTrustDomain:
		domain := strings.TrimSpace(m.trustDomainInput.Value())
		if err := ValidateTrustDomain(domain); err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		m.config.TrustDomain = domain
		m.step = StepEndpointChoice
		return m, nil

	case StepEndpointChoice:
		if item, ok := m.endpointChoiceList.SelectedItem().(listItem); ok {
			if item.value == "auto" {
				m.isDiscovering = true
				m.step = StepEndpointDiscovery
				return m, tea.Batch(m.spinner.Tick, m.startDiscovery())
			}
			m.step = StepEndpointManual
			m.endpointInput.Focus()
		}
		return m, nil

	case StepEndpointDiscovery:
		// User accepted discovered endpoint
		m.step = StepFederationType
		return m, nil

	case StepEndpointManual:
		url := strings.TrimSpace(m.endpointInput.Value())
		if err := ValidateEndpointURL(url); err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		m.config.BundleEndpoint = url
		m.config.EndpointProfile = "https_web"
		m.step = StepFederationType
		return m, nil

	case StepFederationType:
		if item, ok := m.federationTypeList.SelectedItem().(listItem); ok {
			m.config.FederationType = federation.Type(item.value)
		}
		m.step = StepPolicyTemplate
		return m, nil

	case StepPolicyTemplate:
		if item, ok := m.policyTemplateList.SelectedItem().(listItem); ok {
			m.config.PolicyTemplateName = item.value
			tmpl := GetPolicyTemplate(item.value)
			if tmpl != nil && tmpl.Policy != nil {
				m.config.Policy = tmpl.Policy
				m.step = StepRefreshInterval
				m.refreshIntervalInput.Focus()
			} else {
				// Custom policy
				m.step = StepPolicyCustomAllowed
				m.allowedPathsInput.Focus()
			}
		}
		return m, nil

	case StepPolicyCustomAllowed:
		paths := splitPaths(m.allowedPathsInput.Value())
		if errs := ValidatePolicyPaths(paths); len(errs) > 0 {
			m.err = errors.New(strings.Join(errs, "; "))
			return m, nil
		}
		m.err = nil
		// Store temporarily
		m.config.Policy = &federation.TrustPolicy{
			AllowedPaths: paths,
		}
		m.step = StepPolicyCustomDenied
		m.deniedPathsInput.Focus()
		return m, nil

	case StepPolicyCustomDenied:
		paths := splitPaths(m.deniedPathsInput.Value())
		if len(paths) > 0 {
			if errs := ValidatePolicyPaths(paths); len(errs) > 0 {
				m.err = errors.New(strings.Join(errs, "; "))
				return m, nil
			}
		}
		m.err = nil
		m.config.Policy.DeniedPaths = paths
		m.config.Policy.Name = "custom"
		m.config.Policy.Description = "Custom federation policy"
		m.step = StepRefreshInterval
		m.refreshIntervalInput.Focus()
		return m, nil

	case StepRefreshInterval:
		value := strings.TrimSpace(m.refreshIntervalInput.Value())
		if value == "" {
			value = "5m"
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			m.err = fmt.Errorf("invalid duration: %w", err)
			return m, nil
		}
		m.err = nil
		m.config.RefreshInterval = duration
		m.step = StepRequireMTLS
		return m, nil

	case StepRequireMTLS:
		if item, ok := m.mtlsList.SelectedItem().(listItem); ok {
			m.config.RequireMTLS = item.value == "yes"
			if m.config.Policy != nil {
				m.config.Policy.RequireMTLS = m.config.RequireMTLS
			}
		}
		m.step = StepConfirm
		return m, nil

	case StepConfirm:
		m.step = StepDone
		m.done = true
		return m, tea.Quit

	default:
		return m, nil
	}
}

func (m *Model) back() (tea.Model, tea.Cmd) {
	m.err = nil

	switch m.step {
	case StepEndpointChoice:
		m.step = StepTrustDomain
		m.trustDomainInput.Focus()
	case StepEndpointDiscovery, StepEndpointManual:
		m.step = StepEndpointChoice
	case StepFederationType:
		if m.discoveryResult != nil {
			m.step = StepEndpointDiscovery
		} else {
			m.step = StepEndpointManual
			m.endpointInput.Focus()
		}
	case StepPolicyTemplate:
		m.step = StepFederationType
	case StepPolicyCustomAllowed:
		m.step = StepPolicyTemplate
	case StepPolicyCustomDenied:
		m.step = StepPolicyCustomAllowed
		m.allowedPathsInput.Focus()
	case StepRefreshInterval:
		if m.config.PolicyTemplateName == "custom" {
			m.step = StepPolicyCustomDenied
			m.deniedPathsInput.Focus()
		} else {
			m.step = StepPolicyTemplate
		}
	case StepRequireMTLS:
		m.step = StepRefreshInterval
		m.refreshIntervalInput.Focus()
	case StepPolicyTest:
		m.step = StepConfirm
		m.policyTestResults = nil
	case StepConfirm:
		m.step = StepRequireMTLS
	default:
		// StepTrustDomain, StepExecuting, StepDone - no back navigation
	}

	return m, nil
}

func (m *Model) startDiscovery() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := DiscoverBundleEndpoint(ctx, m.config.TrustDomain, nil)
		return discoveryMsg{result: result, err: err}
	}
}

func (m *Model) handlePolicyTest() (tea.Model, tea.Cmd) {
	spiffeID := strings.TrimSpace(m.testSpiffeIDInput.Value())
	if spiffeID == "" {
		// Empty input returns to confirm
		m.step = StepConfirm
		return m, nil
	}

	// Test the policy
	result := TestPolicy(m.config.Policy, spiffeID)
	m.policyTestResults = append(m.policyTestResults, result)
	m.testSpiffeIDInput.SetValue("")
	return m, nil
}

// View renders the wizard UI.
func (m *Model) View() string {
	if m.done {
		return ""
	}

	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("SPIFFE Trust Federation Setup Wizard"))
	b.WriteString("\n\n")

	switch m.step {
	case StepTrustDomain:
		b.WriteString(m.viewTrustDomain())
	case StepEndpointChoice:
		b.WriteString(m.endpointChoiceList.View())
	case StepEndpointDiscovery:
		b.WriteString(m.viewEndpointDiscovery())
	case StepEndpointManual:
		b.WriteString(m.viewEndpointManual())
	case StepFederationType:
		b.WriteString(m.federationTypeList.View())
	case StepPolicyTemplate:
		b.WriteString(m.policyTemplateList.View())
	case StepPolicyCustomAllowed:
		b.WriteString(m.viewPolicyCustomAllowed())
	case StepPolicyCustomDenied:
		b.WriteString(m.viewPolicyCustomDenied())
	case StepRefreshInterval:
		b.WriteString(m.viewRefreshInterval())
	case StepRequireMTLS:
		b.WriteString(m.mtlsList.View())
	case StepPolicyTest:
		b.WriteString(m.viewPolicyTest())
	case StepConfirm:
		b.WriteString(m.viewConfirm())
	default:
		// StepExecuting, StepDone - no view content
	}

	// Error message
	if m.err != nil {
		b.WriteString("\n\n")
		b.WriteString(formatError(m.err.Error()))
	}

	// Help
	b.WriteString("\n\n")
	b.WriteString(formatHelp("[enter] continue  [esc] back  [ctrl+c] quit"))

	return b.String()
}

func (m *Model) viewTrustDomain() string {
	var b strings.Builder
	b.WriteString(formatHeader("Partner Trust Domain", 1, 7))
	b.WriteString("\nEnter the trust domain you want to federate with:\n\n")
	b.WriteString(m.trustDomainInput.View())
	b.WriteString("\n\n")
	b.WriteString(formatHint("Example: partner.example.org"))
	return b.String()
}

func (m *Model) viewEndpointDiscovery() string {
	var b strings.Builder
	b.WriteString(formatHeader("Bundle Endpoint", 2, 7))

	if m.isDiscovering {
		b.WriteString("\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Discovering bundle endpoint for ")
		b.WriteString(m.config.TrustDomain)
		b.WriteString("...\n")
		return b.String()
	}

	if m.discoveryResult == nil || m.discoveryResult.BestEndpoint == nil {
		b.WriteString("\nNo bundle endpoint discovered. Please enter manually.\n")
		return b.String()
	}

	ep := m.discoveryResult.BestEndpoint
	b.WriteString("\n")
	b.WriteString(formatSuccess("Bundle endpoint discovered"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  URL:          %s\n", ep.URL))
	b.WriteString(fmt.Sprintf("  Profile:      %s\n", ep.Profile))
	b.WriteString(fmt.Sprintf("  Certificates: %d\n", ep.CertCount))
	if !ep.ExpiresAt.IsZero() {
		b.WriteString(fmt.Sprintf("  Expires:      %s\n", ep.ExpiresAt.Format("2006-01-02")))
	}
	if ep.ResponseTime > 0 {
		b.WriteString(fmt.Sprintf("  Response:     %s\n", ep.ResponseTime.Round(time.Millisecond)))
	}

	b.WriteString("\n")
	b.WriteString(formatHint("Press [enter] to use this endpoint, [m] to enter manually"))

	return b.String()
}

func (m *Model) viewEndpointManual() string {
	var b strings.Builder
	b.WriteString(formatHeader("Bundle Endpoint", 2, 7))
	b.WriteString("\nEnter the bundle endpoint URL:\n\n")
	b.WriteString(m.endpointInput.View())
	b.WriteString("\n\n")
	b.WriteString(formatHint("Example: https://partner.example.org/.well-known/spiffe-bundle"))
	return b.String()
}

func (m *Model) viewPolicyCustomAllowed() string {
	var b strings.Builder
	b.WriteString(formatHeader("Custom Policy - Allowed Paths", 4, 7))
	b.WriteString("\nEnter allowed SPIFFE ID paths (comma-separated):\n\n")
	b.WriteString(m.allowedPathsInput.View())
	b.WriteString("\n\n")
	b.WriteString(formatHint("Supports glob patterns: /service/**, /ns/*/sa/*"))
	return b.String()
}

func (m *Model) viewPolicyCustomDenied() string {
	var b strings.Builder
	b.WriteString(formatHeader("Custom Policy - Denied Paths", 4, 7))
	b.WriteString("\nEnter denied SPIFFE ID paths (comma-separated, optional):\n\n")
	b.WriteString(m.deniedPathsInput.View())
	b.WriteString("\n\n")
	b.WriteString(formatHint("Denied paths take precedence over allowed paths"))
	return b.String()
}

func (m *Model) viewRefreshInterval() string {
	var b strings.Builder
	b.WriteString(formatHeader("Refresh Interval", 5, 7))
	b.WriteString("\nHow often should the trust bundle be refreshed?\n\n")
	b.WriteString(m.refreshIntervalInput.View())
	b.WriteString("\n\n")
	b.WriteString(formatHint("Examples: 5m, 1h, 30s (default: 5m)"))
	return b.String()
}

func (m *Model) viewPolicyTest() string {
	var b strings.Builder
	b.WriteString(formatHeader("Policy Test", 0, 0))
	b.WriteString("\nTest SPIFFE IDs against your policy configuration:\n\n")

	// Show test results
	for _, result := range m.policyTestResults {
		if result.Allowed {
			b.WriteString(formatSuccess(result.SPIFFEID))
		} else {
			b.WriteString(formatError(result.SPIFFEID))
		}
		b.WriteString("\n  ")
		b.WriteString(result.Reason)
		if result.MatchedRule != "" {
			b.WriteString(" (" + result.MatchedRule + ")")
		}
		b.WriteString("\n\n")
	}

	b.WriteString("Enter a SPIFFE ID to test (empty to return):\n")
	b.WriteString(m.testSpiffeIDInput.View())
	return b.String()
}

func (m *Model) viewConfirm() string {
	var b strings.Builder
	b.WriteString(formatHeader("Review Configuration", 7, 7))
	b.WriteString("\n")

	// Configuration summary
	b.WriteString(fmt.Sprintf("Trust Domain:     %s\n", m.config.TrustDomain))
	b.WriteString(fmt.Sprintf("Bundle Endpoint:  %s\n", m.config.BundleEndpoint))
	b.WriteString(fmt.Sprintf("Profile:          %s\n", m.config.EndpointProfile))
	b.WriteString(fmt.Sprintf("Type:             %s\n", m.config.FederationType))
	b.WriteString(fmt.Sprintf("Refresh Interval: %s\n", m.config.RefreshInterval))
	b.WriteString(fmt.Sprintf("Require mTLS:     %v\n", m.config.RequireMTLS))

	b.WriteString("\nPolicy:\n")
	if m.config.Policy != nil {
		b.WriteString(fmt.Sprintf("  Name: %s\n", m.config.Policy.Name))
		if len(m.config.Policy.AllowedPaths) > 0 {
			b.WriteString(fmt.Sprintf("  Allowed: %v\n", m.config.Policy.AllowedPaths))
		}
		if len(m.config.Policy.DeniedPaths) > 0 {
			b.WriteString(fmt.Sprintf("  Denied:  %v\n", m.config.Policy.DeniedPaths))
		}
	}

	b.WriteString("\n")
	b.WriteString(formatHint("[enter] create federation  [t] test policy  [esc] go back"))

	return b.String()
}

// Result returns the wizard result.
func (m *Model) Result() *Result {
	result := &Result{
		Config:    m.config,
		Cancelled: m.cancelled,
		Error:     m.err,
	}

	if !m.cancelled && m.done && m.err == nil {
		result.Domain = &federation.FederatedDomain{
			TrustDomain:           m.config.TrustDomain,
			Type:                  m.config.FederationType,
			State:                 federation.StatePending,
			BundleEndpoint:        m.config.BundleEndpoint,
			BundleEndpointProfile: m.config.EndpointProfile,
			Policy:                m.config.Policy,
			RefreshInterval:       m.config.RefreshInterval,
			CreatedAt:             time.Now(),
		}
	}

	return result
}

// Run launches the wizard and returns the result.
func Run() (*Result, error) {
	model := New()
	program := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("wizard failed: %w", err)
	}

	result, ok := finalModel.(*Model)
	if !ok {
		return nil, errors.New("unexpected model type")
	}

	return result.Result(), nil
}

// RunWithConfig launches the wizard with initial configuration.
func RunWithConfig(initial *Config) (*Result, error) {
	model := New()
	if initial != nil {
		model.config = initial
		if initial.TrustDomain != "" {
			model.trustDomainInput.SetValue(initial.TrustDomain)
		}
		if initial.BundleEndpoint != "" {
			model.endpointInput.SetValue(initial.BundleEndpoint)
		}
		if initial.RefreshInterval != 0 {
			model.refreshIntervalInput.SetValue(initial.RefreshInterval.String())
		}
	}

	program := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("wizard failed: %w", err)
	}

	result, ok := finalModel.(*Model)
	if !ok {
		return nil, errors.New("unexpected model type")
	}

	return result.Result(), nil
}

// splitPaths splits a comma-separated list of paths.
func splitPaths(value string) []string {
	var paths []string
	for _, part := range strings.Split(value, ",") {
		path := strings.TrimSpace(part)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}
