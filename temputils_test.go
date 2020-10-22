package main

import (
	"context"
	"testing"
)

func InitTest() *Server {
	s := Init()
	s.SkipLog = true
	return s
}

func TestBasic(t *testing.T) {
	s := InitTest()
	err := s.runComputation(context.Background())
	if err != nil {
		t.Errorf("Bad run: %v", err)
	}
}
