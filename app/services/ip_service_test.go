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
		t.Fatal(err)
	}
	assert.Equal(t, ipInfo.CountryCode, "JP")
	assert.Equal(t, ipInfo.ContinentCode, "AS")
	assert.Equal(t, ipInfo.TimeZone, "Asia/Tokyo")
	assert.Equal(t, ipInfo.IsPrivate, false)
	assert.Equal(t, ipInfo.IsInEuropeanUnion, false)
}

func TestService_GetIPLocation_LangZh(t *testing.T) {
	ipInfo, err := service.GetIPLocation("183.11.242.230", "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, ipInfo.CountryCode, "CN")
	assert.Equal(t, ipInfo.CountryName, "中国")
	assert.Equal(t, ipInfo.RegionCode, "GD")
	assert.Equal(t, ipInfo.RegionName, "广东")
	assert.Equal(t, ipInfo.CityName, "深圳")
	assert.Equal(t, ipInfo.TimeZone, "Asia/Shanghai")
	assert.Equal(t, ipInfo.IsPrivate, false)
}

func TestService_GetIPLocation_PrivateIP(t *testing.T) {
	ipInfo, err := service.GetIPLocation("192.168.1.1", "en")
	// 私有 IP 在 GeoIP 库中没有地理信息，但 IsPrivate 应为 true
	if err == nil {
		assert.Equal(t, ipInfo.IsPrivate, true)
	}
}

func TestService_GetIPLocationInLocalDB_InvalidIP(t *testing.T) {
	_, err := service.GetIPLocationInLocalDB("54.248.162.57, 10.0.0.1")
	assert.Equal(t, errors.Is(err, ErrInvalidIP), true)
}
