package main

import (
	"fmt"
	"github.com/gin-gonic/contrib/ginrus"
	"github.com/gin-gonic/contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"ip2loc/app/conf"
	"ip2loc/app/handlers"
	"net"
	"net/http"
	"time"
)

var (
	config *conf.Config
)

func init() {
	config = conf.InitConfig()
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
	err = server.Serve(listener)
	if err != nil {
		panic(err)
	}
	//_ = router.Run(fmt.Sprintf("%s:%d", config.Http.Address, config.Http.Port))
}

func getRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
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
