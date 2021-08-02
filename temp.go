package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbg "github.com/brotherlogic/goserver/proto"
	kmpb "github.com/brotherlogic/keymapper/proto"
)

//Server main server type
type Server struct {
	*goserver.GoServer
	key    string
	client string
}

// Init builds the server
func Init() *Server {
	s := &Server{
		GoServer: &goserver.GoServer{},
	}
	return s
}

// DoRegister does RPC registration
func (s *Server) DoRegister(server *grpc.Server) {

}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// Mote promotes/demotes this server
func (s *Server) Mote(ctx context.Context, master bool) error {
	return nil
}

type kaiteraResponse struct {
	ID      string  `json:"id"`
	InfoAqi infoAqi `json:"info.aqi"`
}

type infoAqi struct {
	Ts   string      `json:"ts"`
	Data kaiteraData `json:"data"`
}

type kaiteraData struct {
	Humidity float32 `json:"humidity"`
	Pm10     float32 `json:"pm10"`
	Pm25     float32 `json:"pm25"`
	St03     float32 `json:"st03.rtvoc"`
	Temp     float32 `json:"temp"`
}

var (
	temp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_temp",
		Help: "The number of server requests",
	})
	tvoc = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_tvoc",
		Help: "TVOC measure",
	})
)

func (s *Server) run() {
	for true {
		url := fmt.Sprintf("https://api.kaiterra.com/v1/lasereggs/%v?key=%v", s.client, s.key)
		resp, err := http.Get(url)
		if err != nil {
			s.Log(fmt.Sprintf("Error on get: %v", err))
			continue
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			s.Log(fmt.Sprintf("Bad read: %v", err))
			continue
		}

		kr := &kaiteraResponse{}
		err = json.Unmarshal(body, kr)
		if err != nil {
			s.Log(fmt.Sprintf("Bad unmarshal: %v", err))
			continue
		}

		s.Log(fmt.Sprintf("%v from %v", kr, string(body)))

		temp.Set(float64(kr.InfoAqi.Data.Temp))
		tvoc.Set(float64(kr.InfoAqi.Data.St03))

		time.Sleep(time.Minute)
	}
}

// GetState gets the state of the server
func (s *Server) GetState() []*pbg.State {
	return []*pbg.State{
		&pbg.State{Key: "magic", Value: int64(12345)},
	}
}

func main() {
	var quiet = flag.Bool("quiet", false, "Show all output")
	flag.Parse()

	//Turn off logging
	if *quiet {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}
	server := Init()
	server.PrepServer()
	server.Register = server

	err := server.RegisterServerV2("temp", false, true)
	if err != nil {
		return
	}

	ctx, cancel := utils.ManualContext("temp", time.Minute)
	conn, err := server.FDialServer(ctx, "keymapper")
	if err != nil {
		if status.Convert(err).Code() == codes.Unknown {
			log.Fatalf("Cannot reach keymapper: %v", err)
		}
		return
	}
	client := kmpb.NewKeymapperServiceClient(conn)
	resp, err := client.Get(ctx, &kmpb.GetRequest{Key: "kaitera_token"})
	if err != nil {
		if status.Convert(err).Code() == codes.Unknown {
			log.Fatalf("Cannot read token: %v", err)
		}
		return
	}
	server.key = resp.GetKey().GetValue()

	resp, err = client.Get(ctx, &kmpb.GetRequest{Key: "kaitera_id"})
	if err != nil {
		if status.Convert(err).Code() == codes.Unknown {
			log.Fatalf("Cannot read token: %v", err)
		}
		return
	}
	server.client = resp.GetKey().GetValue()
	cancel()
	conn.Close()

	go server.run()

	server.Serve()
}
