package models

// IpInfo 短链信息
type IpInfo struct {
	Ip          string  `json:"ip"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	CountryName string  `json:"countryName"`
	CountryCode string  `json:"countryCode"`
	CityName    string  `json:"cityName"`
	RegionName  string  `json:"regionName"`
	RegionCode  string  `json:"regionCode"`
}
