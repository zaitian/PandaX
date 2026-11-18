package api

// ==========================================================================
// 生成日期：2023-06-30 09:19:43 +0800 CST
// 生成路径: apps/device/api/devices.go
// 生成人：panda
// ==========================================================================
import (
	"fmt"
	"github.com/PandaXGO/PandaKit/biz"
	"github.com/PandaXGO/PandaKit/model"
	"github.com/PandaXGO/PandaKit/restfulx"
	"github.com/PandaXGO/PandaKit/utils"
	"pandax/pkg/cache"
	"pandax/pkg/global"
	"strings"
	"time"

	"pandax/apps/device/entity"
	"pandax/apps/device/services"
)

type DeviceApi struct {
	DeviceApp          services.DeviceModel
	DeviceAlarmApp     services.DeviceAlarmModel
	ProductApp         services.ProductModel
	ProductTemplateApp services.ProductTemplateModel
}

func (p *DeviceApi) GetDevicePanel(rc *restfulx.ReqCtx) {
	var data entity.DeviceTotalOutput
	data.DeviceInfo, _ = p.DeviceApp.FindDeviceCount()
	data.DeviceLinkStatusInfo, _ = p.DeviceApp.FindDeviceCountGroupByLinkStatus()
	data.DeviceCountType, _ = p.DeviceApp.FindDeviceCountGroupByType()
	data.AlarmInfo, _ = p.DeviceAlarmApp.FindAlarmCount()
	data.ProductInfo, _ = p.ProductApp.FindProductCount()
	rc.ResData = data
}

// GetDeviceList Device列表数据
func (p *DeviceApi) GetDeviceList(rc *restfulx.ReqCtx) {
	data := entity.Device{}
	pageNum := restfulx.QueryInt(rc, "pageNum", 1)
	pageSize := restfulx.QueryInt(rc, "pageSize", 10)
	data.Name = restfulx.QueryParam(rc, "name")
	data.Status = restfulx.QueryParam(rc, "status")
	data.Pid = restfulx.QueryParam(rc, "pid")
	data.Gid = restfulx.QueryParam(rc, "gid")
	data.DeviceType = restfulx.QueryParam(rc, "deviceType")
	data.ParentId = restfulx.QueryParam(rc, "parentId")
	data.LinkStatus = restfulx.QueryParam(rc, "linkStatus")

	data.RoleId = rc.LoginAccount.RoleId
	data.Owner = rc.LoginAccount.UserName
	list, total, err := p.DeviceApp.FindListPage(pageNum, pageSize, data)
	biz.ErrIsNil(err, "查询设备列表失败")
	rc.ResData = model.ResultPage{
		Total:    total,
		PageNum:  int64(pageNum),
		PageSize: int64(pageSize),
		Data:     list,
	}
}

// GetDeviceListAll Device获取所有
func (p *DeviceApi) GetDeviceListAll(rc *restfulx.ReqCtx) {
	data := entity.Device{}
	data.Name = restfulx.QueryParam(rc, "name")
	data.Status = restfulx.QueryParam(rc, "status")
	data.Pid = restfulx.QueryParam(rc, "pid")
	data.DeviceType = restfulx.QueryParam(rc, "deviceType")
	data.RoleId = rc.LoginAccount.RoleId
	data.Owner = rc.LoginAccount.UserName

	list, err := p.DeviceApp.FindList(data)
	biz.ErrIsNil(err, "查询所有设备失败")
	rc.ResData = list
}

// GetDevice 获取Device
func (p *DeviceApi) GetDevice(rc *restfulx.ReqCtx) {
	id := restfulx.PathParam(rc, "id")
	device, err := p.DeviceApp.FindOne(id)
	biz.ErrIsNil(err, "获取设备失败")
	rc.ResData = device
}

// GetDeviceStatus 获取Device状态信息
func (p *DeviceApi) GetDeviceStatus(rc *restfulx.ReqCtx) {
	id := restfulx.PathParam(rc, "id")
	classify := restfulx.QueryParam(rc, "classify")
	device, err := p.DeviceApp.FindOne(id)
	biz.ErrIsNil(err, "获取设备失败")
	template, err := p.ProductTemplateApp.FindList(entity.ProductTemplate{Classify: classify, Pid: device.Pid})
	biz.ErrIsNil(err, "查询设备模板失败")
	res := make([]entity.DeviceStatusVo, 0)

	for _, tel := range *template {
		sdv := entity.DeviceStatusVo{
			Name:   tel.Name,
			Key:    tel.Key,
			Type:   tel.Type,
			Define: tel.Define,
		}
		var table string
		if classify == global.TslTelemetryType {
			table = fmt.Sprintf("%s_telemetry", strings.ToLower(device.Name))
		}
		if classify == global.TslAttributesType {
			table = fmt.Sprintf("%s_attributes", strings.ToLower(device.Name))
		}
		key := strings.ToLower(tel.Key)
		if !tdIdentifier(key) || !tdIdentifier(table) {
			sdv.Value = "未知"
			sdv.Time = time.Now()
			res = append(res, sdv)
			continue
		}
		sql := fmt.Sprintf("select ts, `%s` from `%s` order by ts desc", key, table)
		one, err := global.TdDb.GetOne(sql)
		if err == nil && len(one) > 0 {
			sdv.Value = derefValue(one[key])
			sdv.Time = time.Now()
		} else {
			if value, ok := tel.Define["default_value"]; ok {
				sdv.Value = value
				sdv.Time = time.Now()
			} else {
				sdv.Value = "未知"
				sdv.Time = time.Now()
			}
		}
		res = append(res, sdv)
	}

	rc.ResData = res
}

func (p *DeviceApi) GetDeviceEvents(rc *restfulx.ReqCtx) {
	pageNum := restfulx.QueryInt(rc, "pageNum", 1)
	pageSize := restfulx.QueryInt(rc, "pageSize", 10)
	deviceId := restfulx.PathParam(rc, "id")
	ty := restfulx.QueryParam(rc, "type")
	offset := pageSize * (pageNum - 1)
	sql := `select * from ? where  deviceId = ? and type = ? DESC LIMIT ?,? `
	list, err := global.TdDb.GetAllEvents(sql, deviceId, ty, offset, pageSize)
	biz.ErrIsNilAppendErr(err, "查询设备事件历史失败")
	rc.ResData = list
}

// GetDeviceTelemetryHistory 获取Device属性的遥测历史
func (p *DeviceApi) GetDeviceTelemetryHistory(rc *restfulx.ReqCtx) {
	id := restfulx.PathParam(rc, "id")
	key := restfulx.QueryParam(rc, "key")
	startTime := restfulx.QueryParam(rc, "startTime")
	endTime := restfulx.QueryParam(rc, "endTime")
	limit := restfulx.QueryInt(rc, "limit", 1000)
	device, err := p.DeviceApp.FindOne(id)
	biz.ErrIsNil(err, "获取设备失败，设备不存在")
	table := fmt.Sprintf("%s_telemetry", strings.ToLower(device.Name))
	if !tdIdentifier(key) || !tdIdentifier(table) {
		biz.ErrIsNil(fmt.Errorf("非法的查询字段"), "查询设备属性的遥测历史失败")
	}
	start := strings.ReplaceAll(startTime, "'", "''")
	end := strings.ReplaceAll(endTime, "'", "''")
	if start == "" {
		start = "1970-01-01 00:00:00"
	}
	if end == "" {
		end = time.Now().Format("2006-01-02 15:04:05")
	}
	sql := fmt.Sprintf("select ts, `%s` from `%s` where ts > '%s' and ts < '%s' and `%s` is not null ORDER BY ts DESC LIMIT %d", strings.ToLower(key), table, start, end, strings.ToLower(key), limit)
	rs, err := global.TdDb.GetAll(sql)
	biz.ErrIsNilAppendErr(err, "查询设备属性的遥测历史失败")
	rc.ResData = rs
}

// tdIdentifier 判断是否为合法的 TDengine 标识符（表名/字段名）
func tdIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func derefValue(value any) any {
	if p, ok := value.(*any); ok {
		if p == nil {
			return nil
		}
		return *p
	}
	return value
}

// InsertDevice 添加Device
func (p *DeviceApi) InsertDevice(rc *restfulx.ReqCtx) {
	var data entity.Device
	restfulx.BindJsonAndValid(rc, &data)
	product, _ := p.ProductApp.FindOne(data.Pid)
	biz.NotNil(product, "未查到所属产品信息")
	data.Owner = rc.LoginAccount.UserName
	data.OrgId = rc.LoginAccount.OrganizationId
	list, _ := p.DeviceApp.FindList(entity.Device{Name: data.Name})
	biz.IsTrue(!(list != nil && len(*list) > 0), fmt.Sprintf("名称%s已存在，设置其他命名", data.Name))
	data.Id = utils.GenerateID("d")
	data.LinkStatus = global.INACTIVE
	data.LastAt = time.Now()
	data.Protocol = product.ProtocolName

	_, err := p.DeviceApp.Insert(data)
	biz.ErrIsNil(err, "添加设备失败")
}

// UpdateDevice 修改Device
func (p *DeviceApi) UpdateDevice(rc *restfulx.ReqCtx) {
	var data entity.Device
	restfulx.BindQuery(rc, &data)
	product, _ := p.ProductApp.FindOne(data.Pid)
	biz.NotNil(product, "未查到所属产品信息")
	data.Protocol = product.ProtocolName

	_, err := p.DeviceApp.Update(data)
	biz.ErrIsNil(err, "修改失败")
}

// DeleteDevice 删除Device
func (p *DeviceApi) DeleteDevice(rc *restfulx.ReqCtx) {
	id := restfulx.PathParam(rc, "id")
	ids := strings.Split(id, ",")

	for _, id := range ids {
		device, err := p.DeviceApp.FindOne(id)
		if err != nil {
			continue
		}
		err = p.DeviceApp.Delete([]string{id})
		if err != nil {
			continue
		}
		// 删除表
		err = deleteDeviceTable(device.Name)
		// 删除所有缓存
		if device.DeviceType == global.GATEWAYS {
			// 因为网关子设备没有Token，使用Name做的存储
			cache.DelDeviceEtoken(device.Name)
		} else {
			cache.DelDeviceEtoken(device.Token)
		}
	}
}

// 删除Tdengine时序数据
func deleteDeviceTable(device string) error {
	err := global.TdDb.DropTable(device + "_" + entity.ATTRIBUTES_TSL)
	if err != nil {
		return err
	}
	err = global.TdDb.DropTable(device + "_" + entity.TELEMETRY_TSL)
	if err != nil {
		return err
	}
	return nil
}

func (p *DeviceApi) ScreenTwinData(rc *restfulx.ReqCtx) {
	pageNum := restfulx.QueryInt(rc, "pageNum", 1)
	pageSize := restfulx.QueryInt(rc, "pageSize", 10)
	classId := restfulx.QueryParam(rc, "classId")
	if classId == "" {
		vp := make([]entity.VisualClass, 0)
		list, _ := p.ProductApp.FindList(entity.Product{})
		for _, pro := range *list {
			data, _ := p.ProductTemplateApp.FindListAttrs(entity.ProductTemplate{Pid: pro.Id})
			vta := make([]entity.VisualTwinAttr, 0)
			for _, attr := range *data {
				twinAttr := entity.VisualTwinAttr{
					Key:  attr.Key,
					Name: attr.Name,
					Type: attr.Type,
				}
				if attr.Classify == "attributes" {
					if rw, ok := attr.Define["rw"].(string); ok {
						twinAttr.Rw = rw
					} else {
						twinAttr.Rw = "r"
					}
				} else {
					twinAttr.Rw = "r"
				}
				vta = append(vta, twinAttr)
			}
			vp = append(vp, entity.VisualClass{
				ClassId: pro.Id,
				Name:    pro.Name,
				Attrs:   vta,
			})
		}
		rc.ResData = vp
	} else {
		device := entity.Device{Pid: classId}
		device.RoleId = rc.LoginAccount.RoleId
		device.Owner = rc.LoginAccount.UserName
		findList, _, _ := p.DeviceApp.FindListPage(pageNum, pageSize, device)
		vt := make([]entity.VisualTwin, 0)
		for _, device := range *findList {
			vt = append(vt, entity.VisualTwin{
				TwinId: device.Id,
				Name:   device.Name + "-" + device.Alias,
			})
		}
		rc.ResData = vt
	}
}

func (p *DeviceApi) DeviceAllotOrg(rc *restfulx.ReqCtx) {
	id := restfulx.PathParam(rc, "id")
	orgId := restfulx.QueryInt(rc, "orgId", 0)
	biz.IsTrue(orgId != 0, "请选择组织")

	device := entity.Device{}
	device.Id = id
	device.OrgId = int64(orgId)
	p.DeviceApp.Update(device)
}
