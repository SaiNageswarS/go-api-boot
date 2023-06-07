package jobs

import "github.com/google/uuid"

// Job status is used for both synchronization and monitoring.
// Before a job executes, it must check for status. If job is already
// running, it must not execute.
type JobStatus string

const (
	JobStatusScheduled JobStatus = "Scheduled"
	JobStatusRunning   JobStatus = "Running"
)

type JobDetailModel struct {
	// Job name
	Name string `bson:"name"`
	// cron expression for schedule
	Cron string `bson:"cron"`

	Status JobStatus `bson:"status"`
}

func (j *JobDetailModel) Id() string {
	return j.Name
}

type JonRunLogModel struct {
	// Job name
	Name string `bson:"name"`
	// Last execution date time in format dd-MMM-yyyy HH:mm:ss
	LastExecutionDateTime string `bson:"lastExecutionDateTime"`

	// Last execution status
	LastExecutionStatus string `bson:"lastExecutionStatus"`
	// Last execution message
	LastExecutionMessage string `bson:"lastExecutionMessage"`

	// Last execution duration in seconds
	LastExecutionDuration int64 `bson:"lastExecutionDuration"`
}

func (j *JonRunLogModel) Id() string {
	// return unique UUID.
	return uuid.New().String()
}
