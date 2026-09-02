// gradeserver exposes the grade management system over HTTP: a JSON API
// under /api and a single-page frontend served from /.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fys05/omp-go-test1/grade"
)

// server bundles the store with an http.Handler.
type server struct {
	db  *grade.DB
	mux *http.ServeMux
}

func main() {
	var (
		addr = flag.String("addr", ":8080", "listen address")
		dsn  = flag.String("dsn", envOr("DATABASE_URL", "postgres://grades:grades@localhost:5432/grades?sslmode=disable"), "PostgreSQL DSN")
	)
	flag.Parse()

	db, err := grade.OpenDB(*dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Printf("database ready, listening on %s", *addr)

	s := &server{db: db, mux: http.NewServeMux()}
	s.routes()
	log.Fatal(http.ListenAndServe(*addr, s.mux))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func (s *server) routes() {
	s.mux.HandleFunc("/api/students", s.handleStudents)     // GET list, POST create
	s.mux.HandleFunc("/api/students/", s.handleStudentByID) // DELETE; GET /{id}/scores
	s.mux.HandleFunc("/api/courses", s.handleCourses)       // GET list, POST create
	s.mux.HandleFunc("/api/courses/", s.handleCourseByID)   // DELETE; GET /{id}/stats
	s.mux.HandleFunc("/api/scores", s.handleScores)         // POST create
	s.mux.HandleFunc("/api/scores/", s.handleScoreByKey)    // PUT update, DELETE remove
	s.mux.HandleFunc("/api/ranking", s.handleRanking)       // GET ranking
	s.mux.HandleFunc("/healthz", s.handleHealthz)           // liveness/readiness probe
	s.mux.HandleFunc("/", s.handleIndex)                    // frontend (catch-all, last)
}

// handleHealthz pings the database so K8s probes reflect real readiness.
func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
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
	default:
		var inv grade.ErrInvalidScore
		if errors.As(err, &inv) {
			status = http.StatusBadRequest
		}
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return false
	}
	return true
}

// pathPart extracts the single path segment after prefix, or "" if the shape
// is wrong.
func pathPart(path, prefix string) string {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return rest
}

// ---- /api/students ----

func (s *server) handleStudents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.db.Students())
	case http.MethodPost:
		var in struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.db.AddStudent(in.ID, in.Name); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": in.ID})
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *server) handleStudentByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/students/")
	// /api/students/{id}/scores
	if strings.HasSuffix(rest, "/scores") {
		id := strings.TrimSuffix(rest, "/scores")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		scores, err := s.db.ScoresOf(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		sum, _ := s.db.StudentSummary(id)
		writeJSON(w, http.StatusOK, map[string]any{"scores": scores, "summary": sum})
		return
	}
	// /api/students/{id}
	id := pathPart(r.URL.Path, "/api/students/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.db.RemoveStudent(id); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ---- /api/courses ----

func (s *server) handleCourses(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.db.Courses())
	case http.MethodPost:
		var in struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.db.AddCourse(in.ID, in.Name); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": in.ID})
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *server) handleCourseByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/courses/")
	// /api/courses/{id}/stats
	if strings.HasSuffix(rest, "/stats") {
		id := strings.TrimSuffix(rest, "/stats")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		st, err := s.db.CourseStats(id)
		if err != nil {
			writeErr(w, err)
			return
		}
		if st == nil {
			writeJSON(w, http.StatusOK, map[string]any{"course_id": id, "count": 0})
			return
		}
		writeJSON(w, http.StatusOK, st)
		return
	}
	id := pathPart(r.URL.Path, "/api/courses/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := s.db.RemoveCourse(id); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ---- /api/scores ----

func (s *server) handleScores(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		StudentID string  `json:"student_id"`
		CourseID  string  `json:"course_id"`
		Value     float64 `json:"value"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.db.AddScore(in.StudentID, in.CourseID, in.Value); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// handleScoreByKey handles /api/scores/{studentID}/{courseID}.
func (s *server) handleScoreByKey(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/scores/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	sid, cid := parts[0], parts[1]
	switch r.Method {
	case http.MethodPut:
		var in struct {
			Value float64 `json:"value"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.db.UpdateScore(sid, cid, in.Value); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if err := s.db.RemoveScore(sid, cid); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PUT, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ---- /api/ranking ----

func (s *server) handleRanking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	names := make(map[string]string)
	for _, st := range s.db.Students() {
		names[st.ID] = st.Name
	}
	type row struct {
		Rank      int     `json:"rank"`
		StudentID string  `json:"student_id"`
		Name      string  `json:"name"`
		Average   float64 `json:"average"`
		HasScores bool    `json:"has_scores"`
	}
	rank := s.db.Ranking()
	out := make([]row, 0, len(rank))
	for _, e := range rank {
		sum, _ := s.db.StudentSummary(e.StudentID)
		out = append(out, row{
			Rank: e.Rank, StudentID: e.StudentID, Name: names[e.StudentID],
			Average: e.Average, HasScores: sum != nil && sum.Count > 0,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- frontend ----

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}
