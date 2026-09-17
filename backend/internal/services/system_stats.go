package services

import (
	"path/filepath"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
)

type DiskStat struct {
	Path    string  `json:"path"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

type SystemStats struct {
	Hostname   string     `json:"hostname"`
	OS         string     `json:"os"`
	Platform   string     `json:"platform"`
	Arch       string     `json:"arch"`
	UptimeSec  uint64     `json:"uptime_sec"`
	CPUCores   int        `json:"cpu_cores"`
	CPUPercent float64    `json:"cpu_percent"`
	MemTotal   uint64     `json:"mem_total"`
	MemUsed    uint64     `json:"mem_used"`
	MemPercent float64    `json:"mem_percent"`
	Disks      []DiskStat `json:"disks"`
}

func CollectSystemStats(diskPaths ...string) SystemStats {
	out := SystemStats{
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	if info, err := host.Info(); err == nil {
		out.Hostname = info.Hostname
		out.OS = info.OS
		out.Platform = info.Platform
		if info.PlatformVersion != "" {
			out.Platform = info.Platform + " " + info.PlatformVersion
		}
		out.UptimeSec = info.Uptime
	}

	if percents, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(percents) > 0 {
		out.CPUPercent = round1(percents[0])
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		out.MemTotal = vm.Total
		out.MemUsed = vm.Used
		out.MemPercent = round1(vm.UsedPercent)
	}

	seen := map[string]struct{}{}
	for _, raw := range diskPaths {
		p := filepath.Clean(raw)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		if usage, err := disk.Usage(p); err == nil {
			out.Disks = append(out.Disks, DiskStat{
				Path:    p,
				Total:   usage.Total,
				Used:    usage.Used,
				Free:    usage.Free,
				Percent: round1(usage.UsedPercent),
			})
		}
	}
	return out
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
