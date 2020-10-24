package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/brotherlogic/goserver"
	"github.com/brotherlogic/goserver/utils"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pbg "github.com/brotherlogic/goserver/proto"
	kmpb "github.com/brotherlogic/keymapper/proto"
)

//Server main server type
type Server struct {
	*goserver.GoServer
	key string
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
	cancel()

	fmt.Printf("%v", server.Serve())
}
