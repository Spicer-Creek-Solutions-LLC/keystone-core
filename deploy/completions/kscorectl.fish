# Fish completion for kscorectl and kscore-* plugins
# Place in ~/.config/fish/completions/kscorectl.fish

# Disable file completion by default
complete -c kscorectl -f

# Top-level commands
complete -c kscorectl -n "__fish_use_subcommand" -a "exec" -d "Execute commands on remote agents"
complete -c kscorectl -n "__fish_use_subcommand" -a "state" -d "Manage declarative state configurations"
complete -c kscorectl -n "__fish_use_subcommand" -a "module" -d "Manage Keystone Core modules"
complete -c kscorectl -n "__fish_use_subcommand" -a "policy" -d "Policy enforcement and compliance"
complete -c kscorectl -n "__fish_use_subcommand" -a "gitops" -d "GitOps integration and deployment"
complete -c kscorectl -n "__fish_use_subcommand" -a "blueprint" -d "Manage reusable infrastructure patterns"
complete -c kscorectl -n "__fish_use_subcommand" -a "cluster" -d "Manage Keystone Core cluster"
complete -c kscorectl -n "__fish_use_subcommand" -a "identity" -d "SPIFFE identity and certificate management"
complete -c kscorectl -n "__fish_use_subcommand" -a "files" -d "File distribution server"
complete -c kscorectl -n "__fish_use_subcommand" -a "audit" -d "Policy audit and compliance reporting"
complete -c kscorectl -n "__fish_use_subcommand" -a "webhook" -d "Webhook handler management"
complete -c kscorectl -n "__fish_use_subcommand" -a "federation" -d "Trust federation management"
complete -c kscorectl -n "__fish_use_subcommand" -a "bootstrap" -d "Cluster bootstrap and initialization"
complete -c kscorectl -n "__fish_use_subcommand" -a "monitor" -d "Real-time TUI monitoring dashboard"
complete -c kscorectl -n "__fish_use_subcommand" -a "migrate" -d "Database migration utilities"
complete -c kscorectl -n "__fish_use_subcommand" -a "version" -d "Show version information"
complete -c kscorectl -n "__fish_use_subcommand" -a "help" -d "Show help information"

# Global flags
complete -c kscorectl -l config -d "Path to configuration file" -r
complete -c kscorectl -l log-level -d "Set log level" -xa "debug info warn error"
complete -c kscorectl -l audit-level -d "Audit logging level" -xa "debug info warn error all"
complete -c kscorectl -l audit-output -d "Audit output backend" -xa "auto file syslog"
complete -c kscorectl -l help -d "Show help"

# exec subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -a "run" -d "Execute command across matching agents"
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -a "status" -d "Get batch job status"
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -a "list" -d "List batch jobs"
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -a "version" -d "Print version information"

# exec flags
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l server -d "Keystone Core server address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l timeout -d "Request timeout" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l tls -d "Enable TLS"
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l tls-ca-cert -d "Path to CA certificate" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l tls-cert -d "Path to client certificate" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l tls-key -d "Path to client private key" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l tls-skip-verify -d "Skip TLS verification"
complete -c kscorectl -n "__fish_seen_subcommand_from exec" -l tls-server-name -d "Server name for TLS verification" -r

# exec run flags
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l concurrency -d "Number of concurrent executions" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l continue-on-failure -d "Continue on errors"
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l target -d "Target expression" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l working-dir -d "Working directory" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l user -d "User to run as" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l command-timeout -d "Command timeout" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l env -d "Environment variable" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l job-id -d "Custom batch job ID" -r
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l show-progress -d "Show progress"
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from run" -l show-results -d "Show per-agent results"

# exec list flags
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from list" -l status -d "Filter by status" -xa "pending running completed failed"
complete -c kscorectl -n "__fish_seen_subcommand_from exec; and __fish_seen_subcommand_from list" -l page-size -d "Number of jobs to return" -r

# state subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from state" -a "apply" -d "Apply state declarations"
complete -c kscorectl -n "__fish_seen_subcommand_from state" -a "check" -d "Check state without applying (dry-run)"
complete -c kscorectl -n "__fish_seen_subcommand_from state" -a "drift" -d "Detect drift from desired state"
complete -c kscorectl -n "__fish_seen_subcommand_from state" -a "version" -d "Print version information"

# state flags
complete -c kscorectl -n "__fish_seen_subcommand_from state" -l target -d "Target expression" -r
complete -c kscorectl -n "__fish_seen_subcommand_from state" -l vars -d "Variables file" -r
complete -c kscorectl -n "__fish_seen_subcommand_from state; and __fish_seen_subcommand_from apply" -l dry-run -d "Check what would change"

# module subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "init" -d "Create new module from template"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "validate" -d "Check module.yaml syntax"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "build" -d "Package module as ZIP"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "resolve" -d "Resolve dependencies"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "tree" -d "Display dependency tree"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "verify" -d "Verify cryptographic signatures"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "sign" -d "Sign module with private key"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "test" -d "Run module tests"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "publish" -d "Publish to registry"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "install" -d "Install from registry"
complete -c kscorectl -n "__fish_seen_subcommand_from module" -a "version" -d "Print version information"

# policy subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "list" -d "List policies from file"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "validate" -d "Validate policy syntax"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "check" -d "Evaluate policy against input"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "show" -d "Show policy details"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "audit" -d "Show audit log (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "report" -d "Generate compliance report (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -a "version" -d "Print version information"

# policy flags
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from list" -l category -d "Filter by category" -r
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from list" -l type -d "Filter by type" -xa "opa cel"
complete -c kscorectl -n "__fish_seen_subcommand_from policy" -s o -l output -d "Output format" -xa "table text json yaml"
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from check" -l policy -d "Policy ID to evaluate" -r
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from check" -l input-file -d "Input JSON file" -r
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from check" -l input -d "Inline input JSON" -r
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from check" -l action -d "Action being performed" -r
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from check" -l user -d "User performing action" -r
complete -c kscorectl -n "__fish_seen_subcommand_from policy; and __fish_seen_subcommand_from check" -l context -d "Additional context as JSON" -r

# gitops subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -a "verify" -d "Run verification workflow"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -a "rollback" -d "Trigger rollback operation"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -a "promote" -d "Promote between environments"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -a "status" -d "Show GitOps operation status"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -a "webhook" -d "Webhook commands (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -a "version" -d "Print version information"

# gitops flags
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from verify" -l parallel -d "Run steps in parallel"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from verify" -l timeout -d "Workflow timeout" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops" -s o -l output -d "Output format" -xa "text json yaml"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l app -d "Application name" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l namespace -d "Namespace" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l type -d "Rollback type" -xa "argocd flux git"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l strategy -d "Strategy" -xa "previous specific last_known_good"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l revision -d "Target revision" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l reason -d "Reason for rollback" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from rollback" -l dry-run -d "Simulate without executing"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from promote" -l pipeline -d "Pipeline name" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from promote" -l from -d "Source environment" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from promote" -l to -d "Target environment" -r
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from promote" -l skip-verify -d "Skip verification step"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from promote" -l force -d "Force promotion despite failures"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from status" -l type -d "Status type" -xa "rollbacks promotions verifications all"
complete -c kscorectl -n "__fish_seen_subcommand_from gitops; and __fish_seen_subcommand_from status" -l limit -d "Maximum entries" -r

# blueprint subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "init" -d "Create new blueprint"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "validate" -d "Check blueprint syntax"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "lint" -d "Run best practices checks"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "test" -d "Run blueprint tests"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "search" -d "Search for blueprints"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "info" -d "Show blueprint details"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "versions" -d "List available versions"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "install" -d "Install blueprint"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "update" -d "Update blueprints"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "remove" -d "Remove blueprint"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "publish" -d "Publish blueprint (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "sign" -d "Sign blueprint (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "verify" -d "Verify signature (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from blueprint" -a "version" -d "Print version information"

# cluster subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "status" -d "View cluster status"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "members" -d "Manage cluster members"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "leader" -d "Monitor leader election"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "add" -d "Add cluster member"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "remove" -d "Remove cluster member"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "transfer-leader" -d "Transfer leader role"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "rebalance" -d "Rebalance cluster"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "backup" -d "Backup cluster (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "restore" -d "Restore from backup (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -a "version" -d "Print version information"

# cluster flags
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -s s -l server -d "Control plane address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -s o -l output -d "Output format" -xa "table text json yaml"
complete -c kscorectl -n "__fish_seen_subcommand_from cluster" -s v -l verbose -d "Enable verbose output"

# identity subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "status" -d "Show identity provider status"
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "token" -d "Manage join tokens"
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "ca" -d "Certificate authority management"
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "federation" -d "Federation management"
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "bundle" -d "Trust bundle management"
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "events" -d "View identity events"
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -a "version" -d "Print version information"

# identity flags
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -l server -d "Control plane API address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from identity" -s o -l output -d "Output format" -xa "table text json yaml"

# identity token subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token" -a "create" -d "Create join token"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token" -a "list" -d "List join tokens"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token" -a "show" -d "Show token details"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token" -a "revoke" -d "Revoke token"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token; and __fish_seen_subcommand_from create" -l agent-id -d "Agent ID" -r
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token; and __fish_seen_subcommand_from create" -l ttl -d "Token time-to-live" -r
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from token; and __fish_seen_subcommand_from create" -l label -d "Labels" -r

# identity ca subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from ca" -a "info" -d "Show CA information"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from ca" -a "backup" -d "Backup CA certificates"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from ca" -a "restore" -d "Restore CA from backup"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from ca" -a "rotate" -d "Trigger CA rotation"

# identity bundle subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from bundle" -a "show" -d "Show local trust bundle"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from bundle" -a "export" -d "Export trust bundle"
complete -c kscorectl -n "__fish_seen_subcommand_from identity; and __fish_seen_subcommand_from bundle; and __fish_seen_subcommand_from export" -l format -d "Export format" -xa "pem jwks spiffe"

# files subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "serve" -d "Start the file server"
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "files" -d "File management"
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "cache" -d "Cache management"
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "namespace" -d "Namespace management"
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "backend" -d "Backend configuration (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "mirrors" -d "Mirror configuration (deprecated)"
complete -c kscorectl -n "__fish_seen_subcommand_from files" -a "version" -d "Print version information"

# files flags
complete -c kscorectl -n "__fish_seen_subcommand_from files" -l config -d "Config file path" -r
complete -c kscorectl -n "__fish_seen_subcommand_from files" -l nats-url -d "NATS server URL" -r
complete -c kscorectl -n "__fish_seen_subcommand_from files" -l cluster-id -d "Cluster ID" -r
complete -c kscorectl -n "__fish_seen_subcommand_from files" -l instance-id -d "Server instance ID" -r

# audit subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -a "log" -d "View policy evaluation audit log"
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -a "report" -d "Generate compliance report"
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -a "export" -d "Export audit data"
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -a "stats" -d "View audit statistics and trends"
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -a "version" -d "Print version information"

# audit flags
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -s s -l server -d "Control plane address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit" -s o -l format -d "Output format" -xa "table text json yaml"
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from log" -l policy -d "Filter by policy ID" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from log" -l resource-type -d "Filter by resource type" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from log" -l denied -d "Show only denied evaluations"
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from log" -l limit -d "Maximum entries" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from log" -l since -d "Show entries since date" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from log" -l until -d "Show entries until date" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from report" -l days -d "Number of days in report" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from report" -l category -d "Filter by policy category" -r
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from report" -l severity -d "Filter by severity" -xa "low medium high critical"
complete -c kscorectl -n "__fish_seen_subcommand_from audit; and __fish_seen_subcommand_from export" -l export-format -d "Format" -xa "json csv yaml"

# webhook subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -a "list" -d "List registered webhook handlers"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -a "show" -d "Show webhook handler details"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -a "test" -d "Send test webhook"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -a "history" -d "View webhook delivery history"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -a "secrets" -d "Manage webhook secrets"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -a "version" -d "Print version information"

# webhook flags
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -s s -l server -d "Control plane address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from webhook" -s o -l format -d "Output format" -xa "table text json yaml"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook; and __fish_seen_subcommand_from show" -a "argocd flux github gitlab" -d "Webhook type"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook; and __fish_seen_subcommand_from test" -a "argocd flux github gitlab" -d "Webhook type"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook; and __fish_seen_subcommand_from history" -l limit -d "Maximum entries to show" -r

# webhook secrets subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from webhook; and __fish_seen_subcommand_from secrets" -a "list" -d "List webhook secrets"
complete -c kscorectl -n "__fish_seen_subcommand_from webhook; and __fish_seen_subcommand_from secrets" -a "rotate" -d "Rotate webhook secret"

# federation subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "list" -d "List federated trust domains"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "add" -d "Add federated domain"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "show" -d "Show federation details"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "suspend" -d "Suspend federation"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "activate" -d "Activate federation"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "remove" -d "Remove federation"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "refresh" -d "Refresh trust bundle"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "bundle" -d "Manage trust bundles"
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -a "version" -d "Print version information"

# federation flags
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -l server -d "Control plane API address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from federation" -s o -l output -d "Output format" -xa "table text json yaml"
complete -c kscorectl -n "__fish_seen_subcommand_from federation; and __fish_seen_subcommand_from add" -l bundle-endpoint -d "URL to fetch trust bundle" -r
complete -c kscorectl -n "__fish_seen_subcommand_from federation; and __fish_seen_subcommand_from add" -l type -d "Federation type" -xa "bidirectional inbound outbound"
complete -c kscorectl -n "__fish_seen_subcommand_from federation; and __fish_seen_subcommand_from add" -l refresh-interval -d "Bundle refresh interval" -r

# bootstrap subcommands
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "seed" -d "Bootstrap from seed configuration"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "restore" -d "Restore from backup"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "import" -d "Import existing installation"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "validate" -d "Validate configuration"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "status" -d "Show bootstrap status"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "cleanup" -d "Clean up bootstrap resources"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -a "version" -d "Print version information"

# bootstrap flags
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -l output-dir -d "Output directory" -r
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -l dry-run -d "Simulation without execution"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -l force -d "Force operation"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -l skip-verification -d "Skip verification"
complete -c kscorectl -n "__fish_seen_subcommand_from bootstrap" -l timeout -d "Operation timeout" -r

# monitor flags
complete -c kscorectl -n "__fish_seen_subcommand_from monitor" -l config -d "Config file path" -r
complete -c kscorectl -n "__fish_seen_subcommand_from monitor" -l control-plane -d "Control plane gRPC address" -r
complete -c kscorectl -n "__fish_seen_subcommand_from monitor" -l nats-url -d "NATS server URL" -r
complete -c kscorectl -n "__fish_seen_subcommand_from monitor" -l theme -d "UI theme" -xa "dark light solarized-dark solarized-light monokai"
complete -c kscorectl -n "__fish_seen_subcommand_from monitor" -l refresh -d "Refresh interval in seconds" -r
complete -c kscorectl -n "__fish_seen_subcommand_from monitor" -l no-color -d "Disable colors"
