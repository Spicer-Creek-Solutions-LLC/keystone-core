package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ============================================================================
// PostgreSQL Database Module
// ============================================================================

// PostgresDatabaseModule manages PostgreSQL databases
type PostgresDatabaseModule struct {
	*BaseModule
}

// NewPostgresDatabaseModule creates a new PostgreSQL database module
func NewPostgresDatabaseModule() *PostgresDatabaseModule {
	return &PostgresDatabaseModule{
		BaseModule: NewBaseModule("postgres_database", []string{"present", "absent"}),
	}
}

// Check verifies if the database exists and matches the desired state
func (m *PostgresDatabaseModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	// Get connection parameters
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 5432)
	user := getStringParameter(decl, "user", "postgres")
	password := getStringParameter(decl, "password", "")
	maintenance_db := getStringParameter(decl, "maintenance_db", "postgres")

	// Check if psql is available
	if _, err := exec.LookPath("psql"); err != nil {
		return nil, fmt.Errorf("psql is not available: %w", err)
	}

	// Build connection string
	connArgs := m.buildConnArgs(host, port, user, password, maintenance_db)

	// Check if database exists
	query := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s'", escapePostgresString(name))
	cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-t", "-c", query)...)
	output, err := cmd.Output()

	exists := err == nil && strings.TrimSpace(string(output)) == "1"

	result := &ModuleCheckResult{
		Present:  exists,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		// Get database details
		detailQuery := fmt.Sprintf(`
			SELECT pg_encoding_to_char(encoding), datcollate, datctype,
				   pg_catalog.pg_get_userbyid(datdba)
			FROM pg_database WHERE datname = '%s'`, escapePostgresString(name))
		cmd = exec.CommandContext(ctx, "psql", append(connArgs, "-t", "-A", "-F", "|", "-c", detailQuery)...)
		detailOutput, _ := cmd.Output()
		if parts := strings.Split(strings.TrimSpace(string(detailOutput)), "|"); len(parts) >= 4 {
			result.Metadata["encoding"] = parts[0]
			result.Metadata["collate"] = parts[1]
			result.Metadata["ctype"] = parts[2]
			result.Metadata["owner"] = parts[3]
		}
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	result.Matches = (decl.State == "present" && exists) || (decl.State == "absent" && !exists)

	return result, nil
}

// Apply creates or removes the PostgreSQL database
func (m *PostgresDatabaseModule) Apply(ctx context.Context, decl *StateDeclaration, check *ModuleCheckResult) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 5432)
	user := getStringParameter(decl, "user", "postgres")
	password := getStringParameter(decl, "password", "")
	maintenance_db := getStringParameter(decl, "maintenance_db", "postgres")

	connArgs := m.buildConnArgs(host, port, user, password, maintenance_db)

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	switch decl.State {
	case "present":
		if check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Database '%s' already exists", name)
			return result, nil
		}

		owner := getStringParameter(decl, "owner", "")
		encoding := getStringParameter(decl, "encoding", "UTF8")
		template := getStringParameter(decl, "template", "template0")
		lc_collate := getStringParameter(decl, "lc_collate", "")
		lc_ctype := getStringParameter(decl, "lc_ctype", "")

		// Build CREATE DATABASE statement
		createSQL := fmt.Sprintf("CREATE DATABASE %s", quotePostgresIdentifier(name))
		if owner != "" {
			createSQL += fmt.Sprintf(" OWNER %s", quotePostgresIdentifier(owner))
		}
		createSQL += fmt.Sprintf(" ENCODING '%s' TEMPLATE %s", encoding, template)
		if lc_collate != "" {
			createSQL += fmt.Sprintf(" LC_COLLATE '%s'", lc_collate)
		}
		if lc_ctype != "" {
			createSQL += fmt.Sprintf(" LC_CTYPE '%s'", lc_ctype)
		}

		cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-c", createSQL)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create database: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Database '%s' created", name)

	case "absent":
		if !check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Database '%s' does not exist", name)
			return result, nil
		}

		dropSQL := fmt.Sprintf("DROP DATABASE %s", quotePostgresIdentifier(name))
		cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-c", dropSQL)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to drop database: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Database '%s' dropped", name)
	}

	return result, nil
}

// Test runs a dry-run check
func (m *PostgresDatabaseModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !check.Matches,
		Comment: fmt.Sprintf("Would %s database '%s'", map[bool]string{true: "create", false: "drop"}[decl.State == "present"], getStringParameter(decl, "name", "")),
	}, nil
}

func (m *PostgresDatabaseModule) buildConnArgs(host string, port int, user, _ /* password */, database string) []string {
	// Note: For security, password should be set via PGPASSWORD environment variable or .pgpass file
	// The password parameter is accepted but not used in command args
	args := []string{"-h", host, "-p", fmt.Sprintf("%d", port), "-U", user, "-d", database}
	return args
}

// ============================================================================
// PostgreSQL User Module
// ============================================================================

// PostgresUserModule manages PostgreSQL users/roles
type PostgresUserModule struct {
	*BaseModule
}

// NewPostgresUserModule creates a new PostgreSQL user module
func NewPostgresUserModule() *PostgresUserModule {
	return &PostgresUserModule{
		BaseModule: NewBaseModule("postgres_user", []string{"present", "absent"}),
	}
}

// Check verifies if the user exists and matches the desired state
func (m *PostgresUserModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 5432)
	user := getStringParameter(decl, "user", "postgres")
	// password should be set via PGPASSWORD env var for security
	_ = getStringParameter(decl, "password", "")
	maintenance_db := getStringParameter(decl, "maintenance_db", "postgres")

	if _, err := exec.LookPath("psql"); err != nil {
		return nil, fmt.Errorf("psql is not available: %w", err)
	}

	connArgs := []string{"-h", host, "-p", fmt.Sprintf("%d", port), "-U", user, "-d", maintenance_db}

	// Check if role exists
	query := fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname = '%s'", escapePostgresString(name))
	cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-t", "-c", query)...)
	output, err := cmd.Output()

	exists := err == nil && strings.TrimSpace(string(output)) == "1"

	result := &ModuleCheckResult{
		Present:  exists,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		// Get role details
		detailQuery := fmt.Sprintf(`
			SELECT rolsuper, rolinherit, rolcreaterole, rolcreatedb,
				   rolcanlogin, rolreplication, rolconnlimit
			FROM pg_roles WHERE rolname = '%s'`, escapePostgresString(name))
		cmd = exec.CommandContext(ctx, "psql", append(connArgs, "-t", "-A", "-F", "|", "-c", detailQuery)...)
		detailOutput, _ := cmd.Output()
		if parts := strings.Split(strings.TrimSpace(string(detailOutput)), "|"); len(parts) >= 7 {
			result.Metadata["superuser"] = parts[0] == "t"
			result.Metadata["inherit"] = parts[1] == "t"
			result.Metadata["createrole"] = parts[2] == "t"
			result.Metadata["createdb"] = parts[3] == "t"
			result.Metadata["login"] = parts[4] == "t"
			result.Metadata["replication"] = parts[5] == "t"
			result.Metadata["connection_limit"] = parts[6]
		}
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	result.Matches = (decl.State == "present" && exists) || (decl.State == "absent" && !exists)

	return result, nil
}

// Apply creates or removes the PostgreSQL user
func (m *PostgresUserModule) Apply(ctx context.Context, decl *StateDeclaration, check *ModuleCheckResult) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 5432)
	adminUser := getStringParameter(decl, "user", "postgres")
	adminPassword := getStringParameter(decl, "password", "")
	maintenance_db := getStringParameter(decl, "maintenance_db", "postgres")

	connArgs := []string{"-h", host, "-p", fmt.Sprintf("%d", port), "-U", adminUser, "-d", maintenance_db}
	_ = adminPassword // Would be set via PGPASSWORD

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	switch decl.State {
	case "present":
		userPassword := getStringParameter(decl, "role_password", "")
		superuser := getBoolParameter(decl, "superuser", false)
		createdb := getBoolParameter(decl, "createdb", false)
		createrole := getBoolParameter(decl, "createrole", false)
		login := getBoolParameter(decl, "login", true)
		replication := getBoolParameter(decl, "replication", false)
		connectionLimit := getIntParameter(decl, "connection_limit", -1)

		var sql string
		if check.Present {
			// ALTER ROLE
			sql = fmt.Sprintf("ALTER ROLE %s", quotePostgresIdentifier(name))
		} else {
			// CREATE ROLE
			sql = fmt.Sprintf("CREATE ROLE %s", quotePostgresIdentifier(name))
		}

		// Add options
		if superuser {
			sql += " SUPERUSER"
		} else {
			sql += " NOSUPERUSER"
		}
		if createdb {
			sql += " CREATEDB"
		} else {
			sql += " NOCREATEDB"
		}
		if createrole {
			sql += " CREATEROLE"
		} else {
			sql += " NOCREATEROLE"
		}
		if login {
			sql += " LOGIN"
		} else {
			sql += " NOLOGIN"
		}
		if replication {
			sql += " REPLICATION"
		} else {
			sql += " NOREPLICATION"
		}
		if connectionLimit >= 0 {
			sql += fmt.Sprintf(" CONNECTION LIMIT %d", connectionLimit)
		}
		if userPassword != "" {
			sql += fmt.Sprintf(" PASSWORD '%s'", escapePostgresString(userPassword))
		}

		cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-c", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to %s role: %s", map[bool]string{true: "alter", false: "create"}[check.Present], string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Role '%s' %s", name, map[bool]string{true: "updated", false: "created"}[check.Present])

	case "absent":
		if !check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Role '%s' does not exist", name)
			return result, nil
		}

		sql := fmt.Sprintf("DROP ROLE %s", quotePostgresIdentifier(name))
		cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-c", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to drop role: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Role '%s' dropped", name)
	}

	return result, nil
}

// Test runs a dry-run check
func (m *PostgresUserModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	action := "create"
	if decl.State == "absent" {
		action = "drop"
	} else if check.Present {
		action = "update"
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !check.Matches,
		Comment: fmt.Sprintf("Would %s role '%s'", action, getStringParameter(decl, "name", "")),
	}, nil
}

// ============================================================================
// PostgreSQL Extension Module
// ============================================================================

// PostgresExtensionModule manages PostgreSQL extensions
type PostgresExtensionModule struct {
	*BaseModule
}

// NewPostgresExtensionModule creates a new PostgreSQL extension module
func NewPostgresExtensionModule() *PostgresExtensionModule {
	return &PostgresExtensionModule{
		BaseModule: NewBaseModule("postgres_extension", []string{"present", "absent"}),
	}
}

// Check verifies if the extension exists
func (m *PostgresExtensionModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	database := getStringParameter(decl, "database", "")
	if database == "" {
		return nil, fmt.Errorf("database parameter is required")
	}

	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 5432)
	user := getStringParameter(decl, "user", "postgres")

	if _, err := exec.LookPath("psql"); err != nil {
		return nil, fmt.Errorf("psql is not available: %w", err)
	}

	connArgs := []string{"-h", host, "-p", fmt.Sprintf("%d", port), "-U", user, "-d", database}

	// Check if extension exists
	query := fmt.Sprintf("SELECT extversion FROM pg_extension WHERE extname = '%s'", escapePostgresString(name))
	cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-t", "-A", "-c", query)...)
	output, err := cmd.Output()

	version := strings.TrimSpace(string(output))
	exists := err == nil && version != ""

	result := &ModuleCheckResult{
		Present:  exists,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.Metadata["version"] = version
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	result.Matches = (decl.State == "present" && exists) || (decl.State == "absent" && !exists)

	return result, nil
}

// Apply creates or removes the extension
func (m *PostgresExtensionModule) Apply(ctx context.Context, decl *StateDeclaration, check *ModuleCheckResult) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	database := getStringParameter(decl, "database", "")
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 5432)
	user := getStringParameter(decl, "user", "postgres")

	connArgs := []string{"-h", host, "-p", fmt.Sprintf("%d", port), "-U", user, "-d", database}

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	switch decl.State {
	case "present":
		if check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Extension '%s' already exists in database '%s'", name, database)
			return result, nil
		}

		schema := getStringParameter(decl, "schema", "")
		version := getStringParameter(decl, "version", "")
		cascade := getBoolParameter(decl, "cascade", false)

		sql := fmt.Sprintf("CREATE EXTENSION %s", quotePostgresIdentifier(name))
		if schema != "" {
			sql += fmt.Sprintf(" SCHEMA %s", quotePostgresIdentifier(schema))
		}
		if version != "" {
			sql += fmt.Sprintf(" VERSION '%s'", escapePostgresString(version))
		}
		if cascade {
			sql += " CASCADE"
		}

		cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-c", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create extension: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Extension '%s' created in database '%s'", name, database)

	case "absent":
		if !check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Extension '%s' does not exist in database '%s'", name, database)
			return result, nil
		}

		cascade := getBoolParameter(decl, "cascade", false)
		sql := fmt.Sprintf("DROP EXTENSION %s", quotePostgresIdentifier(name))
		if cascade {
			sql += " CASCADE"
		}

		cmd := exec.CommandContext(ctx, "psql", append(connArgs, "-c", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to drop extension: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Extension '%s' dropped from database '%s'", name, database)
	}

	return result, nil
}

// Test runs a dry-run check
func (m *PostgresExtensionModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !check.Matches,
		Comment: fmt.Sprintf("Would %s extension '%s'", map[bool]string{true: "create", false: "drop"}[decl.State == "present"], getStringParameter(decl, "name", "")),
	}, nil
}

// ============================================================================
// MySQL Database Module
// ============================================================================

// MySQLDatabaseModule manages MySQL databases
type MySQLDatabaseModule struct {
	*BaseModule
}

// NewMySQLDatabaseModule creates a new MySQL database module
func NewMySQLDatabaseModule() *MySQLDatabaseModule {
	return &MySQLDatabaseModule{
		BaseModule: NewBaseModule("mysql_database", []string{"present", "absent"}),
	}
}

// Check verifies if the database exists
func (m *MySQLDatabaseModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	if _, err := exec.LookPath("mysql"); err != nil {
		return nil, fmt.Errorf("mysql is not available: %w", err)
	}

	connArgs := m.buildConnArgs(decl)

	// Check if database exists
	query := fmt.Sprintf("SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = '%s'", escapeMySQLString(name))
	cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-N", "-e", query)...)
	output, err := cmd.Output()

	exists := err == nil && strings.TrimSpace(string(output)) == name

	result := &ModuleCheckResult{
		Present:  exists,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		// Get database character set and collation
		detailQuery := fmt.Sprintf("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = '%s'", escapeMySQLString(name))
		cmd = exec.CommandContext(ctx, "mysql", append(connArgs, "-N", "-e", detailQuery)...)
		detailOutput, _ := cmd.Output()
		if parts := strings.Fields(strings.TrimSpace(string(detailOutput))); len(parts) >= 2 {
			result.Metadata["charset"] = parts[0]
			result.Metadata["collation"] = parts[1]
		}
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	result.Matches = (decl.State == "present" && exists) || (decl.State == "absent" && !exists)

	return result, nil
}

// Apply creates or removes the MySQL database
func (m *MySQLDatabaseModule) Apply(ctx context.Context, decl *StateDeclaration, check *ModuleCheckResult) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	connArgs := m.buildConnArgs(decl)

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	switch decl.State {
	case "present":
		if check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Database '%s' already exists", name)
			return result, nil
		}

		charset := getStringParameter(decl, "charset", "utf8mb4")
		collation := getStringParameter(decl, "collation", "utf8mb4_unicode_ci")

		sql := fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s COLLATE %s", escapeMySQLIdentifier(name), charset, collation)

		cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-e", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to create database: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Database '%s' created", name)

	case "absent":
		if !check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("Database '%s' does not exist", name)
			return result, nil
		}

		sql := fmt.Sprintf("DROP DATABASE `%s`", escapeMySQLIdentifier(name))
		cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-e", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to drop database: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("Database '%s' dropped", name)
	}

	return result, nil
}

// Test runs a dry-run check
func (m *MySQLDatabaseModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !check.Matches,
		Comment: fmt.Sprintf("Would %s database '%s'", map[bool]string{true: "create", false: "drop"}[decl.State == "present"], getStringParameter(decl, "name", "")),
	}, nil
}

func (m *MySQLDatabaseModule) buildConnArgs(decl *StateDeclaration) []string {
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 3306)
	user := getStringParameter(decl, "user", "root")
	password := getStringParameter(decl, "password", "")
	socket := getStringParameter(decl, "socket", "")

	var args []string
	if socket != "" {
		args = append(args, "-S", socket)
	} else {
		args = append(args, "-h", host, "-P", fmt.Sprintf("%d", port))
	}
	args = append(args, "-u", user)
	if password != "" {
		args = append(args, fmt.Sprintf("-p%s", password))
	}
	return args
}

// ============================================================================
// MySQL User Module
// ============================================================================

// MySQLUserModule manages MySQL users
type MySQLUserModule struct {
	*BaseModule
}

// NewMySQLUserModule creates a new MySQL user module
func NewMySQLUserModule() *MySQLUserModule {
	return &MySQLUserModule{
		BaseModule: NewBaseModule("mysql_user", []string{"present", "absent"}),
	}
}

// Check verifies if the user exists
func (m *MySQLUserModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required")
	}

	userHost := getStringParameter(decl, "host_name", "%") // The MySQL user's allowed host

	if _, err := exec.LookPath("mysql"); err != nil {
		return nil, fmt.Errorf("mysql is not available: %w", err)
	}

	connArgs := m.buildConnArgs(decl)

	// Check if user exists
	query := fmt.Sprintf("SELECT User FROM mysql.user WHERE User = '%s' AND Host = '%s'", escapeMySQLString(name), escapeMySQLString(userHost))
	cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-N", "-e", query)...)
	output, err := cmd.Output()

	exists := err == nil && strings.TrimSpace(string(output)) == name

	result := &ModuleCheckResult{
		Present:  exists,
		Metadata: make(map[string]interface{}),
	}

	if exists {
		result.Metadata["host"] = userHost
		result.CurrentState = "present"
	} else {
		result.CurrentState = "absent"
	}

	result.Matches = (decl.State == "present" && exists) || (decl.State == "absent" && !exists)

	return result, nil
}

// Apply creates or removes the MySQL user
func (m *MySQLUserModule) Apply(ctx context.Context, decl *StateDeclaration, check *ModuleCheckResult) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	userHost := getStringParameter(decl, "host_name", "%")
	connArgs := m.buildConnArgs(decl)

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	switch decl.State {
	case "present":
		userPassword := getStringParameter(decl, "user_password", "")
		priv := getStringParameter(decl, "priv", "")

		if !check.Present {
			// Create user
			sql := fmt.Sprintf("CREATE USER '%s'@'%s'", escapeMySQLString(name), escapeMySQLString(userHost))
			if userPassword != "" {
				sql += fmt.Sprintf(" IDENTIFIED BY '%s'", escapeMySQLString(userPassword))
			}

			cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-e", sql)...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create user: %s", string(output))
				return result, nil
			}
		}

		// Grant privileges if specified
		if priv != "" {
			// priv format: "db.table:PRIV1,PRIV2" or "db.*:ALL"
			grantSQL := m.buildGrantSQL(name, userHost, priv)
			if grantSQL != "" {
				cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-e", grantSQL)...)
				output, err := cmd.CombinedOutput()
				if err != nil {
					result.Success = false
					result.Comment = fmt.Sprintf("Failed to grant privileges: %s", string(output))
					return result, nil
				}
			}
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("User '%s'@'%s' %s", name, userHost, map[bool]string{true: "updated", false: "created"}[check.Present])

	case "absent":
		if !check.Present {
			result.Success = true
			result.Changed = false
			result.Comment = fmt.Sprintf("User '%s'@'%s' does not exist", name, userHost)
			return result, nil
		}

		sql := fmt.Sprintf("DROP USER '%s'@'%s'", escapeMySQLString(name), escapeMySQLString(userHost))
		cmd := exec.CommandContext(ctx, "mysql", append(connArgs, "-e", sql)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Success = false
			result.Comment = fmt.Sprintf("Failed to drop user: %s", string(output))
			return result, nil
		}

		result.Success = true
		result.Changed = true
		result.Comment = fmt.Sprintf("User '%s'@'%s' dropped", name, userHost)
	}

	return result, nil
}

// Test runs a dry-run check
func (m *MySQLUserModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	action := "create"
	if decl.State == "absent" {
		action = "drop"
	} else if check.Present {
		action = "update"
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !check.Matches,
		Comment: fmt.Sprintf("Would %s user '%s'", action, getStringParameter(decl, "name", "")),
	}, nil
}

func (m *MySQLUserModule) buildConnArgs(decl *StateDeclaration) []string {
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 3306)
	user := getStringParameter(decl, "user", "root")
	password := getStringParameter(decl, "password", "")
	socket := getStringParameter(decl, "socket", "")

	var args []string
	if socket != "" {
		args = append(args, "-S", socket)
	} else {
		args = append(args, "-h", host, "-P", fmt.Sprintf("%d", port))
	}
	args = append(args, "-u", user)
	if password != "" {
		args = append(args, fmt.Sprintf("-p%s", password))
	}
	return args
}

func (m *MySQLUserModule) buildGrantSQL(user, host, priv string) string {
	// Parse priv format: "db.table:PRIV1,PRIV2"
	parts := strings.SplitN(priv, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	dbTable := parts[0]
	privs := parts[1]

	return fmt.Sprintf("GRANT %s ON %s TO '%s'@'%s'", privs, dbTable, escapeMySQLString(user), escapeMySQLString(host))
}

// ============================================================================
// Redis Module
// ============================================================================

// RedisModule manages Redis configuration
type RedisModule struct {
	*BaseModule
}

// NewRedisModule creates a new Redis module
func NewRedisModule() *RedisModule {
	return &RedisModule{
		BaseModule: NewBaseModule("redis", []string{"present", "absent"}),
	}
}

// Check verifies Redis configuration
func (m *RedisModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	name := getStringParameter(decl, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name parameter is required (config key or user)")
	}

	configType := getStringParameter(decl, "type", "config") // "config" or "acl"

	if _, err := exec.LookPath("redis-cli"); err != nil {
		return nil, fmt.Errorf("redis-cli is not available: %w", err)
	}

	connArgs := m.buildConnArgs(decl)

	result := &ModuleCheckResult{
		Metadata: make(map[string]interface{}),
	}

	switch configType {
	case "config":
		// Check config value
		cmd := exec.CommandContext(ctx, "redis-cli", append(connArgs, "CONFIG", "GET", name)...)
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get config: %w", err)
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) >= 2 {
			result.Present = true
			result.Metadata["value"] = lines[1]
			result.CurrentState = lines[1]
		}

	case "acl":
		// Check ACL user
		cmd := exec.CommandContext(ctx, "redis-cli", append(connArgs, "ACL", "LIST")...)
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to list ACL: %w", err)
		}

		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, fmt.Sprintf("user %s ", name)) {
				result.Present = true
				result.CurrentState = "present"
				result.Metadata["acl"] = strings.TrimSpace(line)
				break
			}
		}
	}

	if !result.Present {
		result.CurrentState = "absent"
	}

	result.Matches = (decl.State == "present" && result.Present) || (decl.State == "absent" && !result.Present)

	return result, nil
}

// Apply sets or removes Redis configuration
func (m *RedisModule) Apply(ctx context.Context, decl *StateDeclaration, check *ModuleCheckResult) (*StateResult, error) {
	name := getStringParameter(decl, "name", "")
	configType := getStringParameter(decl, "type", "config")
	connArgs := m.buildConnArgs(decl)

	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
	}

	switch decl.State {
	case "present":
		switch configType {
		case "config":
			value := getStringParameter(decl, "value", "")
			if value == "" {
				result.Success = false
				result.Comment = "value parameter is required for config type"
				return result, nil
			}

			cmd := exec.CommandContext(ctx, "redis-cli", append(connArgs, "CONFIG", "SET", name, value)...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to set config: %s", string(output))
				return result, nil
			}

			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("Config '%s' set to '%s'", name, value)

		case "acl":
			// ACL user creation
			aclPassword := getStringParameter(decl, "acl_password", "")
			aclRules := getStringParameter(decl, "acl_rules", "")

			var aclArgs []string
			aclArgs = append(aclArgs, "ACL", "SETUSER", name)
			if aclPassword != "" {
				aclArgs = append(aclArgs, fmt.Sprintf(">%s", aclPassword))
			}
			if aclRules != "" {
				aclArgs = append(aclArgs, strings.Fields(aclRules)...)
			}

			cmd := exec.CommandContext(ctx, "redis-cli", append(connArgs, aclArgs...)...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create ACL user: %s", string(output))
				return result, nil
			}

			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("ACL user '%s' created/updated", name)
		}

	case "absent":
		switch configType {
		case "config":
			// Can't really remove a config, just set to default
			result.Success = true
			result.Changed = false
			result.Comment = "Cannot remove config value, set to default instead"

		case "acl":
			if !check.Present {
				result.Success = true
				result.Changed = false
				result.Comment = fmt.Sprintf("ACL user '%s' does not exist", name)
				return result, nil
			}

			cmd := exec.CommandContext(ctx, "redis-cli", append(connArgs, "ACL", "DELUSER", name)...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete ACL user: %s", string(output))
				return result, nil
			}

			result.Success = true
			result.Changed = true
			result.Comment = fmt.Sprintf("ACL user '%s' deleted", name)
		}
	}

	return result, nil
}

// Test runs a dry-run check
func (m *RedisModule) Test(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	check, err := m.Check(ctx, decl)
	if err != nil {
		return nil, err
	}

	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: !check.Matches,
		Comment: fmt.Sprintf("Would update redis %s '%s'", getStringParameter(decl, "type", "config"), getStringParameter(decl, "name", "")),
	}, nil
}

func (m *RedisModule) buildConnArgs(decl *StateDeclaration) []string {
	host := getStringParameter(decl, "host", "localhost")
	port := getIntParameter(decl, "port", 6379)
	password := getStringParameter(decl, "password", "")
	socket := getStringParameter(decl, "socket", "")

	var args []string
	if socket != "" {
		args = append(args, "-s", socket)
	} else {
		args = append(args, "-h", host, "-p", fmt.Sprintf("%d", port))
	}
	if password != "" {
		args = append(args, "-a", password)
	}
	return args
}

// ============================================================================
// Helper Functions
// ============================================================================

// escapePostgresString escapes single quotes for PostgreSQL strings
func escapePostgresString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// quotePostgresIdentifier quotes an identifier for PostgreSQL
func quotePostgresIdentifier(s string) string {
	// Simple identifier quoting - for production use, validate the identifier
	if regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(s) {
		return s
	}
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(s, `"`, `""`))
}

// escapeMySQLString escapes single quotes for MySQL strings
func escapeMySQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}

// escapeMySQLIdentifier escapes backticks for MySQL identifiers
func escapeMySQLIdentifier(s string) string {
	return strings.ReplaceAll(s, "`", "``")
}
