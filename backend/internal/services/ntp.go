package services

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"time"
)

const ntpEpochOffset = 2208988800

type NTPQueryResult struct {
	ServerTime  time.Time `json:"ntp_time"`
	LocalTime   time.Time `json:"local_time"`
	OffsetMs    int64     `json:"offset_ms"`
	RoundTripMs int64     `json:"round_trip_ms"`
	Server      string    `json:"server"`
}

func QueryNTP(server string) (*NTPQueryResult, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return nil, errors.New("NTP 服务器地址不能为空")
	}
	addr := server
	if !strings.Contains(addr, ":") {
		addr = addr + ":123"
	}
	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	req := make([]byte, 48)
	req[0] = 0x1b
	start := time.Now()
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		return nil, err
	}
	rtt := time.Since(start)
	secs := binary.BigEndian.Uint32(resp[40:44]) - ntpEpochOffset
	nanos := int64(binary.BigEndian.Uint32(resp[44:48])) * 1000 / 0x100000000
	ntpTime := time.Unix(int64(secs), nanos)
	return &NTPQueryResult{
		ServerTime:  ntpTime,
		LocalTime:   time.Now(),
		OffsetMs:    ntpTime.Sub(time.Now()).Milliseconds(),
		RoundTripMs: rtt.Milliseconds(),
		Server:      server,
	}, nil
}

func SyncNTP(server string) error {
	res, err := QueryNTP(server)
	if err != nil {
		return err
	}
	_ = res
	return nil
}
