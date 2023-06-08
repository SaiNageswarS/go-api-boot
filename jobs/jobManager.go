package jobs

import (
	"sync"
	"time"

	"github.com/thoas/go-funk"
)

// Job Detail
type jobDetail struct {
	// Job name
	Name string

	// job repeat/schedule duration
	Duration time.Duration

	// Job
	Job Job
}

// Job Manager
type JobManager struct {
	// Job db
	JobDb JobDb

	// Job Details
	JobDetails []jobDetail

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
		JobDetails: make([]jobDetail, 0),
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
	jd := funk.Find(j.JobDetails, func(j jobDetail) bool {
		return j.Name == name
	})
	// if job already exists, return
	if jd != nil {
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

	j.JobDetails = append(
		j.JobDetails,
		jobDetail{
			Name:     name,
			Duration: duration,
			Job:      job,
		})
}

// Start job
func (j *JobManager) runJob(jd jobDetail) {
	j.wg.Add(1)
	defer j.wg.Done()

	id := jd.Name
	duration := jd.Duration
	job := jd.Job

	for {
		select {
		case <-j.stopChan:
			// Stop the goroutine gracefully
			<-j.JobDb.GetJobRunLogRepository().MarkJob(id, "Job stopped", JobStatusStopped)
			return
		case <-time.After(duration):
			if isRunning := <-j.JobDb.GetJobRunLogRepository().IsRunning(id); !isRunning {
				<-j.JobDb.GetJobRunLogRepository().MarkJob(id, "Job running", JobStatusRunning)
				err := job.Run()
				if err != nil {
					<-j.JobDb.GetJobRunLogRepository().MarkJob(id, "Job failed: "+err.Error(), JobStatusFailed)
				} else {
					<-j.JobDb.GetJobRunLogRepository().MarkJob(id, "Job scheduled", JobStatusScheduled)
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
	j.mutex.Lock()
	defer j.mutex.Unlock()

	for _, jd := range j.JobDetails {
		go j.runJob(jd)
	}
}
