package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	db *sql.DB
}

// TestRecord represents a saved test in the database
type TestRecord struct {
	ID         int64     `json:"id"`
	TestName   string    `json:"test_name"`
	DeviceName string    `json:"device_name"`
	Timestamp  time.Time `json:"timestamp"`
	Config     string    `json:"config"`      // JSON string of test config
	Data       string    `json:"data"`        // JSON string of data points
	Summary    string    `json:"summary"`     // JSON string of test summary stats
	CreatedAt  time.Time `json:"created_at"`
}

// TestSummary contains calculated statistics for a test
type TestSummary struct {
	DurationSeconds      float64            `json:"duration_seconds"`
	AveragePowerMW       float64            `json:"average_power_mw"`
	MaxPowerMW           float64            `json:"max_power_mw"`
	MinPowerMW           float64            `json:"min_power_mw"`
	AverageThroughputMbps float64           `json:"average_throughput_mbps"`
	MaxThroughputMbps    float64            `json:"max_throughput_mbps"`
	TotalDataPoints      int                `json:"total_data_points"`
	PhaseStats           map[string]PhaseStats `json:"phase_stats"`
}

// PhaseStats contains statistics for a specific test phase
type PhaseStats struct {
	DurationSeconds       float64 `json:"duration_seconds"`
	AveragePowerMW        float64 `json:"average_power_mw"`
	PowerStdDevMW         float64 `json:"power_std_dev_mw"`
	AverageThroughputMbps float64 `json:"average_throughput_mbps"`
	ThroughputStdDevMbps  float64 `json:"throughput_std_dev_mbps"`
	DataPointCount        int     `json:"data_point_count"`
}

// New creates a new database connection and initializes schema
func New(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &Database{db: db}

	// Initialize schema
	if err := d.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return d, nil
}

// initSchema creates the necessary tables if they don't exist
func (d *Database) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		test_name TEXT NOT NULL,
		device_name TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		config TEXT NOT NULL,
		data TEXT NOT NULL,
		summary TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_tests_timestamp ON tests(timestamp);
	CREATE INDEX IF NOT EXISTS idx_tests_device_name ON tests(device_name);
	CREATE INDEX IF NOT EXISTS idx_tests_test_name ON tests(test_name);
	CREATE INDEX IF NOT EXISTS idx_tests_created_at ON tests(created_at);
	`

	_, err := d.db.Exec(schema)
	return err
}

// SaveTest saves a test record to the database
func (d *Database) SaveTest(record *TestRecord) (int64, error) {
	query := `
	INSERT INTO tests (test_name, device_name, timestamp, config, data, summary, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := d.db.Exec(query,
		record.TestName,
		record.DeviceName,
		record.Timestamp,
		record.Config,
		record.Data,
		record.Summary,
		time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to save test: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return id, nil
}

// GetTest retrieves a test by ID
func (d *Database) GetTest(id int64) (*TestRecord, error) {
	query := `
	SELECT id, test_name, device_name, timestamp, config, data, summary, created_at
	FROM tests
	WHERE id = ?
	`

	var record TestRecord
	err := d.db.QueryRow(query, id).Scan(
		&record.ID,
		&record.TestName,
		&record.DeviceName,
		&record.Timestamp,
		&record.Config,
		&record.Data,
		&record.Summary,
		&record.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get test: %w", err)
	}

	return &record, nil
}

// ListTests retrieves all tests, ordered by timestamp descending
func (d *Database) ListTests() ([]*TestRecord, error) {
	query := `
	SELECT id, test_name, device_name, timestamp, config, data, summary, created_at
	FROM tests
	ORDER BY timestamp DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tests: %w", err)
	}
	defer rows.Close()

	var tests []*TestRecord
	for rows.Next() {
		var record TestRecord
		err := rows.Scan(
			&record.ID,
			&record.TestName,
			&record.DeviceName,
			&record.Timestamp,
			&record.Config,
			&record.Data,
			&record.Summary,
			&record.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test: %w", err)
		}
		tests = append(tests, &record)
	}

	return tests, rows.Err()
}

// ListTestsByDevice retrieves all tests for a specific device
func (d *Database) ListTestsByDevice(deviceName string) ([]*TestRecord, error) {
	query := `
	SELECT id, test_name, device_name, timestamp, config, data, summary, created_at
	FROM tests
	WHERE device_name = ?
	ORDER BY timestamp DESC
	`

	rows, err := d.db.Query(query, deviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to list tests by device: %w", err)
	}
	defer rows.Close()

	var tests []*TestRecord
	for rows.Next() {
		var record TestRecord
		err := rows.Scan(
			&record.ID,
			&record.TestName,
			&record.DeviceName,
			&record.Timestamp,
			&record.Config,
			&record.Data,
			&record.Summary,
			&record.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test: %w", err)
		}
		tests = append(tests, &record)
	}

	return tests, rows.Err()
}

// DeleteTest deletes a test by ID
func (d *Database) DeleteTest(id int64) error {
	query := `DELETE FROM tests WHERE id = ?`
	_, err := d.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete test: %w", err)
	}
	return nil
}

// UpdateTestSummary updates the summary statistics for a test
func (d *Database) UpdateTestSummary(id int64, summary *TestSummary) error {
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	query := `UPDATE tests SET summary = ? WHERE id = ?`
	_, err = d.db.Exec(query, string(summaryJSON), id)
	if err != nil {
		return fmt.Errorf("failed to update summary: %w", err)
	}

	return nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// SearchTests searches tests by name or device name
func (d *Database) SearchTests(searchTerm string) ([]*TestRecord, error) {
	query := `
	SELECT id, test_name, device_name, timestamp, config, data, summary, created_at
	FROM tests
	WHERE test_name LIKE ? OR device_name LIKE ?
	ORDER BY timestamp DESC
	`

	searchPattern := "%" + searchTerm + "%"
	rows, err := d.db.Query(query, searchPattern, searchPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search tests: %w", err)
	}
	defer rows.Close()

	var tests []*TestRecord
	for rows.Next() {
		var record TestRecord
		err := rows.Scan(
			&record.ID,
			&record.TestName,
			&record.DeviceName,
			&record.Timestamp,
			&record.Config,
			&record.Data,
			&record.Summary,
			&record.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test: %w", err)
		}
		tests = append(tests, &record)
	}

	return tests, rows.Err()
}
