package conf

import (
	"fmt"
	"github.com/spf13/viper"
)

type Config struct {
	Http    *HttpConfig
	Logger  *LogConfig
	General *GeneralConfig
}
type HttpConfig struct {
	Address string
	Port    int
}

// LogConfig 日志配置
type LogConfig struct {
	LogLevel string // 配置日志输出级别: trace,debug,info,warn,error
}

var (
	AppConf *Config
)

func init() {
	v := viper.New()
	v.SetConfigName("app")
	v.AddConfigPath(".")
	v.AddConfigPath("./etc")
	v.AddConfigPath("./conf")
	v.AddConfigPath("./config")
	err := v.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal errors config file: %s \n", err))
	}
	if err := v.Unmarshal(&AppConf); err != nil {
		panic(fmt.Errorf("Fatal errors config file: %s \n", err))
	}
	if len(v.GetStringMap("general")) > 0 {
		AppConf.General.PutAll(v.GetStringMap("general"))
	}
}
