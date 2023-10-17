package jobs

import (
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
)

type JobRunLogRepository struct {
	odm.UnimplementedBootRepository[JobRunLogModel]
}

type JobDb struct {
	Database string
}

func (j *JobRunLogRepository) IsRunning(id string) chan bool {
	ch := make(chan bool)

	go func() {
		resultChan, errorChan := j.FindOneById(id)

		select {
		case res := <-resultChan:
			ch <- res.LastExecutionStatus == JobStatusRunning
		case _ = <-errorChan:
			ch <- false
		}
	}()

	return ch
}

func (j *JobRunLogRepository) MarkJob(id, message string, status JobStatus) chan error {
	ch := make(chan error)

	go func() {
		jobChan, errorChan := j.FindOneById(id)

		select {
		case job := <-jobChan:
			if job.LastExecutionStatus == status {
				ch <- nil
			} else {
				job.LastExecutionStatus = status
				job.LastExecutionDateTime = time.Now().Format("dd-MMM-yyyy HH:mm:ss")
				job.LastExecutionMessage = message
				errorChan := <-j.Save(job)
				ch <- errorChan
			}
		case err := <-errorChan:
			ch <- err
			return
		}

	}()

	return ch
}

func (j *JobDb) GetJobRunLogRepository() *JobRunLogRepository {
	return &JobRunLogRepository{
		UnimplementedBootRepository: odm.UnimplementedBootRepository[JobRunLogModel]{
			Database:       j.Database,
			CollectionName: "JobRunLogs",
		},
	}
}
