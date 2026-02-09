// Package tui implements terminal user interface components for interactive
// bootstrap configuration wizards.
package tui

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type step int

const (
	stepMode step = iota
	stepClusterName
	stepNodeRole
	stepNodeName
	stepNodeLabels
	stepStorage
	stepRegions
	stepHAEnabled
	stepHAReplicas
	stepObservabilityBackend
	stepObservabilityEndpoint
	stepIdentityProvider
	stepIdentityEndpoint
	stepNATS
	stepBindAddress
	stepAdvertiseAddress
	stepPostgresHost
	stepPostgresPort
	stepPostgresDatabase
	stepPostgresUser
	stepPostgresPassword
	stepPostgresSSLMode
	stepNATSURLs
	stepNATSAuth
	stepNATSCredsFile
	stepNATSUser
	stepNATSPassword
	stepGenerateCerts
	stepTLSCertFile
	stepTLSKeyFile
	stepTLSCAFile
	stepJoinEndpoint
	stepJoinToken
	stepBlueprintsDir
	stepApplyBlueprints
	stepBlueprintParams
	stepBlueprintFeatures
	stepBlueprintEntrypoints
	stepExportStatesDir
	stepConfirm
)

type modeItem struct {
	mode        string
	title       string
	description string
}

func (m modeItem) Title() string       { return m.title }
func (m modeItem) Description() string { return m.description }
func (m modeItem) FilterValue() string { return m.title }

type roleItem struct {
	role        string
	description string
}

func (r roleItem) Title() string       { return r.role }
func (r roleItem) Description() string { return r.description }
func (r roleItem) FilterValue() string { return r.role }

type wizardModel struct {
	width  int
	height int
	step   step

	modeList     list.Model
	roleList     list.Model
	storageList  list.Model
	natsList     list.Model
	sslModeList  list.Model
	natsAuthList list.Model
	haList       list.Model
	obsList      list.Model
	idpList      list.Model

	clusterInput               textinput.Model
	nodeNameInput              textinput.Model
	nodeLabelsInput            textinput.Model
	regionsInput               textinput.Model
	haReplicasInput            textinput.Model
	observabilityEndpointInput textinput.Model
	identityEndpointInput      textinput.Model
	bindInput                  textinput.Model
	advertiseInput             textinput.Model
	postgresHostInput          textinput.Model
	postgresPortInput          textinput.Model
	postgresDBInput            textinput.Model
	postgresUserInput          textinput.Model
	postgresPassInput          textinput.Model
	natsURLsInput              textinput.Model
	natsCredsInput             textinput.Model
	natsUserInput              textinput.Model
	natsPassInput              textinput.Model
	certList                   list.Model
	tlsCertInput               textinput.Model
	tlsKeyInput                textinput.Model
	tlsCAInput                 textinput.Model
	joinInput                  textinput.Model
	tokenInput                 textinput.Model
	blueprintsDirInput         textinput.Model
	applyBlueprintsInput       textinput.Model
	blueprintParamsInput       textinput.Model
	blueprintFeaturesInput     textinput.Model
	blueprintEntrypointsInput  textinput.Model
	exportStatesDirInput       textinput.Model

	config WizardConfig
	done   bool
	err    error
}

// WizardConfig is the data collected from the TUI wizard.
type WizardConfig struct {
	Mode                    string
	RecommendedMode         string
	ResourceSummary         string
	ModeHints               map[string]string
	ClusterName             string
	NodeRole                string
	NodeName                string
	NodeLabels              map[string]string
	NodeLabelArgs           []string
	Regions                 []string
	HAEnabled               bool
	HAReplicas              int
	ObservabilityBackend    string
	ObservabilityEndpoint   string
	IdentityProvider        string
	IdentityEndpoint        string
	Storage                 string
	NATSMode                string
	BindAddress             string
	Advertise               string
	PostgresHost            string
	PostgresPort            int
	PostgresDatabase        string
	PostgresUser            string
	PostgresPassword        string
	PostgresSSLMode         string
	NATSURLs                []string
	GenerateCerts           bool
	TLSCertFile             string
	TLSKeyFile              string
	TLSCAFile               string
	TLSCSRFile              string
	TLSRenewalCommand       string
	TLSRenewalScriptPath    string
	NATSCredsFile           string
	NATSUser                string
	NATSPassword            string
	PackageChannel          string
	PackageVersion          string
	MigrateFromSQLite       string
	MigrateBatchSize        int
	MigrateContinueOnError  bool
	MigrateSkipExisting     bool
	BlueprintsDir           string
	ApplyBlueprints         []string
	BlueprintParams         map[string]map[string]interface{}
	BlueprintFeatures       map[string]map[string]bool
	BlueprintEntrypoints    map[string]string
	BlueprintParamArgs      []string
	BlueprintFeatureArgs    []string
	BlueprintEntrypointArgs []string
	ExportStatesDir         string
	Join                    string
	JoinToken               string
}

func newModel(initial WizardConfig) wizardModel {
	modeItems := []list.Item{
		modeItem{mode: "demo", title: "Demo", description: modeDescription(initial, "demo", "Single-node demo with embedded dependencies")},
		modeItem{mode: "production", title: "Production", description: modeDescription(initial, "production", "Production-ready cluster defaults")},
		modeItem{mode: "fullscale", title: "Full Scale", description: modeDescription(initial, "fullscale", "Multi-node deployment with external services")},
		modeItem{mode: "custom", title: "Custom", description: modeDescription(initial, "custom", "Advanced configuration")},
	}

	roleItems := []list.Item{
		roleItem{role: "control-plane", description: "Control plane services only"},
		roleItem{role: "agent", description: "Agent only (join existing cluster)"},
		roleItem{role: "both", description: "Control plane and agent"},
	}

	storageItems := []list.Item{
		roleItem{role: "sqlite", description: "Embedded SQLite storage"},
		roleItem{role: "postgres", description: "External PostgreSQL storage"},
	}

	natsItems := []list.Item{
		roleItem{role: "embedded", description: "Embedded NATS server"},
		roleItem{role: "cluster", description: "NATS cluster (recommended for prod)"},
		roleItem{role: "external", description: "External NATS endpoints"},
	}

	sslModeItems := []list.Item{
		roleItem{role: "prefer", description: "Prefer TLS, fallback to plaintext"},
		roleItem{role: "require", description: "Require TLS, skip verification"},
		roleItem{role: "verify-ca", description: "Require TLS and verify CA"},
		roleItem{role: "verify-full", description: "Require TLS and verify host"},
		roleItem{role: "disable", description: "Disable TLS"},
		roleItem{role: "allow", description: "Prefer plaintext, fallback to TLS"},
	}

	natsAuthItems := []list.Item{
		roleItem{role: "none", description: "No authentication"},
		roleItem{role: "creds", description: "Credentials file"},
		roleItem{role: "userpass", description: "Username/password"},
	}

	haItems := []list.Item{
		roleItem{role: "disabled", description: "Single-node control plane"},
		roleItem{role: "enabled", description: "High availability control plane"},
	}

	obsItems := []list.Item{
		roleItem{role: "none", description: "No external observability backend"},
		roleItem{role: "prometheus", description: "Prometheus metrics"},
		roleItem{role: "grafana", description: "Grafana dashboards"},
		roleItem{role: "loki", description: "Loki logs"},
		roleItem{role: "tempo", description: "Tempo traces"},
		roleItem{role: "otel", description: "OpenTelemetry collector"},
		roleItem{role: "custom", description: "Custom observability endpoint"},
	}

	idpItems := []list.Item{
		roleItem{role: "none", description: "No external identity provider"},
		roleItem{role: "spiffe", description: "SPIFFE/SPIRE"},
		roleItem{role: "oidc", description: "OIDC provider"},
		roleItem{role: "ldap", description: "LDAP directory"},
		roleItem{role: "saml", description: "SAML provider"},
	}

	certItems := []list.Item{
		roleItem{role: "generate", description: "Generate self-signed certificates"},
		roleItem{role: "manual", description: "Provide existing certificates"},
	}

	modeList := list.New(modeItems, list.NewDefaultDelegate(), 0, 0)
	modeList.Title = modeListTitle(initial)
	modeList.SetShowHelp(true)
	modeList.DisableQuitKeybindings()

	roleList := list.New(roleItems, list.NewDefaultDelegate(), 0, 0)
	roleList.Title = "Select node role"
	roleList.SetShowHelp(true)
	roleList.DisableQuitKeybindings()

	storageList := list.New(storageItems, list.NewDefaultDelegate(), 0, 0)
	storageList.Title = "Select storage backend"
	storageList.SetShowHelp(true)
	storageList.DisableQuitKeybindings()

	natsList := list.New(natsItems, list.NewDefaultDelegate(), 0, 0)
	natsList.Title = "Select NATS mode"
	natsList.SetShowHelp(true)
	natsList.DisableQuitKeybindings()

	sslModeList := list.New(sslModeItems, list.NewDefaultDelegate(), 0, 0)
	sslModeList.Title = "Postgres SSL mode"
	sslModeList.SetShowHelp(true)
	sslModeList.DisableQuitKeybindings()

	natsAuthList := list.New(natsAuthItems, list.NewDefaultDelegate(), 0, 0)
	natsAuthList.Title = "NATS authentication"
	natsAuthList.SetShowHelp(true)
	natsAuthList.DisableQuitKeybindings()

	haList := list.New(haItems, list.NewDefaultDelegate(), 0, 0)
	haList.Title = "High availability"
	haList.SetShowHelp(true)
	haList.DisableQuitKeybindings()

	obsList := list.New(obsItems, list.NewDefaultDelegate(), 0, 0)
	obsList.Title = "Observability backend"
	obsList.SetShowHelp(true)
	obsList.DisableQuitKeybindings()

	idpList := list.New(idpItems, list.NewDefaultDelegate(), 0, 0)
	idpList.Title = "Identity provider"
	idpList.SetShowHelp(true)
	idpList.DisableQuitKeybindings()

	certList := list.New(certItems, list.NewDefaultDelegate(), 0, 0)
	certList.Title = "Certificates"
	certList.SetShowHelp(true)
	certList.DisableQuitKeybindings()

	clusterInput := textinput.New()
	clusterInput.Placeholder = "keystone"
	clusterInput.Prompt = "Cluster name: "
	clusterInput.CharLimit = 64
	if initial.ClusterName != "" {
		clusterInput.SetValue(initial.ClusterName)
	}

	nodeNameInput := textinput.New()
	nodeNameInput.Placeholder = "hostname"
	nodeNameInput.Prompt = "Node name (optional): "
	if initial.NodeName != "" {
		nodeNameInput.SetValue(initial.NodeName)
	}

	nodeLabelsInput := textinput.New()
	nodeLabelsInput.Placeholder = "env=prod,role=agent"
	nodeLabelsInput.Prompt = "Node labels (optional): "
	if len(initial.NodeLabelArgs) > 0 {
		nodeLabelsInput.SetValue(strings.Join(initial.NodeLabelArgs, ","))
	} else if len(initial.NodeLabels) > 0 {
		nodeLabelsInput.SetValue(formatNodeLabels(initial.NodeLabels))
	}

	regionsInput := textinput.New()
	regionsInput.Placeholder = "us-east-1,us-west-2"
	regionsInput.Prompt = "Regions (optional): "
	if len(initial.Regions) > 0 {
		regionsInput.SetValue(strings.Join(initial.Regions, ","))
	}

	haReplicasInput := textinput.New()
	haReplicasInput.Placeholder = "3"
	haReplicasInput.Prompt = "Control plane replicas: "
	if initial.HAReplicas != 0 {
		haReplicasInput.SetValue(fmt.Sprintf("%d", initial.HAReplicas))
	}

	observabilityEndpointInput := textinput.New()
	observabilityEndpointInput.Placeholder = "https://obs.example.com"
	observabilityEndpointInput.Prompt = "Observability endpoint (optional): "
	if initial.ObservabilityEndpoint != "" {
		observabilityEndpointInput.SetValue(initial.ObservabilityEndpoint)
	}

	identityEndpointInput := textinput.New()
	identityEndpointInput.Placeholder = "https://id.example.com"
	identityEndpointInput.Prompt = "Identity provider endpoint (optional): "
	if initial.IdentityEndpoint != "" {
		identityEndpointInput.SetValue(initial.IdentityEndpoint)
	}

	joinInput := textinput.New()
	joinInput.Placeholder = "https://control-plane:8443"
	joinInput.Prompt = "Join endpoint (optional): "
	if initial.Join != "" {
		joinInput.SetValue(initial.Join)
	}

	bindInput := textinput.New()
	bindInput.Placeholder = "0.0.0.0"
	bindInput.Prompt = "Bind address (optional): "
	if initial.BindAddress != "" {
		bindInput.SetValue(initial.BindAddress)
	}

	advertiseInput := textinput.New()
	advertiseInput.Placeholder = "10.0.0.1"
	advertiseInput.Prompt = "Advertise address (optional): "
	if initial.Advertise != "" {
		advertiseInput.SetValue(initial.Advertise)
	}

	postgresHostInput := textinput.New()
	postgresHostInput.Placeholder = "db.example.com"
	postgresHostInput.Prompt = "Postgres host: "
	if initial.PostgresHost != "" {
		postgresHostInput.SetValue(initial.PostgresHost)
	}

	postgresPortInput := textinput.New()
	postgresPortInput.Placeholder = "5432"
	postgresPortInput.Prompt = "Postgres port: "
	if initial.PostgresPort != 0 {
		postgresPortInput.SetValue(fmt.Sprintf("%d", initial.PostgresPort))
	}

	postgresDBInput := textinput.New()
	postgresDBInput.Placeholder = "keystone"
	postgresDBInput.Prompt = "Postgres database: "
	if initial.PostgresDatabase != "" {
		postgresDBInput.SetValue(initial.PostgresDatabase)
	}

	postgresUserInput := textinput.New()
	postgresUserInput.Placeholder = "kscore"
	postgresUserInput.Prompt = "Postgres user: "
	if initial.PostgresUser != "" {
		postgresUserInput.SetValue(initial.PostgresUser)
	}

	postgresPassInput := textinput.New()
	postgresPassInput.Placeholder = "password"
	postgresPassInput.Prompt = "Postgres password: "
	postgresPassInput.EchoMode = textinput.EchoPassword
	if initial.PostgresPassword != "" {
		postgresPassInput.SetValue(initial.PostgresPassword)
	}

	natsURLsInput := textinput.New()
	natsURLsInput.Placeholder = "nats://nats1:4222,nats://nats2:4222"
	natsURLsInput.Prompt = "NATS URLs: "
	if len(initial.NATSURLs) > 0 {
		natsURLsInput.SetValue(strings.Join(initial.NATSURLs, ","))
	}

	natsCredsInput := textinput.New()
	natsCredsInput.Placeholder = "/etc/keystone-core/nats.creds"
	natsCredsInput.Prompt = "NATS creds file: "
	if initial.NATSCredsFile != "" {
		natsCredsInput.SetValue(initial.NATSCredsFile)
	}

	natsUserInput := textinput.New()
	natsUserInput.Placeholder = "nats-user"
	natsUserInput.Prompt = "NATS username: "
	if initial.NATSUser != "" {
		natsUserInput.SetValue(initial.NATSUser)
	}

	natsPassInput := textinput.New()
	natsPassInput.Placeholder = "password"
	natsPassInput.Prompt = "NATS password: "
	natsPassInput.EchoMode = textinput.EchoPassword
	if initial.NATSPassword != "" {
		natsPassInput.SetValue(initial.NATSPassword)
	}

	tlsCertInput := textinput.New()
	tlsCertInput.Placeholder = "/etc/keystone-core/tls.crt"
	tlsCertInput.Prompt = "TLS cert file: "
	if initial.TLSCertFile != "" {
		tlsCertInput.SetValue(initial.TLSCertFile)
	}

	tlsKeyInput := textinput.New()
	tlsKeyInput.Placeholder = "/etc/keystone-core/tls.key"
	tlsKeyInput.Prompt = "TLS key file: "
	if initial.TLSKeyFile != "" {
		tlsKeyInput.SetValue(initial.TLSKeyFile)
	}

	tlsCAInput := textinput.New()
	tlsCAInput.Placeholder = "/etc/keystone-core/ca.crt"
	tlsCAInput.Prompt = "TLS CA file (optional): "
	if initial.TLSCAFile != "" {
		tlsCAInput.SetValue(initial.TLSCAFile)
	}

	tokenInput := textinput.New()
	tokenInput.Placeholder = "join token"
	tokenInput.Prompt = "Join token: "
	tokenInput.EchoMode = textinput.EchoPassword
	if initial.JoinToken != "" {
		tokenInput.SetValue(initial.JoinToken)
	}

	blueprintsDirInput := textinput.New()
	blueprintsDirInput.Placeholder = "/etc/keystone-core/blueprints"
	blueprintsDirInput.Prompt = "Blueprints directory (optional): "
	if initial.BlueprintsDir != "" {
		blueprintsDirInput.SetValue(initial.BlueprintsDir)
	}

	applyBlueprintsInput := textinput.New()
	applyBlueprintsInput.Placeholder = "blueprints/demo,blueprints/standard"
	applyBlueprintsInput.Prompt = "Blueprints to apply (optional): "
	if len(initial.ApplyBlueprints) > 0 {
		applyBlueprintsInput.SetValue(strings.Join(initial.ApplyBlueprints, ","))
	}

	blueprintParamsInput := textinput.New()
	blueprintParamsInput.Placeholder = "blueprint:param=value"
	blueprintParamsInput.Prompt = "Blueprint params (optional): "
	if len(initial.BlueprintParamArgs) > 0 {
		blueprintParamsInput.SetValue(strings.Join(initial.BlueprintParamArgs, ","))
	} else if len(initial.BlueprintParams) > 0 {
		blueprintParamsInput.SetValue(strings.Join(formatBlueprintParams(initial.BlueprintParams), ","))
	}

	blueprintFeaturesInput := textinput.New()
	blueprintFeaturesInput.Placeholder = "blueprint:feature=true"
	blueprintFeaturesInput.Prompt = "Blueprint features (optional): "
	if len(initial.BlueprintFeatureArgs) > 0 {
		blueprintFeaturesInput.SetValue(strings.Join(initial.BlueprintFeatureArgs, ","))
	} else if len(initial.BlueprintFeatures) > 0 {
		blueprintFeaturesInput.SetValue(strings.Join(formatBlueprintFeatures(initial.BlueprintFeatures), ","))
	}

	blueprintEntrypointsInput := textinput.New()
	blueprintEntrypointsInput.Placeholder = "blueprint:entrypoint"
	blueprintEntrypointsInput.Prompt = "Blueprint entrypoints (optional): "
	if len(initial.BlueprintEntrypointArgs) > 0 {
		blueprintEntrypointsInput.SetValue(strings.Join(initial.BlueprintEntrypointArgs, ","))
	} else if len(initial.BlueprintEntrypoints) > 0 {
		blueprintEntrypointsInput.SetValue(strings.Join(formatBlueprintEntrypoints(initial.BlueprintEntrypoints), ","))
	}

	exportStatesDirInput := textinput.New()
	exportStatesDirInput.Placeholder = "/tmp/kscore-states"
	exportStatesDirInput.Prompt = "Export rendered states to (optional): "
	if initial.ExportStatesDir != "" {
		exportStatesDirInput.SetValue(initial.ExportStatesDir)
	}

	model := wizardModel{
		step:                       stepMode,
		modeList:                   modeList,
		roleList:                   roleList,
		storageList:                storageList,
		natsList:                   natsList,
		sslModeList:                sslModeList,
		natsAuthList:               natsAuthList,
		haList:                     haList,
		obsList:                    obsList,
		idpList:                    idpList,
		certList:                   certList,
		clusterInput:               clusterInput,
		nodeNameInput:              nodeNameInput,
		nodeLabelsInput:            nodeLabelsInput,
		regionsInput:               regionsInput,
		haReplicasInput:            haReplicasInput,
		observabilityEndpointInput: observabilityEndpointInput,
		identityEndpointInput:      identityEndpointInput,
		bindInput:                  bindInput,
		advertiseInput:             advertiseInput,
		postgresHostInput:          postgresHostInput,
		postgresPortInput:          postgresPortInput,
		postgresDBInput:            postgresDBInput,
		postgresUserInput:          postgresUserInput,
		postgresPassInput:          postgresPassInput,
		natsURLsInput:              natsURLsInput,
		natsCredsInput:             natsCredsInput,
		natsUserInput:              natsUserInput,
		natsPassInput:              natsPassInput,
		tlsCertInput:               tlsCertInput,
		tlsKeyInput:                tlsKeyInput,
		tlsCAInput:                 tlsCAInput,
		joinInput:                  joinInput,
		tokenInput:                 tokenInput,
		blueprintsDirInput:         blueprintsDirInput,
		applyBlueprintsInput:       applyBlueprintsInput,
		blueprintParamsInput:       blueprintParamsInput,
		blueprintFeaturesInput:     blueprintFeaturesInput,
		blueprintEntrypointsInput:  blueprintEntrypointsInput,
		exportStatesDirInput:       exportStatesDirInput,
		config:                     initial,
	}

	selectedMode := initial.Mode
	if selectedMode == "" {
		selectedMode = initial.RecommendedMode
	}
	if selectedMode != "" {
		for i, item := range modeItems {
			if item.(modeItem).mode == selectedMode {
				model.modeList.Select(i)
				break
			}
		}
	}

	if initial.NodeRole != "" {
		for i, item := range roleItems {
			if item.(roleItem).role == initial.NodeRole {
				model.roleList.Select(i)
				break
			}
		}
	}

	if initial.Storage != "" {
		for i, item := range storageItems {
			if item.(roleItem).role == initial.Storage {
				model.storageList.Select(i)
				break
			}
		}
	}

	if initial.NATSMode != "" {
		for i, item := range natsItems {
			if item.(roleItem).role == initial.NATSMode {
				model.natsList.Select(i)
				break
			}
		}
	}

	if initial.PostgresSSLMode != "" {
		for i, item := range sslModeItems {
			if item.(roleItem).role == initial.PostgresSSLMode {
				model.sslModeList.Select(i)
				break
			}
		}
	} else {
		model.sslModeList.Select(0)
	}

	switch {
	case initial.NATSCredsFile != "":
		model.natsAuthList.Select(1)
	case initial.NATSUser != "" || initial.NATSPassword != "":
		model.natsAuthList.Select(2)
	default:
		model.natsAuthList.Select(0)
	}

	if initial.HAEnabled {
		model.haList.Select(1)
	} else {
		model.haList.Select(0)
	}

	if initial.ObservabilityBackend != "" {
		for i, item := range obsItems {
			if item.(roleItem).role == initial.ObservabilityBackend {
				model.obsList.Select(i)
				break
			}
		}
	} else {
		model.obsList.Select(0)
	}

	if initial.IdentityProvider != "" {
		for i, item := range idpItems {
			if item.(roleItem).role == initial.IdentityProvider {
				model.idpList.Select(i)
				break
			}
		}
	} else {
		model.idpList.Select(0)
	}

	if initial.GenerateCerts {
		model.certList.Select(0)
	} else {
		model.certList.Select(1)
	}

	return model
}

func (m wizardModel) Init() tea.Cmd {
	return nil
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.modeList.SetSize(msg.Width, msg.Height-6)
		m.roleList.SetSize(msg.Width, msg.Height-6)
		m.storageList.SetSize(msg.Width, msg.Height-6)
		m.natsList.SetSize(msg.Width, msg.Height-6)
		m.sslModeList.SetSize(msg.Width, msg.Height-6)
		m.natsAuthList.SetSize(msg.Width, msg.Height-6)
		m.haList.SetSize(msg.Width, msg.Height-6)
		m.obsList.SetSize(msg.Width, msg.Height-6)
		m.idpList.SetSize(msg.Width, msg.Height-6)
		m.certList.SetSize(msg.Width, msg.Height-6)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.err = errors.New("bootstrap wizard cancelled")
			m.done = true
			return m, tea.Quit
		case "enter":
			return m.advance()
		case "backspace", "shift+tab":
			return m.back()
		}
	}

	switch m.step {
	case stepMode:
		var cmd tea.Cmd
		m.modeList, cmd = m.modeList.Update(msg)
		return m, cmd
	case stepNodeRole:
		var cmd tea.Cmd
		m.roleList, cmd = m.roleList.Update(msg)
		return m, cmd
	case stepStorage:
		var cmd tea.Cmd
		m.storageList, cmd = m.storageList.Update(msg)
		return m, cmd
	case stepNATS:
		var cmd tea.Cmd
		m.natsList, cmd = m.natsList.Update(msg)
		return m, cmd
	case stepPostgresSSLMode:
		var cmd tea.Cmd
		m.sslModeList, cmd = m.sslModeList.Update(msg)
		return m, cmd
	case stepNATSAuth:
		var cmd tea.Cmd
		m.natsAuthList, cmd = m.natsAuthList.Update(msg)
		return m, cmd
	case stepHAEnabled:
		var cmd tea.Cmd
		m.haList, cmd = m.haList.Update(msg)
		return m, cmd
	case stepObservabilityBackend:
		var cmd tea.Cmd
		m.obsList, cmd = m.obsList.Update(msg)
		return m, cmd
	case stepIdentityProvider:
		var cmd tea.Cmd
		m.idpList, cmd = m.idpList.Update(msg)
		return m, cmd
	case stepGenerateCerts:
		var cmd tea.Cmd
		m.certList, cmd = m.certList.Update(msg)
		return m, cmd
	case stepClusterName:
		var cmd tea.Cmd
		m.clusterInput, cmd = m.clusterInput.Update(msg)
		return m, cmd
	case stepNodeName:
		var cmd tea.Cmd
		m.nodeNameInput, cmd = m.nodeNameInput.Update(msg)
		return m, cmd
	case stepNodeLabels:
		var cmd tea.Cmd
		m.nodeLabelsInput, cmd = m.nodeLabelsInput.Update(msg)
		return m, cmd
	case stepRegions:
		var cmd tea.Cmd
		m.regionsInput, cmd = m.regionsInput.Update(msg)
		return m, cmd
	case stepHAReplicas:
		var cmd tea.Cmd
		m.haReplicasInput, cmd = m.haReplicasInput.Update(msg)
		return m, cmd
	case stepObservabilityEndpoint:
		var cmd tea.Cmd
		m.observabilityEndpointInput, cmd = m.observabilityEndpointInput.Update(msg)
		return m, cmd
	case stepIdentityEndpoint:
		var cmd tea.Cmd
		m.identityEndpointInput, cmd = m.identityEndpointInput.Update(msg)
		return m, cmd
	case stepBindAddress:
		var cmd tea.Cmd
		m.bindInput, cmd = m.bindInput.Update(msg)
		return m, cmd
	case stepAdvertiseAddress:
		var cmd tea.Cmd
		m.advertiseInput, cmd = m.advertiseInput.Update(msg)
		return m, cmd
	case stepPostgresHost:
		var cmd tea.Cmd
		m.postgresHostInput, cmd = m.postgresHostInput.Update(msg)
		return m, cmd
	case stepPostgresPort:
		var cmd tea.Cmd
		m.postgresPortInput, cmd = m.postgresPortInput.Update(msg)
		return m, cmd
	case stepPostgresDatabase:
		var cmd tea.Cmd
		m.postgresDBInput, cmd = m.postgresDBInput.Update(msg)
		return m, cmd
	case stepPostgresUser:
		var cmd tea.Cmd
		m.postgresUserInput, cmd = m.postgresUserInput.Update(msg)
		return m, cmd
	case stepPostgresPassword:
		var cmd tea.Cmd
		m.postgresPassInput, cmd = m.postgresPassInput.Update(msg)
		return m, cmd
	case stepNATSURLs:
		var cmd tea.Cmd
		m.natsURLsInput, cmd = m.natsURLsInput.Update(msg)
		return m, cmd
	case stepNATSCredsFile:
		var cmd tea.Cmd
		m.natsCredsInput, cmd = m.natsCredsInput.Update(msg)
		return m, cmd
	case stepNATSUser:
		var cmd tea.Cmd
		m.natsUserInput, cmd = m.natsUserInput.Update(msg)
		return m, cmd
	case stepNATSPassword:
		var cmd tea.Cmd
		m.natsPassInput, cmd = m.natsPassInput.Update(msg)
		return m, cmd
	case stepTLSCertFile:
		var cmd tea.Cmd
		m.tlsCertInput, cmd = m.tlsCertInput.Update(msg)
		return m, cmd
	case stepTLSKeyFile:
		var cmd tea.Cmd
		m.tlsKeyInput, cmd = m.tlsKeyInput.Update(msg)
		return m, cmd
	case stepTLSCAFile:
		var cmd tea.Cmd
		m.tlsCAInput, cmd = m.tlsCAInput.Update(msg)
		return m, cmd
	case stepJoinEndpoint:
		var cmd tea.Cmd
		m.joinInput, cmd = m.joinInput.Update(msg)
		return m, cmd
	case stepJoinToken:
		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(msg)
		return m, cmd
	case stepBlueprintsDir:
		var cmd tea.Cmd
		m.blueprintsDirInput, cmd = m.blueprintsDirInput.Update(msg)
		return m, cmd
	case stepApplyBlueprints:
		var cmd tea.Cmd
		m.applyBlueprintsInput, cmd = m.applyBlueprintsInput.Update(msg)
		return m, cmd
	case stepBlueprintParams:
		var cmd tea.Cmd
		m.blueprintParamsInput, cmd = m.blueprintParamsInput.Update(msg)
		return m, cmd
	case stepBlueprintFeatures:
		var cmd tea.Cmd
		m.blueprintFeaturesInput, cmd = m.blueprintFeaturesInput.Update(msg)
		return m, cmd
	case stepBlueprintEntrypoints:
		var cmd tea.Cmd
		m.blueprintEntrypointsInput, cmd = m.blueprintEntrypointsInput.Update(msg)
		return m, cmd
	case stepExportStatesDir:
		var cmd tea.Cmd
		m.exportStatesDirInput, cmd = m.exportStatesDirInput.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m wizardModel) View() string {
	switch m.step {
	case stepMode:
		return m.modeList.View()
	case stepClusterName:
		return fmt.Sprintf("Cluster configuration\n\n%s\n\n(enter to continue, esc to cancel)", m.clusterInput.View())
	case stepNodeRole:
		return m.roleList.View()
	case stepNodeName:
		return fmt.Sprintf("Node name\n\n%s\n\n(leave empty to use hostname)", m.nodeNameInput.View())
	case stepNodeLabels:
		return fmt.Sprintf("Node labels\n\n%s\n\n(comma-separated key=value)", m.nodeLabelsInput.View())
	case stepStorage:
		return m.storageList.View()
	case stepRegions:
		return fmt.Sprintf("Regions\n\n%s\n\n(comma-separated, leave empty to skip)", m.regionsInput.View())
	case stepHAEnabled:
		return m.haList.View()
	case stepHAReplicas:
		return fmt.Sprintf("HA control plane replicas\n\n%s\n\n(leave empty to use defaults)", m.haReplicasInput.View())
	case stepObservabilityBackend:
		return m.obsList.View()
	case stepObservabilityEndpoint:
		return fmt.Sprintf("Observability endpoint\n\n%s\n\n(leave empty to skip)", m.observabilityEndpointInput.View())
	case stepIdentityProvider:
		return m.idpList.View()
	case stepIdentityEndpoint:
		return fmt.Sprintf("Identity provider endpoint\n\n%s\n\n(leave empty to skip)", m.identityEndpointInput.View())
	case stepNATS:
		return m.natsList.View()
	case stepPostgresSSLMode:
		return m.sslModeList.View()
	case stepNATSAuth:
		return m.natsAuthList.View()
	case stepGenerateCerts:
		return m.certList.View()
	case stepTLSCertFile:
		return fmt.Sprintf("TLS certificate file\n\n%s", m.tlsCertInput.View())
	case stepTLSKeyFile:
		return fmt.Sprintf("TLS key file\n\n%s", m.tlsKeyInput.View())
	case stepTLSCAFile:
		return fmt.Sprintf("TLS CA file (optional)\n\n%s", m.tlsCAInput.View())
	case stepBindAddress:
		return fmt.Sprintf("Bind address\n\n%s\n\n(leave empty to use defaults)", m.bindInput.View())
	case stepAdvertiseAddress:
		return fmt.Sprintf("Advertise address\n\n%s\n\n(leave empty to use defaults)", m.advertiseInput.View())
	case stepPostgresHost:
		return fmt.Sprintf("Postgres host\n\n%s", m.postgresHostInput.View())
	case stepPostgresPort:
		return fmt.Sprintf("Postgres port\n\n%s", m.postgresPortInput.View())
	case stepPostgresDatabase:
		return fmt.Sprintf("Postgres database\n\n%s", m.postgresDBInput.View())
	case stepPostgresUser:
		return fmt.Sprintf("Postgres user\n\n%s", m.postgresUserInput.View())
	case stepPostgresPassword:
		return fmt.Sprintf("Postgres password\n\n%s", m.postgresPassInput.View())
	case stepNATSURLs:
		return fmt.Sprintf("NATS URLs\n\n%s", m.natsURLsInput.View())
	case stepNATSCredsFile:
		return fmt.Sprintf("NATS creds file\n\n%s", m.natsCredsInput.View())
	case stepNATSUser:
		return fmt.Sprintf("NATS username\n\n%s", m.natsUserInput.View())
	case stepNATSPassword:
		return fmt.Sprintf("NATS password\n\n%s", m.natsPassInput.View())
	case stepJoinEndpoint:
		return fmt.Sprintf("Join existing cluster\n\n%s\n\n(leave empty to skip)", m.joinInput.View())
	case stepJoinToken:
		return fmt.Sprintf("Join token\n\n%s\n", m.tokenInput.View())
	case stepBlueprintsDir:
		return fmt.Sprintf("Blueprints directory\n\n%s\n\n(leave empty to use defaults)", m.blueprintsDirInput.View())
	case stepApplyBlueprints:
		return fmt.Sprintf("Blueprints to apply\n\n%s\n\n(comma-separated, leave empty to skip)", m.applyBlueprintsInput.View())
	case stepBlueprintParams:
		return fmt.Sprintf("Blueprint parameters\n\n%s\n\n(format: blueprint:key=value, comma-separated)", m.blueprintParamsInput.View())
	case stepBlueprintFeatures:
		return fmt.Sprintf("Blueprint features\n\n%s\n\n(format: blueprint:feature=true, comma-separated)", m.blueprintFeaturesInput.View())
	case stepBlueprintEntrypoints:
		return fmt.Sprintf("Blueprint entrypoints\n\n%s\n\n(format: blueprint:entrypoint, comma-separated)", m.blueprintEntrypointsInput.View())
	case stepExportStatesDir:
		return fmt.Sprintf("Export rendered states\n\n%s\n\n(leave empty to apply directly)", m.exportStatesDirInput.View())
	case stepConfirm:
		return m.confirmView()
	default:
		return ""
	}
}

func (m wizardModel) advance() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepMode:
		if item, ok := m.modeList.SelectedItem().(modeItem); ok {
			m.config.Mode = item.mode
		}
		m.step = stepClusterName
		m.clusterInput.Focus()
		return m, nil
	case stepClusterName:
		value := strings.TrimSpace(m.clusterInput.Value())
		if value == "" {
			value = m.clusterInput.Placeholder
		}
		m.config.ClusterName = value
		m.step = stepNodeRole
		return m, nil
	case stepNodeRole:
		if item, ok := m.roleList.SelectedItem().(roleItem); ok {
			m.config.NodeRole = item.role
		}
		m.step = stepNodeName
		m.nodeNameInput.Focus()
		return m, nil
	case stepNodeName:
		m.config.NodeName = strings.TrimSpace(m.nodeNameInput.Value())
		m.step = stepNodeLabels
		m.nodeLabelsInput.Focus()
		return m, nil
	case stepNodeLabels:
		m.config.NodeLabelArgs = splitCSV(m.nodeLabelsInput.Value())
		m.step = stepStorage
		return m, nil
	case stepStorage:
		if item, ok := m.storageList.SelectedItem().(roleItem); ok {
			m.config.Storage = item.role
		}
		if m.config.Storage == "postgres" {
			m.step = stepPostgresHost
			m.postgresHostInput.Focus()
			return m, nil
		}
		if isFullscaleMode(m.config.Mode) {
			m.step = stepRegions
			m.regionsInput.Focus()
			return m, nil
		}
		m.step = stepNATS
		return m, nil
	case stepPostgresHost:
		m.config.PostgresHost = strings.TrimSpace(m.postgresHostInput.Value())
		m.step = stepPostgresPort
		m.postgresPortInput.Focus()
		return m, nil
	case stepPostgresPort:
		m.config.PostgresPort = parsePort(m.postgresPortInput.Value())
		m.step = stepPostgresDatabase
		m.postgresDBInput.Focus()
		return m, nil
	case stepPostgresDatabase:
		m.config.PostgresDatabase = strings.TrimSpace(m.postgresDBInput.Value())
		m.step = stepPostgresUser
		m.postgresUserInput.Focus()
		return m, nil
	case stepPostgresUser:
		m.config.PostgresUser = strings.TrimSpace(m.postgresUserInput.Value())
		m.step = stepPostgresPassword
		m.postgresPassInput.Focus()
		return m, nil
	case stepPostgresPassword:
		m.config.PostgresPassword = strings.TrimSpace(m.postgresPassInput.Value())
		m.step = stepPostgresSSLMode
		return m, nil
	case stepPostgresSSLMode:
		if item, ok := m.sslModeList.SelectedItem().(roleItem); ok {
			m.config.PostgresSSLMode = item.role
		}
		if isFullscaleMode(m.config.Mode) {
			m.step = stepRegions
			m.regionsInput.Focus()
			return m, nil
		}
		m.step = stepNATS
		return m, nil
	case stepRegions:
		m.config.Regions = splitCSV(m.regionsInput.Value())
		m.step = stepHAEnabled
		return m, nil
	case stepHAEnabled:
		if item, ok := m.haList.SelectedItem().(roleItem); ok {
			m.config.HAEnabled = item.role == "enabled"
		}
		if m.config.HAEnabled {
			m.step = stepHAReplicas
			m.haReplicasInput.Focus()
			return m, nil
		}
		m.step = stepObservabilityBackend
		return m, nil
	case stepHAReplicas:
		m.config.HAReplicas = parsePort(m.haReplicasInput.Value())
		m.step = stepObservabilityBackend
		return m, nil
	case stepObservabilityBackend:
		if item, ok := m.obsList.SelectedItem().(roleItem); ok {
			m.config.ObservabilityBackend = item.role
		}
		if m.config.ObservabilityBackend != "" && m.config.ObservabilityBackend != "none" {
			m.step = stepObservabilityEndpoint
			m.observabilityEndpointInput.Focus()
			return m, nil
		}
		m.step = stepIdentityProvider
		return m, nil
	case stepObservabilityEndpoint:
		m.config.ObservabilityEndpoint = strings.TrimSpace(m.observabilityEndpointInput.Value())
		m.step = stepIdentityProvider
		return m, nil
	case stepIdentityProvider:
		if item, ok := m.idpList.SelectedItem().(roleItem); ok {
			m.config.IdentityProvider = item.role
		}
		if m.config.IdentityProvider != "" && m.config.IdentityProvider != "none" {
			m.step = stepIdentityEndpoint
			m.identityEndpointInput.Focus()
			return m, nil
		}
		m.step = stepNATS
		return m, nil
	case stepIdentityEndpoint:
		m.config.IdentityEndpoint = strings.TrimSpace(m.identityEndpointInput.Value())
		m.step = stepNATS
		return m, nil
	case stepNATS:
		if item, ok := m.natsList.SelectedItem().(roleItem); ok {
			m.config.NATSMode = item.role
		}
		if m.config.NATSMode == "external" {
			m.step = stepNATSURLs
			m.natsURLsInput.Focus()
			return m, nil
		}
		if m.config.NATSMode == "cluster" {
			m.step = stepNATSAuth
			return m, nil
		}
		m.step = stepGenerateCerts
		return m, nil
	case stepNATSURLs:
		m.config.NATSURLs = splitCSV(m.natsURLsInput.Value())
		m.step = stepNATSAuth
		return m, nil
	case stepNATSAuth:
		if item, ok := m.natsAuthList.SelectedItem().(roleItem); ok {
			switch item.role {
			case "creds":
				m.config.NATSCredsFile = ""
				m.config.NATSUser = ""
				m.config.NATSPassword = ""
				m.step = stepNATSCredsFile
				m.natsCredsInput.Focus()
				return m, nil
			case "userpass":
				m.config.NATSCredsFile = ""
				m.config.NATSUser = ""
				m.config.NATSPassword = ""
				m.step = stepNATSUser
				m.natsUserInput.Focus()
				return m, nil
			default:
				m.config.NATSCredsFile = ""
				m.config.NATSUser = ""
				m.config.NATSPassword = ""
			}
		}
		m.step = stepGenerateCerts
		return m, nil
	case stepNATSCredsFile:
		m.config.NATSCredsFile = strings.TrimSpace(m.natsCredsInput.Value())
		m.config.NATSUser = ""
		m.config.NATSPassword = ""
		m.step = stepGenerateCerts
		return m, nil
	case stepNATSUser:
		m.config.NATSUser = strings.TrimSpace(m.natsUserInput.Value())
		m.step = stepNATSPassword
		m.natsPassInput.Focus()
		return m, nil
	case stepNATSPassword:
		m.config.NATSPassword = strings.TrimSpace(m.natsPassInput.Value())
		m.step = stepGenerateCerts
		return m, nil
	case stepGenerateCerts:
		if item, ok := m.certList.SelectedItem().(roleItem); ok {
			m.config.GenerateCerts = item.role == "generate"
		}
		if !m.config.GenerateCerts {
			m.step = stepTLSCertFile
			m.tlsCertInput.Focus()
			return m, nil
		}
		m.step = stepBindAddress
		m.bindInput.Focus()
		return m, nil
	case stepTLSCertFile:
		m.config.TLSCertFile = strings.TrimSpace(m.tlsCertInput.Value())
		m.step = stepTLSKeyFile
		m.tlsKeyInput.Focus()
		return m, nil
	case stepTLSKeyFile:
		m.config.TLSKeyFile = strings.TrimSpace(m.tlsKeyInput.Value())
		m.step = stepTLSCAFile
		m.tlsCAInput.Focus()
		return m, nil
	case stepTLSCAFile:
		m.config.TLSCAFile = strings.TrimSpace(m.tlsCAInput.Value())
		m.step = stepBindAddress
		m.bindInput.Focus()
		return m, nil
	case stepBindAddress:
		m.config.BindAddress = strings.TrimSpace(m.bindInput.Value())
		m.step = stepAdvertiseAddress
		m.advertiseInput.Focus()
		return m, nil
	case stepAdvertiseAddress:
		m.config.Advertise = strings.TrimSpace(m.advertiseInput.Value())
		m.step = stepJoinEndpoint
		m.joinInput.Focus()
		return m, nil
	case stepJoinEndpoint:
		m.config.Join = strings.TrimSpace(m.joinInput.Value())
		if m.config.Join != "" {
			m.step = stepJoinToken
			m.tokenInput.Focus()
			return m, nil
		}
		m.step = stepBlueprintsDir
		m.blueprintsDirInput.Focus()
		return m, nil
	case stepJoinToken:
		m.config.JoinToken = strings.TrimSpace(m.tokenInput.Value())
		m.step = stepBlueprintsDir
		m.blueprintsDirInput.Focus()
		return m, nil
	case stepBlueprintsDir:
		m.config.BlueprintsDir = strings.TrimSpace(m.blueprintsDirInput.Value())
		m.step = stepApplyBlueprints
		m.applyBlueprintsInput.Focus()
		return m, nil
	case stepApplyBlueprints:
		m.config.ApplyBlueprints = splitCSV(m.applyBlueprintsInput.Value())
		m.step = stepBlueprintParams
		m.blueprintParamsInput.Focus()
		return m, nil
	case stepBlueprintParams:
		m.config.BlueprintParamArgs = splitCSV(m.blueprintParamsInput.Value())
		m.step = stepBlueprintFeatures
		m.blueprintFeaturesInput.Focus()
		return m, nil
	case stepBlueprintFeatures:
		m.config.BlueprintFeatureArgs = splitCSV(m.blueprintFeaturesInput.Value())
		m.step = stepBlueprintEntrypoints
		m.blueprintEntrypointsInput.Focus()
		return m, nil
	case stepBlueprintEntrypoints:
		m.config.BlueprintEntrypointArgs = splitCSV(m.blueprintEntrypointsInput.Value())
		m.step = stepExportStatesDir
		m.exportStatesDirInput.Focus()
		return m, nil
	case stepExportStatesDir:
		m.config.ExportStatesDir = strings.TrimSpace(m.exportStatesDirInput.Value())
		m.step = stepConfirm
		return m, nil
	case stepConfirm:
		m.done = true
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m wizardModel) back() (tea.Model, tea.Cmd) {
	if m.step == stepMode {
		return m, nil
	}
	m.step--
	return m, nil
}

func (m wizardModel) confirmView() string {
	var builder strings.Builder
	builder.WriteString("Confirm bootstrap configuration\n\n")
	builder.WriteString(fmt.Sprintf("Mode: %s\n", m.config.Mode))
	builder.WriteString(fmt.Sprintf("Cluster: %s\n", m.config.ClusterName))
	builder.WriteString(fmt.Sprintf("Node role: %s\n", m.config.NodeRole))
	if m.config.NodeName != "" {
		builder.WriteString(fmt.Sprintf("Node name: %s\n", m.config.NodeName))
	}
	nodeLabels := m.config.NodeLabelArgs
	if len(nodeLabels) == 0 && len(m.config.NodeLabels) > 0 {
		nodeLabels = splitCSV(formatNodeLabels(m.config.NodeLabels))
	}
	if len(nodeLabels) > 0 {
		builder.WriteString(fmt.Sprintf("Node labels: %s\n", strings.Join(nodeLabels, ", ")))
	}
	if len(m.config.Regions) > 0 {
		builder.WriteString(fmt.Sprintf("Regions: %s\n", strings.Join(m.config.Regions, ", ")))
	}
	if m.config.HAEnabled {
		builder.WriteString("HA enabled: true\n")
		if m.config.HAReplicas != 0 {
			builder.WriteString(fmt.Sprintf("HA replicas: %d\n", m.config.HAReplicas))
		}
	}
	if m.config.ObservabilityBackend != "" && m.config.ObservabilityBackend != "none" {
		builder.WriteString(fmt.Sprintf("Observability: %s\n", m.config.ObservabilityBackend))
		if m.config.ObservabilityEndpoint != "" {
			builder.WriteString(fmt.Sprintf("Observability endpoint: %s\n", m.config.ObservabilityEndpoint))
		}
	}
	if m.config.IdentityProvider != "" && m.config.IdentityProvider != "none" {
		builder.WriteString(fmt.Sprintf("Identity provider: %s\n", m.config.IdentityProvider))
		if m.config.IdentityEndpoint != "" {
			builder.WriteString(fmt.Sprintf("Identity endpoint: %s\n", m.config.IdentityEndpoint))
		}
	}
	if m.config.Storage != "" {
		builder.WriteString(fmt.Sprintf("Storage: %s\n", m.config.Storage))
	}
	if m.config.Storage == "postgres" {
		builder.WriteString(fmt.Sprintf("Postgres host: %s\n", m.config.PostgresHost))
		builder.WriteString(fmt.Sprintf("Postgres port: %d\n", m.config.PostgresPort))
		builder.WriteString(fmt.Sprintf("Postgres database: %s\n", m.config.PostgresDatabase))
		builder.WriteString(fmt.Sprintf("Postgres user: %s\n", m.config.PostgresUser))
		if m.config.PostgresPassword != "" {
			builder.WriteString("Postgres password: provided\n")
		}
		if m.config.PostgresSSLMode != "" {
			builder.WriteString(fmt.Sprintf("Postgres SSL mode: %s\n", m.config.PostgresSSLMode))
		}
	}
	if m.config.NATSMode != "" {
		builder.WriteString(fmt.Sprintf("NATS mode: %s\n", m.config.NATSMode))
	}
	if len(m.config.NATSURLs) > 0 {
		builder.WriteString(fmt.Sprintf("NATS URLs: %s\n", strings.Join(m.config.NATSURLs, ", ")))
	}
	switch {
	case m.config.NATSCredsFile != "":
		builder.WriteString(fmt.Sprintf("NATS creds file: %s\n", m.config.NATSCredsFile))
	case m.config.NATSUser != "" || m.config.NATSPassword != "":
		builder.WriteString(fmt.Sprintf("NATS user: %s\n", m.config.NATSUser))
		if m.config.NATSPassword != "" {
			builder.WriteString("NATS password: provided\n")
		}
	case m.config.NATSMode == "external" || m.config.NATSMode == "cluster":
		builder.WriteString("NATS auth: none\n")
	}
	builder.WriteString(fmt.Sprintf("Generate certs: %t\n", m.config.GenerateCerts))
	if !m.config.GenerateCerts {
		builder.WriteString(fmt.Sprintf("TLS cert: %s\n", m.config.TLSCertFile))
		builder.WriteString(fmt.Sprintf("TLS key: %s\n", m.config.TLSKeyFile))
		if m.config.TLSCAFile != "" {
			builder.WriteString(fmt.Sprintf("TLS CA: %s\n", m.config.TLSCAFile))
		}
	}
	if m.config.BindAddress != "" {
		builder.WriteString(fmt.Sprintf("Bind address: %s\n", m.config.BindAddress))
	}
	if m.config.Advertise != "" {
		builder.WriteString(fmt.Sprintf("Advertise address: %s\n", m.config.Advertise))
	}
	if m.config.Join != "" {
		builder.WriteString(fmt.Sprintf("Join: %s\n", m.config.Join))
		builder.WriteString("Join token: provided\n")
	}
	if m.config.BlueprintsDir != "" {
		builder.WriteString(fmt.Sprintf("Blueprints dir: %s\n", m.config.BlueprintsDir))
	}
	if len(m.config.ApplyBlueprints) > 0 {
		builder.WriteString(fmt.Sprintf("Apply blueprints: %s\n", strings.Join(m.config.ApplyBlueprints, ", ")))
	}
	blueprintParams := m.config.BlueprintParamArgs
	if len(blueprintParams) == 0 && len(m.config.BlueprintParams) > 0 {
		blueprintParams = formatBlueprintParams(m.config.BlueprintParams)
	}
	if len(blueprintParams) > 0 {
		builder.WriteString(fmt.Sprintf("Blueprint params: %s\n", strings.Join(blueprintParams, ", ")))
	}
	blueprintFeatures := m.config.BlueprintFeatureArgs
	if len(blueprintFeatures) == 0 && len(m.config.BlueprintFeatures) > 0 {
		blueprintFeatures = formatBlueprintFeatures(m.config.BlueprintFeatures)
	}
	if len(blueprintFeatures) > 0 {
		builder.WriteString(fmt.Sprintf("Blueprint features: %s\n", strings.Join(blueprintFeatures, ", ")))
	}
	blueprintEntrypoints := m.config.BlueprintEntrypointArgs
	if len(blueprintEntrypoints) == 0 && len(m.config.BlueprintEntrypoints) > 0 {
		blueprintEntrypoints = formatBlueprintEntrypoints(m.config.BlueprintEntrypoints)
	}
	if len(blueprintEntrypoints) > 0 {
		builder.WriteString(fmt.Sprintf("Blueprint entrypoints: %s\n", strings.Join(blueprintEntrypoints, ", ")))
	}
	if m.config.ExportStatesDir != "" {
		builder.WriteString(fmt.Sprintf("Export states dir: %s\n", m.config.ExportStatesDir))
	}
	builder.WriteString("\nPress enter to continue or esc to cancel.")
	return builder.String()
}

// RunWizard launches the bootstrap wizard and returns the config.
func RunWizard(initial WizardConfig) (WizardConfig, error) {
	wizard := newModel(initial)
	program := tea.NewProgram(wizard)
	finalModel, err := program.Run()
	if err != nil {
		return WizardConfig{}, fmt.Errorf("bootstrap wizard failed: %w", err)
	}

	result, ok := finalModel.(wizardModel)
	if !ok {
		return WizardConfig{}, errors.New("unexpected wizard model")
	}

	if result.err != nil {
		return WizardConfig{}, result.err
	}

	if !result.done {
		return WizardConfig{}, errors.New("wizard did not complete")
	}

	return result.config, nil
}

func parsePort(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func formatBlueprintParams(params map[string]map[string]interface{}) []string {
	if len(params) == 0 {
		return nil
	}
	blueprints := make([]string, 0, len(params))
	for blueprint := range params {
		blueprints = append(blueprints, blueprint)
	}
	sort.Strings(blueprints)
	var result []string
	for _, blueprint := range blueprints {
		keys := make([]string, 0, len(params[blueprint]))
		for key := range params[blueprint] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result = append(result, fmt.Sprintf("%s:%s=%v", blueprint, key, params[blueprint][key]))
		}
	}
	return result
}

func formatBlueprintFeatures(features map[string]map[string]bool) []string {
	if len(features) == 0 {
		return nil
	}
	blueprints := make([]string, 0, len(features))
	for blueprint := range features {
		blueprints = append(blueprints, blueprint)
	}
	sort.Strings(blueprints)
	var result []string
	for _, blueprint := range blueprints {
		keys := make([]string, 0, len(features[blueprint]))
		for key := range features[blueprint] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result = append(result, fmt.Sprintf("%s:%s=%t", blueprint, key, features[blueprint][key]))
		}
	}
	return result
}

func formatBlueprintEntrypoints(entrypoints map[string]string) []string {
	if len(entrypoints) == 0 {
		return nil
	}
	blueprints := make([]string, 0, len(entrypoints))
	for blueprint := range entrypoints {
		blueprints = append(blueprints, blueprint)
	}
	sort.Strings(blueprints)
	result := make([]string, 0, len(entrypoints))
	for _, blueprint := range blueprints {
		result = append(result, fmt.Sprintf("%s:%s", blueprint, entrypoints[blueprint]))
	}
	return result
}

func formatNodeLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, labels[key]))
	}
	return strings.Join(parts, ",")
}

func isFullscaleMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "fullscale")
}

func modeDescription(initial WizardConfig, mode, base string) string {
	description := base
	if initial.ModeHints != nil {
		if hint := strings.TrimSpace(initial.ModeHints[mode]); hint != "" {
			description = fmt.Sprintf("%s. %s", description, hint)
		}
	}
	if strings.EqualFold(initial.RecommendedMode, mode) {
		description = fmt.Sprintf("%s (recommended)", description)
	}
	return description
}

func modeListTitle(initial WizardConfig) string {
	title := "Select deployment mode"
	if initial.ResourceSummary != "" {
		title = fmt.Sprintf("%s (%s)", title, initial.ResourceSummary)
	}
	return title
}
