// Package selfmgmt provides self-management capabilities for Keystone Core components.
package selfmgmt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"
)

// BackupModule manages backup state for Keystone Core.
type BackupModule struct {
	name        string
	validStates []ComponentState
}

// NewBackupModule creates a new backup state module.
func NewBackupModule() *BackupModule {
	return &BackupModule{
		name: "kscore_backup",
		validStates: []ComponentState{
			StateEnabled,
			StateDisabled,
			StateConfigured,
		},
	}
}

// Name returns the module name.
func (m *BackupModule) Name() string {
	return m.name
}

// ComponentType returns the component type.
func (m *BackupModule) ComponentType() ComponentType {
	return ComponentBackup
}

// ValidStates returns valid states for backup.
func (m *BackupModule) ValidStates() []ComponentState {
	return m.validStates
}

// Validate validates the backup configuration.
func (m *BackupModule) Validate(config interface{}) error {
	cfg, ok := config.(*BackupConfig)
	if !ok {
		return fmt.Errorf("invalid config type: expected *BackupConfig")
	}

	var errs ValidationErrors

	// Validate state
	if cfg.State == "" {
		errs = append(errs, ValidationError{Field: "state", Message: "state is required"})
	} else {
		valid := false
		for _, s := range m.validStates {
			if cfg.State == s {
				valid = true
				break
			}
		}
		if !valid {
			errs = append(errs, ValidationError{
				Field:   "state",
				Message: fmt.Sprintf("invalid state: %s", cfg.State),
			})
		}
	}

	// Validate schedule and destinations for enabled state
	if cfg.State == StateEnabled || cfg.State == StateConfigured {
		if cfg.Schedule == "" {
			errs = append(errs, ValidationError{Field: "schedule", Message: "schedule is required for enabled state"})
		} else if !m.isValidCronSchedule(cfg.Schedule) {
			errs = append(errs, ValidationError{Field: "schedule", Message: "invalid cron schedule format"})
		}

		if len(cfg.Destinations) == 0 {
			errs = append(errs, ValidationError{Field: "destinations", Message: "at least one destination is required"})
		}
	}

	// Validate retention
	if cfg.Retention != nil && cfg.Retention.KeepDaily < 0 {
		errs = append(errs, ValidationError{Field: "retention.keep_daily", Message: "must be >= 0"})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// Check checks the current state of backup configuration.
func (m *BackupModule) Check(ctx context.Context, config interface{}) (*CheckResult, error) {
	cfg, ok := config.(*BackupConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *BackupConfig")
	}

	if err := m.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	result := &CheckResult{
		Component:    ComponentBackup,
		CurrentState: StateDisabled,
		DesiredState: cfg.State,
		Matches:      false,
	}

	// Check if backup is configured
	backupConfigPath := m.getBackupConfigPath()
	cronExists := m.backupCronExists()

	if cronExists {
		result.CurrentState = StateEnabled
		result.Enabled = true
	} else if _, err := os.Stat(backupConfigPath); err == nil {
		result.CurrentState = StateConfigured
		result.ConfigValid = true
	}

	result.Matches = (result.CurrentState == cfg.State)
	return result, nil
}

// Apply applies the desired backup state.
func (m *BackupModule) Apply(ctx context.Context, config interface{}, dryRun bool) (*ApplyResult, error) {
	cfg, ok := config.(*BackupConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type: expected *BackupConfig")
	}

	checkResult, err := m.Check(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("check failed: %w", err)
	}

	result := &ApplyResult{
		Component:     ComponentBackup,
		Success:       true,
		Changed:       false,
		Changes:       make(map[string]interface{}),
		PreviousState: checkResult.CurrentState,
		NewState:      checkResult.CurrentState,
	}

	if checkResult.Matches {
		return result, nil
	}

	if dryRun {
		result.Changes["action"] = fmt.Sprintf("Would change from %s to %s", checkResult.CurrentState, cfg.State)
		result.Changed = true
		result.NewState = cfg.State
		return result, nil
	}

	switch cfg.State {
	case StateEnabled:
		// Write backup config
		if err := m.writeBackupConfig(cfg); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["wrote_config"] = true
		result.Changed = true

		// Write backup script
		if err := m.writeBackupScript(cfg); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["wrote_script"] = true

		// Create cron job or systemd timer
		if err := m.enableBackupSchedule(cfg); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["enabled_schedule"] = cfg.Schedule

	case StateDisabled:
		// Remove cron job or systemd timer
		if err := m.disableBackupSchedule(); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["disabled_schedule"] = true
		result.Changed = true

	case StateConfigured:
		// Write backup config
		if err := m.writeBackupConfig(cfg); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["wrote_config"] = true
		result.Changed = true

		// Write backup script
		if err := m.writeBackupScript(cfg); err != nil {
			result.Success = false
			result.Error = err
			return result, nil
		}
		result.Changes["wrote_script"] = true
	}

	result.NewState = cfg.State
	return result, nil
}

// getBackupConfigPath returns the backup configuration path.
func (m *BackupModule) getBackupConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("ProgramData"), "kscore", "backup.json")
	default:
		return "/etc/kscore/backup.json"
	}
}

// getBackupScriptPath returns the backup script path.
func (m *BackupModule) getBackupScriptPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("ProgramData"), "kscore", "backup.ps1")
	default:
		return "/usr/local/bin/kscore-backup"
	}
}

// backupCronExists checks if backup cron/timer exists.
func (m *BackupModule) backupCronExists() bool {
	initSystem := DetectInitSystem()

	switch initSystem {
	case "systemd":
		cmd := exec.Command("systemctl", "is-enabled", "--quiet", "kscore-backup.timer")
		return cmd.Run() == nil
	case "launchd":
		plistPath := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents", "com.keystone.backup.plist")
		if _, err := os.Stat(plistPath); err == nil {
			return true
		}
		plistPath = "/Library/LaunchDaemons/com.keystone.backup.plist"
		_, err := os.Stat(plistPath)
		return err == nil
	default:
		cmd := exec.Command("crontab", "-l")
		output, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(output), "kscore-backup")
	}
}

// writeBackupConfig writes the backup configuration file.
func (m *BackupModule) writeBackupConfig(cfg *BackupConfig) error {
	configPath := m.getBackupConfigPath()

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	content, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, content, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// writeBackupScript writes the backup script.
func (m *BackupModule) writeBackupScript(cfg *BackupConfig) error {
	scriptPath := m.getBackupScriptPath()

	dir := filepath.Dir(scriptPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create script directory: %w", err)
	}

	var script string
	if runtime.GOOS == "windows" {
		script = m.buildWindowsBackupScript(cfg)
	} else {
		script = m.buildUnixBackupScript(cfg)
	}

	perm := os.FileMode(0755)
	if runtime.GOOS == "windows" {
		perm = 0644
	}

	if err := os.WriteFile(scriptPath, []byte(script), perm); err != nil {
		return fmt.Errorf("failed to write script: %w", err)
	}

	return nil
}

// buildUnixBackupScript builds a Unix backup script.
func (m *BackupModule) buildUnixBackupScript(cfg *BackupConfig) string {
	destination := cfg.GetDestination()
	retentionDays := cfg.GetRetentionDays()

	tmpl := `#!/bin/bash
# Keystone Core Backup Script
# Generated by kscore self-management
# DO NOT EDIT - changes will be overwritten

set -e

CONFIG_FILE="{{.ConfigPath}}"
DESTINATION="{{.Destination}}"
RETENTION_DAYS={{.RetentionDays}}
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="kscore_backup_${TIMESTAMP}"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

log "Starting Keystone Core backup"

# Create backup directory
BACKUP_DIR="${DESTINATION}/${BACKUP_NAME}"
mkdir -p "${BACKUP_DIR}"

# Backup configuration
log "Backing up configuration"
if [ -d "/etc/kscore" ]; then
    cp -r /etc/kscore "${BACKUP_DIR}/config"
fi

# Backup SQLite database if exists
if [ -f "/var/lib/kscore/state.db" ]; then
    log "Backing up SQLite database"
    sqlite3 /var/lib/kscore/state.db ".backup '${BACKUP_DIR}/state.db'"
fi

# Backup NATS JetStream if exists
if [ -d "/var/lib/kscore/jetstream" ]; then
    log "Backing up JetStream data"
    cp -r /var/lib/kscore/jetstream "${BACKUP_DIR}/jetstream"
fi

{{if .EncryptionKey}}
# Encrypt backup
log "Encrypting backup"
tar -czf "${DESTINATION}/${BACKUP_NAME}.tar.gz" -C "${DESTINATION}" "${BACKUP_NAME}"
openssl enc -aes-256-cbc -pbkdf2 -in "${DESTINATION}/${BACKUP_NAME}.tar.gz" \
    -out "${DESTINATION}/${BACKUP_NAME}.tar.gz.enc" -pass pass:"{{.EncryptionKey}}"
rm -rf "${BACKUP_DIR}" "${DESTINATION}/${BACKUP_NAME}.tar.gz"
BACKUP_FILE="${DESTINATION}/${BACKUP_NAME}.tar.gz.enc"
{{else}}
# Create archive
log "Creating archive"
tar -czf "${DESTINATION}/${BACKUP_NAME}.tar.gz" -C "${DESTINATION}" "${BACKUP_NAME}"
rm -rf "${BACKUP_DIR}"
BACKUP_FILE="${DESTINATION}/${BACKUP_NAME}.tar.gz"
{{end}}

# Apply retention policy
if [ ${RETENTION_DAYS} -gt 0 ]; then
    log "Applying retention policy (keeping ${RETENTION_DAYS} days)"
    find "${DESTINATION}" -name "kscore_backup_*.tar.gz*" -mtime +${RETENTION_DAYS} -delete
fi

log "Backup completed: ${BACKUP_FILE}"
`

	t := template.Must(template.New("backup").Parse(tmpl))
	var buf strings.Builder
	t.Execute(&buf, map[string]interface{}{
		"ConfigPath":    m.getBackupConfigPath(),
		"Destination":   destination,
		"RetentionDays": retentionDays,
		"EncryptionKey": cfg.EncryptionKey,
	})
	return buf.String()
}

// buildWindowsBackupScript builds a Windows backup script.
func (m *BackupModule) buildWindowsBackupScript(cfg *BackupConfig) string {
	destination := cfg.GetDestination()
	retentionDays := cfg.GetRetentionDays()

	tmpl := `# Keystone Core Backup Script
# Generated by kscore self-management
# DO NOT EDIT - changes will be overwritten

$ErrorActionPreference = "Stop"

$ConfigFile = "{{.ConfigPath}}"
$Destination = "{{.Destination}}"
$RetentionDays = {{.RetentionDays}}
$Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
$BackupName = "kscore_backup_$Timestamp"

function Log {
    param([string]$Message)
    Write-Host "[$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')] $Message"
}

Log "Starting Keystone Core backup"

# Create backup directory
$BackupDir = Join-Path $Destination $BackupName
New-Item -ItemType Directory -Path $BackupDir -Force | Out-Null

# Backup configuration
Log "Backing up configuration"
$ConfigDir = "$env:ProgramData\kscore"
if (Test-Path $ConfigDir) {
    Copy-Item -Path $ConfigDir -Destination (Join-Path $BackupDir "config") -Recurse
}

# Backup SQLite database if exists
$DbPath = "$env:ProgramData\kscore\data\state.db"
if (Test-Path $DbPath) {
    Log "Backing up SQLite database"
    Copy-Item -Path $DbPath -Destination (Join-Path $BackupDir "state.db")
}

# Create archive
Log "Creating archive"
$ArchivePath = Join-Path $Destination "$BackupName.zip"
Compress-Archive -Path $BackupDir -DestinationPath $ArchivePath
Remove-Item -Path $BackupDir -Recurse -Force

# Apply retention policy
if ($RetentionDays -gt 0) {
    Log "Applying retention policy (keeping $RetentionDays days)"
    $CutoffDate = (Get-Date).AddDays(-$RetentionDays)
    Get-ChildItem -Path $Destination -Filter "kscore_backup_*.zip" |
        Where-Object { $_.LastWriteTime -lt $CutoffDate } |
        Remove-Item -Force
}

Log "Backup completed: $ArchivePath"
`

	t := template.Must(template.New("backup").Parse(tmpl))
	var buf strings.Builder
	t.Execute(&buf, map[string]interface{}{
		"ConfigPath":    m.getBackupConfigPath(),
		"Destination":   destination,
		"RetentionDays": retentionDays,
		"EncryptionKey": cfg.EncryptionKey,
	})
	return buf.String()
}

// enableBackupSchedule enables the backup schedule.
func (m *BackupModule) enableBackupSchedule(cfg *BackupConfig) error {
	initSystem := DetectInitSystem()

	switch initSystem {
	case "systemd":
		return m.enableSystemdTimer(cfg)
	case "launchd":
		return m.enableLaunchdJob(cfg)
	case "windows":
		return m.enableWindowsTask(cfg)
	default:
		return m.enableCronJob(cfg)
	}
}

// disableBackupSchedule disables the backup schedule.
func (m *BackupModule) disableBackupSchedule() error {
	initSystem := DetectInitSystem()

	switch initSystem {
	case "systemd":
		return m.disableSystemdTimer()
	case "launchd":
		return m.disableLaunchdJob()
	case "windows":
		return m.disableWindowsTask()
	default:
		return m.disableCronJob()
	}
}

// enableSystemdTimer creates and enables a systemd timer.
func (m *BackupModule) enableSystemdTimer(cfg *BackupConfig) error {
	serviceContent := fmt.Sprintf(`[Unit]
Description=Keystone Core Backup Service
After=network.target

[Service]
Type=oneshot
ExecStart=%s
User=root

[Install]
WantedBy=multi-user.target
`, m.getBackupScriptPath())

	servicePath := "/etc/systemd/system/kscore-backup.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service unit: %w", err)
	}

	onCalendar := m.cronToSystemdCalendar(cfg.Schedule)

	timerContent := fmt.Sprintf(`[Unit]
Description=Keystone Core Backup Timer

[Timer]
OnCalendar=%s
Persistent=true
RandomizedDelaySec=300

[Install]
WantedBy=timers.target
`, onCalendar)

	timerPath := "/etc/systemd/system/kscore-backup.timer"
	if err := os.WriteFile(timerPath, []byte(timerContent), 0644); err != nil {
		return fmt.Errorf("failed to write timer unit: %w", err)
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	if err := exec.Command("systemctl", "enable", "--now", "kscore-backup.timer").Run(); err != nil {
		return fmt.Errorf("failed to enable timer: %w", err)
	}

	return nil
}

// disableSystemdTimer disables and removes the systemd timer.
func (m *BackupModule) disableSystemdTimer() error {
	exec.Command("systemctl", "stop", "kscore-backup.timer").Run()
	exec.Command("systemctl", "disable", "kscore-backup.timer").Run()
	os.Remove("/etc/systemd/system/kscore-backup.service")
	os.Remove("/etc/systemd/system/kscore-backup.timer")
	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

// enableLaunchdJob creates a launchd plist.
func (m *BackupModule) enableLaunchdJob(cfg *BackupConfig) error {
	interval := m.cronToLaunchdInterval(cfg.Schedule)

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.keystone.backup</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>StartCalendarInterval</key>
    %s
    <key>StandardOutPath</key>
    <string>/var/log/kscore-backup.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/kscore-backup.log</string>
</dict>
</plist>
`, m.getBackupScriptPath(), interval)

	plistPath := "/Library/LaunchDaemons/com.keystone.backup.plist"
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		return fmt.Errorf("failed to load launchd job: %w", err)
	}

	return nil
}

// disableLaunchdJob removes the launchd job.
func (m *BackupModule) disableLaunchdJob() error {
	plistPath := "/Library/LaunchDaemons/com.keystone.backup.plist"
	exec.Command("launchctl", "unload", plistPath).Run()
	os.Remove(plistPath)
	return nil
}

// enableCronJob adds a cron entry.
func (m *BackupModule) enableCronJob(cfg *BackupConfig) error {
	cmd := exec.Command("crontab", "-l")
	output, _ := cmd.Output()

	lines := strings.Split(string(output), "\n")
	var newLines []string
	for _, line := range lines {
		if !strings.Contains(line, "kscore-backup") && strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	newLines = append(newLines, fmt.Sprintf("%s %s # kscore-backup",
		cfg.Schedule, m.getBackupScriptPath()))

	cronContent := strings.Join(newLines, "\n") + "\n"
	cmd = exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(cronContent)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update crontab: %w", err)
	}

	return nil
}

// disableCronJob removes the cron entry.
func (m *BackupModule) disableCronJob() error {
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(string(output), "\n")
	var newLines []string
	for _, line := range lines {
		if !strings.Contains(line, "kscore-backup") && strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	if len(newLines) == 0 {
		exec.Command("crontab", "-r").Run()
	} else {
		cronContent := strings.Join(newLines, "\n") + "\n"
		cmd = exec.Command("crontab", "-")
		cmd.Stdin = strings.NewReader(cronContent)
		cmd.Run()
	}

	return nil
}

// enableWindowsTask creates a Windows scheduled task.
func (m *BackupModule) enableWindowsTask(cfg *BackupConfig) error {
	trigger := m.cronToWindowsTrigger(cfg.Schedule)

	cmd := exec.Command("schtasks", "/Create", "/F",
		"/TN", "KscoreBackup",
		"/TR", fmt.Sprintf("powershell.exe -ExecutionPolicy Bypass -File \"%s\"", m.getBackupScriptPath()),
		"/SC", trigger.schedule,
		"/ST", trigger.startTime,
		"/RU", "SYSTEM")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// disableWindowsTask removes the Windows scheduled task.
func (m *BackupModule) disableWindowsTask() error {
	cmd := exec.Command("schtasks", "/Delete", "/F", "/TN", "KscoreBackup")
	return cmd.Run()
}

// isValidCronSchedule validates a cron schedule.
func (m *BackupModule) isValidCronSchedule(schedule string) bool {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return false
	}

	for _, field := range fields {
		if !m.isValidCronField(field) {
			return false
		}
	}

	return true
}

// isValidCronField validates a single cron field.
func (m *BackupModule) isValidCronField(field string) bool {
	if field == "*" {
		return true
	}

	if strings.HasPrefix(field, "*/") {
		return true
	}

	parts := strings.Split(field, ",")
	for _, part := range parts {
		if strings.Contains(part, "-") {
			continue
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}

	return true
}

// cronToSystemdCalendar converts cron schedule to systemd OnCalendar format.
func (m *BackupModule) cronToSystemdCalendar(schedule string) string {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return "daily"
	}

	minute, hour, dayMonth, month, dayWeek := fields[0], fields[1], fields[2], fields[3], fields[4]

	if minute == "0" && hour == "0" && dayMonth == "*" && month == "*" && dayWeek == "*" {
		return "daily"
	}
	if minute == "0" && hour == "0" && dayMonth == "*" && month == "*" && dayWeek == "0" {
		return "weekly"
	}
	if minute == "0" && hour == "0" && dayMonth == "1" && month == "*" && dayWeek == "*" {
		return "monthly"
	}

	var parts []string

	if dayWeek != "*" {
		parts = append(parts, m.cronDayOfWeekToSystemd(dayWeek))
	}

	datePart := "*-" + m.cronFieldToSystemd(month) + "-" + m.cronFieldToSystemd(dayMonth)
	parts = append(parts, datePart)

	timePart := m.cronFieldToSystemd(hour) + ":" + m.cronFieldToSystemd(minute) + ":00"
	parts = append(parts, timePart)

	return strings.Join(parts, " ")
}

// cronFieldToSystemd converts a cron field to systemd format.
func (m *BackupModule) cronFieldToSystemd(field string) string {
	if field == "*" {
		return "*"
	}
	return field
}

// cronDayOfWeekToSystemd converts cron day of week to systemd format.
func (m *BackupModule) cronDayOfWeekToSystemd(field string) string {
	days := map[string]string{
		"0": "Sun", "7": "Sun",
		"1": "Mon",
		"2": "Tue",
		"3": "Wed",
		"4": "Thu",
		"5": "Fri",
		"6": "Sat",
	}
	if d, ok := days[field]; ok {
		return d
	}
	return field
}

// cronToLaunchdInterval converts cron schedule to launchd StartCalendarInterval.
func (m *BackupModule) cronToLaunchdInterval(schedule string) string {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return "<dict><key>Hour</key><integer>0</integer><key>Minute</key><integer>0</integer></dict>"
	}

	minute, hour, dayMonth, month, dayWeek := fields[0], fields[1], fields[2], fields[3], fields[4]

	var interval strings.Builder
	interval.WriteString("<dict>")

	if minute != "*" {
		interval.WriteString(fmt.Sprintf("<key>Minute</key><integer>%s</integer>", minute))
	}
	if hour != "*" {
		interval.WriteString(fmt.Sprintf("<key>Hour</key><integer>%s</integer>", hour))
	}
	if dayMonth != "*" {
		interval.WriteString(fmt.Sprintf("<key>Day</key><integer>%s</integer>", dayMonth))
	}
	if month != "*" {
		interval.WriteString(fmt.Sprintf("<key>Month</key><integer>%s</integer>", month))
	}
	if dayWeek != "*" {
		interval.WriteString(fmt.Sprintf("<key>Weekday</key><integer>%s</integer>", dayWeek))
	}

	interval.WriteString("</dict>")
	return interval.String()
}

// windowsTrigger represents a Windows task trigger.
type windowsTrigger struct {
	schedule  string
	startTime string
}

// cronToWindowsTrigger converts cron schedule to Windows schtasks format.
func (m *BackupModule) cronToWindowsTrigger(schedule string) windowsTrigger {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return windowsTrigger{schedule: "DAILY", startTime: "00:00"}
	}

	minute, hour, dayMonth, _, dayWeek := fields[0], fields[1], fields[2], fields[3], fields[4]

	h := "00"
	if hour != "*" {
		h = hour
	}
	min := "00"
	if minute != "*" {
		min = minute
	}
	startTime := fmt.Sprintf("%s:%s", h, min)

	if dayWeek != "*" {
		return windowsTrigger{schedule: "WEEKLY", startTime: startTime}
	}
	if dayMonth != "*" {
		return windowsTrigger{schedule: "MONTHLY", startTime: startTime}
	}

	return windowsTrigger{schedule: "DAILY", startTime: startTime}
}

// TriggerBackup triggers an immediate backup.
func (m *BackupModule) TriggerBackup(ctx context.Context) error {
	scriptPath := m.getBackupScriptPath()
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("backup script not found: %s", scriptPath)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", scriptPath)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ListBackups lists available backups.
func (m *BackupModule) ListBackups(destination string) ([]BackupInfoResult, error) {
	var backups []BackupInfoResult

	pattern := filepath.Join(destination, "kscore_backup_*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		name := filepath.Base(match)
		var timestamp time.Time
		if strings.HasPrefix(name, "kscore_backup_") {
			tsStr := strings.TrimPrefix(name, "kscore_backup_")
			tsStr = strings.Split(tsStr, ".")[0]
			timestamp, _ = time.Parse("20060102_150405", tsStr)
		}

		backups = append(backups, BackupInfoResult{
			Name:      name,
			Path:      match,
			Size:      info.Size(),
			Timestamp: timestamp,
			Encrypted: strings.HasSuffix(match, ".enc"),
		})
	}

	return backups, nil
}

// BackupInfoResult contains information about a backup.
type BackupInfoResult struct {
	Name      string
	Path      string
	Size      int64
	Timestamp time.Time
	Encrypted bool
}
