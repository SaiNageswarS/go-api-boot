package jobs

// Job Detail
type jobDetail struct {
	// Job name
	Name string
	// cron expression for schedule
	Cron string
	// Job
	Job Job
}

// Job Manager
type JobManager struct {
	// Job db
	JobDb JobDb

	// Job details
	JobDetails []jobDetail
}

// Global job manager
func NewJobManager(database string) *JobManager {
	return &JobManager{
		JobDb:      JobDb{Database: database},
		JobDetails: make([]jobDetail, 0),
	}
}

// Add job
func (j *JobManager) RegisterJob(name string, cron string, job Job) {
	// save job details to db.
	j.JobDb.GetJobDetailRepository().Save(&JobDetailModel{
		Name:   name,
		Cron:   cron,
		Status: JobStatusScheduled,
	})

	j.JobDetails = append(
		j.JobDetails,
		jobDetail{
			Name: name,
			Cron: cron,
			Job:  job,
		})

	// Todo: schedule job as per cron expression.
}
