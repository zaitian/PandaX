package middleware

import (
	"fmt"

	"github.com/PandaXGO/PandaKit/biz"
	"github.com/PandaXGO/PandaKit/restfulx"
	"github.com/PandaXGO/PandaKit/utils"
	"net/http"
	"pandax/apps/log/entity"
	"pandax/apps/log/services"
	"pandax/pkg/flowlog"
)

func OperationHandler(rc *restfulx.ReqCtx) error {
	c := rc.Request
	// 请求操作不做记录
	if c.Request.Method == http.MethodGet || rc.LoginAccount == nil {
		return nil
	}
	if rc.RequiredPermission == nil || !rc.RequiredPermission.NeedToken {
		return nil
	}
	status, errorMsg := operationResult(rc.Err)

	// 追加审计节点并渲染整条链路流程图
	audit := flowlog.Step{
		Name:   "审计落库",
		Status: flowlog.StatusOK,
		Fields: []flowlog.Field{{Label: "to", Value: "log_opers"}},
	}
	flow := finishFlow(rc, &audit)

	go func() {
		oper := entity.LogOper{
			Title:        rc.LogInfo.Description,
			BusinessType: "0",
			Method:       c.Request.Method,
			OperName:     rc.LoginAccount.UserName,
			OperUrl:      c.Request.URL.Path,
			OperIp:       c.Request.RemoteAddr,
			OperLocation: utils.GetRealAddressByIP(c.Request.RemoteAddr),
			OperParam:    "",
			ErrorMsg:     errorMsg,
			Status:       status,
		}
		if c.Request.Method == "POST" {
			oper.BusinessType = "1"
		} else if c.Request.Method == "PUT" {
			oper.BusinessType = "2"
		} else if c.Request.Method == "DELETE" {
			oper.BusinessType = "3"
		}
		if flow != nil {
			oper.TraceId = flow.TraceID()
			oper.Flow = flow.Render()
			oper.FlowData = flow.StepsJSON()
		}
		services.LogOperModelDao.Insert(oper)
		printFlow(flow)
	}()

	return nil
}

// operationResult 根据请求错误返回操作状态及失败原因
func operationResult(err any) (string, string) {
	if err == nil {
		return "0", ""
	}
	var msg string
	switch t := err.(type) {
	case *biz.BizError:
		msg = t.Error()
	case error:
		msg = t.Error()
	case string:
		msg = t
	default:
		msg = fmt.Sprint(t)
	}
	if runes := []rune(msg); len(runes) > 1000 {
		msg = string(runes[:1000])
	}
	return "1", msg
}
