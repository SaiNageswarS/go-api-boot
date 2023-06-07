package jobs

import (
	"time"

	"github.com/SaiNageswarS/go-api-boot/odm"
)

type JobRunLogRepository struct {
	odm.AbstractRepository[JobRunLogModel]
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

func (j *JobRunLogRepository) MarkJobAsRunning(id string) chan error {
	ch := make(chan error)

	go func() {
		jobChan, errorChan := j.FindOneById(id)

		select {
		case job := <-jobChan:
			if job.LastExecutionStatus == JobStatusRunning {
				ch <- nil
			} else {
				job.LastExecutionStatus = JobStatusRunning
				job.LastExecutionDateTime = time.Now().Format("dd-MMM-yyyy HH:mm:ss")
				job.LastExecutionMessage = "Job running"
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

func (j *JobRunLogRepository) MarkJobAsFailed(id, message string) chan error {
	ch := make(chan error)

	go func() {
		jobChan, errorChan := j.FindOneById(id)

		select {
		case job := <-jobChan:
			if job.LastExecutionStatus == JobStatusFailed {
				ch <- nil
			} else {
				job.LastExecutionStatus = JobStatusFailed
				job.LastExecutionMessage = message
				job.LastExecutionDateTime = time.Now().Format("dd-MMM-yyyy HH:mm:ss")
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

func (j *JobRunLogRepository) MarkJobAsScheduled(id string) chan error {
	ch := make(chan error)

	go func() {
		jobChan, errorChan := j.FindOneById(id)

		select {
		case job := <-jobChan:
			if job.LastExecutionStatus == JobStatusScheduled {
				ch <- nil
			} else {
				job.LastExecutionStatus = JobStatusScheduled
				job.LastExecutionDateTime = time.Now().Format("dd-MMM-yyyy HH:mm:ss")
				job.LastExecutionMessage = "Job scheduled"
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

func (j *JobRunLogRepository) MarkJobAsStopped(id string) chan error {
	ch := make(chan error)

	go func() {
		jobChan, errorChan := j.FindOneById(id)

		select {
		case job := <-jobChan:
			if job.LastExecutionStatus == JobStatusStopped {
				ch <- nil
			} else {
				job.LastExecutionStatus = JobStatusStopped
				job.LastExecutionDateTime = time.Now().Format("dd-MMM-yyyy HH:mm:ss")
				job.LastExecutionMessage = "Job stopped"
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
		AbstractRepository: odm.AbstractRepository[JobRunLogModel]{
			Database:       j.Database,
			CollectionName: "JobRunLogs",
		},
	}
}
