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
	ts   string      `json:"ts"`
	data kaiteraData `json:"data"`
}

type kaiteraData struct {
	humidity float32 `json:"humidity"`
	pm10     float32 `json:"pm10"`
	pm25     float32 `json:"pm25"`
	st03     float32 `json:"st03.rtvoc"`
	temp     float32 `json:"temp"`
}

var (
	temp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_temp",
		Help: "The number of server requests",
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

		temp.Set(float64(kr.InfoAqi.data.temp))

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

	ctx, cancel := utils.ManualContext("temp", "temp", time.Minute, false)
	conn, err := server.FDialServer(ctx, "keymapper")
	if err != nil {
		log.Fatalf("Cannot reach keymapper: %v", err)
	}
	client := kmpb.NewKeymapperServiceClient(conn)
	resp, err := client.Get(ctx, &kmpb.GetRequest{Key: "kaitera_token"})
	if err != nil {
		log.Fatalf("Cannot read token: %v", err)
	}
	server.key = resp.GetKey().GetValue()

	resp, err = client.Get(ctx, &kmpb.GetRequest{Key: "kaitera_id"})
	if err != nil {
		log.Fatalf("Cannot read token: %v", err)
	}
	server.client = resp.GetKey().GetValue()
	cancel()
	conn.Close()

	fmt.Printf("%v", server.Serve())
}
