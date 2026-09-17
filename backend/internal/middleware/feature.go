package middleware

import (
	"fmt"

	"baseplatform/internal/license"
	"baseplatform/internal/response"

	"github.com/gin-gonic/gin"
)

var globalLicense *license.Service

func SetLicenseService(s *license.Service) {
	globalLicense = s
}

func featureDeniedMessage(code string) string {
	switch code {
	case license.FeatureBrandingCustom, license.FeatureWatermark, license.FeatureGateway,
		license.FeatureStorageSync, license.FeatureAdminTOTP, license.FeatureSSO,
		license.FeatureOps, license.FeatureExtensions, license.FeatureOnlineUpgrade:
		return fmt.Sprintf("需要旗舰版授权：%s", code)
	default:
		return "功能需要旗舰版授权"
	}
}

func RequireFeature(code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalLicense == nil || !globalLicense.HasFeature(code) {
			response.BizFail(c, featureDeniedMessage(code), 403)
			c.Abort()
			return
		}
		c.Next()
	}
}
