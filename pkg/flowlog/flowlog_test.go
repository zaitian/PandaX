package flowlog

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAllLinesSameWidth(t *testing.T) {
	f := New("添加组织信息", "7f3a9c2e")
	f.Add(Step{
		Name:   "请求接入",
		Status: StatusOK,
		Fields: []Field{
			{Label: "url", Value: "POST /system/organization"},
			{Label: "body", Value: `{"organizationName":"科研中心test","leader":"在天"}`},
		},
	})
	f.Add(Step{
		Name:   "鉴权校验",
		Status: StatusOK,
		Cost:   2 * time.Millisecond,
		Fields: []Field{{Label: "user", Value: "admin(role=manage)"}},
	})
	f.Add(Step{Name: "添加组织信息", Status: StatusOK, Cost: 12 * time.Millisecond})
	f.Add(Step{Name: "审计落库", Status: StatusOK})

	lines := strings.Split(f.Render(), "\n")
	if len(lines) < 6 {
		t.Fatalf("unexpected render output: %q", f.Render())
	}
	var want int
	for i, line := range lines {
		if !strings.HasPrefix(line, "│") && !strings.HasPrefix(line, "┌") && !strings.HasPrefix(line, "└") {
			continue
		}
		w := displayWidth(line)
		if i == 0 || want == 0 {
			want = w
			continue
		}
		if w != want {
			t.Errorf("line %d width = %d, want %d: %q", i, w, want, line)
		}
	}
	if !strings.Contains(lines[len(lines)-2], "FLOW DONE: SUCCESS") {
		t.Errorf("missing done border: %q", lines[len(lines)-2])
	}
	if !strings.HasSuffix(lines[len(lines)-1], "failed=0") {
		t.Errorf("missing summary: %q", lines[len(lines)-1])
	}
}

func TestRenderFailedFlowSkipsDownstream(t *testing.T) {
	f := New("查询库存", "9a1b2c3d")
	f.Add(Step{Name: "请求接入", Status: StatusOK})
	f.Add(Step{
		Name:   "鉴权校验",
		Status: StatusFail,
		Cost:   1500 * time.Millisecond,
		Fields: []Field{{Label: "error", Value: "没有API接口访问权限，请联系管理员"}},
	})
	f.Add(Step{
		Name:   "业务处理",
		Status: StatusSkip,
		Fields: []Field{{Label: "reason", Value: "上游鉴权失败"}},
	})

	out := f.Render()
	for _, want := range []string{"FLOW DONE: FAILED", "status : FAIL", "status : SKIP", "failed=1", "skipped=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}

func TestTruncateKeepsWidth(t *testing.T) {
	long := strings.Repeat("温度", 100)
	got := truncate(long, 20)
	if w := displayWidth(got); w > 20 {
		t.Errorf("display width = %d, want <= 20: %q", w, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis: %q", got)
	}
}
