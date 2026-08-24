package services

import (
	"errors"
	"ip2loc/app/conf"
	"ip2loc/app/models"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
)

var (
	cityDBConnection *geoip2.Reader         // 城市地址库
	cityDBFile       = "GeoLite2-City.mmdb" // 城市地址库文件
	asnDBConnection  *geoip2.Reader         // ASN 地址库
	asnDBFile        = "GeoLite2-ASN.mmdb"  // ASN 地址库文件
	dbMu             sync.RWMutex
)

var (
	ErrEmptyIP   = errors.New("ip不能为空")
	ErrInvalidIP = errors.New("ip格式不正确")
)

// initConnection 初始化DB链接
func initConnection(config *conf.Config) {
	dbMu.Lock()
	defer dbMu.Unlock()

	dbPath := config.General.GetStringDefault("db-path", "db/")

	// 初始化 City DB
	if cityDBConnection == nil {
		cityFilePath := filepath.Join(dbPath, cityDBFile)
		if _, err := os.Stat(cityFilePath); err == nil {
			reader, err := geoip2.Open(cityFilePath)
			if err != nil {
				logrus.Errorf("open city db [%s] error: %s", cityFilePath, err.Error())
			} else {
				cityDBConnection = reader
			}
		} else {
			logrus.Debugf("city db file [%s] does not exist yet", cityFilePath)
		}
	}

	// 初始化 ASN DB（可选，存在即加载）
	if asnDBConnection == nil {
		asnFilePath := filepath.Join(dbPath, asnDBFile)
		if _, err := os.Stat(asnFilePath); err == nil {
			reader, err := geoip2.Open(asnFilePath)
			if err != nil {
				logrus.Warnf("open asn db [%s] error: %s", asnFilePath, err.Error())
			} else {
				asnDBConnection = reader
			}
		} else {
			logrus.Debugf("asn db file [%s] does not exist yet", asnFilePath)
		}
	}
}

// ResetConnection 重置并重新打开DB链接
func ResetConnection(config *conf.Config) {
	dbMu.Lock()
	defer dbMu.Unlock()

	if cityDBConnection != nil {
		_ = cityDBConnection.Close()
		cityDBConnection = nil
	}
	if asnDBConnection != nil {
		_ = asnDBConnection.Close()
		asnDBConnection = nil
	}

	dbPath := config.General.GetStringDefault("db-path", "db/")
	cityFilePath := filepath.Join(dbPath, cityDBFile)
	if _, err := os.Stat(cityFilePath); err == nil {
		if reader, err := geoip2.Open(cityFilePath); err == nil {
			cityDBConnection = reader
		} else {
			logrus.Errorf("reopen city db error: %s", err.Error())
		}
	}

	asnFilePath := filepath.Join(dbPath, asnDBFile)
	if _, err := os.Stat(asnFilePath); err == nil {
		if reader, err := geoip2.Open(asnFilePath); err == nil {
			asnDBConnection = reader
		} else {
			logrus.Warnf("reopen asn db error: %s", err.Error())
		}
	}
}

// RestConnection 兼容旧接口拼写
func RestConnection(config *conf.Config) {
	ResetConnection(config)
}

// getLocalizedName 获取指定语言的名称，未找到时退避到 en -> zh-CN -> 第一个非空名称
func getLocalizedName(names map[string]string, lang string) string {
	if len(names) == 0 {
		return ""
	}
	if lang != "" {
		if v, ok := names[lang]; ok && v != "" {
			return v
		}
		if parts := strings.Split(lang, "-"); len(parts) > 1 {
			if v, ok := names[parts[0]]; ok && v != "" {
				return v
			}
		}
	}
	if v, ok := names["en"]; ok && v != "" {
		return v
	}
	if v, ok := names["zh-CN"]; ok && v != "" {
		return v
	}
	for _, v := range names {
		if v != "" {
			return v
		}
	}
	return ""
}

// GetIPLocation 查询 IP 定位信息与网络特征，支持多语言（lang: "zh-CN", "en" 等）
func (s *Service) GetIPLocation(inIp string, lang string) (*models.IpInfo, error) {
	initConnection(s.conf)
	inIp = strings.TrimSpace(inIp)
	if inIp == "" {
		return nil, ErrEmptyIP
	}
	ip := net.ParseIP(inIp)
	if ip == nil {
		return nil, ErrInvalidIP
	}

	isPrivate := ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()

	dbMu.RLock()
	cityReader := cityDBConnection
	asnReader := asnDBConnection
	dbMu.RUnlock()

	if cityReader == nil {
		return nil, errors.New("city db连接未初始化")
	}

	city, err := cityReader.City(ip)
	if err != nil {
		return nil, err
	}

	var regionCode, regionName string
	if len(city.Subdivisions) > 0 {
		regionCode = city.Subdivisions[0].IsoCode
		regionName = getLocalizedName(city.Subdivisions[0].Names, lang)
	}

	ipInfoDto := models.IpInfo{
		Ip:                inIp,
		Latitude:          city.Location.Latitude,
		Longitude:         city.Location.Longitude,
		ContinentCode:     city.Continent.Code,
		ContinentName:     getLocalizedName(city.Continent.Names, lang),
		CountryCode:       city.Country.IsoCode,
		CountryName:       getLocalizedName(city.Country.Names, lang),
		RegionCode:        regionCode,
		RegionName:        regionName,
		CityName:          getLocalizedName(city.City.Names, lang),
		PostalCode:        city.Postal.Code,
		TimeZone:          city.Location.TimeZone,
		IsInEuropeanUnion: city.Country.IsInEuropeanUnion,
		AccuracyRadius:    city.Location.AccuracyRadius,
		IsPrivate:         isPrivate,
	}

	// 如果加载了 ASN 数据库，补充 ASN 与组织信息
	if asnReader != nil {
		if asnRecord, err := asnReader.ASN(ip); err == nil && asnRecord != nil {
			ipInfoDto.Asn = asnRecord.AutonomousSystemNumber
			ipInfoDto.AsOrg = asnRecord.AutonomousSystemOrganization
		}
	}

	return &ipInfoDto, nil
}

// GetIPLocationInLocalDB 从本地DB文件读取ip信息（默认英文，保持向后兼容）
func (s *Service) GetIPLocationInLocalDB(inIp string) (*models.IpInfo, error) {
	return s.GetIPLocation(inIp, "en")
}
