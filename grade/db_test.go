package grade

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// testDB opens a Postgres connection using TEST_DATABASE_URL, skipping when
// unset. Each test gets a migrated schema; tests must use distinct IDs.
func testDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestDBStudentCourseLifecycle(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	// isolate from other runs
	d.db.ExecContext(ctx, `DELETE FROM students WHERE id LIKE 't1-%'`)

	if err := d.AddStudent("t1-s1", "测试甲"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddStudent("t1-s1", "dup"); !errors.Is(err, ErrDuplicateStudent) {
		t.Fatalf("want ErrDuplicateStudent, got %v", err)
	}
	if err := d.AddCourse("t1-c1", "测试课"); err != nil {
		t.Fatal(err)
	}
	if err := d.AddCourse("t1-c1", "dup"); !errors.Is(err, ErrDuplicateCourse) {
		t.Fatalf("want ErrDuplicateCourse, got %v", err)
	}

	st, err := d.GetStudent("t1-s1")
	if err != nil || st.Name != "测试甲" {
		t.Fatalf("GetStudent: %v, %v", st, err)
	}
	if _, err := d.GetStudent("t1-ghost"); !errors.Is(err, ErrStudentNotFound) {
		t.Fatalf("want ErrStudentNotFound, got %v", err)
	}
}

func TestDBScoreLifecycleAndCascade(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	d.db.ExecContext(ctx, `DELETE FROM students WHERE id LIKE 't2-%'`)

	d.AddStudent("t2-s1", "甲")
	d.AddCourse("t2-c1", "数学")

	var inv ErrInvalidScore
	if err := d.AddScore("t2-s1", "t2-c1", 150); !errors.As(err, &inv) {
		t.Fatalf("want ErrInvalidScore, got %v", err)
	}
	if err := d.AddScore("t2-ghost", "t2-c1", 90); !errors.Is(err, ErrStudentNotFound) {
		t.Fatalf("want ErrStudentNotFound, got %v", err)
	}
	if err := d.AddScore("t2-s1", "t2-c1", 90); err != nil {
		t.Fatal(err)
	}
	if err := d.AddScore("t2-s1", "t2-c1", 80); !errors.Is(err, ErrDuplicateScore) {
		t.Fatalf("want ErrDuplicateScore, got %v", err)
	}
	if err := d.UpdateScore("t2-s1", "t2-c1", 95); err != nil {
		t.Fatal(err)
	}
	scores, _ := d.ScoresOf("t2-s1")
	if len(scores) != 1 || scores[0].Value != 95 {
		t.Fatalf("update not applied: %v", scores)
	}

	// cascade: removing the course deletes its scores via ON DELETE CASCADE.
	if err := d.RemoveCourse("t2-c1"); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveScore("t2-s1", "t2-c1"); !errors.Is(err, ErrScoreNotFound) {
		t.Fatalf("score survived course removal: %v", err)
	}
}

func TestDBStatsAndRanking(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	d.db.ExecContext(ctx, `DELETE FROM students WHERE id LIKE 't3-%'`)

	d.AddStudent("t3-s1", "甲")
	d.AddStudent("t3-s2", "乙")
	d.AddStudent("t3-s3", "丙") // no scores
	d.AddCourse("t3-c1", "数学")
	d.AddScore("t3-s1", "t3-c1", 100)
	d.AddScore("t3-s2", "t3-c1", 50)

	st, err := d.CourseStats("t3-c1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Count != 2 || st.Average != 75 || st.Max != 100 || st.Min != 50 || st.PassRate != 0.5 {
		t.Fatalf("unexpected stats: %+v", st)
	}

	rank := d.Ranking()
	// find our three students in the global ranking
	byID := make(map[string]RankEntry)
	for _, r := range rank {
		byID[r.StudentID] = r
	}
	if byID["t3-s1"].Rank >= byID["t3-s2"].Rank {
		t.Fatalf("s1 should outrank s2: %v vs %v", byID["t3-s1"], byID["t3-s2"])
	}
	if byID["t3-s2"].Rank >= byID["t3-s3"].Rank {
		t.Fatalf("scoreless student should rank last: %v vs %v", byID["t3-s2"], byID["t3-s3"])
	}
	if byID["t3-s1"].Average != 100 || byID["t3-s2"].Average != 50 {
		t.Fatalf("wrong averages: %v, %v", byID["t3-s1"].Average, byID["t3-s2"].Average)
	}
}
