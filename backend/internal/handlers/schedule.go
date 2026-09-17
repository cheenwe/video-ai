package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"baseplatform/internal/middleware"
	"baseplatform/internal/models"
	"baseplatform/internal/response"
	"baseplatform/internal/scheduler"
	"baseplatform/internal/services"
)

type ScheduleHandler struct {
	DB *gorm.DB
	M  *scheduler.Manager
}

func (h *ScheduleHandler) ListJobs(c *gin.Context) {
	jobs, err := h.M.ListJobs()
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"items": jobs}, "")
}

func (h *ScheduleHandler) GetJob(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	job, err := h.M.GetJob(uint(id))
	if err != nil {
		response.BizFail(c, "任务不存在", 404)
		return
	}
	response.OK(c, job, "")
}

func (h *ScheduleHandler) CreateJob(c *gin.Context) {
	var j models.ScheduleJob
	if err := c.ShouldBindJSON(&j); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if err := h.M.CreateJob(&j); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.job.create", &me, j.Name)
	response.OK(c, j, "已创建")
}

func (h *ScheduleHandler) UpdateJob(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	j, err := h.M.UpdateJob(uint(id), patch)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.job.update", &me, j.Name)
	response.OK(c, j, "已更新")
}

func (h *ScheduleHandler) DeleteJob(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if err := h.M.DeleteJob(uint(id)); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.job.delete", &me, "")
	response.OK(c, nil, "已删除")
}

func (h *ScheduleHandler) RunJob(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if err := h.M.RunJob(uint(id), "manual"); err != nil {
		response.BizFail(c, err.Error(), 409)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.job.run", &me, "")
	response.OK(c, nil, "已触发执行")
}

func (h *ScheduleHandler) ListCommands(c *gin.Context) {
	list, err := h.M.ListCommands()
	if err != nil {
		response.BizFail(c, "加载失败", 500)
		return
	}
	response.OK(c, gin.H{"items": list}, "")
}

func (h *ScheduleHandler) CreateCommand(c *gin.Context) {
	var cmd models.ScheduleCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if err := h.M.CreateCommand(&cmd); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.cmd.create", &me, cmd.Name)
	response.OK(c, cmd, "已创建")
}

func (h *ScheduleHandler) UpdateCommand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	cmd, err := h.M.UpdateCommand(uint(id), patch)
	if err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.cmd.update", &me, cmd.Name)
	response.OK(c, cmd, "已更新")
}

func (h *ScheduleHandler) DeleteCommand(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效 ID", 400)
		return
	}
	if err := h.M.DeleteCommand(uint(id)); err != nil {
		response.BizFail(c, err.Error(), 400)
		return
	}
	me := middleware.CurrentUser(c)
	services.LogAction(h.DB, c, "sync", "success", "schedule.cmd.delete", &me, "")
	response.OK(c, nil, "已删除")
}

func (h *ScheduleHandler) Stats(c *gin.Context) {
	response.OK(c, h.M.StatsSummary(), "")
}

func (h *ScheduleHandler) ValidateCron(c *gin.Context) {
	var req struct {
		Cron string `json:"cron" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BizFail(c, "参数错误", 422)
		return
	}
	if err := h.M.ValidateCron(req.Cron); err != nil {
		response.BizFail(c, err.Error(), 422)
		return
	}
	response.OK(c, gin.H{"valid": true}, "")
}

func (h *ScheduleHandler) Runs(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		response.BizFail(c, "无效ID", 400)
		return
	}
	var rows []models.ScheduleRun
	if h.DB.Where("job_id = ?", id).Order("id desc").Limit(100).Find(&rows).Error != nil {
		response.BizFail(c, "读取执行记录失败", 500)
		return
	}
	response.OK(c, gin.H{"items": rows}, "")
}
