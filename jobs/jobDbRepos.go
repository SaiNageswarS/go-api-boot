package jobs

import "github.com/SaiNageswarS/go-api-boot/odm"

type JobDetailRepository struct {
	odm.AbstractRepository[JobDetailModel]
}

type JobRunLogRepository struct {
	odm.AbstractRepository[JonRunLogModel]
}

type JobDb struct {
	Database string
}

func (j *JobDb) GetJobDetailRepository() *JobDetailRepository {
	return &JobDetailRepository{
		AbstractRepository: odm.AbstractRepository[JobDetailModel]{
			Database:       j.Database,
			CollectionName: "JobDetails",
		},
	}
}

func (j *JobDb) GetJobRunLogRepository() *JobRunLogRepository {
	return &JobRunLogRepository{
		AbstractRepository: odm.AbstractRepository[JonRunLogModel]{
			Database:       j.Database,
			CollectionName: "JobRunLogs",
		},
	}
}
