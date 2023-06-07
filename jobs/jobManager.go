package jobs

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/thoas/go-funk"
)

// Job Run Log
type jobRunLog struct {
	// Job name
	Name string
	// last execution duration
	Duration time.Duration
	// Job
	Job Job
}

// Job Manager
type JobManager struct {
	// Job db
	JobDb JobDb

	// Job Run Logs
	JobRunLogs []jobRunLog

	// Stop chan
	stopChan chan struct{}

	// Mutex to protect JobRunLogs
	mutex *sync.Mutex

	// Wait group
	wg *sync.WaitGroup
}

// Global job manager
func NewJobManager(database string) *JobManager {
	return &JobManager{
		JobDb:      JobDb{Database: database},
		JobRunLogs: make([]jobRunLog, 0),
		stopChan:   make(chan struct{}),
		wg:         &sync.WaitGroup{},
		mutex:      &sync.Mutex{},
	}
}

// Add job
func (j *JobManager) RegisterJob(name string, duration time.Duration, job Job) {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	// check if job already exists
	jobInterface := funk.Find(j.JobRunLogs, func(j *JobRunLogModel) bool {
		return j.Name == name
	})
	// if job already exists, return
	if jobInterface != nil {
		return
	}

	// save job details to db.
	jobRunLogModel := &JobRunLogModel{
		Name:                  name,
		LastExecutionDuration: duration,
		LastExecutionDateTime: time.Now().Format("dd-MMM-yyyy HH:mm:ss"),
		LastExecutionStatus:   JobStatusScheduled,
		LastExecutionMessage:  "Job scheduled",
	}
	j.JobDb.GetJobRunLogRepository().Save(jobRunLogModel)

	j.JobRunLogs = append(
		j.JobRunLogs,
		jobRunLog{
			Name:     name,
			Duration: duration,
			Job:      job,
		})
}

// Start job
func (j *JobManager) runJob(jobRunLog jobRunLog) {
	defer j.wg.Done()

	id := jobRunLog.Name
	duration := jobRunLog.Duration
	job := jobRunLog.Job

	for {
		select {
		case <-j.stopChan:
			// Stop the goroutine gracefully
			<-j.JobDb.GetJobRunLogRepository().MarkJobAsStopped(id)
			return
		case <-time.After(duration):
			if isRunning := <-j.JobDb.GetJobRunLogRepository().IsRunning(id); !isRunning {
				<-j.JobDb.GetJobRunLogRepository().MarkJobAsRunning(id)
				err := job.Run()
				if err != nil {
					<-j.JobDb.GetJobRunLogRepository().MarkJobAsFailed(id, err.Error())
				} else {
					<-j.JobDb.GetJobRunLogRepository().MarkJobAsScheduled(id)
				}
			}
		}
	}
}

// Stop method to signal the registered jobs to stop
func (j *JobManager) Stop() {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	select {
	case <-j.stopChan:
		return
	default:
		close(j.stopChan)
		j.wg.Wait()
	}
}

// Start method to start the registered jobs
func (j *JobManager) Start() {
	// Register signal handler for interrupt signal (Ctrl+C)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start a goroutine to handle the cleanup function when the interrupt signal is received
	go func() {
		<-c
		j.Stop()
	}()

	j.mutex.Lock()
	defer j.mutex.Unlock()

	for _, jobRunLog := range j.JobRunLogs {
		go j.runJob(jobRunLog)
	}
}
