package grade

import (
	"encoding/json"
	"fmt"
	"os"
)

// snapshot is the on-disk representation of a System.
type snapshot struct {
	Students []Student `json:"students"`
	Courses  []Course  `json:"courses"`
	Scores   []Score   `json:"scores"`
}

// SaveToFile persists the system as indented JSON, atomically replacing any
// existing file via a temp file + rename.
func (s *System) SaveToFile(path string) error {
	snap := snapshot{
		Students: s.Students(),
		Courses:  s.Courses(),
	}
	for sid, perCourse := range s.scores {
		for cid, v := range perCourse {
			snap.Scores = append(snap.Scores, Score{StudentID: sid, CourseID: cid, Value: v})
		}
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// LoadFromFile reads a JSON snapshot previously written by SaveToFile.
// A missing file yields an empty System. All entries are validated through
// the normal Add* paths, so a corrupt file fails loudly instead of
// producing an inconsistent state.
func LoadFromFile(path string) (*System, error) {
	s := New()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var snap snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, st := range snap.Students {
		if err := s.AddStudent(st.ID, st.Name); err != nil {
			return nil, fmt.Errorf("load student %q: %w", st.ID, err)
		}
	}
	for _, c := range snap.Courses {
		if err := s.AddCourse(c.ID, c.Name); err != nil {
			return nil, fmt.Errorf("load course %q: %w", c.ID, err)
		}
	}
	for _, sc := range snap.Scores {
		if err := s.AddScore(sc.StudentID, sc.CourseID, sc.Value); err != nil {
			return nil, fmt.Errorf("load score %s/%s: %w", sc.StudentID, sc.CourseID, err)
		}
	}
	return s, nil
}
