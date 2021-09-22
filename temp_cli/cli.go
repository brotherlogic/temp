package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/brotherlogic/goserver/utils"
	"google.golang.org/grpc/resolver"

	pb "github.com/brotherlogic/temp/proto"
)

func init() {
	resolver.Register(&utils.DiscoveryClientResolverBuilder{})
}

func main() {
	ctx, cancel := utils.ManualContext("temp-cli", time.Second*10)
	defer cancel()

	conn, err := utils.LFDialServer(ctx, "temp")
	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewTempServiceClient(conn)

	switch os.Args[1] {
	case "set":
		setFlags := flag.NewFlagSet("SetConfig", flag.ContinueOnError)
		var clientId = setFlags.String("client_id", "", "Id of the record to add")
		var clientSecret = setFlags.String("client_secret", "", "Cost of the record")
		var code = setFlags.String("code", "", "")
		var project = setFlags.String("project_id", "", "")

		if err := setFlags.Parse(os.Args[2:]); err == nil {
			if (*clientId != "" && *clientSecret != "") || *code != "" || *project != "" {
				_, err := client.SetConfig(ctx, &pb.SetConfigRequest{ProjectId: *project, ClientId: *clientId, ClientSecret: *clientSecret, AuthCode: *code})
				if err != nil {
					log.Fatalf("Bad request: %v", err)
				}
			}
		}
	case "get":
		res, err := client.Proc(ctx, &pb.ProcRequest{})
		log.Printf("%v -> %v", res, err)
	}
}
