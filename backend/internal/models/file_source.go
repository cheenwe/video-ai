package models

// FileSource* 文件操作来源（只存 ID）。
const (
	FileSourceWeb            uint8 = 1
	FileSourceAPI            uint8 = 2
	FileSourceWebDAV         uint8 = 3
	FileSourceSMB            uint8 = 4
	FileSourceFTP            uint8 = 5
	FileSourceSFTP           uint8 = 6
	FileSourceS3             uint8 = 7
	FileSourceNFS            uint8 = 8
	FileSourceOnlyOffice     uint8 = 9
	FileSourceSync           uint8 = 10 // 服务端跨存储同步
	FileSourceDesktop        uint8 = 11 // 桌面 Sync Drive（未区分平台时的通用值）
	FileSourceDesktopLinux   uint8 = 12
	FileSourceDesktopDarwin  uint8 = 13 // macOS
	FileSourceDesktopWindows uint8 = 14
)

var fileSourceNames = map[uint8]string{
	FileSourceWeb:            "Web",
	FileSourceAPI:            "API",
	FileSourceWebDAV:         "WebDAV",
	FileSourceSMB:            "SMB",
	FileSourceFTP:            "FTP",
	FileSourceSFTP:           "SFTP",
	FileSourceS3:             "S3",
	FileSourceNFS:            "NFS",
	FileSourceOnlyOffice:     "OnlyOffice",
	FileSourceSync:           "Sync",
	FileSourceDesktop:        "Desktop",
	FileSourceDesktopLinux:   "Desktop-Linux",
	FileSourceDesktopDarwin:  "Desktop-macOS",
	FileSourceDesktopWindows: "Desktop-Windows",
}

// FileSourceDesktopIDs 返回桌面 Sync Drive 家族全部 source 值（含通用 11）。
func FileSourceDesktopIDs() []uint8 {
	return []uint8{
		FileSourceDesktop,
		FileSourceDesktopLinux,
		FileSourceDesktopDarwin,
		FileSourceDesktopWindows,
	}
}

func IsDesktopFileSource(id uint8) bool {
	switch id {
	case FileSourceDesktop, FileSourceDesktopLinux, FileSourceDesktopDarwin, FileSourceDesktopWindows:
		return true
	}
	return false
}

func FileSourceName(id uint8) string {
	if name, ok := fileSourceNames[id]; ok {
		return name
	}
	if id == 0 {
		return ""
	}
	return "Unknown"
}
