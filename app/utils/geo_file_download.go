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

func DownloadFile() {
	logrus.Info("begin download")
	cityFileUrl := conf.GetConfig().General.GetStringDefault("file-city-url", "")
	dbPath := conf.GetConfig().General.GetStringDefault("db-path", "")
	if len(cityFileUrl) == 0 {
		logrus.Info("download url is not config")
	}
	cityFileName := path.Base(cityFileUrl)
	dbPathTemp := dbPath + "/temp/" + cityFileName
	dbPath = dbPath + cityFileName

	client := resty.New()
	// 文件先下载到临时文件夹
	resp, err := client.R().SetOutput(dbPathTemp).Get(cityFileUrl)
	if err != nil || resp.StatusCode() != http.StatusOK {
		logrus.Errorf("download city file [%s],status code: [%d],error: %s", cityFileUrl, resp.StatusCode(), err)
		return
	}
	// 对比两个文件大小一致则返回
	oldFileMD5Sum, err := getMD5SumString(dbPath)
	if err != nil {
		logrus.Errorf("calc old file md5 sum error: %s", err)
		return
	}
	newFileMD5Sum, err := getMD5SumString(dbPathTemp)
	if err != nil {
		logrus.Errorf("calc new file md5 sum error: %s", err)
		return
	}
	if oldFileMD5Sum == newFileMD5Sum {
		logrus.Info("file not modify")
		return
	}
	err = os.Rename(dbPathTemp, dbPath)
	if err != nil {
		logrus.Errorf("move db file error: %s", err)
		return
	}

	services.RestConnection(conf.GetConfig())
}
func getMD5SumString(filePath string) (string, error) {
	oldFile, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	file1Sum := md5.New()
	_, err = io.Copy(file1Sum, oldFile)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%X", file1Sum.Sum(nil)), nil
}
