// Package grade implements the core domain logic of a class grade
// management system: students, courses, scores, statistics and ranking.
package grade

import (
	"errors"
	"fmt"
	"sort"
)

// Score bounds shared by every validation path.
const (
	MinScore = 0.0
	MaxScore = 100.0
)

// Common domain errors.
var (
	ErrStudentNotFound  = errors.New("student not found")
	ErrCourseNotFound   = errors.New("course not found")
	ErrDuplicateStudent = errors.New("student already exists")
	ErrDuplicateCourse  = errors.New("course already exists")
	ErrDuplicateScore   = errors.New("score already recorded for student and course")
	ErrScoreNotFound    = errors.New("score not recorded for student and course")
	ErrEmptyName        = errors.New("name must not be empty")
)

// ErrInvalidScore is returned when a score falls outside [MinScore, MaxScore].
type ErrInvalidScore struct{ Score float64 }

func (e ErrInvalidScore) Error() string {
	return fmt.Sprintf("invalid score %.2f: must be between %.0f and %.0f", e.Score, MinScore, MaxScore)
}

// Student is a class member identified by a unique ID.
type Student struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Course is a subject for which scores are recorded.
type Course struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Score is one student's result in one course.
type Score struct {
	StudentID string  `json:"student_id"`
	CourseID  string  `json:"course_id"`
	Value     float64 `json:"value"`
}

// System holds the whole grade book in memory.
type System struct {
	students map[string]Student
	courses  map[string]Course
	scores   map[string]map[string]float64 // studentID -> courseID -> value
}

// New returns an empty System.
func New() *System {
	return &System{
		students: make(map[string]Student),
		courses:  make(map[string]Course),
		scores:   make(map[string]map[string]float64),
	}
}

// validateScore rejects values outside [MinScore, MaxScore].
func validateScore(v float64) error {
	if v < MinScore || v > MaxScore {
		return ErrInvalidScore{Score: v}
	}
	return nil
}

// AddStudent registers a student. ID and Name must be non-empty; IDs are unique.
func (s *System) AddStudent(id, name string) error {
	if id == "" || name == "" {
		return ErrEmptyName
	}
	if _, ok := s.students[id]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateStudent, id)
	}
	s.students[id] = Student{ID: id, Name: name}
	return nil
}

// RemoveStudent deletes a student and all of their scores.
func (s *System) RemoveStudent(id string) error {
	if _, ok := s.students[id]; !ok {
		return fmt.Errorf("%w: %s", ErrStudentNotFound, id)
	}
	delete(s.students, id)
	delete(s.scores, id)
	return nil
}

// GetStudent returns a student by ID.
func (s *System) GetStudent(id string) (Student, error) {
	st, ok := s.students[id]
	if !ok {
		return Student{}, fmt.Errorf("%w: %s", ErrStudentNotFound, id)
	}
	return st, nil
}

// Students lists every student ordered by ID.
func (s *System) Students() []Student {
	out := make([]Student, 0, len(s.students))
	for _, st := range s.students {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddCourse registers a course. ID and Name must be non-empty; IDs are unique.
func (s *System) AddCourse(id, name string) error {
	if id == "" || name == "" {
		return ErrEmptyName
	}
	if _, ok := s.courses[id]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateCourse, id)
	}
	s.courses[id] = Course{ID: id, Name: name}
	return nil
}

// RemoveCourse deletes a course and every score recorded for it.
func (s *System) RemoveCourse(id string) error {
	if _, ok := s.courses[id]; !ok {
		return fmt.Errorf("%w: %s", ErrCourseNotFound, id)
	}
	delete(s.courses, id)
	for sid := range s.scores {
		delete(s.scores[sid], id)
	}
	return nil
}

// GetCourse returns a course by ID.
func (s *System) GetCourse(id string) (Course, error) {
	c, ok := s.courses[id]
	if !ok {
		return Course{}, fmt.Errorf("%w: %s", ErrCourseNotFound, id)
	}
	return c, nil
}

// Courses lists every course ordered by ID.
func (s *System) Courses() []Course {
	out := make([]Course, 0, len(s.courses))
	for _, c := range s.courses {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AddScore records a score; the (student, course) pair must not already exist.
func (s *System) AddScore(studentID, courseID string, value float64) error {
	if err := validateScore(value); err != nil {
		return err
	}
	if _, err := s.GetStudent(studentID); err != nil {
		return err
	}
	if _, err := s.GetCourse(courseID); err != nil {
		return err
	}
	if s.scores[studentID] == nil {
		s.scores[studentID] = make(map[string]float64)
	}
	if _, ok := s.scores[studentID][courseID]; ok {
		return fmt.Errorf("%w: %s/%s", ErrDuplicateScore, studentID, courseID)
	}
	s.scores[studentID][courseID] = value
	return nil
}

// UpdateScore overwrites an existing score.
func (s *System) UpdateScore(studentID, courseID string, value float64) error {
	if err := validateScore(value); err != nil {
		return err
	}
	if _, ok := s.scores[studentID][courseID]; !ok {
		return fmt.Errorf("%w: %s/%s", ErrScoreNotFound, studentID, courseID)
	}
	s.scores[studentID][courseID] = value
	return nil
}

// RemoveScore deletes one score entry.
func (s *System) RemoveScore(studentID, courseID string) error {
	if _, ok := s.scores[studentID][courseID]; !ok {
		return fmt.Errorf("%w: %s/%s", ErrScoreNotFound, studentID, courseID)
	}
	delete(s.scores[studentID], courseID)
	return nil
}

// ScoresOf returns one student's scores ordered by course ID.
func (s *System) ScoresOf(studentID string) ([]Score, error) {
	if _, err := s.GetStudent(studentID); err != nil {
		return nil, err
	}
	out := make([]Score, 0, len(s.scores[studentID]))
	for cid, v := range s.scores[studentID] {
		out = append(out, Score{StudentID: studentID, CourseID: cid, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CourseID < out[j].CourseID })
	return out, nil
}

// ScoresIn returns every score recorded in one course, ordered by student ID.
func (s *System) ScoresIn(courseID string) ([]Score, error) {
	if _, err := s.GetCourse(courseID); err != nil {
		return nil, err
	}
	var out []Score
	for sid, perCourse := range s.scores {
		if v, ok := perCourse[courseID]; ok {
			out = append(out, Score{StudentID: sid, CourseID: courseID, Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StudentID < out[j].StudentID })
	return out, nil
}
