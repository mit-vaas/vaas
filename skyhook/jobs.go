package skyhook

import (
	"log"
	"net/http"
	"sync"
)

type Job struct {
	ID int
	Name string
	Status string
	Type string
}

const JobQuery = "SELECT id, name, status, type FROM jobs"

func jobListHelper(rows *Rows) []Job {
	jobs := []Job{}
	for rows.Next() {
		var job Job
		rows.Scan(&job.ID, &job.Name, &job.Status, &job.Type)
		jobs = append(jobs, job)
	}
	return jobs
}

func ListJobs() []Job {
	rows := db.Query(JobQuery + " ORDER BY id DESC")
	return jobListHelper(rows)
}

func GetJob(id int) *Job {
	rows := db.Query(JobQuery + " WHERE id = ?", id)
	jobs := jobListHelper(rows)
	if len(jobs) == 1 {
		job := jobs[0]
		return &job
	} else {
		return nil
	}
}

type JobRunnable interface {
	Name() string

	// returns the front-end job module that displays this job based on Status()
	Type() string

	Run(func(string)) error
	Detail() interface{}
}

var runningJobs = make(map[int]JobRunnable)
var jobMu sync.Mutex

func RunJob(runnable JobRunnable) error {
	name := runnable.Name()
	log.Printf("[job %s] starting", name)
	res := db.Exec("INSERT INTO jobs (name, type) VALUES (?, ?)", name, runnable.Type())
	jobID := res.LastInsertId()

	jobMu.Lock()
	runningJobs[jobID] = runnable
	jobMu.Unlock()
	defer func() {
		jobMu.Lock()
		delete(runningJobs, jobID)
		jobMu.Unlock()
		detail := runnable.Detail()
		bytes := JsonMarshal(detail)
		db.Exec("UPDATE jobs SET detail = ? WHERE id = ?", string(bytes), jobID)
	}()

	statusFunc := func(status string) {
		log.Printf("[job %s] update status: %s", name, status)
		db.Exec("UPDATE jobs SET status = ? WHERE id = ?", status, jobID)
	}
	err := runnable.Run(statusFunc)
	if err != nil {
		log.Printf("[job %s] error: %v", name, err)
		db.Exec("UPDATE jobs SET status = ? WHERE id = ?", "Error: " + err.Error(), jobID)
		return err
	}
	log.Printf("[job %s] success", name)
	db.Exec("UPDATE jobs SET status = 'Done' WHERE id = ?", jobID)
	return nil
}

func init() {
	http.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		JsonResponse(w, ListJobs())
	})

	http.HandleFunc("/jobs/detail", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		jobID := ParseInt(r.Form.Get("job_id"))

		rows := db.Query("SELECT detail FROM jobs WHERE id = ?", jobID)
		if !rows.Next() {
			http.Error(w, "no such job", 404)
			return
		}
		var detail string
		rows.Scan(&detail)
		rows.Close()
		if detail != "" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(detail))
			return
		}

		jobMu.Lock()
		job := runningJobs[jobID]
		jobMu.Unlock()
		if job == nil {
			http.Error(w, "no such job", 404)
			return
		}
		JsonResponse(w, job.Detail())
	})
}
