package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Storage provides persistent storage for workflow run history
type Storage interface {
	// SaveRun stores a workflow run record
	SaveRun(run *WorkflowRun) error

	// GetRuns retrieves runs for a repository within a time range
	GetRuns(owner, repo string, since, until time.Time) ([]*WorkflowRun, error)

	// GetRecentRuns retrieves the most recent N runs for a repository
	GetRecentRuns(owner, repo string, limit int) ([]*WorkflowRun, error)

	// SaveJob stores a job record
	SaveJob(job *Job) error

	// GetJobs retrieves jobs for a specific run
	GetJobs(runID int64) ([]*Job, error)

	// GetJobHistory retrieves historical data for a specific job name
	GetJobHistory(owner, repo, jobName string, since time.Time) ([]*Job, error)

	// Close closes the storage connection
	Close() error
}

// WorkflowRun represents a stored workflow run
type WorkflowRun struct {
	ID          int64
	Owner       string
	Repo        string
	RunID       int64  // GitHub run ID
	HeadSHA     string
	Branch      string
	Event       string
	Status      string
	Conclusion  string
	StartTime   time.Time
	EndTime     time.Time
	Duration    int64 // milliseconds
	SuccessRate float64
	TotalJobs   int
	FailedJobs  int
	CreatedAt   time.Time
}

// Job represents a stored job
type Job struct {
	ID         int64
	RunID      int64 // Foreign key to WorkflowRun
	JobID      int64 // GitHub job ID
	Name       string
	Status     string
	Conclusion string
	StartTime  time.Time
	EndTime    time.Time
	Duration   int64 // milliseconds
	RunnerType string
	CreatedAt  time.Time
}

// SQLiteStorage implements Storage using SQLite
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables
	s := &SQLiteStorage{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return s, nil
}

// DefaultStoragePath returns the default storage path
func DefaultStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gha-analyzer/history.db"
	}
	return filepath.Join(home, ".gha-analyzer", "history.db")
}

// initSchema creates the database schema
func (s *SQLiteStorage) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS workflow_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			run_id INTEGER NOT NULL,
			head_sha TEXT NOT NULL,
			branch TEXT,
			event TEXT,
			status TEXT,
			conclusion TEXT,
			start_time DATETIME,
			end_time DATETIME,
			duration INTEGER,
			success_rate REAL,
			total_jobs INTEGER,
			failed_jobs INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(owner, repo, run_id)
		);

		CREATE INDEX IF NOT EXISTS idx_runs_repo ON workflow_runs(owner, repo);
		CREATE INDEX IF NOT EXISTS idx_runs_time ON workflow_runs(owner, repo, start_time);

		CREATE TABLE IF NOT EXISTS jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			job_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			status TEXT,
			conclusion TEXT,
			start_time DATETIME,
			end_time DATETIME,
			duration INTEGER,
			runner_type TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE,
			UNIQUE(run_id, job_id)
		);

		CREATE INDEX IF NOT EXISTS idx_jobs_run ON jobs(run_id);
		CREATE INDEX IF NOT EXISTS idx_jobs_name ON jobs(name);
	`)
	return err
}

// SaveRun stores a workflow run
func (s *SQLiteStorage) SaveRun(run *WorkflowRun) error {
	result, err := s.db.Exec(`
		INSERT OR REPLACE INTO workflow_runs
		(owner, repo, run_id, head_sha, branch, event, status, conclusion,
		 start_time, end_time, duration, success_rate, total_jobs, failed_jobs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.Owner, run.Repo, run.RunID, run.HeadSHA, run.Branch, run.Event,
		run.Status, run.Conclusion, run.StartTime, run.EndTime, run.Duration,
		run.SuccessRate, run.TotalJobs, run.FailedJobs,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	run.ID = id
	return nil
}

// GetRuns retrieves runs within a time range
func (s *SQLiteStorage) GetRuns(owner, repo string, since, until time.Time) ([]*WorkflowRun, error) {
	rows, err := s.db.Query(`
		SELECT id, owner, repo, run_id, head_sha, branch, event, status, conclusion,
		       start_time, end_time, duration, success_rate, total_jobs, failed_jobs, created_at
		FROM workflow_runs
		WHERE owner = ? AND repo = ? AND start_time >= ? AND start_time <= ?
		ORDER BY start_time DESC
	`, owner, repo, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanRuns(rows)
}

// GetRecentRuns retrieves the most recent N runs
func (s *SQLiteStorage) GetRecentRuns(owner, repo string, limit int) ([]*WorkflowRun, error) {
	rows, err := s.db.Query(`
		SELECT id, owner, repo, run_id, head_sha, branch, event, status, conclusion,
		       start_time, end_time, duration, success_rate, total_jobs, failed_jobs, created_at
		FROM workflow_runs
		WHERE owner = ? AND repo = ?
		ORDER BY start_time DESC
		LIMIT ?
	`, owner, repo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanRuns(rows)
}

// SaveJob stores a job record
func (s *SQLiteStorage) SaveJob(job *Job) error {
	result, err := s.db.Exec(`
		INSERT OR REPLACE INTO jobs
		(run_id, job_id, name, status, conclusion, start_time, end_time, duration, runner_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		job.RunID, job.JobID, job.Name, job.Status, job.Conclusion,
		job.StartTime, job.EndTime, job.Duration, job.RunnerType,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	job.ID = id
	return nil
}

// GetJobs retrieves jobs for a specific run
func (s *SQLiteStorage) GetJobs(runID int64) ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, run_id, job_id, name, status, conclusion,
		       start_time, end_time, duration, runner_type, created_at
		FROM jobs
		WHERE run_id = ?
		ORDER BY start_time
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

// GetJobHistory retrieves historical data for a specific job name
func (s *SQLiteStorage) GetJobHistory(owner, repo, jobName string, since time.Time) ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT j.id, j.run_id, j.job_id, j.name, j.status, j.conclusion,
		       j.start_time, j.end_time, j.duration, j.runner_type, j.created_at
		FROM jobs j
		JOIN workflow_runs r ON j.run_id = r.id
		WHERE r.owner = ? AND r.repo = ? AND j.name = ? AND j.start_time >= ?
		ORDER BY j.start_time DESC
	`, owner, repo, jobName, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanJobs(rows)
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// Helper functions

func (s *SQLiteStorage) scanRuns(rows *sql.Rows) ([]*WorkflowRun, error) {
	var runs []*WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		err := rows.Scan(
			&run.ID, &run.Owner, &run.Repo, &run.RunID, &run.HeadSHA,
			&run.Branch, &run.Event, &run.Status, &run.Conclusion,
			&run.StartTime, &run.EndTime, &run.Duration, &run.SuccessRate,
			&run.TotalJobs, &run.FailedJobs, &run.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		runs = append(runs, &run)
	}
	return runs, rows.Err()
}

func (s *SQLiteStorage) scanJobs(rows *sql.Rows) ([]*Job, error) {
	var jobs []*Job
	for rows.Next() {
		var job Job
		err := rows.Scan(
			&job.ID, &job.RunID, &job.JobID, &job.Name, &job.Status,
			&job.Conclusion, &job.StartTime, &job.EndTime, &job.Duration,
			&job.RunnerType, &job.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}
