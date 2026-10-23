package videoai

import (
	"encoding/json"

	"baseplatform/internal/models"

	"gorm.io/gorm"
)

// SeedDefaults inserts built-in rules, the grab model skeleton and the grab
// business template. There is no demo/mock model version: a fresh database
// ships an unpublished onnx version that an operator completes with a real
// model file. Legacy databases that still reference a mock version are
// migrated to the onnx version (the mock rows are archived) so nothing can
// silently fall back to synthetic inference.
func SeedDefaults(db *gorm.DB) error {
	rules := []models.RuleVersion{
		{Code: RuleNearEdge, Version: "1.0.0", Name: "抓斗临边", Status: "published", Description: "抓斗靠近坑边缓冲带", ConfigJSON: `{"metric":"distance_norm"}`},
		{Code: RuleCrossBoundary, Version: "1.0.0", Name: "抓斗越界", Status: "published", Description: "抓斗越过坑边线", ConfigJSON: `{"metric":"distance_norm"}`},
	}
	for _, r := range rules {
		var n int64
		db.Model(&models.RuleVersion{}).Where("code = ? AND version = ?", r.Code, r.Version).Count(&n)
		if n == 0 {
			if err := db.Create(&r).Error; err != nil {
				return err
			}
		}
	}

	var model models.AlgoModel
	err := db.Where("code = ?", "grab_bucket_seg").First(&model).Error
	if err == gorm.ErrRecordNotFound {
		model = models.AlgoModel{Code: "grab_bucket_seg", Name: "抓斗分割", Framework: "onnx", Remark: "真实 ONNX 模型骨架；需上传模型文件并发布后才能推理"}
		if err := db.Create(&model).Error; err != nil {
			return err
		}
	}

	// The shipped version is a draft without a file: publishing requires a
	// real model path, and self-check proves it runs before it can be used.
	var mv models.AlgoModelVersion
	err = db.Where("model_id = ? AND version = ?", model.ID, "v1").First(&mv).Error
	if err == gorm.ErrRecordNotFound {
		mv = models.AlgoModelVersion{
			ModelID: model.ID, Version: "v1", Status: "draft",
			Adapter: "onnx", Backend: "cpu",
			InputSchema:  `{"type":"image","width":640,"height":640,"confidence":0.25,"iou":0.45}`,
			OutputSchema: `{"type":"instances"}`,
			ClassesJSON:  `["grab_bucket"]`,
		}
		if err := db.Create(&mv).Error; err != nil {
			return err
		}
	}

	// Migration: templates bound to a legacy mock/demo version are re-pointed
	// at the onnx version, and legacy mock versions are archived so the UI
	// stops offering them.
	var legacy []models.AlgoModelVersion
	db.Where("adapter = ?", "mock").Find(&legacy)
	for _, lv := range legacy {
		if lv.Status != "archived" {
			_ = db.Model(&lv).Update("status", "archived").Error
		}
	}
	if _, aerr := AdapterByName("mock"); aerr != nil {
		// mock is unregistered in this build: any binding to it is dead.
		db.Model(&models.BusinessTemplate{}).
			Where("model_version_id IN (?)", db.Model(&models.AlgoModelVersion{}).Select("id").Where("adapter = ?", "mock")).
			Update("model_version_id", mv.ID)
	}

	var tpl models.BusinessTemplate
	err = db.Where("code = ?", "grab_edge_guard").First(&tpl).Error
	if err == gorm.ErrRecordNotFound {
		codes, _ := json.Marshal([]string{RuleNearEdge, RuleCrossBoundary})
		tpl = models.BusinessTemplate{
			Code: "grab_edge_guard", Name: "抓斗越界监测", Version: "1.0.0", Status: "published",
			ModelVersionID: &mv.ID, RuleCodesJSON: string(codes),
			Description: "抓斗分割 + 坑边识别 + 越界判定（需上传并发布真实 ONNX 模型）",
		}
		if err := db.Create(&tpl).Error; err != nil {
			return err
		}
	} else if tpl.ModelVersionID == nil {
		_ = db.Model(&tpl).Update("model_version_id", mv.ID).Error
	}
	return nil
}
