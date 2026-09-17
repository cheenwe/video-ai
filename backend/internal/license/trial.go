package license

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type TrialStatus struct {
	ID        uint   `json:"id,omitempty"`
	Status    string `json:"status"`
	Edition   string `json:"edition,omitempty"`
	LicenseID string `json:"license_id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Message   string `json:"message,omitempty"`
}

type trialApplyBody struct {
	MachineID      string `json:"machine_id"`
	ShortMachineID string `json:"short_machine_id"`
	TeamName       string `json:"team_name"`
	Contact        string `json:"contact"`
	Edition        string `json:"edition"`
	Hostname       string `json:"hostname"`
}

func postTrialJSON(serverURL, path string, body any) (*TrialStatus, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return nil, errors.New("请先配置授权服务器地址")
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, serverURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return parseTrialResponse(data)
}

func getTrialJSON(serverURL, path string) (*TrialStatus, error) {
	serverURL = strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if serverURL == "" {
		return nil, errors.New("请先配置授权服务器地址")
	}
	resp, err := http.Get(serverURL + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return parseTrialResponse(data)
}

func parseTrialResponse(raw []byte) (*TrialStatus, error) {
	var wrap struct {
		Success int         `json:"success"`
		Data    TrialStatus `json:"data"`
		Msg     string      `json:"msg"`
	}
	if json.Unmarshal(raw, &wrap) != nil {
		return nil, errors.New("授权服务器响应无效")
	}
	if wrap.Success != 1 {
		if wrap.Msg != "" {
			return nil, errors.New(wrap.Msg)
		}
		return nil, errors.New("授权服务器请求失败")
	}
	return &wrap.Data, nil
}

func (s *Service) ApplyTrial(serverURL, teamName, contact, edition string) (*TrialStatus, error) {
	info := CollectMachineInfo()
	return postTrialJSON(serverURL, "/api/v1/trial/apply", trialApplyBody{
		MachineID:      info.MachineID,
		ShortMachineID: info.ShortMachineID,
		TeamName:       strings.TrimSpace(teamName),
		Contact:        strings.TrimSpace(contact),
		Edition:        strings.TrimSpace(edition),
		Hostname:       info.Hostname,
	})
}

func (s *Service) TrialStatus(serverURL string) (*TrialStatus, error) {
	info := CollectMachineInfo()
	q := url.Values{"machine_id": {info.MachineID}}
	return getTrialJSON(serverURL, "/api/v1/trial/status?"+q.Encode())
}
