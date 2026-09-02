package grade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	// Pure-Go PostgreSQL driver registered as "postgres" with database/sql.
	"github.com/lib/pq"
)

// pgErrUniqueViolation is the SQLSTATE Postgres returns on PK/unique conflicts.
const pgErrUniqueViolation = "23505"

// DB is a PostgreSQL-backed System. It satisfies the same operations as the
// in-memory System and adds Migrate plus Close.
type DB struct {
	db *sql.DB
}

func OpenDB(dsn string) (*DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	ctx := context.Background()
	var lastErr error
	for i := 0; i < 30; i++ {
		if lastErr = db.PingContext(ctx); lastErr == nil {
			return &DB{db: db}, nil
		}
		time.Sleep(time.Second)
	}
	db.Close()
	return nil, fmt.Errorf("ping db: %w", lastErr)
}

// Close releases the connection pool.
func (d *DB) Close() error { return d.db.Close() }

// PingContext checks database liveness.
func (d *DB) PingContext(ctx context.Context) error { return d.db.PingContext(ctx) }

// Migrate creates the schema if it does not yet exist. Safe to run on every
// startup.
func (d *DB) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS students (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS courses (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scores (
			student_id TEXT NOT NULL REFERENCES students(id) ON DELETE CASCADE,
			course_id  TEXT NOT NULL REFERENCES courses(id)  ON DELETE CASCADE,
			value      DOUBLE PRECISION NOT NULL CHECK (value >= 0 AND value <= 100),
			PRIMARY KEY (student_id, course_id)
		)`,
	}
	for _, q := range stmts {
		if _, err := d.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// isUniqueViolation reports whether err wraps a Postgres unique-violation.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == pgErrUniqueViolation
	}
	return false
}

func (d *DB) AddStudent(id, name string) error {
	if id == "" || name == "" {
		return ErrEmptyName
	}
	_, err := d.db.Exec(`INSERT INTO students (id, name) VALUES ($1, $2)`, id, name)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: %s", ErrDuplicateStudent, id)
	}
	return err
}

func (d *DB) RemoveStudent(id string) error {
	res, err := d.db.Exec(`DELETE FROM students WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s", ErrStudentNotFound, id)
	}
	return nil
}

func (d *DB) GetStudent(id string) (Student, error) {
	var st Student
	err := d.db.QueryRow(`SELECT id, name FROM students WHERE id = $1`, id).Scan(&st.ID, &st.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Student{}, fmt.Errorf("%w: %s", ErrStudentNotFound, id)
	}
	return st, err
}

func (d *DB) Students() []Student {
	rows, err := d.db.Query(`SELECT id, name FROM students ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Student
	for rows.Next() {
		var st Student
		if rows.Scan(&st.ID, &st.Name) == nil {
			out = append(out, st)
		}
	}
	return out
}

func (d *DB) AddCourse(id, name string) error {
	if id == "" || name == "" {
		return ErrEmptyName
	}
	_, err := d.db.Exec(`INSERT INTO courses (id, name) VALUES ($1, $2)`, id, name)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: %s", ErrDuplicateCourse, id)
	}
	return err
}

func (d *DB) RemoveCourse(id string) error {
	res, err := d.db.Exec(`DELETE FROM courses WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s", ErrCourseNotFound, id)
	}
	return nil
}

func (d *DB) GetCourse(id string) (Course, error) {
	var c Course
	err := d.db.QueryRow(`SELECT id, name FROM courses WHERE id = $1`, id).Scan(&c.ID, &c.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return Course{}, fmt.Errorf("%w: %s", ErrCourseNotFound, id)
	}
	return c, err
}

func (d *DB) Courses() []Course {
	rows, err := d.db.Query(`SELECT id, name FROM courses ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Course
	for rows.Next() {
		var c Course
		if rows.Scan(&c.ID, &c.Name) == nil {
			out = append(out, c)
		}
	}
	return out
}

// mustExist wraps a lookup error into the caller-visible domain errors.
func (d *DB) AddScore(studentID, courseID string, value float64) error {
	if err := validateScore(value); err != nil {
		return err
	}
	if _, err := d.GetStudent(studentID); err != nil {
		return err
	}
	if _, err := d.GetCourse(courseID); err != nil {
		return err
	}
	_, err := d.db.Exec(
		`INSERT INTO scores (student_id, course_id, value) VALUES ($1, $2, $3)`,
		studentID, courseID, value)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: %s/%s", ErrDuplicateScore, studentID, courseID)
	}
	return err
}

func (d *DB) UpdateScore(studentID, courseID string, value float64) error {
	if err := validateScore(value); err != nil {
		return err
	}
	res, err := d.db.Exec(
		`UPDATE scores SET value = $3 WHERE student_id = $1 AND course_id = $2`,
		studentID, courseID, value)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s/%s", ErrScoreNotFound, studentID, courseID)
	}
	return nil
}

func (d *DB) RemoveScore(studentID, courseID string) error {
	res, err := d.db.Exec(
		`DELETE FROM scores WHERE student_id = $1 AND course_id = $2`, studentID, courseID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s/%s", ErrScoreNotFound, studentID, courseID)
	}
	return nil
}

func (d *DB) ScoresOf(studentID string) ([]Score, error) {
	if _, err := d.GetStudent(studentID); err != nil {
		return nil, err
	}
	rows, err := d.db.Query(
		`SELECT course_id, value FROM scores WHERE student_id = $1 ORDER BY course_id`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Score
	for rows.Next() {
		var sc Score
		sc.StudentID = studentID
		if err := rows.Scan(&sc.CourseID, &sc.Value); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (d *DB) ScoresIn(courseID string) ([]Score, error) {
	if _, err := d.GetCourse(courseID); err != nil {
		return nil, err
	}
	rows, err := d.db.Query(
		`SELECT student_id, value FROM scores WHERE course_id = $1 ORDER BY student_id`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Score
	for rows.Next() {
		var sc Score
		sc.CourseID = courseID
		if err := rows.Scan(&sc.StudentID, &sc.Value); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (d *DB) CourseStats(courseID string) (*CourseStats, error) {
	scores, err := d.ScoresIn(courseID)
	if err != nil {
		return nil, err
	}
	if len(scores) == 0 {
		return nil, nil
	}
	st := &CourseStats{CourseID: courseID, Count: len(scores), Max: scores[0].Value, Min: scores[0].Value}
	passed := 0
	var sum float64
	for _, sc := range scores {
		sum += sc.Value
		if sc.Value > st.Max {
			st.Max = sc.Value
		}
		if sc.Value < st.Min {
			st.Min = sc.Value
		}
		if sc.Value >= PassScore {
			passed++
		}
	}
	st.Average = sum / float64(len(scores))
	st.PassRate = float64(passed) / float64(len(scores))
	return st, nil
}

func (d *DB) StudentSummary(studentID string) (*StudentSummary, error) {
	scores, err := d.ScoresOf(studentID)
	if err != nil {
		return nil, err
	}
	sum := &StudentSummary{StudentID: studentID, Count: len(scores)}
	for _, sc := range scores {
		sum.Total += sc.Value
	}
	if sum.Count > 0 {
		sum.Average = sum.Total / float64(sum.Count)
	}
	return sum, nil
}

// Ranking computes class ranking in SQL: one row per student, average over
// their scores (NULL when none), ordered average-desc with NULLs last.
func (d *DB) Ranking() []RankEntry {
	rows, err := d.db.Query(`
		SELECT s.id, AVG(sc.value), COUNT(sc.value)
		FROM students s
		LEFT JOIN scores sc ON sc.student_id = s.id
		GROUP BY s.id
		ORDER BY AVG(sc.value) DESC NULLS LAST, s.id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	type row struct {
		id  string
		avg sql.NullFloat64
		n   int
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.avg, &r.n); err != nil {
			return nil
		}
		rs = append(rs, r)
	}
	out := make([]RankEntry, 0, len(rs))
	for i, r := range rs {
		avg := 0.0
		if r.avg.Valid {
			avg = r.avg.Float64
		}
		rank := i + 1
		if i > 0 && r.avg.Valid && rs[i-1].avg.Valid && avg == rs[i-1].avg.Float64 {
			rank = out[i-1].Rank
		}
		out = append(out, RankEntry{StudentID: r.id, Average: avg, Rank: rank})
	}
	return out
}
