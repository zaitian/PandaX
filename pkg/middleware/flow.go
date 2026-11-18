package middleware

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PandaXGO/PandaKit/restfulx"
	"github.com/sirupsen/logrus"
	"pandax/pkg/flowlog"
	"pandax/pkg/global"
)

// 请求体最多采集的字节数，超过则不采集，避免影响大报文与文件上传
const maxCapturedBody = 4096

var flowStore sync.Map

// FlowBeforeHandler 采集请求上下文并执行鉴权，逐步构建链路流程图
func FlowBeforeHandler(rc *restfulx.ReqCtx) error {
	f := flowlog.New(flowName(rc), newTraceID())
	flowStore.Store(rc, f)

	req := rc.Request.Request
	fields := []flowlog.Field{
		{Label: "url", Value: req.Method + " " + req.URL.Path},
		{Label: "from", Value: "client " + req.RemoteAddr},
	}
	if q := req.URL.RawQuery; q != "" {
		fields = append(fields, flowlog.Field{Label: "query", Value: q})
	}
	if b := captureBody(req); b != "" {
		fields = append(fields, flowlog.Field{Label: "body", Value: b})
	}
	if h := maskedHeaders(req); h != "" {
		fields = append(fields, flowlog.Field{Label: "headers", Value: h})
	}
	f.Add(flowlog.Step{Name: "请求接入", Status: flowlog.StatusOK, Fields: fields})

	begin := time.Now()
	err := PermissionHandler(rc)
	auth := flowlog.Step{Name: "鉴权校验", Status: flowlog.StatusOK, Cost: time.Since(begin)}
	if la := rc.LoginAccount; la != nil {
		auth.Fields = append(auth.Fields, flowlog.Field{Label: "user", Value: fmt.Sprintf("%s(role=%s)", la.UserName, la.RoleKey)})
	} else {
		auth.Fields = append(auth.Fields, flowlog.Field{Label: "user", Value: "anonymous"})
	}
	if err != nil {
		auth.Status = flowlog.StatusFail
		auth.Fields = append(auth.Fields, flowlog.Field{Label: "error", Value: errText(err)})
	}
	f.Add(auth)
	return err
}

// FlowAfterHandler 兜底渲染未落库的链路，GET 成功请求不打印以免刷屏
func FlowAfterHandler(rc *restfulx.ReqCtx) error {
	if _, ok := flowStore.Load(rc); !ok {
		return nil
	}
	f := finishFlow(rc, nil)
	flowStore.Delete(rc)
	if f == nil || f.Printed() {
		return nil
	}
	if rc.Request.Request.Method == http.MethodGet && rc.Err == nil {
		return nil
	}
	printFlow(f)
	return nil
}

// finishFlow 补齐业务处理节点与可选的审计节点，并标记已渲染
func finishFlow(rc *restfulx.ReqCtx, audit *flowlog.Step) *flowlog.Flow {
	v, ok := flowStore.Load(rc)
	if !ok {
		return nil
	}
	f := v.(*flowlog.Flow)
	if f.Done() {
		return f
	}
	f.MarkDone()

	step := flowlog.Step{Name: "业务处理"}
	if rc.LogInfo != nil && rc.LogInfo.Description != "" {
		step.Name = rc.LogInfo.Description
	}
	// 前置链路(鉴权)失败时，业务处理不会执行
	if f.HasFailure() {
		step.Status = flowlog.StatusSkip
		step.Fields = append(step.Fields, flowlog.Field{Label: "reason", Value: "上游鉴权失败"})
	} else {
		step.Cost = time.Duration(rc.Timed) * time.Millisecond
		if out := flowlog.JSON(rc.ResData); out != "" {
			step.Fields = append(step.Fields, flowlog.Field{Label: "output", Value: out})
		}
		if rc.Err != nil {
			step.Status = flowlog.StatusFail
			step.Fields = append(step.Fields, flowlog.Field{Label: "error", Value: errText(rc.Err)})
		} else {
			step.Status = flowlog.StatusOK
		}
	}
	f.Add(step)

	if audit != nil {
		f.Add(*audit)
	}
	return f
}

func printFlow(f *flowlog.Flow) {
	if f == nil || f.Printed() {
		return
	}
	f.MarkPrinted()
	msg := "\n" + f.Render()
	fields := logrus.Fields{"traceId": f.TraceID(), "flow": f.Name()}
	if global.Log == nil {
		fmt.Println(msg)
		return
	}
	global.Log.WithFields(fields).Info(msg)
}

func flowName(rc *restfulx.ReqCtx) string {
	if rc.LogInfo != nil && rc.LogInfo.Description != "" {
		return rc.LogInfo.Description
	}
	return rc.Request.Request.Method + " " + rc.Request.Request.URL.Path
}

func newTraceID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(b)
}

func errText(err any) string {
	switch t := err.(type) {
	case string:
		return t
	case error:
		return t.Error()
	default:
		return fmt.Sprint(t)
	}
}

// captureBody 读取并还原请求体，便于流程图中展示入参；敏感字段做脱敏
func captureBody(req *http.Request) string {
	if req.Body == nil || req.ContentLength <= 0 || req.ContentLength > maxCapturedBody {
		return ""
	}
	if strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxCapturedBody))
	if err != nil {
		return ""
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return maskSensitive(string(body))
}

var sensitiveKeys = []string{"password", "passwd", "pwd", "secret", "token", "oldpassword", "newpassword"}

// maskSensitive 对 JSON 请求体中的密码、令牌等字段脱敏
func maskSensitive(body string) string {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return body
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return body
	}
	maskMap(data)
	b, err := json.Marshal(data)
	if err != nil {
		return body
	}
	return string(b)
}

func maskMap(data map[string]any) {
	for k, v := range data {
		lower := strings.ToLower(k)
		for _, s := range sensitiveKeys {
			if strings.Contains(lower, s) {
				data[k] = "***"
				break
			}
		}
		if nested, ok := v.(map[string]any); ok {
			maskMap(nested)
		}
	}
}

func maskedHeaders(req *http.Request) string {
	var parts []string
	if ct := req.Header.Get("Content-Type"); ct != "" {
		parts = append(parts, `"Content-Type":"`+ct+`"`)
	}
	if req.Header.Get("X-TOKEN") != "" {
		parts = append(parts, `"X-TOKEN":"***"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}
