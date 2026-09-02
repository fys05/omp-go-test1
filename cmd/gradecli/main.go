// gradecli is the command-line front end of the class grade management system.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/fys05/omp-go-test1/grade"
)

var errUsage = errors.New("usage error")

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// shared flags parsed before the subcommand.
type globals struct {
	data string
}

func parseGlobals(args []string) (globals, []string, error) {
	var g globals
	fs := flag.NewFlagSet("gradecli", flag.ContinueOnError)
	fs.StringVar(&g.data, "data", "grades.json", "path to the JSON data file")
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return g, nil, errUsage
	}
	return g, fs.Args(), nil
}

func usage() {
	fmt.Fprint(os.Stderr, `班级成绩管理系统 (gradecli)

Usage: gradecli [-data file] <command> [args]

Commands:
  add-student <id> <name>            添加学生
  del-student <id>                   删除学生及其成绩
  list-students                      列出学生
  add-course <id> <name>             添加课程
  del-course <id>                    删除课程及其成绩
  list-courses                       列出课程
  add-score <student> <course> <分>  录入成绩
  update-score <student> <course> <分> 修改成绩
  del-score <student> <course>       删除成绩
  scores <student>                   查询学生成绩
  course-stats <course>              课程统计(平均/最高/最低/及格率)
  rank                               全班排名
`)
}

func run(args []string) error {
	g, rest, err := parseGlobals(args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		usage()
		return errUsage
	}
	sys, err := grade.LoadFromFile(g.data)
	if err != nil {
		return err
	}

	cmd, cmdArgs := rest[0], rest[1:]
	// save is true for mutating commands; queried below after a successful run.
	save := true
	var runErr error

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	switch cmd {
	case "add-student":
		id, name, err := twoArgs(cmd, cmdArgs)
		if err != nil {
			return err
		}
		runErr = sys.AddStudent(id, name)
	case "del-student":
		id, err := oneArg(cmd, cmdArgs)
		if err != nil {
			return err
		}
		runErr = sys.RemoveStudent(id)
	case "list-students":
		save = false
		fmt.Fprintln(w, "ID\t姓名")
		for _, st := range sys.Students() {
			fmt.Fprintf(w, "%s\t%s\n", st.ID, st.Name)
		}
	case "add-course":
		id, name, err := twoArgs(cmd, cmdArgs)
		if err != nil {
			return err
		}
		runErr = sys.AddCourse(id, name)
	case "del-course":
		id, err := oneArg(cmd, cmdArgs)
		if err != nil {
			return err
		}
		runErr = sys.RemoveCourse(id)
	case "list-courses":
		save = false
		fmt.Fprintln(w, "ID\t课程")
		for _, c := range sys.Courses() {
			fmt.Fprintf(w, "%s\t%s\n", c.ID, c.Name)
		}
	case "add-score", "update-score":
		sid, cid, v, err := scoreArgs(cmd, cmdArgs)
		if err != nil {
			return err
		}
		if cmd == "add-score" {
			runErr = sys.AddScore(sid, cid, v)
		} else {
			runErr = sys.UpdateScore(sid, cid, v)
		}
	case "del-score":
		sid, cid, err := twoArgs(cmd, cmdArgs)
		if err != nil {
			return err
		}
		runErr = sys.RemoveScore(sid, cid)
	case "scores":
		sid, err := oneArg(cmd, cmdArgs)
		if err != nil {
			return err
		}
		save = false
		runErr = printScores(w, sys, sid)
	case "course-stats":
		cid, err := oneArg(cmd, cmdArgs)
		if err != nil {
			return err
		}
		save = false
		runErr = printCourseStats(w, sys, cid)
	case "rank":
		save = false
		runErr = printRanking(w, sys)
	default:
		usage()
		return errUsage
	}
	if runErr != nil {
		return runErr
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if save {
		if err := sys.SaveToFile(g.data); err != nil {
			return err
		}
		fmt.Println("已保存。")
	}
	return nil
}

func oneArg(cmd string, args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("%s: 需要 1 个参数, 得到 %d%w", cmd, len(args), errUsage)
	}
	return args[0], nil
}

func twoArgs(cmd string, args []string) (string, string, error) {
	if len(args) != 2 {
		return "", "", fmt.Errorf("%s: 需要 2 个参数, 得到 %d%w", cmd, len(args), errUsage)
	}
	return args[0], args[1], nil
}

func scoreArgs(cmd string, args []string) (sid, cid string, v float64, err error) {
	if len(args) != 3 {
		return "", "", 0, fmt.Errorf("%s: 需要 3 个参数, 得到 %d%w", cmd, len(args), errUsage)
	}
	v, err = strconv.ParseFloat(args[2], 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("%s: 分数 %q 无效: %w", cmd, args[2], errUsage)
	}
	return args[0], args[1], v, nil
}

func printScores(w *tabwriter.Writer, sys *grade.System, sid string) error {
	scores, err := sys.ScoresOf(sid)
	if err != nil {
		return err
	}
	sum, err := sys.StudentSummary(sid)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "课程\t成绩")
	for _, sc := range scores {
		name := sc.CourseID
		if c, err := sys.GetCourse(sc.CourseID); err == nil {
			name = c.Name
		}
		fmt.Fprintf(w, "%s\t%.1f\n", name, sc.Value)
	}
	fmt.Fprintf(w, "总分\t%.1f\n平均分\t%.2f\n", sum.Total, sum.Average)
	return nil
}

func printCourseStats(w *tabwriter.Writer, sys *grade.System, cid string) error {
	st, err := sys.CourseStats(cid)
	if err != nil {
		return err
	}
	if st == nil {
		fmt.Fprintln(w, "该课程暂无成绩记录。")
		return nil
	}
	fmt.Fprintf(w, "人数\t%d\n平均分\t%.2f\n最高分\t%.1f\n最低分\t%.1f\n及格率\t%.1f%%\n",
		st.Count, st.Average, st.Max, st.Min, st.PassRate*100)
	return nil
}

func printRanking(w *tabwriter.Writer, sys *grade.System) error {
	rank := sys.Ranking()
	if len(rank) == 0 {
		fmt.Fprintln(w, "暂无学生。")
		return nil
	}
	fmt.Fprintln(w, "名次\t学号\t姓名\t平均分")
	names := make(map[string]string)
	for _, st := range sys.Students() {
		names[st.ID] = st.Name
	}
	// deterministic output: Ranking already sorted; nothing else to do.
	sort.SliceStable(rank, func(i, j int) bool { return rank[i].Rank < rank[j].Rank })
	for _, r := range rank {
		avg := "-"
		if _, err := sys.ScoresOf(r.StudentID); err == nil {
			if sum, _ := sys.StudentSummary(r.StudentID); sum.Count > 0 {
				avg = fmt.Sprintf("%.2f", r.Average)
			}
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.Rank, r.StudentID, names[r.StudentID], avg)
	}
	return nil
}
