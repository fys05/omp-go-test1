package grade

import "sort"

// CourseStats aggregates the scores of one course.
type CourseStats struct {
	CourseID string  `json:"course_id"`
	Count    int     `json:"count"`
	Average  float64 `json:"average"`
	Max      float64 `json:"max"`
	Min      float64 `json:"min"`
	PassRate float64 `json:"pass_rate"` // fraction of scores >= 60, in [0,1]
}

// PassScore is the threshold used by CourseStats.PassRate.
const PassScore = 60.0

// StudentSummary aggregates one student's scores across courses.
type StudentSummary struct {
	StudentID string  `json:"student_id"`
	Count     int     `json:"count"`
	Total     float64 `json:"total"`
	Average   float64 `json:"average"`
}

// RankEntry pairs a student with their average and 1-based rank.
// Equal averages share the same rank (competition ranking).
type RankEntry struct {
	StudentID string  `json:"student_id"`
	Average   float64 `json:"average"`
	Rank      int     `json:"rank"`
}

// CourseStats computes statistics for one course. Returns nil when the
// course has no recorded scores.
func (s *System) CourseStats(courseID string) (*CourseStats, error) {
	scores, err := s.ScoresIn(courseID)
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

// StudentSummary aggregates all scores of one student. Count is 0 when the
// student has no scores.
func (s *System) StudentSummary(studentID string) (*StudentSummary, error) {
	scores, err := s.ScoresOf(studentID)
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

// Ranking orders students by average score, descending. Students with no
// scores rank last, ordered by ID. Ties share a rank; the next distinct
// average skips ahead (competition ranking: 1, 1, 3).
func (s *System) Ranking() []RankEntry {
	type avg struct {
		id  string
		avg float64
		n   int
	}
	avgs := make([]avg, 0, len(s.students))
	for sid := range s.students {
		a := avg{id: sid}
		var sum float64
		for _, v := range s.scores[sid] {
			sum += v
			a.n++
		}
		if a.n > 0 {
			a.avg = sum / float64(a.n)
		}
		avgs = append(avgs, a)
	}
	sort.Slice(avgs, func(i, j int) bool {
		// Students without scores always sort last.
		if (avgs[i].n == 0) != (avgs[j].n == 0) {
			return avgs[j].n == 0
		}
		if avgs[i].avg != avgs[j].avg {
			return avgs[i].avg > avgs[j].avg
		}
		return avgs[i].id < avgs[j].id
	})
	out := make([]RankEntry, 0, len(avgs))
	for i, a := range avgs {
		rank := i + 1
		if i > 0 && a.n > 0 && avgs[i-1].n > 0 && a.avg == avgs[i-1].avg {
			rank = out[i-1].Rank
		}
		out = append(out, RankEntry{StudentID: a.id, Average: a.avg, Rank: rank})
	}
	return out
}
