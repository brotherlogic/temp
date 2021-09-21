package main

import (
	"fmt"

	"golang.org/x/net/context"

	pb "github.com/brotherlogic/temp/proto"
)

func (s *Server) Proc(ctx context.Context, req *pb.ProcRequest) (*pb.ProcResponse, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (s *Server) SetConfig(ctx context.Context, req *pb.SetConfigRequest) (*pb.SetConfigResponse, error) {
	return nil, fmt.Errorf("Not implemented yet")
}
