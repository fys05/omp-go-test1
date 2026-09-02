# omp-go-test1 — 班级成绩管理系统

Go 实现的班级成绩管理系统,三种使用方式:

- **`gradeserver`** — 全栈版: PostgreSQL 数据库 + JSON HTTP API + 内嵌单页前端
- **`gradecli`** — 命令行版: 内存 + JSON 文件持久化
- **`grade`** — 领域库: 两套实现(`System` 内存版 / `DB` Postgres 版)共享同一套领域错误与统计逻辑

## 全栈版 (gradeserver)

启动数据库并运行服务:

```bash
docker run -d --name grades-db \
  -e POSTGRES_USER=grades -e POSTGRES_PASSWORD=grades -e POSTGRES_DB=grades \
  -p 5432:5432 postgres:16-alpine

go run ./cmd/gradeserver -addr :8080
# 或用 DATABASE_URL 指定连接串
```

启动时自动建表(`Migrate` 幂等)。浏览器打开 http://localhost:8080 使用前端。

### HTTP API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/api/students` | 列出 / 添加学生 `{id, name}` |
| DELETE | `/api/students/{id}` | 删除学生(级联成绩) |
| GET | `/api/students/{id}/scores` | 学生成绩 + 总分/平均分 |
| GET/POST | `/api/courses` | 列出 / 添加课程 `{id, name}` |
| DELETE | `/api/courses/{id}` | 删除课程(级联成绩) |
| GET | `/api/courses/{id}/stats` | 人数/平均分/最高/最低/及格率 |
| POST | `/api/scores` | 录入成绩 `{student_id, course_id, value}` |
| PUT/DELETE | `/api/scores/{sid}/{cid}` | 修改 / 删除成绩 |
| GET | `/api/ranking` | 全班排名(按平均分, 并列同名次, 无成绩排最后) |

错误以 JSON 返回: 404 不存在 / 409 重复 / 400 分数越界(0–100)。

## 部署 (K8s)

`k8s/app/` 含应用与 PostgreSQL 的 Deployment/Service/Ingress;`.github/workflows/ci.yml` 提供 lint → test(带 Postgres service) → 构建镜像 → 部署 → 集群内 smoke test 的完整流水线。需配置的 GitHub secrets/vars:

- secrets: `K8S_CLIENT_CERT` / `K8S_CLIENT_KEY` / `K8S_CA_CERT` / `DB_PASSWORD`
- vars: `K8S_HOST` / `K8S_PORT` / `K8S_NAMESPACE` / `K8S_EXPECTED_API_SERVER` / `K8S_EXPECTED_CA_SHA256` / `APP_NAME` / `DOMAIN`

部署时由 CI 创建 `${APP_NAME}-db` secret(含 `DATABASE_URL` 与 `POSTGRES_PASSWORD`),数据库与应用同命名空间内网通信。

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

# Postgres 集成测试需要 TEST_DATABASE_URL, 未设置时自动跳过:
TEST_DATABASE_URL="postgres://grades:grades@localhost:5432/grades?sslmode=disable" go test ./...
```
