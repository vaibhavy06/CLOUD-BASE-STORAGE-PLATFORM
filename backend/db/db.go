package db

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/000001_init_schema.up.sql
var initSchemaSQL string

// Pool is the global database connection pool
var Pool *pgxpool.Pool

// InitDB initializes the PostgreSQL connection pool and runs migrations
func InitDB(connStr string) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	// Configure connection pool limits for production readiness
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	// Connect to the DB
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Ping the DB to ensure viability
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database connection failed ping: %w", err)
	}

	log.Println("Connected to PostgreSQL successfully. Running migrations...")
	Pool = pool

	// Execute migrations
	if err := RunMigrations(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	log.Println("Database migrations executed successfully.")
	return Pool, nil
}

// RunMigrations executes the embedded schema migration SQL
func RunMigrations(ctx context.Context) error {
	if Pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}

	// Execute the schema script
	_, err := Pool.Exec(ctx, initSchemaSQL)
	if err != nil {
		return fmt.Errorf("migration execution error: %w", err)
	}

	// Alter files table to add AI metadata columns dynamically
	_, _ = Pool.Exec(ctx, "ALTER TABLE files ADD COLUMN IF NOT EXISTS extracted_text TEXT")
	_, _ = Pool.Exec(ctx, "ALTER TABLE files ADD COLUMN IF NOT EXISTS summary TEXT")
	_, _ = Pool.Exec(ctx, "ALTER TABLE files ADD COLUMN IF NOT EXISTS tags VARCHAR(50)[]")

	// Seed roles and permissions if they don't exist
	if err := SeedRolesAndPermissions(ctx); err != nil {
		return fmt.Errorf("failed to seed roles and permissions: %w", err)
	}

	return nil
}

// SeedRolesAndPermissions inserts standard system roles & permissions
func SeedRolesAndPermissions(ctx context.Context) error {
	// Seed roles
	roles := []struct {
		name        string
		description string
	}{
		{"Admin", "Administrator with full system privileges"},
		{"User", "Standard user with read, write, delete, and share rights on their own files"},
		{"Viewer", "Guest user with read-only access to specific shared links"},
	}

	for _, r := range roles {
		_, err := Pool.Exec(ctx, `
			INSERT INTO roles (name, description) 
			VALUES ($1, $2) 
			ON CONFLICT (name) DO NOTHING`, r.name, r.description)
		if err != nil {
			return err
		}
	}

	// Seed permissions
	permissions := []struct {
		name        string
		description string
	}{
		{"Read", "Allows downloading and viewing files"},
		{"Write", "Allows uploading, renaming, and modifying files/folders"},
		{"Delete", "Allows deleting files/folders"},
		{"Share", "Allows sharing files/folders"},
	}

	for _, p := range permissions {
		_, err := Pool.Exec(ctx, `
			INSERT INTO permissions (name, description) 
			VALUES ($1, $2) 
			ON CONFLICT (name) DO NOTHING`, p.name, p.description)
		if err != nil {
			return err
		}
	}

	// Map Admin role to all permissions
	var adminRoleID string
	err := Pool.QueryRow(ctx, "SELECT id FROM roles WHERE name = 'Admin'").Scan(&adminRoleID)
	if err != nil {
		return err
	}

	permsRows, err := Pool.Query(ctx, "SELECT id FROM permissions")
	if err != nil {
		return err
	}
	defer permsRows.Close()

	var permIDs []string
	for permsRows.Next() {
		var id string
		if err := permsRows.Scan(&id); err != nil {
			return err
		}
		permIDs = append(permIDs, id)
	}

	for _, permID := range permIDs {
		_, err := Pool.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id) 
			VALUES ($1, $2) 
			ON CONFLICT (role_id, permission_id) DO NOTHING`, adminRoleID, permID)
		if err != nil {
			return err
		}
	}

	// Map User role to standard permissions (Read, Write, Delete, Share)
	var userRoleID string
	err = Pool.QueryRow(ctx, "SELECT id FROM roles WHERE name = 'User'").Scan(&userRoleID)
	if err != nil {
		return err
	}

	for _, permID := range permIDs {
		_, err := Pool.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id) 
			VALUES ($1, $2) 
			ON CONFLICT (role_id, permission_id) DO NOTHING`, userRoleID, permID)
		if err != nil {
			return err
		}
	}

	return nil
}
