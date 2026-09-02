# omp-go-test1 — 班级成绩管理系统

Go 实现的班级成绩管理系统:`grade` 包提供领域逻辑(学生/课程/成绩、统计、排名、JSON 持久化),`gradecli` 提供命令行界面。

## 构建与运行

```bash
go build ./...
go run ./cmd/gradecli -- help
```

## CLI 用法

数据默认保存在 `grades.json`,可用 `-data` 指定路径。

```bash
# 学生与课程
gradecli add-student s001 张三
gradecli add-course  math 数学
gradecli list-students
gradecli list-courses

# 成绩
gradecli add-score    s001 math 92
gradecli update-score s001 math 95
gradecli scores       s001
gradecli del-score    s001 math

# 统计与排名
gradecli course-stats math   # 平均分/最高分/最低分/及格率
gradecli rank                # 全班按平均分排名(并列同名次)
```

分数取值范围 0–100;删除学生或课程会级联删除其成绩;修改操作成功后自动写回数据文件。

## 库 API 概览

```go
sys := grade.New()
sys.AddStudent("s001", "张三")
sys.AddCourse("math", "数学")
sys.AddScore("s001", "math", 92)

stats, _ := sys.CourseStats("math")   // Count/Average/Max/Min/PassRate
rank  := sys.Ranking()                // 竞赛排名: 1, 1, 3
sys.SaveToFile("grades.json")
sys, _ = grade.LoadFromFile("grades.json")
```

所有校验错误可通过 `errors.Is` 判定(`ErrDuplicateStudent`、`ErrScoreNotFound`、`ErrInvalidScore` 等)。

## 测试

```bash
go test ./...
```
