package conf

import (
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	LogLevel     string // 配置日志输出级别: trace,debug,info,warn,error
	Output       string
	MaxAge       int // 日志保留天数
	RotationTime int // 日志分割时间,单位秒,默认86400秒
}

var (
	appConf *Config
)

func GetConfig() *Config {
	return appConf
}
func InitConfig() *Config {
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
	if err := v.Unmarshal(&appConf); err != nil {
		panic(fmt.Errorf("Fatal errors config file: %s \n", err))
	}
	if len(v.GetStringMap("general")) > 0 {
		appConf.General.PutAll(v.GetStringMap("general"))
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: appConf.General.GetStringDefault("date-format", time.DateTime),
		FullTimestamp:   true,
		ForceColors:     true,
		DisableQuote:    true,
		PadLevelText:    true,
	})
	if appConf.Logger != nil && len(appConf.Logger.LogLevel) == 0 {
		appConf.Logger.LogLevel = "debug"
	}
	ll, err := logrus.ParseLevel(appConf.Logger.LogLevel)
	if err != nil {
		ll = logrus.DebugLevel
	}
	logrus.SetLevel(ll)

	rl, err := newMultiWriter(appConf.Logger)
	if err != nil {
		panic(err.Error())
	}
	logrus.SetOutput(io.MultiWriter(rl...))
	return appConf
}

// NewMultiWriter 返回一个 多个writer,可以在 logrus 中使用
func newMultiWriter(logConfig *LogConfig) ([]io.Writer, error) {
	outputArray := strings.Split(logConfig.Output, ",")
	var writers []io.Writer
	for i := range outputArray {
		output := outputArray[i]
		switch output {
		case "stdout":
			writers = append(writers, os.Stdout)
		case "stderr":
			writers = append(writers, os.Stderr)
		default:
			if !strings.HasPrefix(output, `file://`) {
				continue
			}
			logPath := strings.ReplaceAll(output, `file://`, "")
			linkName := filepath.Join(filepath.Dir(logPath), "current")

			// 默认保留1年日志
			if logConfig.MaxAge == 0 {
				logConfig.MaxAge = 365
			}

			if logConfig.RotationTime <= 0 {
				logConfig.RotationTime = 86400
			}

			rl, err := rotatelogs.New(logPath,
				rotatelogs.WithMaxAge(24*time.Hour*time.Duration(logConfig.MaxAge)),
				rotatelogs.WithRotationTime(time.Second*time.Duration(logConfig.RotationTime)),
				rotatelogs.WithLinkName(linkName),
			)
			if err != nil {
				return nil, err
			}
			writers = append(writers, rl)
		}
	}
	return writers, nil
}
