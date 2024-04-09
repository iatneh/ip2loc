package main

import (
	"fmt"
	"github.com/gin-gonic/contrib/ginrus"
	"github.com/gin-gonic/contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"ip2loc/app/conf"
	"ip2loc/app/handlers"
	"time"
)

var (
	config *conf.Config
)

func init() {
	config = conf.AppConf
}
func main() {
	router := getRouter()
	_ = router.Run(fmt.Sprintf("%s:%d", conf.AppConf.Http.Address, conf.AppConf.Http.Port))
}

func getRouter() *gin.Engine {
	router := gin.New()
	dateFormat := time.DateTime
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
