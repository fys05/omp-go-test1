package main

// indexHTML is the single-page frontend, embedded into the binary so the
// server is self-contained. It talks to /api with fetch().
const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>班级成绩管理系统</title>
<style>
  * { box-sizing: border-box; }
  body { font-family: -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif;
         margin: 0; background: #f0f2f5; color: #222; }
  header { background: #1f3a5f; color: #fff; padding: 14px 24px; }
  header h1 { margin: 0; font-size: 18px; }
  main { max-width: 1100px; margin: 20px auto; padding: 0 16px; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  @media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }
  .card { background: #fff; border-radius: 8px; padding: 16px;
          box-shadow: 0 1px 3px rgba(0,0,0,.08); }
  .card h2 { margin: 0 0 12px; font-size: 15px; color: #1f3a5f; }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th, td { padding: 6px 8px; border-bottom: 1px solid #eee; text-align: left; }
  th { background: #f7f9fb; }
  input, select { padding: 6px 8px; border: 1px solid #ccc; border-radius: 4px;
                  font-size: 14px; margin-right: 6px; }
  button { padding: 6px 12px; border: 0; border-radius: 4px; background: #1f3a5f;
           color: #fff; cursor: pointer; font-size: 14px; }
  button.del { background: #b3382c; padding: 3px 8px; font-size: 12px; }
  button:hover { opacity: .9; }
  form { margin-bottom: 12px; display: flex; flex-wrap: wrap; gap: 4px; }
  #toast { position: fixed; top: 16px; right: 16px; padding: 10px 16px;
           border-radius: 6px; color: #fff; display: none; z-index: 10; }
  #toast.ok { background: #2e7d32; } #toast.err { background: #b3382c; }
  .muted { color: #888; font-size: 13px; }
  .full { grid-column: 1 / -1; }
</style>
</head>
<body>
<header><h1>班级成绩管理系统</h1></header>
<main>
  <div class="grid">
    <div class="card">
      <h2>学生管理</h2>
      <form id="f-student">
        <input id="s-id" placeholder="学号" required>
        <input id="s-name" placeholder="姓名" required>
        <button>添加学生</button>
      </form>
      <table><thead><tr><th>学号</th><th>姓名</th><th></th></tr></thead>
        <tbody id="t-students"></tbody></table>
    </div>
    <div class="card">
      <h2>课程管理</h2>
      <form id="f-course">
        <input id="c-id" placeholder="课程号" required>
        <input id="c-name" placeholder="课程名" required>
        <button>添加课程</button>
      </form>
      <table><thead><tr><th>课程号</th><th>课程名</th><th></th></tr></thead>
        <tbody id="t-courses"></tbody></table>
    </div>
    <div class="card">
      <h2>成绩录入 / 修改</h2>
      <form id="f-score">
        <select id="sc-student" required></select>
        <select id="sc-course" required></select>
        <input id="sc-value" type="number" min="0" max="100" step="0.5"
               placeholder="分数" required style="width:90px">
        <button type="submit">录入</button>
        <button type="button" id="sc-update">修改</button>
      </form>
      <p class="muted">“录入”新增一条成绩; “修改”覆盖已有成绩。</p>
      <h2 style="margin-top:16px">学生成绩查询</h2>
      <form id="f-query">
        <select id="q-student" required></select>
        <button>查询</button>
      </form>
      <div id="query-result"></div>
    </div>
    <div class="card">
      <h2>课程统计</h2>
      <form id="f-stats">
        <select id="st-course" required></select>
        <button>统计</button>
      </form>
      <div id="stats-result"></div>
    </div>
    <div class="card full">
      <h2>全班排名 <button id="btn-rank" style="font-size:12px;padding:3px 10px">刷新</button></h2>
      <table><thead><tr><th>名次</th><th>学号</th><th>姓名</th><th>平均分</th></tr></thead>
        <tbody id="t-rank"></tbody></table>
    </div>
  </div>
</main>
<div id="toast"></div>
<script>
const $ = id => document.getElementById(id);

function toast(msg, ok) {
  const t = $("toast");
  t.textContent = msg;
  t.className = ok ? "ok" : "err";
  t.style.display = "block";
  setTimeout(() => t.style.display = "none", 2500);
}

async function api(method, path, body) {
  const r = await fetch(path, {
    method,
    headers: body ? {"Content-Type": "application/json"} : {},
    body: body ? JSON.stringify(body) : undefined,
  });
  if (r.status === 204) return null;
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error(data.error || ("HTTP " + r.status));
  return data;
}

async function refreshAll() {
  const [students, courses] = await Promise.all([
    api("GET", "/api/students"), api("GET", "/api/courses"),
  ]);
  renderStudents(students || []);
  renderCourses(courses || []);
  fillSelects(students || [], courses || []);
  refreshRanking();
}

function renderStudents(list) {
  $("t-students").innerHTML = list.map(s =>
    "<tr><td>" + esc(s.id) + "</td><td>" + esc(s.name) + "</td>" +
    "<td><button class='del' onclick=\"delStudent('" + esc(s.id) + "')\">删除</button></td></tr>"
  ).join("") || "<tr><td colspan=3 class='muted'>暂无学生</td></tr>";
}

function renderCourses(list) {
  $("t-courses").innerHTML = list.map(c =>
    "<tr><td>" + esc(c.id) + "</td><td>" + esc(c.name) + "</td>" +
    "<td><button class='del' onclick=\"delCourse('" + esc(c.id) + "')\">删除</button></td></tr>"
  ).join("") || "<tr><td colspan=3 class='muted'>暂无课程</td></tr>";
}

function fillSelects(students, courses) {
  for (const id of ["sc-student", "q-student"]) {
    $(id).innerHTML = students.map(s =>
      "<option value='" + esc(s.id) + "'>" + esc(s.name) + " (" + esc(s.id) + ")</option>").join("");
  }
  for (const id of ["sc-course", "st-course"]) {
    $(id).innerHTML = courses.map(c =>
      "<option value='" + esc(c.id) + "'>" + esc(c.name) + " (" + esc(c.id) + ")</option>").join("");
  }
}

async function refreshRanking() {
  const rows = await api("GET", "/api/ranking");
  $("t-rank").innerHTML = (rows || []).map(r =>
    "<tr><td>" + r.rank + "</td><td>" + esc(r.student_id) + "</td><td>" + esc(r.name) +
    "</td><td>" + (r.has_scores ? r.average.toFixed(2) : "-") + "</td></tr>"
  ).join("") || "<tr><td colspan=4 class='muted'>暂无数据</td></tr>";
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, c =>
    ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
}

async function run(fn) {
  try { await fn(); } catch (e) { toast(e.message, false); }
}

$("f-student").onsubmit = e => { e.preventDefault(); run(async () => {
  await api("POST", "/api/students", {id: $("s-id").value.trim(), name: $("s-name").value.trim()});
  toast("学生已添加", true); e.target.reset(); refreshAll();
});};

$("f-course").onsubmit = e => { e.preventDefault(); run(async () => {
  await api("POST", "/api/courses", {id: $("c-id").value.trim(), name: $("c-name").value.trim()});
  toast("课程已添加", true); e.target.reset(); refreshAll();
});};

function scoreBody() {
  return {student_id: $("sc-student").value, course_id: $("sc-course").value,
          value: parseFloat($("sc-value").value)};
}
$("f-score").onsubmit = e => { e.preventDefault(); run(async () => {
  await api("POST", "/api/scores", scoreBody());
  toast("成绩已录入", true); refreshAll();
});};
$("sc-update").onclick = () => run(async () => {
  const b = scoreBody();
  await api("PUT", "/api/scores/" + encodeURIComponent(b.student_id) + "/" + encodeURIComponent(b.course_id),
            {value: b.value});
  toast("成绩已修改", true); refreshAll();
});

$("f-query").onsubmit = e => { e.preventDefault(); run(async () => {
  const id = $("q-student").value;
  const data = await api("GET", "/api/students/" + encodeURIComponent(id) + "/scores");
  const rows = (data.scores || []).map(sc =>
    "<tr><td>" + esc(sc.course_id) + "</td><td>" + sc.value.toFixed(1) + "</td>" +
    "<td><button class='del' onclick=\"delScore('" + esc(id) + "','" + esc(sc.course_id) + "')\">删除</button></td></tr>"
  ).join("");
  const sum = data.summary;
  $("query-result").innerHTML =
    "<table><thead><tr><th>课程</th><th>成绩</th><th></th></tr></thead><tbody>" +
    (rows || "<tr><td colspan=3 class='muted'>暂无成绩</td></tr>") + "</tbody></table>" +
    (sum && sum.count > 0
      ? "<p>总分 " + sum.total.toFixed(1) + " · 平均分 " + sum.average.toFixed(2) + "</p>"
      : "");
});};

$("f-stats").onsubmit = e => { e.preventDefault(); run(async () => {
  const id = $("st-course").value;
  const st = await api("GET", "/api/courses/" + encodeURIComponent(id) + "/stats");
  $("stats-result").innerHTML = !st.count
    ? "<p class='muted'>该课程暂无成绩记录</p>"
    : "<table><tbody>" +
      "<tr><th>人数</th><td>" + st.count + "</td></tr>" +
      "<tr><th>平均分</th><td>" + st.average.toFixed(2) + "</td></tr>" +
      "<tr><th>最高分</th><td>" + st.max.toFixed(1) + "</td></tr>" +
      "<tr><th>最低分</th><td>" + st.min.toFixed(1) + "</td></tr>" +
      "<tr><th>及格率</th><td>" + (st.pass_rate * 100).toFixed(1) + "%</td></tr>" +
      "</tbody></table>";
});};

async function delStudent(id) { run(async () => {
  await api("DELETE", "/api/students/" + encodeURIComponent(id));
  toast("学生已删除", true); refreshAll();
});}
async function delCourse(id) { run(async () => {
  await api("DELETE", "/api/courses/" + encodeURIComponent(id));
  toast("课程已删除", true); refreshAll();
});}
async function delScore(sid, cid) { run(async () => {
  await api("DELETE", "/api/scores/" + encodeURIComponent(sid) + "/" + encodeURIComponent(cid));
  toast("成绩已删除", true); refreshAll();
});}

$("btn-rank").onclick = () => run(refreshRanking);
refreshAll().catch(e => toast(e.message, false));
</script>
</body>
</html>
`
