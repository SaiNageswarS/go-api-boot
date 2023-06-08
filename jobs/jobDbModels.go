package jobs

import "time"

// Job status is used for both synchronization and monitoring.
// Before a job executes, it must check for status. If job is already
// running, it must not execute.
type JobStatus string

const (
	JobStatusScheduled JobStatus = "Scheduled"
	JobStatusRunning   JobStatus = "Running"
	JobStatusFailed    JobStatus = "Failed"
	JobStatusStopped   JobStatus = "Stopped"
)

type JobRunLogModel struct {
	// Job name
	Name string `bson:"name"`
	// Last execution date time in format dd-MMM-yyyy HH:mm:ss
	LastExecutionDateTime string `bson:"lastExecutionDateTime"`

	// Last execution status
	LastExecutionStatus JobStatus `bson:"lastExecutionStatus"`
	// Last execution message
	LastExecutionMessage string `bson:"lastExecutionMessage"`

	// Last execution duration
	LastExecutionDuration time.Duration `bson:"lastExecutionDuration"`
}

func (j *JobRunLogModel) Id() string {
	return j.Name
}
