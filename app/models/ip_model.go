package models

// IpInfo IP 定位与网络特征信息
type IpInfo struct {
	Ip                string  `json:"ip"`
	Latitude          float64 `json:"latitude"`
	Longitude         float64 `json:"longitude"`
	ContinentCode     string  `json:"continentCode,omitempty"`
	ContinentName     string  `json:"continentName,omitempty"`
	CountryCode       string  `json:"countryCode"`
	CountryName       string  `json:"countryName"`
	RegionCode        string  `json:"regionCode,omitempty"`
	RegionName        string  `json:"regionName,omitempty"`
	CityName          string  `json:"cityName,omitempty"`
	PostalCode        string  `json:"postalCode,omitempty"`
	TimeZone          string  `json:"timeZone,omitempty"`
	IsInEuropeanUnion bool    `json:"isInEuropeanUnion"`
	AccuracyRadius    uint16  `json:"accuracyRadius,omitempty"`
	Asn               uint    `json:"asn,omitempty"`
	AsOrg             string  `json:"asOrg,omitempty"`
	IsPrivate         bool    `json:"isPrivate"`
}
