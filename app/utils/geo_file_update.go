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
	"strings"
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

	logrus.Info("db file update, begin")
	dbDir := conf.GetConfig().General.GetStringDefault("db-path", "db/")

	cityFileUrl := os.Getenv("CITY_FILE_URL")
	if len(cityFileUrl) == 0 {
		cityFileUrl = conf.GetConfig().General.GetStringDefault("file-city-url", "")
	}
	cityUpdated := false
	if len(cityFileUrl) > 0 {
		cityUpdated = downloadAndReplace(cityFileUrl, dbDir, "GeoLite2-City.mmdb")
	}

	asnFileUrl := os.Getenv("ASN_FILE_URL")
	if len(asnFileUrl) == 0 {
		asnFileUrl = conf.GetConfig().General.GetStringDefault("file-asn-url", "https://git.io/GeoLite2-ASN.mmdb")
	}
	asnUpdated := false
	if len(asnFileUrl) > 0 {
		asnUpdated = downloadAndReplace(asnFileUrl, dbDir, "GeoLite2-ASN.mmdb")
	}

	if cityUpdated || asnUpdated {
		logrus.Info("db file update, reset connection")
		services.ResetConnection(conf.GetConfig())
	}

	logrus.Info("db file update, end")
}

func downloadAndReplace(fileUrl string, dbDir string, defaultFileName string) bool {
	fileName := filepath.Base(fileUrl)
	if !strings.HasSuffix(strings.ToLower(fileName), ".mmdb") && defaultFileName != "" {
		fileName = defaultFileName
	}
	targetPath := filepath.Join(dbDir, fileName)
	targetDir := filepath.Dir(targetPath)

	client := resty.New()
	client.SetTimeout(120 * time.Second)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		logrus.Errorf("db file update, create target dir [%s], error: %s", targetDir, err.Error())
		return false
	}

	tmpFile, err := os.CreateTemp(targetDir, fileName+".*.tmp")
	if err != nil {
		logrus.Errorf("db file update, create temp file error: %s", err.Error())
		return false
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(tmpPath)
	defer func() { _ = os.Remove(tmpPath) }()

	logrus.Infof("db file update downloading [%s] from %s", fileName, fileUrl)
	resp, err := client.R().SetOutput(tmpPath).Get(fileUrl)
	if err != nil {
		logrus.Errorf("db file update, download file [%s] error: %s", fileUrl, err.Error())
		return false
	}
	if resp.StatusCode() != http.StatusOK {
		logrus.Errorf("db file update, download file [%s] status code: [%d]", fileUrl, resp.StatusCode())
		return false
	}

	oldFileMD5Sum, err := getMD5SumString(targetPath)
	if err != nil && !os.IsNotExist(err) {
		logrus.Errorf("db file update, calc old file md5 sum error: %s", err)
		return false
	}
	newFileMD5Sum, err := getMD5SumString(tmpPath)
	if err != nil {
		logrus.Errorf("db file update, calc new file md5 sum error: %s", err)
		return false
	}
	if oldFileMD5Sum == newFileMD5Sum && oldFileMD5Sum != "" {
		logrus.Infof("db file [%s] not modified, skip replace", fileName)
		return false
	}

	backupPath := targetPath + ".bak"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Rename(targetPath, backupPath); err != nil {
			logrus.Errorf("db file update, backup db file error: %s", err.Error())
			return false
		}
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		if _, statErr := os.Stat(backupPath); statErr == nil {
			_ = os.Rename(backupPath, targetPath)
		}
		logrus.Errorf("db file update, replace db file error: %s", err.Error())
		return false
	}
	_ = os.Remove(backupPath)
	logrus.Infof("db file [%s] updated successfully", fileName)
	return true
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
