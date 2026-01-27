---
title: "Runbook Examples"
weight: 13
description: >
  Example runbooks for common operational scenarios
---

This page contains ready-to-use runbook examples for common operational scenarios.

## Deployment Runbooks

### Blue-Green Deployment

Safely deploy with zero downtime using blue-green strategy.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: blue-green-deploy
  namespace: deployments
spec:
  description: Blue-green deployment with automated rollback
  inputs:
    - name: app_name
      type: string
      required: true
    - name: version
      type: string
      required: true
    - name: environment
      type: string
      required: true
      validation: "^(staging|production)$"

  steps:
    - name: validate_image
      type: command
      config:
        target: control-plane
        command: |
          docker pull {{ .inputs.app_name }}:{{ .inputs.version }}
          docker inspect {{ .inputs.app_name }}:{{ .inputs.version }}

    - name: deploy_green
      type: deploy
      dependsOn: [validate_image]
      config:
        target: "{{ .inputs.environment }}-green"
        manifest: |
          apiVersion: apps/v1
          kind: Deployment
          metadata:
            name: {{ .inputs.app_name }}-green
          spec:
            replicas: 3
            template:
              spec:
                containers:
                  - name: app
                    image: {{ .inputs.app_name }}:{{ .inputs.version }}

    - name: health_check_green
      type: wait
      dependsOn: [deploy_green]
      config:
        poll_interval: 10s
        timeout: 5m
        condition: |
          {{ eq (httpGet "http://green.internal/health" | jsonPath "$.status") "healthy" }}

    - name: request_approval
      type: approval
      dependsOn: [health_check_green]
      condition: "{{ eq .inputs.environment \"production\" }}"
      config:
        message: |
          Ready to switch traffic to version {{ .inputs.version }}
          Health check passed on green deployment.
        approvers:
          - group: release-managers
        timeout: 30m

    - name: switch_traffic
      type: command
      dependsOn: [request_approval]
      config:
        target: load-balancer
        command: |
          kubectl patch service {{ .inputs.app_name }} \
            -p '{"spec":{"selector":{"deployment":"green"}}}'

    - name: verify_traffic
      type: wait
      dependsOn: [switch_traffic]
      config:
        poll_interval: 5s
        timeout: 2m
        condition: |
          {{ gt (metric "http_requests_total{deployment=\"green\"}") 100 }}

    - name: cleanup_blue
      type: command
      dependsOn: [verify_traffic]
      config:
        target: control-plane
        command: |
          kubectl scale deployment {{ .inputs.app_name }}-blue --replicas=0

  onSuccess:
    - name: notify_success
      type: notification
      config:
        channel: slack
        message: |
          :white_check_mark: Blue-green deployment complete
          App: {{ .inputs.app_name }}
          Version: {{ .inputs.version }}
          Environment: {{ .inputs.environment }}

  onFailure:
    - name: rollback
      type: command
      config:
        target: load-balancer
        command: |
          kubectl patch service {{ .inputs.app_name }} \
            -p '{"spec":{"selector":{"deployment":"blue"}}}'

    - name: notify_failure
      type: notification
      config:
        channel: pagerduty
        message: |
          :x: Blue-green deployment failed
          App: {{ .inputs.app_name }}
          Version: {{ .inputs.version }}
          Error: {{ .error }}
```

### Canary Deployment

Gradually roll out changes with traffic splitting.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: canary-deploy
  namespace: deployments
spec:
  description: Canary deployment with progressive traffic shift
  inputs:
    - name: app_name
      type: string
      required: true
    - name: version
      type: string
      required: true
    - name: canary_percent
      type: int
      default: 10

  steps:
    - name: deploy_canary
      type: deploy
      config:
        manifest: "deployments/canary.yaml"
        parameters:
          version: "{{ .inputs.version }}"
          replicas: 1

    - name: shift_traffic_10
      type: command
      dependsOn: [deploy_canary]
      config:
        command: |
          istioctl install \
            --set values.pilot.autoscaleMin=1 \
            -y
          kubectl apply -f - <<EOF
          apiVersion: networking.istio.io/v1beta1
          kind: VirtualService
          spec:
            http:
              - route:
                  - destination:
                      host: {{ .inputs.app_name }}
                      subset: stable
                    weight: 90
                  - destination:
                      host: {{ .inputs.app_name }}
                      subset: canary
                    weight: 10
          EOF

    - name: monitor_canary
      type: wait
      dependsOn: [shift_traffic_10]
      config:
        duration: 10m

    - name: check_error_rate
      type: command
      dependsOn: [monitor_canary]
      config:
        command: |
          error_rate=$(promql 'rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m])' | jq '.[0].value[1]')
          if (( $(echo "$error_rate > 0.01" | bc -l) )); then
            echo "Error rate too high: $error_rate"
            exit 1
          fi
      outputs:
        - name: error_rate
          source: stdout
          parser: regex
          path: "Error rate: (\\d+\\.\\d+)"

    - name: progressive_rollout
      type: parallel
      dependsOn: [check_error_rate]
      config:
        items: [25, 50, 75, 100]
        sequential: true
        delay: 5m
      steps:
        - name: shift_traffic
          type: command
          config:
            command: |
              kubectl patch virtualservice {{ .inputs.app_name }} \
                --type=merge \
                -p '{"spec":{"http":[{"route":[{"destination":{"host":"{{ .inputs.app_name }}","subset":"stable"},"weight":{{ sub 100 .item }}},{"destination":{"host":"{{ .inputs.app_name }}","subset":"canary"},"weight":{{ .item }}}]}]}}'

  onFailure:
    - name: rollback_canary
      type: command
      config:
        command: |
          kubectl patch virtualservice {{ .inputs.app_name }} \
            --type=merge \
            -p '{"spec":{"http":[{"route":[{"destination":{"host":"{{ .inputs.app_name }}","subset":"stable"},"weight":100}]}]}}'
          kubectl delete deployment {{ .inputs.app_name }}-canary
```

## Database Runbooks

### Database Backup

Automated database backup with verification.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: database-backup
  namespace: maintenance
spec:
  description: Create and verify database backup
  inputs:
    - name: database_name
      type: string
      required: true
    - name: retention_days
      type: int
      default: 30

  steps:
    - name: create_backup
      type: command
      config:
        target: db-primary
        command: |
          backup_file="/backups/{{ .inputs.database_name }}_$(date +%Y%m%d_%H%M%S).sql.gz"
          pg_dump {{ .inputs.database_name }} | gzip > "$backup_file"
          echo "$backup_file"
      outputs:
        - name: backup_file
          source: stdout
          parser: line
          path: 0

    - name: verify_backup
      type: command
      dependsOn: [create_backup]
      config:
        target: db-primary
        command: |
          gunzip -c {{ .steps.create_backup.outputs.backup_file }} | head -100
          file_size=$(stat -f %z {{ .steps.create_backup.outputs.backup_file }})
          if [ "$file_size" -lt 1000 ]; then
            echo "Backup file too small"
            exit 1
          fi
          echo "Backup verified: $file_size bytes"

    - name: upload_to_s3
      type: command
      dependsOn: [verify_backup]
      config:
        target: db-primary
        command: |
          aws s3 cp {{ .steps.create_backup.outputs.backup_file }} \
            s3://backups/databases/{{ .inputs.database_name }}/

    - name: cleanup_old_backups
      type: command
      dependsOn: [upload_to_s3]
      config:
        target: db-primary
        command: |
          find /backups -name "{{ .inputs.database_name }}_*.sql.gz" \
            -mtime +{{ .inputs.retention_days }} -delete

  onSuccess:
    - name: log_backup
      type: api
      config:
        method: POST
        url: "https://backup-registry.internal/backups"
        body: |
          {
            "database": "{{ .inputs.database_name }}",
            "file": "{{ .steps.create_backup.outputs.backup_file }}",
            "timestamp": "{{ .now }}",
            "status": "success"
          }
```

### Database Migration

Safe database schema migration with validation.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: database-migration
  namespace: maintenance
spec:
  description: Apply database migrations with rollback support
  inputs:
    - name: database_name
      type: string
      required: true
    - name: migration_version
      type: string
      required: true

  steps:
    - name: backup_before
      type: subrunbook
      config:
        runbook: database-backup
        inputs:
          database_name: "{{ .inputs.database_name }}"

    - name: dry_run
      type: command
      dependsOn: [backup_before]
      config:
        target: db-primary
        command: |
          flyway -url=jdbc:postgresql://localhost/{{ .inputs.database_name }} \
            -target={{ .inputs.migration_version }} \
            migrate -dryRun

    - name: request_approval
      type: approval
      dependsOn: [dry_run]
      config:
        message: |
          Database migration dry-run completed.
          Target version: {{ .inputs.migration_version }}
          Please review the changes and approve.
        approvers:
          - group: dba-team
        timeout: 2h

    - name: apply_migration
      type: command
      dependsOn: [request_approval]
      config:
        target: db-primary
        command: |
          flyway -url=jdbc:postgresql://localhost/{{ .inputs.database_name }} \
            -target={{ .inputs.migration_version }} \
            migrate

    - name: validate_schema
      type: command
      dependsOn: [apply_migration]
      config:
        target: db-primary
        command: |
          flyway -url=jdbc:postgresql://localhost/{{ .inputs.database_name }} \
            validate

  onFailure:
    - name: notify_dba
      type: notification
      config:
        channel: pagerduty
        message: |
          Database migration failed!
          Database: {{ .inputs.database_name }}
          Target: {{ .inputs.migration_version }}
          Backup available: {{ .steps.backup_before.outputs.backup_file }}
```

## Incident Response Runbooks

### Service Restart

Emergency service restart with diagnostics.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: emergency-restart
  namespace: incidents
spec:
  description: Emergency service restart with diagnostics capture
  inputs:
    - name: service_name
      type: string
      required: true
    - name: target
      type: string
      required: true
    - name: incident_id
      type: string
      required: false

  steps:
    - name: capture_diagnostics
      type: command
      config:
        target: "{{ .inputs.target }}"
        command: |
          mkdir -p /tmp/diagnostics/{{ .execution.id }}
          ps aux > /tmp/diagnostics/{{ .execution.id }}/ps.txt
          netstat -tlnp > /tmp/diagnostics/{{ .execution.id }}/netstat.txt
          journalctl -u {{ .inputs.service_name }} --since "10 minutes ago" \
            > /tmp/diagnostics/{{ .execution.id }}/logs.txt
          tar -czf /tmp/diagnostics-{{ .execution.id }}.tar.gz \
            /tmp/diagnostics/{{ .execution.id }}

    - name: graceful_stop
      type: command
      dependsOn: [capture_diagnostics]
      config:
        target: "{{ .inputs.target }}"
        command: |
          systemctl stop {{ .inputs.service_name }}
        timeout: 2m
      continueOnError: true

    - name: force_kill
      type: command
      dependsOn: [graceful_stop]
      condition: "{{ ne .steps.graceful_stop.state \"completed\" }}"
      config:
        target: "{{ .inputs.target }}"
        command: |
          pkill -9 -f {{ .inputs.service_name }}
          sleep 5

    - name: start_service
      type: command
      dependsOn: [graceful_stop, force_kill]
      config:
        target: "{{ .inputs.target }}"
        command: |
          systemctl start {{ .inputs.service_name }}

    - name: verify_health
      type: wait
      dependsOn: [start_service]
      config:
        poll_interval: 5s
        timeout: 2m
        condition: |
          {{ eq (exec "systemctl is-active {{ .inputs.service_name }}") "active" }}

    - name: upload_diagnostics
      type: command
      dependsOn: [verify_health]
      config:
        target: "{{ .inputs.target }}"
        command: |
          aws s3 cp /tmp/diagnostics-{{ .execution.id }}.tar.gz \
            s3://incident-artifacts/{{ .inputs.incident_id | default .execution.id }}/

  onSuccess:
    - name: update_incident
      type: itsm
      condition: "{{ .inputs.incident_id }}"
      config:
        provider: pagerduty
        action: add_note
        incident_id: "{{ .inputs.incident_id }}"
        note: |
          Service {{ .inputs.service_name }} restarted successfully.
          Diagnostics: s3://incident-artifacts/{{ .inputs.incident_id }}/
```

### Disk Space Cleanup

Automated disk space recovery.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: disk-cleanup
  namespace: incidents
spec:
  description: Recover disk space on target hosts
  inputs:
    - name: target
      type: string
      required: true
    - name: threshold_percent
      type: int
      default: 90

  steps:
    - name: check_disk_usage
      type: command
      config:
        target: "{{ .inputs.target }}"
        command: |
          df -h / | awk 'NR==2 {print $5}' | tr -d '%'
      outputs:
        - name: usage_percent
          source: stdout
          parser: line
          path: 0

    - name: should_cleanup
      type: branch
      dependsOn: [check_disk_usage]
      config:
        expression: "{{ gt (int .steps.check_disk_usage.outputs.usage_percent) .inputs.threshold_percent }}"
        cases:
          true:
            - name: cleanup_logs
              type: command
              config:
                target: "{{ .inputs.target }}"
                command: |
                  journalctl --vacuum-size=500M
                  find /var/log -name "*.gz" -mtime +7 -delete
                  find /var/log -name "*.log.*" -mtime +7 -delete

            - name: cleanup_apt
              type: command
              dependsOn: [cleanup_logs]
              config:
                target: "{{ .inputs.target }}"
                command: |
                  apt-get clean
                  apt-get autoremove -y

            - name: cleanup_docker
              type: command
              dependsOn: [cleanup_apt]
              config:
                target: "{{ .inputs.target }}"
                command: |
                  docker system prune -af --volumes
              continueOnError: true

            - name: final_check
              type: command
              dependsOn: [cleanup_docker]
              config:
                target: "{{ .inputs.target }}"
                command: |
                  df -h /
          false:
            - name: skip_cleanup
              type: noop
              config:
                message: "Disk usage {{ .steps.check_disk_usage.outputs.usage_percent }}% is below threshold"
```

## Maintenance Runbooks

### Certificate Rotation

Rotate TLS certificates with zero downtime.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: certificate-rotation
  namespace: maintenance
spec:
  description: Rotate TLS certificates across services
  inputs:
    - name: domain
      type: string
      required: true
    - name: services
      type: list
      required: true

  steps:
    - name: generate_certificate
      type: command
      config:
        target: pki-server
        command: |
          certbot certonly --dns-cloudflare \
            -d {{ .inputs.domain }} \
            --cert-name {{ .inputs.domain }} \
            --non-interactive

    - name: verify_certificate
      type: command
      dependsOn: [generate_certificate]
      config:
        target: pki-server
        command: |
          openssl x509 -in /etc/letsencrypt/live/{{ .inputs.domain }}/fullchain.pem \
            -noout -dates -subject

          # Verify expiry is > 30 days
          expiry=$(openssl x509 -in /etc/letsencrypt/live/{{ .inputs.domain }}/fullchain.pem \
            -noout -enddate | cut -d= -f2)
          expiry_epoch=$(date -d "$expiry" +%s)
          now_epoch=$(date +%s)
          days_left=$(( (expiry_epoch - now_epoch) / 86400 ))

          if [ $days_left -lt 30 ]; then
            echo "Certificate expires in $days_left days"
            exit 1
          fi

    - name: distribute_certificates
      type: parallel
      dependsOn: [verify_certificate]
      config:
        items: "{{ .inputs.services }}"
        max_parallel: 5
      steps:
        - name: copy_cert
          type: command
          config:
            target: "{{ .item }}"
            command: |
              scp pki-server:/etc/letsencrypt/live/{{ .inputs.domain }}/fullchain.pem /etc/ssl/certs/
              scp pki-server:/etc/letsencrypt/live/{{ .inputs.domain }}/privkey.pem /etc/ssl/private/
              chmod 600 /etc/ssl/private/privkey.pem

        - name: reload_service
          type: command
          dependsOn: [copy_cert]
          config:
            target: "{{ .item }}"
            command: |
              systemctl reload nginx || systemctl reload apache2 || true

    - name: verify_tls
      type: parallel
      dependsOn: [distribute_certificates]
      config:
        items: "{{ .inputs.services }}"
      steps:
        - name: check_tls
          type: command
          config:
            target: control-plane
            command: |
              echo | openssl s_client -connect {{ .item }}:443 -servername {{ .inputs.domain }} 2>/dev/null \
                | openssl x509 -noout -dates

  onSuccess:
    - name: log_rotation
      type: api
      config:
        method: POST
        url: "https://audit.internal/certificates"
        body: |
          {
            "domain": "{{ .inputs.domain }}",
            "rotated_at": "{{ .now }}",
            "services": {{ .inputs.services | toJson }}
          }
```

## Scheduled Runbooks

### Weekly Maintenance

Comprehensive weekly maintenance tasks.

```yaml
apiVersion: runbook.keystone.io/v1
kind: Runbook
metadata:
  name: weekly-maintenance
  namespace: maintenance
spec:
  description: Weekly system maintenance tasks
  triggers:
    - type: schedule
      config:
        cron: "0 2 * * 0"  # Sunday at 2 AM
        timezone: UTC

  steps:
    - name: backup_databases
      type: parallel
      config:
        items:
          - postgres-main
          - postgres-analytics
          - mysql-legacy
      steps:
        - name: backup_db
          type: subrunbook
          config:
            runbook: database-backup
            inputs:
              database_name: "{{ .item }}"

    - name: update_packages
      type: parallel
      dependsOn: [backup_databases]
      config:
        items: "{{ targets \"group=app-servers\" }}"
        max_parallel: 10
      steps:
        - name: apt_update
          type: command
          config:
            target: "{{ .item }}"
            command: |
              apt-get update
              apt-get upgrade -y --with-new-pkgs
              apt-get autoremove -y

    - name: rotate_logs
      type: command
      dependsOn: [update_packages]
      config:
        target: "group=all"
        command: |
          logrotate -f /etc/logrotate.conf

    - name: cleanup_docker
      type: command
      dependsOn: [rotate_logs]
      config:
        target: "group=docker-hosts"
        command: |
          docker system prune -af --filter "until=168h"

    - name: verify_health
      type: parallel
      dependsOn: [cleanup_docker]
      config:
        items: "{{ targets \"group=app-servers\" }}"
      steps:
        - name: health_check
          type: api
          config:
            method: GET
            url: "http://{{ .item }}:8080/health"
            expected_status: 200

  onSuccess:
    - name: report
      type: notification
      config:
        channel: email
        to: ops-team@example.com
        subject: "Weekly Maintenance Report - {{ .now | date \"2006-01-02\" }}"
        body: |
          Weekly maintenance completed successfully.

          - Databases backed up: 3
          - Servers updated: {{ len (targets "group=app-servers") }}
          - Duration: {{ .execution.duration }}
```
