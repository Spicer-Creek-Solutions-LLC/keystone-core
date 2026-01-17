package deprecation

// Epic30Migrations defines all command migrations planned for Epic 30 (CLI UX Restructuring).
// This file serves as the single source of truth for deprecated command paths and their replacements.

// RegisterEpic30Migrations registers all Epic 30 command migrations.
// Call this during CLI initialization to populate the migrations registry.
func RegisterEpic30Migrations() {
	// Blueprint command split (17 subcommands → 3 commands)
	// kscore-blueprint-publish: Publication workflow
	DefaultMigrations.Register("kscore-blueprint publish", "kscore-blueprint-publish publish")
	DefaultMigrations.Register("kscore-blueprint sign", "kscore-blueprint-publish sign")
	DefaultMigrations.Register("kscore-blueprint verify", "kscore-blueprint-publish verify")
	DefaultMigrations.Register("kscore-blueprint versions", "kscore-blueprint-publish versions")
	DefaultMigrations.Register("kscore-blueprint docs", "kscore-blueprint-publish docs")

	// kscore-blueprint-state: State/snapshot management
	DefaultMigrations.Register("kscore-blueprint snapshot", "kscore-blueprint-state snapshot")
	DefaultMigrations.Register("kscore-blueprint rollback", "kscore-blueprint-state rollback")

	// Identity/Federation split
	DefaultMigrations.Register("kscore-identity federation list", "kscore-federation list")
	DefaultMigrations.Register("kscore-identity federation add", "kscore-federation add")
	DefaultMigrations.Register("kscore-identity federation show", "kscore-federation show")
	DefaultMigrations.Register("kscore-identity federation suspend", "kscore-federation suspend")
	DefaultMigrations.Register("kscore-identity federation activate", "kscore-federation activate")
	DefaultMigrations.Register("kscore-identity federation remove", "kscore-federation remove")
	DefaultMigrations.Register("kscore-identity federation refresh", "kscore-federation refresh")

	// Cluster/Backup split
	DefaultMigrations.Register("kscore-cluster backup", "kscore-cluster-backup create")
	DefaultMigrations.Register("kscore-cluster restore", "kscore-cluster-backup restore")

	// Files/Storage split
	DefaultMigrations.Register("kscore-files backend", "kscore-files-storage backend")
	DefaultMigrations.Register("kscore-files cache", "kscore-files-storage cache")
	DefaultMigrations.Register("kscore-files namespace", "kscore-files-storage namespace")
	DefaultMigrations.Register("kscore-files mirrors", "kscore-files-storage mirrors")

	// Policy/Audit split
	DefaultMigrations.Register("kscore-policy audit", "kscore-audit log")
	DefaultMigrations.Register("kscore-policy report", "kscore-audit report")

	// GitOps/Webhook split
	DefaultMigrations.Register("kscore-gitops webhook list", "kscore-webhook list")
	DefaultMigrations.Register("kscore-gitops webhook test", "kscore-webhook test")
}

// BlueprintPublishDeprecations returns deprecation info for commands moving to kscore-blueprint-publish.
func BlueprintPublishDeprecations() map[string]*Info {
	return map[string]*Info{
		"publish": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-publish publish",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-publish",
			Message:        "Publishing commands have been moved to a dedicated kscore-blueprint-publish command.",
		},
		"sign": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-publish sign",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-publish",
			Message:        "Signing commands have been moved to kscore-blueprint-publish.",
		},
		"verify": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-publish verify",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-publish",
			Message:        "Verification commands have been moved to kscore-blueprint-publish.",
		},
		"versions": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-publish versions",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-publish",
			Message:        "Version management has been moved to kscore-blueprint-publish.",
		},
		"docs": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-publish docs",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-publish",
			Message:        "Documentation generation has been moved to kscore-blueprint-publish.",
		},
	}
}

// BlueprintStateDeprecations returns deprecation info for commands moving to kscore-blueprint-state.
func BlueprintStateDeprecations() map[string]*Info {
	return map[string]*Info{
		"snapshot": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-state snapshot",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-state",
			Message:        "Snapshot management has been moved to kscore-blueprint-state.",
		},
		"rollback": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-blueprint-state rollback",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#blueprint-state",
			Message:        "Rollback functionality has been moved to kscore-blueprint-state.",
		},
	}
}

// FederationDeprecations returns deprecation info for commands moving to kscore-federation.
func FederationDeprecations() map[string]*Info {
	base := &Info{
		DeprecatedIn:   "0.30.0",
		RemoveIn:       "1.0.0",
		MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#federation",
		Message:        "Federation commands have been moved to the dedicated kscore-federation command.",
	}

	return map[string]*Info{
		"list":     {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation list", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"add":      {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation add", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"show":     {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation show", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"suspend":  {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation suspend", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"activate": {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation activate", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"remove":   {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation remove", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"refresh":  {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-federation refresh", MigrationGuide: base.MigrationGuide, Message: base.Message},
	}
}

// ClusterBackupDeprecations returns deprecation info for commands moving to kscore-cluster-backup.
func ClusterBackupDeprecations() map[string]*Info {
	return map[string]*Info{
		"backup": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-cluster-backup create",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#cluster-backup",
			Message:        "Backup commands have been moved to kscore-cluster-backup.",
		},
		"restore": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-cluster-backup restore",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#cluster-backup",
			Message:        "Restore commands have been moved to kscore-cluster-backup.",
		},
	}
}

// FilesStorageDeprecations returns deprecation info for commands moving to kscore-files-storage.
func FilesStorageDeprecations() map[string]*Info {
	base := &Info{
		DeprecatedIn:   "0.30.0",
		RemoveIn:       "1.0.0",
		MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#files-storage",
		Message:        "Storage administration commands have been moved to kscore-files-storage.",
	}

	return map[string]*Info{
		"backend":   {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-files-storage backend", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"cache":     {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-files-storage cache", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"namespace": {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-files-storage namespace", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"mirrors":   {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-files-storage mirrors", MigrationGuide: base.MigrationGuide, Message: base.Message},
	}
}

// AuditDeprecations returns deprecation info for commands moving to kscore-audit.
func AuditDeprecations() map[string]*Info {
	return map[string]*Info{
		"audit": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-audit log",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#audit",
			Message:        "Audit commands have been moved to the dedicated kscore-audit command.",
		},
		"report": {
			DeprecatedIn:   "0.30.0",
			RemoveIn:       "1.0.0",
			Replacement:    "kscore-audit report",
			MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#audit",
			Message:        "Compliance reporting has been moved to kscore-audit.",
		},
	}
}

// WebhookDeprecations returns deprecation info for commands moving to kscore-webhook.
func WebhookDeprecations() map[string]*Info {
	base := &Info{
		DeprecatedIn:   "0.30.0",
		RemoveIn:       "1.0.0",
		MigrationGuide: "https://docs.keystonecore.io/cli/migration/epic-30#webhook",
		Message:        "Webhook commands have been moved to the dedicated kscore-webhook command.",
	}

	return map[string]*Info{
		"list": {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-webhook list", MigrationGuide: base.MigrationGuide, Message: base.Message},
		"test": {DeprecatedIn: base.DeprecatedIn, RemoveIn: base.RemoveIn, Replacement: "kscore-webhook test", MigrationGuide: base.MigrationGuide, Message: base.Message},
	}
}
