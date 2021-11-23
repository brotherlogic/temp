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
	nreup = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_nestreup",
		Help: "Temperature from the thermostat",
	})
	nmode = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_mdoe",
		Help: "Temperature from the thermostat",
	})
	nset = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_set_point",
		Help: "Temperature from the thermostat",
	})
)

type Humidity struct {
	Value int `json:"ambientHumidityPercent"`
}

type Temperature struct {
	Value float32 `json:"ambientTemperatureCelsius"`
}

type On struct {
	Mode string `json:"mode"`
}

type SetP struct {
	Value float32 `json:"heatCelsius"`
}

type Trait struct {
	HumidityVal    Humidity    `json:"sdm.devices.traits.Humidity"`
	TemperatureVal Temperature `json:"sdm.devices.traits.Temperature"`
	OnVal          On          `json:"sdm.devices.traits.ThermostatMode"`
	SetPoint       SetP        `json:"sdm.devices.traits.ThermostatTemperatureSetpoint"`
}

type Device struct {
	Name   string `json:"name"`
	Traits Trait  `json:"traits"`
}

type DevResp struct {
	Devices []Device `json:"devices"`
}

func (s *Server) refresh(ctx context.Context, config *pb.Config) error {
	nreup.Inc()
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

	devices := &DevResp{}

	// 429 is resource exhausted
	if resp.StatusCode != 429 {

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(body, devices)
		if err != nil {
			return nil, err
		}

		if len(devices.Devices) == 0 {
			s.Log(fmt.Sprintf("Failed to read %v", string(body)))
			err = s.refresh(ctx, config)
			if err != nil {
				s.Log(fmt.Sprintf("Failed to refresh: %v", err))
				return nil, err
			}
			return s.Proc(ctx, req)
		}

		s.Log(fmt.Sprintf("I READ: %v from %v", devices, string(body)))
		ntemp.Set(float64(devices.Devices[0].Traits.TemperatureVal.Value))
		nhumid.Set(float64(devices.Devices[0].Traits.HumidityVal.Value))
		nset.Set(float64(devices.Devices[0].Traits.SetPoint.Value))
		if devices.Devices[0].Traits.OnVal.Mode == "HEAT" {
			nmode.Set(float64(1))
		} else {
			nmode.Set(float64(0))
		}
		lastPull.With(prometheus.Labels{"source": "nest"}).Set(float64(time.Now().Unix()))
	}

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
			Key:       fmt.Sprintf("ntemp-%v", time.Now().Add(time.Minute).Minute()),
		})
	}

	if len(devices.Devices) > 0 {
		return &pb.ProcResponse{
			NestTemperature: devices.Devices[0].Traits.TemperatureVal.Value,
			NestHumidity:    float32(devices.Devices[0].Traits.HumidityVal.Value),
		}, err3
	}
	return &pb.ProcResponse{}, err3
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
