package services

import (
	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
	"ip2loc/app/models"
	"net"
)

var (
	countryDBConnection *geoip2.Reader            // 国家地址库
	countryDBFile       = "GeoLite2-Country.mmdb" // 国家地址库文件

	cityDBConnection *geoip2.Reader         // 城市地址库
	cityDBFile       = "GeoLite2-City.mmdb" // 城市地址库文件
	// 需要获取的信息语言
	infoLang = []string{"en"}
)

func initConnection(dbPath string) {
	if countryDBConnection != nil {
		err := countryDBConnection.Close()
		if err != nil {
			logrus.Errorf("close country db connection error: %v", err)
		}
	}
	if cityDBConnection != nil {
		err := countryDBConnection.Close()
		if err != nil {
			logrus.Errorf("close city db connection error: %v", err)
		}
	}
	var err error
	countryDBConnection, err = geoip2.Open(dbPath + countryDBFile)
	if err != nil {
		panic(err.Error())
	}
	cityDBConnection, err = geoip2.Open(dbPath + cityDBFile)
	if err != nil {
		panic(err.Error())
	}
}

// GetIPLocationInLocalDB 生成一个短链接
func (s *Service) GetIPLocationInLocalDB(inIp string) (*models.IpInfo, error) {
	dbPath, err := s.conf.General.GetString("db-path")
	if err != nil {
		panic(err.Error())
	}
	initConnection(dbPath)
	ip := net.ParseIP(inIp)
	city, err := cityDBConnection.City(ip)
	if err != nil {
		return nil, err
	}
	country, err := countryDBConnection.Country(ip)
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
		CountryName: getNames(country.Country.Names, infoLang)[infoLang[0]],
		CountryCode: country.Country.IsoCode,
		RegionName:  getNames(city.City.Names, infoLang)[infoLang[0]],
		RegionCode:  regionCode,
		CityName:    getNames(city.City.Names, infoLang)[infoLang[0]],
	}

	return &ipInfoDto, nil
}
