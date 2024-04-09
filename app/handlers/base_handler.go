package handlers

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/en"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
	chTranslations "github.com/go-playground/validator/v10/translations/zh"
	"ip2loc/app/conf"
	"ip2loc/app/services"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *services.Service
}

var trans ut.Translator

// loca 通常取决于 http 请求头的 'Accept-Language'
func transInit(local string) (err error) {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		zhT := zh.New() //chinese
		enT := en.New() //english
		uni := ut.New(enT, zhT, enT)

		var o bool
		trans, o = uni.GetTranslator(local)
		if !o {
			return fmt.Errorf("uni.GetTranslator(%s) failed", local)
		}
		//register translate
		// 注册翻译器
		switch local {
		case "en":
			err = enTranslations.RegisterDefaultTranslations(v, trans)
		case "zh":
			err = chTranslations.RegisterDefaultTranslations(v, trans)
		default:
			err = enTranslations.RegisterDefaultTranslations(v, trans)
		}
		return
	}
	return
}
func init() {
	if err := transInit("zh"); err != nil {
		fmt.Printf("init trans failed, err:%v\n", err)
		return
	}
}
func New(config *conf.Config) *Handler {
	return &Handler{
		service: services.New(config),
	}
}

func (h *Handler) SuccessJSON(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    data,
	})
}
func (h *Handler) Success(c *gin.Context, data string) {
	c.String(http.StatusOK, data)
}

func (h *Handler) ErrorJSON(c *gin.Context, errs []error) {
	c.JSON(http.StatusOK, gin.H{
		"code": 9000,
		"data": nil,
		"message": func() string {
			var rs []string
			for _, err := range errs {
				transError(err, &rs)
			}
			return strings.Join(rs, ",")
		}(),
	})
}

func (h *Handler) ErrorJSONWithHttpCode(code int, c *gin.Context, errs []error) {
	c.JSON(code, gin.H{
		"code": 9000,
		"data": nil,
		"message": func() string {
			var rs []string
			for _, err := range errs {
				transError(err, &rs)
			}
			return strings.Join(rs, ",")
		}(),
	})
}
func (h *Handler) ErrorJSONWithResponseCode(code int, c *gin.Context, errs []error) {
	c.JSON(http.StatusOK, gin.H{
		"code": code,
		"data": nil,
		"message": func() string {
			var rs []string
			for _, err := range errs {
				transError(err, &rs)
			}
			return strings.Join(rs, ",")
		}(),
	})
}

func transError(err error, rs *[]string) {
	// 获取validator.ValidationErrors类型的errors
	var validationErr validator.ValidationErrors
	ok := errors.As(err, &validationErr)
	if !ok {
		*rs = append(*rs, err.Error())
	} else {
		for _, v := range validationErr.Translate(trans) {
			*rs = append(*rs, v)
		}
	}
}
