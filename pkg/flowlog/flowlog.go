package flowlog

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	StatusOK   = "OK"
	StatusFail = "FAIL"
	StatusSkip = "SKIP"

	minInner      = 66
	maxValueWidth = 110
	arrowColumn   = 13
)

// Field 步骤中的一行明细，例如 url / input / output / error
type Field struct {
	Label string
	Value string
}

// Step 流程图中的一个节点
type Step struct {
	Name   string
	Status string
	Cost   time.Duration
	Fields []Field
}

// Flow 一次请求的链路，携带 traceId 并按步骤累积节点
type Flow struct {
	mu      sync.Mutex
	name    string
	trace   string
	start   time.Time
	steps   []Step
	done    bool
	printed bool
}

func New(name, trace string) *Flow {
	return &Flow{name: name, trace: trace, start: time.Now()}
}

func (f *Flow) TraceID() string { return f.trace }

func (f *Flow) Name() string { return f.name }

func (f *Flow) Add(s Step) {
	f.mu.Lock()
	f.steps = append(f.steps, s)
	f.mu.Unlock()
}

// Done 标记流程已渲染，避免重复输出
func (f *Flow) Done() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.done
}

func (f *Flow) MarkDone() {
	f.mu.Lock()
	f.done = true
	f.mu.Unlock()
}

// Printed 是否已经输出到控制台
func (f *Flow) Printed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.printed
}

func (f *Flow) MarkPrinted() {
	f.mu.Lock()
	f.printed = true
	f.mu.Unlock()
}

// HasFailure 判断链路中是否已有失败节点
func (f *Flow) HasFailure() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.steps {
		if s.Status == StatusFail {
			return true
		}
	}
	return false
}

// JSON 将任意值压缩成单行字符串，便于写入流程图
func JSON(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

type stepJSON struct {
	Name   string      `json:"name"`
	Status string      `json:"status"`
	CostMs int64       `json:"costMs"`
	Fields []FieldJSON `json:"fields"`
}

type FieldJSON struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// StepsJSON 结构化步骤，供前端渲染流程图
func (f *Flow) StepsJSON() string {
	f.mu.Lock()
	steps := make([]Step, len(f.steps))
	copy(steps, f.steps)
	f.mu.Unlock()

	list := make([]stepJSON, 0, len(steps))
	for _, s := range steps {
		item := stepJSON{Name: s.Name, Status: s.Status, CostMs: s.Cost.Milliseconds()}
		for _, fd := range s.Fields {
			if fd.Value == "" {
				continue
			}
			item.Fields = append(item.Fields, FieldJSON{Label: fd.Label, Value: truncate(fd.Value, 400)})
		}
		list = append(list, item)
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// Render 输出 ASCII 流程图
func (f *Flow) Render() string {
	f.mu.Lock()
	name, start := f.name, f.start
	steps := make([]Step, len(f.steps))
	copy(steps, f.steps)
	f.mu.Unlock()

	lines := []string{""}
	for i, s := range steps {
		if i > 0 {
			lines = append(lines, strings.Repeat(" ", arrowColumn)+"│", strings.Repeat(" ", arrowColumn)+"▼")
		}
		lines = append(lines, stepLines(i+1, len(steps), s)...)
	}
	lines = append(lines, "")

	inner := minInner
	for _, l := range lines {
		if w := displayWidth(l) + 2; w > inner {
			inner = w
		}
	}

	var b strings.Builder
	b.WriteString(border("┌", "┐", "FLOW: "+name, inner))
	b.WriteByte('\n')
	for _, l := range lines {
		b.WriteString("│ " + l + strings.Repeat(" ", inner-2-displayWidth(l)) + " │\n")
	}

	success, failed, skipped := 0, 0, 0
	for _, s := range steps {
		switch s.Status {
		case StatusFail:
			failed++
		case StatusSkip:
			skipped++
		default:
			success++
		}
	}
	state := "SUCCESS"
	if failed > 0 {
		state = "FAILED"
	}

	b.WriteString(border("└", "┘", "FLOW DONE: "+state, inner))
	b.WriteByte('\n')

	summary := fmt.Sprintf("  total=%dms  steps=%d/%d  success=%d  failed=%d", time.Since(start).Milliseconds(), len(steps), len(steps), success, failed)
	if skipped > 0 {
		summary += fmt.Sprintf("  skipped=%d", skipped)
	}
	b.WriteString(summary)
	return b.String()
}

func stepLines(index, total int, s Step) []string {
	lines := []string{fmt.Sprintf("  [STEP %d/%d] ──▶ %s", index, total, s.Name)}
	for _, fd := range s.Fields {
		if fd.Value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("    %-7s: %s", fd.Label, truncate(fd.Value, maxValueWidth)))
	}
	if s.Cost > 0 {
		lines = append(lines, fmt.Sprintf("    %-7s: %dms", "cost", s.Cost.Milliseconds()))
	}
	if s.Status != "" {
		lines = append(lines, fmt.Sprintf("    %-7s: %s", "status", s.Status))
	}
	return lines
}

func border(left, right, label string, inner int) string {
	text := " " + label + " "
	fill := inner - displayWidth(text)
	if fill < 2 {
		fill = 2
	}
	l := fill / 2
	return left + strings.Repeat("─", l) + text + strings.Repeat("─", fill-l) + right
}

// truncate 压缩空白并按显示宽度截断
func truncate(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if displayWidth(s) <= max {
		return s
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runeWidth(r)
		if w+rw > max-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + "…"
}

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// runeWidth 返回字符终端显示宽度（中日韩全角字符占两列）
func runeWidth(r rune) int {
	switch {
	case r == 0x200B || r == 0x200C || r == 0x200D:
		return 0
	case r < 0x1100:
		return 1
	case r <= 0x115F,
		r == 0x2329, r == 0x232A,
		r >= 0x2E80 && r <= 0xA4CF && r != 0x303F,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F64F,
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}
