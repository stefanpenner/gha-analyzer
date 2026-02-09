package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStorage(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer storage.Close()

	// Test SaveRun and GetRecentRuns
	t.Run("SaveAndRetrieveRun", func(t *testing.T) {
		run := &WorkflowRun{
			Owner:       "test-owner",
			Repo:        "test-repo",
			RunID:       12345,
			HeadSHA:     "abc123",
			Branch:      "main",
			Event:       "push",
			Status:      "completed",
			Conclusion:  "success",
			StartTime:   time.Now().Add(-10 * time.Minute),
			EndTime:     time.Now(),
			Duration:    600000, // 10 minutes in ms
			SuccessRate: 100.0,
			TotalJobs:   5,
			FailedJobs:  0,
		}

		err := storage.SaveRun(run)
		require.NoError(t, err)
		assert.NotZero(t, run.ID)

		// Retrieve runs
		runs, err := storage.GetRecentRuns("test-owner", "test-repo", 10)
		require.NoError(t, err)
		require.Len(t, runs, 1)

		retrieved := runs[0]
		assert.Equal(t, run.Owner, retrieved.Owner)
		assert.Equal(t, run.Repo, retrieved.Repo)
		assert.Equal(t, run.RunID, retrieved.RunID)
		assert.Equal(t, run.HeadSHA, retrieved.HeadSHA)
		assert.Equal(t, run.Branch, retrieved.Branch)
		assert.Equal(t, run.Conclusion, retrieved.Conclusion)
	})

	// Test SaveJob and GetJobs
	t.Run("SaveAndRetrieveJob", func(t *testing.T) {
		// First create a run
		run := &WorkflowRun{
			Owner:      "test-owner",
			Repo:       "test-repo",
			RunID:      67890,
			HeadSHA:    "def456",
			StartTime:  time.Now(),
			EndTime:    time.Now(),
		}
		err := storage.SaveRun(run)
		require.NoError(t, err)

		// Create jobs
		job1 := &Job{
			RunID:      run.ID,
			JobID:      1,
			Name:       "build",
			Status:     "completed",
			Conclusion: "success",
			StartTime:  time.Now().Add(-5 * time.Minute),
			EndTime:    time.Now(),
			Duration:   300000, // 5 minutes in ms
			RunnerType: "ubuntu-latest",
		}

		job2 := &Job{
			RunID:      run.ID,
			JobID:      2,
			Name:       "test",
			Status:     "completed",
			Conclusion: "success",
			StartTime:  time.Now().Add(-3 * time.Minute),
			EndTime:    time.Now(),
			Duration:   180000, // 3 minutes in ms
			RunnerType: "ubuntu-latest",
		}

		err = storage.SaveJob(job1)
		require.NoError(t, err)
		assert.NotZero(t, job1.ID)

		err = storage.SaveJob(job2)
		require.NoError(t, err)
		assert.NotZero(t, job2.ID)

		// Retrieve jobs
		jobs, err := storage.GetJobs(run.ID)
		require.NoError(t, err)
		require.Len(t, jobs, 2)

		assert.Equal(t, "build", jobs[0].Name)
		assert.Equal(t, "test", jobs[1].Name)
	})

	// Test GetRuns with time range
	t.Run("GetRunsWithTimeRange", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)
		twoDaysAgo := now.Add(-48 * time.Hour)

		// Create runs at different times
		oldRun := &WorkflowRun{
			Owner:     "test-owner2",
			Repo:      "test-repo2",
			RunID:     111,
			HeadSHA:   "old",
			StartTime: twoDaysAgo,
			EndTime:   twoDaysAgo,
		}
		recentRun := &WorkflowRun{
			Owner:     "test-owner2",
			Repo:      "test-repo2",
			RunID:     222,
			HeadSHA:   "recent",
			StartTime: yesterday,
			EndTime:   yesterday,
		}

		require.NoError(t, storage.SaveRun(oldRun))
		require.NoError(t, storage.SaveRun(recentRun))

		// Query for runs in the last 36 hours
		runs, err := storage.GetRuns("test-owner2", "test-repo2", now.Add(-36*time.Hour), now)
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, int64(222), runs[0].RunID)
	})

	// Test GetJobHistory
	t.Run("GetJobHistory", func(t *testing.T) {
		now := time.Now()

		// Create multiple runs with the same job name
		for i := 0; i < 3; i++ {
			run := &WorkflowRun{
				Owner:     "test-owner3",
				Repo:      "test-repo3",
				RunID:     int64(1000 + i),
				HeadSHA:   "sha" + string(rune('a'+i)),
				StartTime: now.Add(time.Duration(-i) * time.Hour),
				EndTime:   now.Add(time.Duration(-i) * time.Hour),
			}
			require.NoError(t, storage.SaveRun(run))

			job := &Job{
				RunID:      run.ID,
				JobID:      int64(100 + i),
				Name:       "build",
				Conclusion: "success",
				StartTime:  run.StartTime,
				EndTime:    run.EndTime,
				Duration:   int64(300000 + i*1000),
			}
			require.NoError(t, storage.SaveJob(job))
		}

		// Get history for "build" job
		history, err := storage.GetJobHistory("test-owner3", "test-repo3", "build", now.Add(-5*time.Hour))
		require.NoError(t, err)
		require.Len(t, history, 3)

		// Verify they're ordered by time (most recent first)
		assert.True(t, history[0].StartTime.After(history[1].StartTime))
		assert.True(t, history[1].StartTime.After(history[2].StartTime))
	})
}

func TestDefaultStoragePath(t *testing.T) {
	path := DefaultStoragePath()
	assert.NotEmpty(t, path)
	assert.Contains(t, path, ".gha-analyzer")
	assert.Contains(t, path, "history.db")
}

func TestNewSQLiteStorage_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "nested", "test.db")

	storage, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer storage.Close()

	// Verify directory was created
	dir := filepath.Dir(dbPath)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}
