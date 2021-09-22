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

func (s *Server) Proc(ctx context.Context, req *pb.ProcRequest) (*pb.ProcResponse, error) {
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
	s.Log(fmt.Sprintf("GOT %v -> %v", string(body), cr))

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
