# bash completion for kscorectl and kscore-* plugins
# Source this file or add to /etc/bash_completion.d/

_kscorectl_completions()
{
    local cur prev words cword
    _init_completion || return

    # Top-level commands (plugins)
    local commands="exec state module policy gitops blueprint cluster identity files audit webhook federation bootstrap monitor migrate agent server registry telemetry-gateway version help"

    # Common global flags
    local global_flags="--config --log-level --help --audit-level --audit-output"

    # Determine which plugin we're completing for
    local plugin=""
    local i=1
    while [[ $i -lt $cword ]]; do
        case "${words[i]}" in
            exec|state|module|policy|gitops|blueprint|cluster|identity|files|audit|webhook|federation|bootstrap|monitor)
                plugin="${words[i]}"
                break
                ;;
        esac
        ((i++))
    done

    case "${plugin}" in
        exec)
            local exec_cmds="run status list version"
            local exec_flags="--server --timeout --tls --tls-ca-cert --tls-cert --tls-key --tls-skip-verify --tls-server-name"
            local run_flags="--concurrency --continue-on-failure --target --working-dir --user --command-timeout --env --job-id --show-progress --show-results"
            local list_flags="--status --page-size"

            case "${prev}" in
                run)
                    COMPREPLY=( $(compgen -W "${run_flags}" -- ${cur}) )
                    ;;
                status)
                    # Could complete job IDs here
                    ;;
                list)
                    COMPREPLY=( $(compgen -W "${list_flags}" -- ${cur}) )
                    ;;
                --status)
                    COMPREPLY=( $(compgen -W "pending running completed failed" -- ${cur}) )
                    ;;
                --log-level|--audit-level)
                    COMPREPLY=( $(compgen -W "debug info warn error" -- ${cur}) )
                    ;;
                exec)
                    COMPREPLY=( $(compgen -W "${exec_cmds} ${exec_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${exec_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        state)
            local state_cmds="apply check drift version"
            local state_flags="--target --vars --dry-run"

            case "${prev}" in
                apply|check|drift)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${state_flags}" -- ${cur}) )
                    else
                        COMPREPLY=( $(compgen -f -X '!*.@(yaml|yml)' -- ${cur}) )
                    fi
                    ;;
                --vars)
                    COMPREPLY=( $(compgen -f -X '!*.@(yaml|yml)' -- ${cur}) )
                    ;;
                state)
                    COMPREPLY=( $(compgen -W "${state_cmds}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${state_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        module)
            local module_cmds="init validate build resolve tree verify sign test publish install version"

            case "${prev}" in
                module)
                    COMPREPLY=( $(compgen -W "${module_cmds}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        policy)
            local policy_cmds="list validate check show audit report version"
            local check_flags="--policy --input-file --input --action --user --context --output"
            local list_flags="--category --type --output"
            local audit_flags="--policy --resource-type --denied --limit --output"
            local report_flags="--days --output"

            case "${prev}" in
                list)
                    COMPREPLY=( $(compgen -W "${list_flags}" -- ${cur}) )
                    ;;
                check)
                    COMPREPLY=( $(compgen -W "${check_flags}" -- ${cur}) )
                    ;;
                audit)
                    COMPREPLY=( $(compgen -W "${audit_flags}" -- ${cur}) )
                    ;;
                report)
                    COMPREPLY=( $(compgen -W "${report_flags}" -- ${cur}) )
                    ;;
                --type)
                    COMPREPLY=( $(compgen -W "opa cel" -- ${cur}) )
                    ;;
                --output|-o)
                    COMPREPLY=( $(compgen -W "table text json yaml" -- ${cur}) )
                    ;;
                policy)
                    COMPREPLY=( $(compgen -W "${policy_cmds}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        gitops)
            local gitops_cmds="verify rollback promote status webhook version"
            local verify_flags="--parallel --timeout --output"
            local rollback_flags="--app --namespace --type --strategy --revision --reason --user --dry-run --output"
            local promote_flags="--pipeline --from --to --revision --reason --user --skip-verify --force --dry-run --output"
            local status_flags="--type --limit --output"

            case "${prev}" in
                verify)
                    COMPREPLY=( $(compgen -W "${verify_flags}" -- ${cur}) )
                    ;;
                rollback)
                    COMPREPLY=( $(compgen -W "${rollback_flags}" -- ${cur}) )
                    ;;
                promote)
                    COMPREPLY=( $(compgen -W "${promote_flags}" -- ${cur}) )
                    ;;
                status)
                    COMPREPLY=( $(compgen -W "${status_flags}" -- ${cur}) )
                    ;;
                --type)
                    COMPREPLY=( $(compgen -W "argocd flux git all rollbacks promotions verifications" -- ${cur}) )
                    ;;
                --strategy)
                    COMPREPLY=( $(compgen -W "previous specific last_known_good" -- ${cur}) )
                    ;;
                --output|-o)
                    COMPREPLY=( $(compgen -W "text json yaml" -- ${cur}) )
                    ;;
                gitops)
                    COMPREPLY=( $(compgen -W "${gitops_cmds}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        blueprint)
            local blueprint_cmds="init validate lint test search info versions install update remove publish sign verify docs rollback snapshot version"

            case "${prev}" in
                blueprint)
                    COMPREPLY=( $(compgen -W "${blueprint_cmds}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        cluster)
            local cluster_cmds="status members leader add remove transfer-leader rebalance backup restore version"
            local cluster_flags="--server --output --verbose"

            case "${prev}" in
                --output|-o)
                    COMPREPLY=( $(compgen -W "table text json yaml" -- ${cur}) )
                    ;;
                cluster)
                    COMPREPLY=( $(compgen -W "${cluster_cmds} ${cluster_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${cluster_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        identity)
            local identity_cmds="status token ca federation bundle events version"
            local identity_flags="--server --output"
            local token_cmds="create list show revoke"
            local ca_cmds="info backup restore rotate"
            local federation_cmds="list add show suspend activate remove refresh"
            local bundle_cmds="show export"

            case "${prev}" in
                token)
                    COMPREPLY=( $(compgen -W "${token_cmds}" -- ${cur}) )
                    ;;
                ca)
                    COMPREPLY=( $(compgen -W "${ca_cmds}" -- ${cur}) )
                    ;;
                federation)
                    COMPREPLY=( $(compgen -W "${federation_cmds}" -- ${cur}) )
                    ;;
                bundle)
                    COMPREPLY=( $(compgen -W "${bundle_cmds}" -- ${cur}) )
                    ;;
                create)
                    COMPREPLY=( $(compgen -W "--agent-id --ttl --label" -- ${cur}) )
                    ;;
                export)
                    COMPREPLY=( $(compgen -W "--format" -- ${cur}) )
                    ;;
                --format)
                    COMPREPLY=( $(compgen -W "pem jwks spiffe" -- ${cur}) )
                    ;;
                --output|-o)
                    COMPREPLY=( $(compgen -W "table text json yaml" -- ${cur}) )
                    ;;
                identity)
                    COMPREPLY=( $(compgen -W "${identity_cmds} ${identity_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${identity_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        files)
            local files_cmds="serve files cache namespace backend mirrors version"
            local files_flags="--config --nats-url --cluster-id --instance-id"

            case "${prev}" in
                files)
                    COMPREPLY=( $(compgen -W "${files_cmds} ${files_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${files_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        audit)
            local audit_cmds="log report export stats version"
            local audit_flags="--server --format"
            local log_flags="--policy --resource-type --denied --limit --since --until"
            local report_flags="--days --category --severity"
            local export_flags="--days --output --export-format"
            local stats_flags="--days"

            case "${prev}" in
                log)
                    COMPREPLY=( $(compgen -W "${log_flags}" -- ${cur}) )
                    ;;
                report)
                    COMPREPLY=( $(compgen -W "${report_flags}" -- ${cur}) )
                    ;;
                export)
                    COMPREPLY=( $(compgen -W "${export_flags}" -- ${cur}) )
                    ;;
                stats)
                    COMPREPLY=( $(compgen -W "${stats_flags}" -- ${cur}) )
                    ;;
                --severity)
                    COMPREPLY=( $(compgen -W "low medium high critical" -- ${cur}) )
                    ;;
                --export-format)
                    COMPREPLY=( $(compgen -W "json csv yaml" -- ${cur}) )
                    ;;
                --format|-o)
                    COMPREPLY=( $(compgen -W "table text json yaml" -- ${cur}) )
                    ;;
                audit)
                    COMPREPLY=( $(compgen -W "${audit_cmds} ${audit_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${audit_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        webhook)
            local webhook_cmds="list show test history secrets version"
            local webhook_flags="--server --format"
            local secrets_cmds="list rotate"

            case "${prev}" in
                secrets)
                    COMPREPLY=( $(compgen -W "${secrets_cmds}" -- ${cur}) )
                    ;;
                show|test)
                    COMPREPLY=( $(compgen -W "argocd flux github gitlab" -- ${cur}) )
                    ;;
                history)
                    COMPREPLY=( $(compgen -W "--limit" -- ${cur}) )
                    ;;
                --format|-o)
                    COMPREPLY=( $(compgen -W "table text json yaml" -- ${cur}) )
                    ;;
                webhook)
                    COMPREPLY=( $(compgen -W "${webhook_cmds} ${webhook_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${webhook_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        federation)
            local federation_cmds="list add show suspend activate remove refresh bundle version"
            local federation_flags="--server --output"
            local add_flags="--bundle-endpoint --type --refresh-interval"
            local bundle_cmds="show export"

            case "${prev}" in
                add)
                    COMPREPLY=( $(compgen -W "${add_flags}" -- ${cur}) )
                    ;;
                bundle)
                    COMPREPLY=( $(compgen -W "${bundle_cmds}" -- ${cur}) )
                    ;;
                --type)
                    COMPREPLY=( $(compgen -W "bidirectional inbound outbound" -- ${cur}) )
                    ;;
                --output|-o)
                    COMPREPLY=( $(compgen -W "table text json yaml" -- ${cur}) )
                    ;;
                federation)
                    COMPREPLY=( $(compgen -W "${federation_cmds} ${federation_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${federation_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        bootstrap)
            local bootstrap_cmds="seed restore import validate status cleanup version"
            local bootstrap_flags="--output-dir --dry-run --force --skip-verification --timeout"

            case "${prev}" in
                seed|restore|import)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${bootstrap_flags}" -- ${cur}) )
                    else
                        COMPREPLY=( $(compgen -f -- ${cur}) )
                    fi
                    ;;
                bootstrap)
                    COMPREPLY=( $(compgen -W "${bootstrap_cmds}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${bootstrap_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        monitor)
            local monitor_flags="--config --control-plane --nats-url --theme --refresh --no-color"

            case "${prev}" in
                --theme)
                    COMPREPLY=( $(compgen -W "dark light solarized-dark solarized-light monokai" -- ${cur}) )
                    ;;
                monitor)
                    COMPREPLY=( $(compgen -W "${monitor_flags}" -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${monitor_flags} ${global_flags}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
        *)
            # Top-level completion
            case "${prev}" in
                kscorectl)
                    COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
                    ;;
                --log-level|--audit-level)
                    COMPREPLY=( $(compgen -W "debug info warn error" -- ${cur}) )
                    ;;
                --config)
                    COMPREPLY=( $(compgen -f -- ${cur}) )
                    ;;
                *)
                    if [[ ${cur} == -* ]]; then
                        COMPREPLY=( $(compgen -W "${global_flags}" -- ${cur}) )
                    else
                        COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
                    fi
                    ;;
            esac
            ;;
    esac
}

complete -F _kscorectl_completions kscorectl
