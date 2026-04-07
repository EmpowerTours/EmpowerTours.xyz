package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// Connect opens a SQLite database optimized for concurrent mobile app usage.
// WAL mode allows concurrent reads while a write is happening.
// These pragmas let SQLite handle 500+ concurrent readers comfortably.
func Connect(dbPath string) (*sqlx.DB, error) {
	dsn := dbPath + "?" + strings.Join([]string{
		"_journal_mode=WAL",    // Write-Ahead Logging - concurrent reads during writes
		"_foreign_keys=on",     // Enforce foreign key constraints
		"_busy_timeout=10000",  // Wait 10s for locks instead of failing immediately
		"_synchronous=NORMAL",  // Faster writes, still safe with WAL
		"_cache_size=-20000",   // 20MB page cache (negative = KB)
		"_mmap_size=268435456", // 256MB memory-mapped I/O for faster reads
		"_temp_store=MEMORY",   // Temp tables in memory
	}, "&")

	db, err := sqlx.Connect("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// SQLite allows unlimited concurrent readers with WAL mode.
	// Only writes are serialized (one at a time), which is fine since
	// reads vastly outnumber writes in a mobile app.
	db.SetMaxOpenConns(4)  // Pool of 4 connections (1 writer + 3 readers)
	db.SetMaxIdleConns(4)

	return db, nil
}

// RunMigrations reads all .sql files from the migrations directory and executes
// them in alphabetical order. A migrations tracking table ensures each file
// runs only once.
func RunMigrations(db *sqlx.DB, migrationsDir string) error {
	// Create tracking table
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Collect and sort .sql files
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		// Check if already applied
		var count int
		err := db.Get(&count, "SELECT COUNT(*) FROM schema_migrations WHERE filename = ?", name)
		if err != nil {
			return fmt.Errorf("failed to check migration %s: %w", name, err)
		}
		if count > 0 {
			continue
		}

		// Read and execute
		content, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", name, err)
		}

		_, err = db.Exec(string(content))
		if err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", name, err)
		}

		// Record
		_, err = db.Exec("INSERT INTO schema_migrations (filename) VALUES (?)", name)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", name, err)
		}

		fmt.Printf("Applied migration: %s\n", name)
	}

	return nil
}
