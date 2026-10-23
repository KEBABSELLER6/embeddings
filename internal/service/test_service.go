package service

import "log/slog"

type TestService struct {
}

func NewTestService() *TestService {
	return &TestService{}
}

func (s *TestService) Test() {
	slog.Info("service hello")
}
