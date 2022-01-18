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
	"google.golang.org/protobuf/proto"

	dspb "github.com/brotherlogic/dstore/proto"
	pbg "github.com/brotherlogic/goserver/proto"
	kmpb "github.com/brotherlogic/keymapper/proto"
	pb "github.com/brotherlogic/temp/proto"
	google_protobuf "github.com/golang/protobuf/ptypes/any"
)

const (
	CONFIG_KEY = "github.com/brotherlogic/temp/config"
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
	pb.RegisterTempServiceServer(server, s)
}

// ReportHealth alerts if we're not healthy
func (s *Server) ReportHealth() bool {
	return true
}

// Shutdown the server
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

func (s *Server) loadConfig(ctx context.Context) (*pb.Config, error) {
	conn, err := s.FDialServer(ctx, "dstore")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := dspb.NewDStoreServiceClient(conn)
	res, err := client.Read(ctx, &dspb.ReadRequest{Key: CONFIG_KEY})
	if err != nil {
		if status.Convert(err).Code() == codes.InvalidArgument {
			return &pb.Config{}, nil
		}

		return nil, err
	}

	if res.GetConsensus() < 0.5 {
		return nil, fmt.Errorf("could not get read consensus (%v)", res.GetConsensus())
	}

	config := &pb.Config{}
	err = proto.Unmarshal(res.GetValue().GetValue(), config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (s *Server) saveConfig(ctx context.Context, config *pb.Config) error {
	conn, err := s.FDialServer(ctx, "dstore")
	if err != nil {
		return err
	}
	defer conn.Close()

	data, err := proto.Marshal(config)
	if err != nil {
		return err
	}

	client := dspb.NewDStoreServiceClient(conn)
	res, err := client.Write(ctx, &dspb.WriteRequest{Key: CONFIG_KEY, Value: &google_protobuf.Any{Value: data}})
	if err != nil {
		return err
	}

	if res.GetConsensus() < 0.5 {
		return fmt.Errorf("could not get write consensus (%v)", res.GetConsensus())
	}

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
	humid = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "temp_humidity",
		Help: "TVOC measure",
	})
	lastPull = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "temp_last_pull",
		Help: "TVOC measure",
	}, []string{"source"})
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

		temp.Set(float64(kr.InfoAqi.Data.Temp))
		tvoc.Set(float64(kr.InfoAqi.Data.St03))
		humid.Set(float64(kr.InfoAqi.Data.Humidity))

		timev, err := time.Parse("2006-01-02T15:04:05Z", kr.InfoAqi.Ts)
		if err != nil {
			s.Log(fmt.Sprintf("PARSE ERROR from %v -> %v", kr.InfoAqi.Ts, err))
		} else {
			lastPull.With(prometheus.Labels{"source": "kaiterra"}).Set(float64(timev.Unix()))
		}

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

	_, err = server.Proc(ctx, &pb.ProcRequest{Debug: true})
	if err != nil {
		log.Fatalf("Unable to do initial proc: %v", err)
	}

	cancel()
	conn.Close()

	go server.run()

	server.Serve()
}
