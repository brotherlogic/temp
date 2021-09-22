package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	pb "github.com/brotherlogic/temp/proto"
)

type Humidity struct {
	Value int `json:"ambientHumidityPercent"`
}

type Temperature struct {
	Value float32 `json:"ambientTemperatureCelsius"`
}

type Trait struct {
	HumidityVal    Humidity    `json:"sdm.devices.traits.Humidity"`
	TemperatureVal Temperature `json:"sdm.devices.traits.Temperature"`
}

type Device struct {
	Name   string `json:"name"`
	Traits Trait  `json:"traits"`
}

type DevResp struct {
	Devices []Device `json:"devices"`
}

func (s *Server) Proc(ctx context.Context, req *pb.ProcRequest) (*pb.ProcResponse, error) {
	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(
		"https://smartdevicemanagement.googleapis.com/v1/enterprises/%v/devices", config.GetProjectId())

	hr, err := http.NewRequest("GET", url, strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	hr.Header.Add("Content-Type", "application/json")
	hr.Header.Add("Authorization", fmt.Sprintf("Bearer %v", config.GetCode()))

	client := &http.Client{}
	resp, err := client.Do(hr)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	devices := &DevResp{}
	err = json.Unmarshal(body, devices)
	if err != nil {
		return nil, err
	}

	s.Log(fmt.Sprintf("NowHERE %v -> %+v", string(body), devices))

	return nil, fmt.Errorf("Not implemented yet")
}

type CodeResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) getAuth(ctx context.Context, config *pb.Config, code string) (string, string, error) {
	url := fmt.Sprintf(
		"https://www.googleapis.com/oauth2/v4/token?client_id=%v&client_secret=%v&code=%v&grant_type=authorization_code&redirect_uri=https://www.google.com",
		config.GetClientId(), config.GetClientSecret(), code)

	post, err := http.Post(url, "", strings.NewReader(""))
	if err != nil {
		return "", "", err
	}

	defer post.Body.Close()
	body, err := ioutil.ReadAll(post.Body)
	if err != nil {
		return "", "", err
	}

	cr := &CodeResp{}
	err = json.Unmarshal(body, cr)
	if err != nil {
		return "", "", err
	}
	s.Log(fmt.Sprintf("NOW GOT %v -> %v", string(body), cr))

	return cr.AccessToken, cr.RefreshToken, nil

}

func (s *Server) SetConfig(ctx context.Context, req *pb.SetConfigRequest) (*pb.SetConfigResponse, error) {
	config, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	if req.GetClientId() != "" {
		config.ClientId = req.GetClientId()
	}

	if req.GetClientSecret() != "" {
		config.ClientSecret = req.GetClientSecret()
	}

	if req.GetProjectId() != "" {
		config.ProjectId = req.GetProjectId()
	}

	if req.GetAuthCode() != "" {
		auth, refresh, err := s.getAuth(ctx, config, req.GetAuthCode())
		if err != nil {
			return nil, err
		}

		config.Code = auth
		config.Refresh = refresh
	}

	return &pb.SetConfigResponse{}, s.saveConfig(ctx, config)
}
