package main

import (
	"fmt"
	"github.com/gin-gonic/contrib/ginrus"
	"github.com/gin-gonic/contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"
	"ip2loc/app/conf"
	"ip2loc/app/handlers"
	"ip2loc/app/utils"
	"net"
	"net/http"
	"time"
)

var (
	config *conf.Config
)

func init() {
	config = conf.InitConfig()
	// 初始化定时任务
	initSchedules()
}
func main() {
	router := getRouter()
	// Listen on IPv4 address
	listener, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", config.Http.Address, config.Http.Port))
	if err != nil {
		panic(err)
	}
	server := &http.Server{
		Handler: router,
	}
	utils.UpdateDBFile()
	err = server.Serve(listener)
	if err != nil {
		panic(err)
	}
	//_ = router.Run(fmt.Sprintf("%s:%d", config.Http.Address, config.Http.Port))
}

func getRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()
	router := gin.New()
	dateFormat := config.General.GetStringDefault("date-format", time.DateTime)
	router.Use(ginrus.Ginrus(logrus.StandardLogger(), dateFormat, false))
	router.Use(gin.Recovery())
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	hand := handlers.New(config)
	router.GET("/ip2loc", func(c *gin.Context) {
		hand.Ip2Location(c)
	})

	router.GET("/", func(c *gin.Context) {
		hand.PublicIP(c)
	})
	return router
}

func initSchedules() {
	c := cron.New()
	downloadCron := config.General.GetStringDefault("download-cron", "0 0 8 0/2 * *")
	logrus.Infof("init schedules: download file, cron: %s ", downloadCron)
	err := c.AddFunc(downloadCron, func() {
		utils.UpdateDBFile()
	})
	if err != nil {
		logrus.Errorf("init schedules error: %s", err)
		return
	}
	c.Start()
}
