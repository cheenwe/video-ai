package handlers

import (
	"sort"
	"strconv"
	"strings"

	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 支持的字段类型。
const (
	CustomFieldTypeText        = "text"
	CustomFieldTypeTextarea    = "textarea"
	CustomFieldTypeNumber      = "number"
	CustomFieldTypeDate        = "date"
	CustomFieldTypeDatetime    = "datetime"
	CustomFieldTypeSelect      = "select"
	CustomFieldTypeMultiSelect = "multi_select"
)

// 内置推荐模块（仅用于前端下拉引导，非白名单——任意合法模块名都允许创建）。
var customFieldBuiltinModules = []struct{ Code, Name string }{
	{"user", "用户"},
	{"project", "项目"},
	{"department", "部门"},
}

// validModuleCode 校验模块编码：1-64 位，仅小写字母、数字、下划线、短横线。
func validModuleCode(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func validFieldType(t string) bool {
	switch t {
	case CustomFieldTypeText, CustomFieldTypeTextarea, CustomFieldTypeNumber,
		CustomFieldTypeDate, CustomFieldTypeDatetime,
		CustomFieldTypeSelect, CustomFieldTypeMultiSelect:
		return true
	}
	return false
}

func isSelectType(t string) bool {
	return t == CustomFieldTypeSelect || t == CustomFieldTypeMultiSelect
}

type CustomFieldHandler struct{ DB *gorm.DB }

// ---------- 字段定义（管理员） ----------

type customFieldDefCreateReq struct {
	Module      string `json:"module" binding:"required"`
	FieldCode   string `json:"field_code" binding:"required"`
	FieldName   string `json:"field_name" binding:"required"`
	FieldType   string `json:"field_type"`
	DictCode    string `json:"dict_code"`
	Description string `json:"description"`
	Required    *int   `json:"required"`
	SortOrder   int    `json:"sort_order"`
	Enabled     *int   `json:"enabled"`
}

type customFieldDefUpdateReq struct {
	FieldName   *string `json:"field_name"`
	FieldType   *string `json:"field_type"`
	DictCode    *string `json:"dict_code"`
	Description *string `json:"description"`
	Required    *int    `json:"required"`
	SortOrder   *int    `json:"sort_order"`
	Enabled     *int    `json:"enabled"`
}

// ListDefs 列出字段定义，支持按 module 过滤。
func (h *CustomFieldHandler) ListDefs(c *gin.Context) {
	q := h.DB.Model(&models.CustomFieldDef{})
	if mod := c.Query("module"); mod != "" {
		q = q.Where("module = ?", mod)
	}
	if kw := c.Query("keyword"); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("field_code LIKE ? OR field_name LIKE ?", like, like)
	}
	var total int64
	q.Count(&total)
	var items []models.CustomFieldDef
	q.Order("module asc, sort_order asc, id asc").Find(&items)
	response.OK(c, gin.H{"items": items, "total": total}, "")
}

// ListModules 列出可挂载模块：内置推荐 + 数据库已用模块，去重合并。
func (h *CustomFieldHandler) ListModules(c *gin.Context) {
	type mod struct {
		Code    string `json:"code"`
		Name    string `json:"name"`
		Builtin int    `json:"builtin"` // 1=内置推荐，0=自定义
	}
	seen := map[string]mod{}
	// 内置推荐
	for _, m := range customFieldBuiltinModules {
		seen[m.Code] = mod{Code: m.Code, Name: m.Name, Builtin: 1}
	}
	// 数据库已用模块
	var usedCodes []string
	h.DB.Model(&models.CustomFieldDef{}).
		Distinct("module").
		Order("module asc").
		Pluck("module", &usedCodes)
	for _, code := range usedCodes {
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = mod{Code: code, Name: code, Builtin: 0}
	}
	out := make([]mod, 0, len(seen))
	for _, m := range seen {
		out = append(out, m)
	}
	// 内置在前、其余按 code 排序
	sort.Slice(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin > out[j].Builtin
		}
		return out[i].Code < out[j].Code
	})
	response.OK(c, out, "")
}

func (h *CustomFieldHandler) CreateDef(c *gin.Context) {
	var req customFieldDefCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	req.Module = strings.TrimSpace(req.Module)
	req.FieldCode = strings.TrimSpace(req.FieldCode)
	req.FieldName = strings.TrimSpace(req.FieldName)
	req.FieldType = strings.TrimSpace(req.FieldType)
	req.DictCode = strings.TrimSpace(req.DictCode)
	if req.Module == "" || req.FieldCode == "" || req.FieldName == "" {
		response.BizFail(c, "模块、编码与名称不能为空", 422)
		return
	}
	if req.Module != "user" && req.Module != "department" && req.Module != "project" {
		response.BizFail(c, "该模块尚未注册资源授权，不能创建字段", 422)
		return
	}
	if !validModuleCode(req.Module) {
		response.BizFail(c, "模块编码格式无效（仅小写字母、数字、下划线、短横线，1-64 位）", 422)
		return
	}
	if req.FieldCode == "" || len(req.FieldCode) > 64 {
		response.BizFail(c, "字段编码长度需在 1-64 之间", 422)
		return
	}
	if req.FieldType == "" {
		req.FieldType = CustomFieldTypeText
	}
	if !validFieldType(req.FieldType) {
		response.BizFail(c, "字段类型不合法", 422)
		return
	}
	if isSelectType(req.FieldType) {
		if req.DictCode == "" {
			response.BizFail(c, "select/multi_select 类型必须指定 dict_code", 422)
			return
		}
		var dictCount int64
		h.DB.Model(&models.SysDict{}).Where("code = ?", req.DictCode).Count(&dictCount)
		if dictCount == 0 {
			response.BizFail(c, "引用的字典不存在", 422)
			return
		}
	} else if req.DictCode != "" {
		response.BizFail(c, "非 select 类型不应设置 dict_code", 422)
		return
	}
	required := 0
	if req.Required != nil {
		if *req.Required != 0 && *req.Required != 1 {
			response.BizFail(c, "required 取值无效", 422)
			return
		}
		required = *req.Required
	}
	enabled := 1
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			response.BizFail(c, "enabled 取值无效", 422)
			return
		}
		enabled = *req.Enabled
	}
	// 同模块下编码唯一
	var dup int64
	h.DB.Model(&models.CustomFieldDef{}).
		Where("module = ? AND field_code = ?", req.Module, req.FieldCode).
		Count(&dup)
	if dup > 0 {
		response.BizFail(c, "该模块下字段编码已存在", 400)
		return
	}
	def := models.CustomFieldDef{
		Module:      req.Module,
		FieldCode:   req.FieldCode,
		FieldName:   req.FieldName,
		FieldType:   req.FieldType,
		DictCode:    req.DictCode,
		Description: req.Description,
		Required:    required,
		SortOrder:   req.SortOrder,
		Enabled:     enabled,
	}
	if err := h.DB.Create(&def).Error; err != nil {
		response.BizFail(c, "创建失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "custom_field.create", &me, def.Module+":"+def.FieldCode)
	response.OK(c, def, "创建成功")
}

func (h *CustomFieldHandler) UpdateDef(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var def models.CustomFieldDef
	if err := h.DB.First(&def, id).Error; err != nil {
		response.BizFail(c, "字段定义不存在", 404)
		return
	}
	var req customFieldDefUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	updates := map[string]interface{}{}
	if req.FieldName != nil {
		if strings.TrimSpace(*req.FieldName) == "" {
			response.BizFail(c, "名称不能为空", 422)
			return
		}
		updates["field_name"] = strings.TrimSpace(*req.FieldName)
	}
	if req.FieldType != nil {
		ft := strings.TrimSpace(*req.FieldType)
		if !validFieldType(ft) {
			response.BizFail(c, "字段类型不合法", 422)
			return
		}
		updates["field_type"] = ft
		// 切换为 select 类型时必须校验 dict_code
		if isSelectType(ft) {
			dc := def.DictCode
			if req.DictCode != nil {
				dc = strings.TrimSpace(*req.DictCode)
			}
			if dc == "" {
				response.BizFail(c, "select/multi_select 类型必须指定 dict_code", 422)
				return
			}
			var dictCount int64
			h.DB.Model(&models.SysDict{}).Where("code = ?", dc).Count(&dictCount)
			if dictCount == 0 {
				response.BizFail(c, "引用的字典不存在", 422)
				return
			}
		}
	}
	if req.DictCode != nil {
		dc := strings.TrimSpace(*req.DictCode)
		ft := def.FieldType
		if req.FieldType != nil {
			ft = strings.TrimSpace(*req.FieldType)
		}
		if isSelectType(ft) {
			if dc == "" {
				response.BizFail(c, "select/multi_select 类型必须指定 dict_code", 422)
				return
			}
			var dictCount int64
			h.DB.Model(&models.SysDict{}).Where("code = ?", dc).Count(&dictCount)
			if dictCount == 0 {
				response.BizFail(c, "引用的字典不存在", 422)
				return
			}
			updates["dict_code"] = dc
		} else if dc != "" {
			response.BizFail(c, "非 select 类型不应设置 dict_code", 422)
			return
		} else {
			updates["dict_code"] = ""
		}
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Required != nil {
		if *req.Required != 0 && *req.Required != 1 {
			response.BizFail(c, "required 取值无效", 422)
			return
		}
		updates["required"] = *req.Required
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.Enabled != nil {
		if *req.Enabled != 0 && *req.Enabled != 1 {
			response.BizFail(c, "enabled 取值无效", 422)
			return
		}
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		response.OK(c, def, "")
		return
	}
	if err := h.DB.Model(&def).Updates(updates).Error; err != nil {
		response.BizFail(c, "更新失败", 500)
		return
	}
	h.DB.First(&def, id)
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "custom_field.update", &me, def.Module+":"+def.FieldCode)
	response.OK(c, def, "更新成功")
}

func (h *CustomFieldHandler) DeleteDef(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var def models.CustomFieldDef
	if err := h.DB.First(&def, id).Error; err != nil {
		response.BizFail(c, "字段定义不存在", 404)
		return
	}
	// 删除定义与其下所有值
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("field_id = ?", def.ID).Delete(&models.CustomFieldValue{}).Error; err != nil {
			return err
		}
		return tx.Delete(&def).Error
	}); err != nil {
		response.BizFail(c, "删除失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "custom_field.delete", &me, def.Module+":"+def.FieldCode)
	response.OK(c, nil, "已删除")
}

// ---------- 通用读写接口（已认证用户） ----------

// fieldWithValueOut 供前端表单渲染：字段定义 + 当前实体的值 +（select 时）选项列表。
type fieldWithValueOut struct {
	models.CustomFieldDef
	Value   string               `json:"value"`
	Options []models.SysDictItem `json:"options,omitempty"`
}

// GetDefinitions 返回某模块下启用字段定义（用于新建表单渲染，无值）。
func (h *CustomFieldHandler) GetDefinitions(c *gin.Context) {
	mod := c.Param("module")
	if mod == "" {
		response.BizFail(c, "模块不能为空", 400)
		return
	}
	out, err := h.loadFieldsWithValues(mod, 0)
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"fields": out}, "")
}

// GetValues 返回某模块下启用字段定义 + 指定实体的当前值。
func (h *CustomFieldHandler) GetValues(c *gin.Context) {
	mod := c.Param("module")
	if mod == "" {
		response.BizFail(c, "模块不能为空", 400)
		return
	}
	eid, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
	if err != nil || eid < 1 {
		response.BizFail(c, "无效实体 ID", 400)
		return
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), mod, uint(eid), false) {
		response.BizFail(c, "资源不存在或无访问权限", 403)
		return
	}

	out, err := h.loadFieldsWithValues(mod, uint(eid))
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"fields": out}, "")
}

// GetBatchValues 批量返回多实体的字段值（供列表页动态列渲染）。
// 查询参数 entity_ids=1,2,3。返回 { fields: [...定义...], values: { "1": [{field_id,value}...] } }
func (h *CustomFieldHandler) GetBatchValues(c *gin.Context) {
	mod := c.Param("module")
	if mod == "" {
		response.BizFail(c, "模块不能为空", 400)
		return
	}
	if !validModuleCode(mod) {
		response.BizFail(c, "模块编码格式无效", 422)
		return
	}
	rawIDs := c.Query("entity_ids")
	if rawIDs == "" {
		response.BizFail(c, "entity_ids 不能为空", 400)
		return
	}
	parts := strings.Split(rawIDs, ",")
	ids := make([]uint, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || n < 1 {
			continue
		}
		ids = append(ids, uint(n))
	}
	if len(ids) == 0 {
		response.BizFail(c, "无有效 entity_ids", 400)
		return
	}
	if len(ids) > 200 {
		response.BizFail(c, "一次最多查询200个实体", 422)
		return
	}
	for _, id := range ids {
		if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), mod, id, false) {
			response.BizFail(c, "资源不存在或无访问权限", 403)
			return
		}
	}

	// 取字段定义
	var defs []models.CustomFieldDef
	if err := h.DB.Where("module = ? AND enabled = ?", mod, 1).
		Order("sort_order asc, id asc").Find(&defs).Error; err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	// 取这些实体在此模块下的所有值
	var vals []models.CustomFieldValue
	if len(defs) > 0 {
		if err := h.DB.Where("module = ? AND entity_id IN ?", mod, ids).Find(&vals).Error; err != nil {
			response.BizFail(c, "加载失败", 500)
			return
		}
	}
	valuesMap := map[uint][]gin.H{}
	for _, v := range vals {
		valuesMap[v.EntityID] = append(valuesMap[v.EntityID], gin.H{
			"field_id": v.FieldID,
			"value":    v.Value,
		})
	}
	out := gin.H{
		"fields": defs,
		"values": valuesMap,
	}
	response.OK(c, out, "")
}

func (h *CustomFieldHandler) loadFieldsWithValues(mod string, entityID uint) ([]fieldWithValueOut, error) {
	var defs []models.CustomFieldDef
	if err := h.DB.Where("module = ? AND enabled = ?", mod, 1).
		Order("sort_order asc, id asc").Find(&defs).Error; err != nil {
		return nil, err
	}
	// 批量读取该实体的值
	valMap := map[uint]string{}
	if entityID > 0 && len(defs) > 0 {
		fieldIDs := make([]uint, 0, len(defs))
		for _, d := range defs {
			fieldIDs = append(fieldIDs, d.ID)
		}
		var vals []models.CustomFieldValue
		if err := h.DB.Where("module = ? AND entity_id = ? AND field_id IN ?", mod, entityID, fieldIDs).
			Find(&vals).Error; err != nil {
			return nil, err
		}
		for _, v := range vals {
			valMap[v.FieldID] = v.Value
		}
	}
	// 收集 select 类型字段用到的字典编码
	dictCodes := map[string]struct{}{}
	for _, d := range defs {
		if isSelectType(d.FieldType) && d.DictCode != "" {
			dictCodes[d.DictCode] = struct{}{}
		}
	}
	dictItems := map[string][]models.SysDictItem{}
	if len(dictCodes) > 0 {
		codes := make([]string, 0, len(dictCodes))
		for c := range dictCodes {
			codes = append(codes, c)
		}
		var dicts []models.SysDict
		if err := h.DB.Where("code IN ? AND enabled = ?", codes, 1).Find(&dicts).Error; err != nil {
			return nil, err
		}
		dictID2Code := map[uint]string{}
		dictIDs := make([]uint, 0, len(dicts))
		for _, d := range dicts {
			dictID2Code[d.ID] = d.Code
			dictIDs = append(dictIDs, d.ID)
		}
		var items []models.SysDictItem
		if err := h.DB.Where("dict_id IN ? AND enabled = ?", dictIDs, 1).
			Order("sort_order asc, id asc").Find(&items).Error; err != nil {
			return nil, err
		}
		for _, it := range items {
			code := dictID2Code[it.DictID]
			dictItems[code] = append(dictItems[code], it)
		}
	}
	out := make([]fieldWithValueOut, 0, len(defs))
	for _, d := range defs {
		row := fieldWithValueOut{CustomFieldDef: d, Value: valMap[d.ID]}
		if isSelectType(d.FieldType) && d.DictCode != "" {
			row.Options = dictItems[d.DictCode]
		}
		out = append(out, row)
	}
	return out, nil
}

type customFieldValueItem struct {
	FieldID uint   `json:"field_id" binding:"required"`
	Value   string `json:"value"`
}

type customFieldValuesReq struct {
	Values []customFieldValueItem `json:"values" binding:"required"`
}

// SaveValues 保存某模块某实体的字段值（upsert）。只允许写入启用字段。
func (h *CustomFieldHandler) SaveValues(c *gin.Context) {
	mod := c.Param("module")
	if mod == "" {
		response.BizFail(c, "模块不能为空", 400)
		return
	}
	if !validModuleCode(mod) {
		response.BizFail(c, "模块编码格式无效", 422)
		return
	}
	eid, err := strconv.ParseUint(c.Param("entityId"), 10, 64)
	if err != nil || eid < 1 {
		response.BizFail(c, "无效实体 ID", 400)
		return
	}
	if !services.NewRBACService(h.DB).CanAccessEntity(middleware.CurrentUser(c), mod, uint(eid), true) {
		response.BizFail(c, "资源不存在或无访问权限", 403)
		return
	}

	var req customFieldValuesReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	// 一次性取出该模块所有启用字段
	var defs []models.CustomFieldDef
	if err := h.DB.Where("module = ? AND enabled = ?", mod, 1).Find(&defs).Error; err != nil {
		response.BizFail(c, "加载字段失败", 500)
		return
	}
	defMap := map[uint]models.CustomFieldDef{}
	for _, d := range defs {
		defMap[d.ID] = d
	}
	// 必填校验：传来的值集合中，对 required 字段必须有非空值
	provided := map[uint]string{}
	for _, v := range req.Values {
		if _, ok := defMap[v.FieldID]; !ok {
			// 字段不存在或未启用，跳过
			continue
		}
		provided[v.FieldID] = v.Value
	}
	for _, d := range defs {
		if d.Required == 0 {
			continue
		}
		val := strings.TrimSpace(provided[d.ID])
		if val == "" {
			response.BizFail(c, "字段「"+d.FieldName+"」必填", 422)
			return
		}
	}
	// upsert
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		for fid, val := range provided {
			var row models.CustomFieldValue
			err := tx.Where("module = ? AND entity_id = ? AND field_id = ?", mod, eid, fid).
				First(&row).Error
			if err == gorm.ErrRecordNotFound {
				row = models.CustomFieldValue{
					Module: mod, EntityID: uint(eid), FieldID: fid, Value: val,
				}
				if e := tx.Create(&row).Error; e != nil {
					return e
				}
			} else if err != nil {
				return err
			} else {
				if e := tx.Model(&row).Updates(map[string]interface{}{"value": val}).Error; e != nil {
					return e
				}
			}
		}
		return nil
	}); err != nil {
		response.BizFail(c, "保存失败", 500)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "audit", "success", "custom_field.save_values", &me, mod+":"+strconv.FormatUint(eid, 10))
	response.OK(c, nil, "已保存")
}
