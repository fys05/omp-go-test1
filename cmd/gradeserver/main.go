// gradeserver exposes the grade management system as an HTTP JSON API so the
// CLI domain package can run as a Kubernetes workload behind an ingress.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/fys05/omp-go-test1/grade"
)

// server wraps a grade.System with the mutex required for concurrent HTTP
// access. The grade package itself is not goroutine-safe.
type server struct {
	mu   sync.Mutex
	sys  *grade.System
	path string
}

func newServer(path string) (*server, error) {
	sys, err := grade.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	return &server{sys: sys, path: path}, nil
}

// save persists the current snapshot. Callers must hold s.mu.
func (s *server) save() error {
	return s.sys.SaveToFile(s.path)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, grade.ErrStudentNotFound),
		errors.Is(err, grade.ErrCourseNotFound),
		errors.Is(err, grade.ErrScoreNotFound):
		status = http.StatusNotFound
	case errors.Is(err, grade.ErrDuplicateStudent),
		errors.Is(err, grade.ErrDuplicateCourse),
		errors.Is(err, grade.ErrDuplicateScore),
		errors.Is(err, grade.ErrEmptyName):
		status = http.StatusConflict
	}
	var invErr grade.ErrInvalidScore
	if errors.As(err, &invErr) {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (s *server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/students", s.handleStudents)
	mux.HandleFunc("/students/", s.handleStudentSub)
	mux.HandleFunc("/courses", s.handleCourses)
	mux.HandleFunc("/courses/", s.handleCourseSub)
	mux.HandleFunc("/scores", s.handleScores)
	mux.HandleFunc("/ranking", s.handleRanking)
	return mux
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleStudents: GET /students, POST /students {"id","name"}
func (s *server) handleStudents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, s.sys.Students())
	case http.MethodPost:
		var st grade.Student
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			writeErr(w, err)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.sys.AddStudent(st.ID, st.Name); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.save(); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleStudentSub: GET|DELETE /students/{id},
// GET /students/{id}/summary, GET /students/{id}/scores
func (s *server) handleStudentSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/students/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case sub == "" && r.Method == http.MethodGet:
		st, err := s.sys.GetStudent(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	case sub == "" && r.Method == http.MethodDelete:
		if err := s.sys.RemoveStudent(id); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.save(); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case sub == "summary" && r.Method == http.MethodGet:
		sum, err := s.sys.StudentSummary(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sum)
	case sub == "scores" && r.Method == http.MethodGet:
		scores, err := s.sys.ScoresOf(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, scores)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleCourses: GET /courses, POST /courses {"id","name"}
func (s *server) handleCourses(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, s.sys.Courses())
	case http.MethodPost:
		var c grade.Course
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			writeErr(w, err)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := s.sys.AddCourse(c.ID, c.Name); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.save(); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleCourseSub: GET|DELETE /courses/{id}, GET /courses/{id}/stats
func (s *server) handleCourseSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/courses/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case sub == "" && r.Method == http.MethodGet:
		c, err := s.sys.GetCourse(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	case sub == "" && r.Method == http.MethodDelete:
		if err := s.sys.RemoveCourse(id); err != nil {
			writeErr(w, err)
			return
		}
		if err := s.save(); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case sub == "stats" && r.Method == http.MethodGet:
		st, err := s.sys.CourseStats(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		if st == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, st)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleScores: POST /scores {"student_id","course_id","value"},
// PUT /scores (update), DELETE /scores
func (s *server) handleScores(w http.ResponseWriter, r *http.Request) {
	var sc grade.Score
	if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
		writeErr(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	switch r.Method {
	case http.MethodPost:
		err = s.sys.AddScore(sc.StudentID, sc.CourseID, sc.Value)
	case http.MethodPut:
		err = s.sys.UpdateScore(sc.StudentID, sc.CourseID, sc.Value)
	case http.MethodDelete:
		err = s.sys.RemoveScore(sc.StudentID, sc.CourseID)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.save(); err != nil {
		writeErr(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

// handleRanking: GET /ranking
func (s *server) handleRanking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.sys.Ranking())
}

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	data := os.Getenv("DATA_FILE")
	if data == "" {
		data = "/data/grades.json"
	}
	srv, err := newServer(data)
	if err != nil {
		log.Fatalf("load data: %v", err)
	}
	log.Printf("gradeserver listening on %s, data=%s", addr, data)
	log.Fatal(http.ListenAndServe(addr, srv.routes()))
}
