package main

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
)

func (s *Server) runComputation(ctx context.Context) error {
	t := time.Now()
	sum := 0
	for i := 0; i < 10000; i++ {
		sum += i
	}
	s.Log(fmt.Sprintf("Sum is %v -> %v", sum, time.Now().Sub(t).Nanoseconds()/1000000))
	return nil
}
