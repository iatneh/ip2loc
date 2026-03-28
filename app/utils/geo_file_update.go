package utils

import (
	"crypto/md5"
	"fmt"
	"io"
	"ip2loc/app/conf"
	"ip2loc/app/services"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

var updateDBLock = make(chan struct{}, 1)

func init() {
	updateDBLock <- struct{}{}
}

func UpdateDBFile() {
	select {
	case <-updateDBLock:
		defer func() { updateDBLock <- struct{}{} }()
	default:
		logrus.Info("db file update skipped: previous update still running")
		return
	}

	logrus.Info("db file update,begin")
	cityFileUrl := os.Getenv("CITY_FILE_URL")
	if len(cityFileUrl) == 0 {
		cityFileUrl = conf.GetConfig().General.GetStringDefault("file-city-url", "")
	}
	logrus.Infof("db file update url:%s", cityFileUrl)
	dbDir := conf.GetConfig().General.GetStringDefault("db-path", "")
	if len(cityFileUrl) == 0 {
		logrus.Info("db file update, download url is not config")
		return
	}
	cityFileName := filepath.Base(cityFileUrl)
	targetPath := filepath.Join(dbDir, cityFileName)
	targetDir := filepath.Dir(targetPath)

	client := resty.New()
	client.SetTimeout(60 * time.Second)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		logrus.Errorf("db file update, create target dir [%s],error: %s", targetDir, err.Error())
		return
	}

	tmpFile, err := os.CreateTemp(targetDir, cityFileName+".*.tmp")
	if err != nil {
		logrus.Errorf("db file update, create temp file error: %s", err.Error())
		return
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(tmpPath)
	defer func() { _ = os.Remove(tmpPath) }()

	resp, err := client.R().SetOutput(tmpPath).Get(cityFileUrl)
	if err != nil {
		logrus.Errorf("db file update, download city file [%s],error: %s", cityFileUrl, err.Error())
		return
	}
	if resp.StatusCode() != http.StatusOK {
		logrus.Errorf("db file update, download city file [%s],status code: [%d]", cityFileUrl, resp.StatusCode())
		return
	}
	logrus.Info("db file update, download db file success")

	oldFileMD5Sum, err := getMD5SumString(targetPath)
	if err != nil {
		logrus.Errorf("db file update, calc old file md5 sum error: %s", err)
		return
	}
	newFileMD5Sum, err := getMD5SumString(tmpPath)
	if err != nil {
		logrus.Errorf("db file update, calc new file md5 sum error: %s", err)
		return
	}
	if oldFileMD5Sum == newFileMD5Sum {
		logrus.Info("db file update, not modify,delete temp file")
		return
	}

	backupPath := targetPath + ".bak"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Rename(targetPath, backupPath); err != nil {
			logrus.Errorf("db file update, backup db file error: %s", err.Error())
			return
		}
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		if _, statErr := os.Stat(backupPath); statErr == nil {
			_ = os.Rename(backupPath, targetPath)
		}
		logrus.Errorf("db file update, replace db file error: %s", err.Error())
		return
	}
	_ = os.Remove(backupPath)

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
