package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	s, err := newServer(t.TempDir() + "/grades.json")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", rec.Code)
	}
}

func TestStudentCourseScoreFlow(t *testing.T) {
	s, err := newServer(t.TempDir() + "/grades.json")
	if err != nil {
		t.Fatal(err)
	}
	h := s.routes()

	do := func(method, path, body string) *httptest.ResponseRecorder {
		var rdr *strings.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		} else {
			rdr = strings.NewReader("")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, rdr))
		return rec
	}

	if rec := do(http.MethodPost, "/students", `{"id":"s1","name":"Alice"}`); rec.Code != http.StatusCreated {
		t.Fatalf("add student = %d, want 201: %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/courses", `{"id":"c1","name":"Math"}`); rec.Code != http.StatusCreated {
		t.Fatalf("add course = %d, want 201: %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, "/scores", `{"student_id":"s1","course_id":"c1","value":95}`); rec.Code != http.StatusOK {
		t.Fatalf("add score = %d, want 200: %s", rec.Code, rec.Body)
	}

	rec := do(http.MethodGet, "/courses/c1/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("course stats = %d, want 200", rec.Code)
	}
	var st map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st["count"].(float64) != 1 || st["average"].(float64) != 95 {
		t.Fatalf("unexpected stats: %v", st)
	}

	rec = do(http.MethodGet, "/ranking", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ranking = %d, want 200", rec.Code)
	}

	// duplicate student → conflict
	if rec := do(http.MethodPost, "/students", `{"id":"s1","name":"Alice"}`); rec.Code != http.StatusConflict {
		t.Fatalf("dup student = %d, want 409", rec.Code)
	}
	// unknown student → not found
	if rec := do(http.MethodGet, "/students/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing student = %d, want 404", rec.Code)
	}
	// invalid score → 422
	if rec := do(http.MethodPost, "/scores", `{"student_id":"s1","course_id":"c1","value":101}`); rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusConflict {
		t.Fatalf("invalid score = %d, want 422 or 409", rec.Code)
	}
}
