package services

import (
	"errors"
	"github.com/magiconair/properties/assert"
	"ip2loc/app/conf"
	"testing"
)

var (
	service *Service
)

func init() {
	service = New(&conf.Config{
		Logger:  &conf.LogConfig{LogLevel: "debug"},
		General: conf.NewGeneralConfig(),
	})
	service.conf.General.Put("db-path", "../../db/")
}

func TestService_GetIPLocationInLocalDB(t *testing.T) {
	ipInfo, err := service.GetIPLocationInLocalDB("54.248.162.57")
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, ipInfo.CountryCode, "JP")
}

func TestService_GetIPLocationInLocalDB_InvalidIP(t *testing.T) {
	_, err := service.GetIPLocationInLocalDB("54.248.162.57, 10.0.0.1")
	assert.Equal(t, errors.Is(err, ErrInvalidIP), true)
}
