package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	google_protobuf "github.com/golang/protobuf/ptypes/any"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/proto"

	qpb "github.com/brotherlogic/queue/proto"
	pb "github.com/brotherlogic/temp/proto"
)

var (
	ntemp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_nesttemp",
		Help: "Temperature from the thermostat",
	})
	nhumid = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_nesthum",
		Help: "Humidity from the thermostat",
	})
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

func (s *Server) refresh(ctx context.Context, config *pb.Config) error {
	url := fmt.Sprintf("https://www.googleapis.com/oauth2/v4/token?client_id=%v&client_secret=%v&refresh_token=%v&grant_type=refresh_token",
		config.GetClientId(), config.GetClientSecret(), config.GetRefresh())
	post, err := http.Post(url, "", strings.NewReader(""))
	if err != nil {
		return err
	}

	defer post.Body.Close()
	body, err := ioutil.ReadAll(post.Body)
	if err != nil {
		return err
	}

	s.Log(fmt.Sprintf("BOUNCER with client %v, %v, %v, %v -> %v", config.GetClientId(), config.GetClientSecret(), config.GetRefresh(), config.GetProjectId(), string(body)))

	cr := &CodeResp{}
	err = json.Unmarshal(body, cr)
	if err != nil {
		return err
	}

	config.Code = cr.AccessToken
	return s.saveConfig(ctx, config)
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

	if len(devices.Devices) == 0 {
		err = s.refresh(ctx, config)
		if err != nil {
			return nil, err
		}
		return s.Proc(ctx, req)
	}

	s.Log(fmt.Sprintf("NowHERE %v -> %+v", string(body), devices))

	ntemp.Set(float64(devices.Devices[0].Traits.TemperatureVal.Value))
	nhumid.Set(float64(devices.Devices[0].Traits.HumidityVal.Value))

	var err3 error
	if !req.GetDebug() {
		conn2, err2 := s.FDialServer(ctx, "queue")
		if err2 != nil {
			return nil, err2
		}
		defer conn2.Close()
		qclient := qpb.NewQueueServiceClient(conn2)
		upup := &pb.ProcRequest{}
		data, _ := proto.Marshal(upup)
		_, err3 = qclient.AddQueueItem(ctx, &qpb.AddQueueItemRequest{
			QueueName: "temp",
			RunTime:   time.Now().Add(time.Minute).Unix(),
			Payload:   &google_protobuf.Any{Value: data},
			Key:       "ntemp",
		})
	}

	return &pb.ProcResponse{
		NestTemperature: devices.Devices[0].Traits.TemperatureVal.Value,
		NestHumidity:    float32(devices.Devices[0].Traits.HumidityVal.Value),
	}, err3
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

	s.Log(fmt.Sprintf("New config %v", config))
	return &pb.SetConfigResponse{}, s.saveConfig(ctx, config)
}
