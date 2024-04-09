package services

import (
	"ip2loc/app/conf"
)

type Service struct {
	conf *conf.Config
}

// New service
func New(c *conf.Config) *Service {
	return &Service{
		conf: c,
	}
}
