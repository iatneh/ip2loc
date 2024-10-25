package utils

import (
	"crypto/md5"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
	"io"
	"ip2loc/app/conf"
	"ip2loc/app/services"
	"net/http"
	"os"
	"path"
)

func UpdateDBFile() {
	logrus.Info("db file update,begin")
	cityFileUrl := os.Getenv("CITY_FILE_URL")
	if len(cityFileUrl) == 0 {
		cityFileUrl = conf.GetConfig().General.GetStringDefault("file-city-url", "")
	}
	logrus.Infof("db file update url:%s", cityFileUrl)
	dbPath := conf.GetConfig().General.GetStringDefault("db-path", "")
	if len(cityFileUrl) == 0 {
		logrus.Info("db file update, download url is not config")
		return
	}
	cityFileName := path.Base(cityFileUrl)
	dbPathTemp := "temp/" + cityFileName
	dbPath = dbPath + cityFileName

	client := resty.New()
	// 文件先下载到临时文件夹
	resp, err := client.R().SetOutput(dbPathTemp).Get(cityFileUrl)
	if err != nil || resp.StatusCode() != http.StatusOK {
		logrus.Errorf("db file update, download city file [%s],status code: [%d],error: %s", cityFileUrl, resp.StatusCode(), err)
		return
	}
	logrus.Info("db file update, download db file success")
	// 对比两个文件大小一致则返回
	oldFileMD5Sum, err := getMD5SumString(dbPath)
	if err != nil {
		logrus.Errorf("db file update, calc old file md5 sum error: %s", err)
		return
	}
	newFileMD5Sum, err := getMD5SumString(dbPathTemp)
	if err != nil {
		logrus.Errorf("db file update, calc new file md5 sum error: %s", err)
		return
	}
	if oldFileMD5Sum == newFileMD5Sum {
		err := os.Remove(dbPathTemp)
		if err != nil {
			logrus.Errorf("db file update, delete temp file error: %s", err)
		}
		logrus.Info("db file update, not modify,delete temp file")
		return
	}
	err = os.Rename(dbPathTemp, dbPath)
	if err != nil {
		logrus.Errorf("db file update, move db file error: %s", err)
		return
	}

	logrus.Info("db file update, reset connection")
	services.RestConnection(conf.GetConfig())

	logrus.Info("db file update, end")
}
func getMD5SumString(filePath string) (string, error) {
	f, err := os.Open(filePath)
	defer func(f *os.File) {
		if f == nil {
			return
		}
		err := f.Close()
		if err != nil {
			logrus.Errorf("close file error: %s", err)
		}
	}(f)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	file1Sum := md5.New()
	_, err = io.Copy(file1Sum, f)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%X", file1Sum.Sum(nil)), nil
}
