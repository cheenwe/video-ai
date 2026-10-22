package videoai

import (
	"encoding/json"

	"baseplatform/internal/models"

	"gorm.io/gorm"
)

// SeedDefaults inserts built-in rules, mock model and grab business template.
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
		model = models.AlgoModel{Code: "grab_bucket_seg", Name: "抓斗分割（演示）", Framework: "onnx", Remark: "演示用占位模型，适配器为 mock"}
		if err := db.Create(&model).Error; err != nil {
			return err
		}
	}
	var mv models.AlgoModelVersion
	err = db.Where("model_id = ? AND version = ?", model.ID, "demo-1.0.0").First(&mv).Error
	if err == gorm.ErrRecordNotFound {
		now := db.NowFunc()
		mv = models.AlgoModelVersion{
			ModelID: model.ID, Version: "demo-1.0.0", Status: "published",
			Adapter: "mock", Backend: "cpu", ClassesJSON: `["grab_bucket"]`,
			InputSchema: `{"type":"image"}`, OutputSchema: `{"type":"instances"}`,
			PublishedAt: &now,
		}
		if err := db.Create(&mv).Error; err != nil {
			return err
		}
	}

	var tpl models.BusinessTemplate
	err = db.Where("code = ?", "grab_edge_guard").First(&tpl).Error
	if err == gorm.ErrRecordNotFound {
		codes, _ := json.Marshal([]string{RuleNearEdge, RuleCrossBoundary})
		tpl = models.BusinessTemplate{
			Code: "grab_edge_guard", Name: "抓斗越界监测", Version: "1.0.0", Status: "published",
			ModelVersionID: &mv.ID, RuleCodesJSON: string(codes),
			Description: "抓斗分割 + 坑边识别 + 越界判定（演示适配器）",
		}
		if err := db.Create(&tpl).Error; err != nil {
			return err
		}
	}
	return nil
}
