package jobs

type Job interface {
	// Run the job
	Run() error
}
