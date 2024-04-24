package services

import (
	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
	"ip2loc/app/conf"
	"ip2loc/app/models"
	"net"
)

var (
	cityDBConnection *geoip2.Reader         // 城市地址库
	cityDBFile       = "GeoLite2-City.mmdb" // 城市地址库文件

	// 需要获取的信息语言
	infoLang = []string{"en"}
)

// initConnection 初始化DB链接
func initConnection(config *conf.Config) {
	dbPath, err := config.General.GetString("db-path")
	if err != nil {
		panic(err.Error())
	}
	if cityDBConnection == nil {
		cityDBConnection, err = geoip2.Open(dbPath + cityDBFile)
		if err != nil {
			panic(err.Error())
		}
	}
}

// RestConnection 初始化DB链接
func RestConnection(config *conf.Config) {
	if cityDBConnection != nil {
		err := cityDBConnection.Close()
		if err != nil {
			logrus.Errorf("db connection close error: %s", err)
			return
		}
		cityDBConnection = nil
		initConnection(config)
	}
}

// GetIPLocationInLocalDB 从本地DB文件读取ip信息
func (s *Service) GetIPLocationInLocalDB(inIp string) (*models.IpInfo, error) {
	initConnection(s.conf)
	ip := net.ParseIP(inIp)
	city, err := cityDBConnection.City(ip)
	if err != nil {
		return nil, err
	}

	// 返回的记录包含多种语言，不需要那么多，只取特定 key 的name
	getNames := func(names map[string]string, keys []string) map[string]string {
		m := make(map[string]string)
		for idx := range keys {
			if v, ok := names[keys[idx]]; ok {
				m[keys[idx]] = v
			}
		}
		return m
	}
	var regionCode string
	if len(city.Subdivisions) > 0 {
		regionCode = city.Subdivisions[0].IsoCode
	}
	var ipInfoDto = models.IpInfo{
		Ip:          inIp,
		Longitude:   city.Location.Longitude,
		Latitude:    city.Location.Latitude,
		CountryName: getNames(city.Country.Names, infoLang)[infoLang[0]],
		CountryCode: city.Country.IsoCode,
		RegionName:  getNames(city.City.Names, infoLang)[infoLang[0]],
		RegionCode:  regionCode,
		CityName:    getNames(city.City.Names, infoLang)[infoLang[0]],
	}

	return &ipInfoDto, nil
}
