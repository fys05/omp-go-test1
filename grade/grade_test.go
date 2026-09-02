package grade

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mustAddStudent(t *testing.T, s *System, id, name string) {
	t.Helper()
	if err := s.AddStudent(id, name); err != nil {
		t.Fatalf("AddStudent(%q): %v", id, err)
	}
}

func mustAddCourse(t *testing.T, s *System, id, name string) {
	t.Helper()
	if err := s.AddCourse(id, name); err != nil {
		t.Fatalf("AddCourse(%q): %v", id, err)
	}
}

func mustAddScore(t *testing.T, s *System, sid, cid string, v float64) {
	t.Helper()
	if err := s.AddScore(sid, cid, v); err != nil {
		t.Fatalf("AddScore(%q,%q,%.1f): %v", sid, cid, v, err)
	}
}

func TestAddStudentRejectsDuplicatesAndEmptyFields(t *testing.T) {
	s := New()
	mustAddStudent(t, s, "s1", "张三")
	if err := s.AddStudent("s1", "李四"); !errors.Is(err, ErrDuplicateStudent) {
		t.Fatalf("want ErrDuplicateStudent, got %v", err)
	}
	for _, tc := range [][2]string{{"", "x"}, {"s2", ""}} {
		if err := s.AddStudent(tc[0], tc[1]); !errors.Is(err, ErrEmptyName) {
			t.Fatalf("AddStudent%v: want ErrEmptyName, got %v", tc, err)
		}
	}
}

func TestRemoveStudentCascadesScores(t *testing.T) {
	s := New()
	mustAddStudent(t, s, "s1", "张三")
	mustAddCourse(t, s, "c1", "数学")
	mustAddScore(t, s, "s1", "c1", 90)
	if err := s.RemoveStudent("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetStudent("s1"); !errors.Is(err, ErrStudentNotFound) {
		t.Fatalf("want ErrStudentNotFound, got %v", err)
	}
	// The score must be gone: re-adding the student yields an empty gradebook.
	mustAddStudent(t, s, "s1", "张三")
	scores, err := s.ScoresOf("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 0 {
		t.Fatalf("scores survived student removal: %v", scores)
	}
}

func TestRemoveCourseCascadesScores(t *testing.T) {
	s := New()
	mustAddStudent(t, s, "s1", "张三")
	mustAddCourse(t, s, "c1", "数学")
	mustAddScore(t, s, "s1", "c1", 90)
	if err := s.RemoveCourse("c1"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveScore("s1", "c1"); !errors.Is(err, ErrScoreNotFound) {
		t.Fatalf("score survived course removal: %v", err)
	}
}

func TestScoreValidationAndLifecycle(t *testing.T) {
	s := New()
	mustAddStudent(t, s, "s1", "张三")
	mustAddCourse(t, s, "c1", "数学")

	for _, v := range []float64{-1, 100.5} {
		var inv ErrInvalidScore
		if err := s.AddScore("s1", "c1", v); !errors.As(err, &inv) {
			t.Fatalf("AddScore(%.1f): want ErrInvalidScore, got %v", v, err)
		}
	}
	if err := s.AddScore("ghost", "c1", 90); !errors.Is(err, ErrStudentNotFound) {
		t.Fatalf("want ErrStudentNotFound, got %v", err)
	}
	if err := s.AddScore("s1", "ghost", 90); !errors.Is(err, ErrCourseNotFound) {
		t.Fatalf("want ErrCourseNotFound, got %v", err)
	}

	mustAddScore(t, s, "s1", "c1", 90)
	if err := s.AddScore("s1", "c1", 80); !errors.Is(err, ErrDuplicateScore) {
		t.Fatalf("want ErrDuplicateScore, got %v", err)
	}
	if err := s.UpdateScore("s1", "c1", 95); err != nil {
		t.Fatal(err)
	}
	scores, _ := s.ScoresOf("s1")
	if scores[0].Value != 95 {
		t.Fatalf("update not applied: %v", scores[0].Value)
	}
	if err := s.UpdateScore("s1", "c1", 101); err == nil {
		t.Fatal("UpdateScore accepted out-of-range value")
	}
	if err := s.UpdateScore("s1", "nope", 50); !errors.Is(err, ErrScoreNotFound) {
		t.Fatalf("want ErrScoreNotFound, got %v", err)
	}
}

func TestCourseStats(t *testing.T) {
	s := New()
	mustAddStudent(t, s, "s1", "甲")
	mustAddStudent(t, s, "s2", "乙")
	mustAddCourse(t, s, "c1", "数学")
	mustAddScore(t, s, "s1", "c1", 100)
	mustAddScore(t, s, "s2", "c1", 50)

	st, err := s.CourseStats("c1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Count != 2 || st.Average != 75 || st.Max != 100 || st.Min != 50 || st.PassRate != 0.5 {
		t.Fatalf("unexpected stats: %+v", st)
	}

	// No scores yet → nil stats, no error.
	mustAddCourse(t, s, "c2", "英语")
	empty, err := s.CourseStats("c2")
	if err != nil || empty != nil {
		t.Fatalf("want (nil, nil), got (%v, %v)", empty, err)
	}
	if _, err := s.CourseStats("ghost"); !errors.Is(err, ErrCourseNotFound) {
		t.Fatalf("want ErrCourseNotFound, got %v", err)
	}
}

func TestRankingCompetitionRanksAndScorelessLast(t *testing.T) {
	s := New()
	mustAddCourse(t, s, "c1", "数学")
	mustAddCourse(t, s, "c2", "英语")
	for _, st := range []Student{{"s1", "甲"}, {"s2", "乙"}, {"s3", "丙"}, {"s4", "丁"}} {
		mustAddStudent(t, s, st.ID, st.Name)
	}
	// s1 avg 90, s2 avg 90 (tie), s3 avg 70, s4 no scores.
	mustAddScore(t, s, "s1", "c1", 90)
	mustAddScore(t, s, "s2", "c1", 80)
	mustAddScore(t, s, "s2", "c2", 100)
	mustAddScore(t, s, "s3", "c1", 70)

	rank := s.Ranking()
	want := []RankEntry{
		{StudentID: "s1", Average: 90, Rank: 1},
		{StudentID: "s2", Average: 90, Rank: 1},
		{StudentID: "s3", Average: 70, Rank: 3},
		{StudentID: "s4", Average: 0, Rank: 4},
	}
	if len(rank) != len(want) {
		t.Fatalf("len = %d, want %d: %v", len(rank), len(want), rank)
	}
	for i, w := range want {
		if rank[i] != w {
			t.Fatalf("rank[%d] = %+v, want %+v", i, rank[i], w)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grades.json")

	s := New()
	mustAddStudent(t, s, "s1", "张三")
	mustAddStudent(t, s, "s2", "李四")
	mustAddCourse(t, s, "c1", "数学")
	mustAddScore(t, s, "s1", "c1", 88.5)
	mustAddScore(t, s, "s2", "c1", 76)
	if err := s.SaveToFile(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Students()) != 2 || len(loaded.Courses()) != 1 {
		t.Fatalf("round trip lost entities: %v students, %v courses", len(loaded.Students()), len(loaded.Courses()))
	}
	scores, err := loaded.ScoresOf("s1")
	if err != nil || len(scores) != 1 || scores[0].Value != 88.5 {
		t.Fatalf("round trip lost scores: %v, %v", scores, err)
	}

	// Mutating the loaded copy persists independently.
	if err := loaded.UpdateScore("s1", "c1", 99); err != nil {
		t.Fatal(err)
	}
	if err := loaded.SaveToFile(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	scores, _ = reloaded.ScoresOf("s1")
	if scores[0].Value != 99 {
		t.Fatalf("second save lost update: %v", scores[0].Value)
	}
}

func TestLoadFromMissingFileYieldsEmptySystem(t *testing.T) {
	s, err := LoadFromFile(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Students()) != 0 || len(s.Courses()) != 0 {
		t.Fatal("missing file should yield an empty system")
	}
}

func TestLoadRejectsCorruptData(t *testing.T) {
	dir := t.TempDir()
	// A duplicate student in the snapshot must fail loudly.
	path := filepath.Join(dir, "dup.json")
	if err := os.WriteFile(path, []byte(`{"students":[{"id":"s1","name":"a"},{"id":"s1","name":"b"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromFile(path); err == nil {
		t.Fatal("corrupt snapshot loaded without error")
	}
}
