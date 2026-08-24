package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"ip2loc/app/services"
	"ip2loc/app/utils"
	"net/http"
	"strings"
)

func (h *Handler) Ip2Location(c *gin.Context) {
	rawIP := strings.TrimSpace(c.Query("ip"))
	lang := strings.TrimSpace(c.Query("lang"))
	var ip string
	if rawIP != "" {
		ip = utils.NormalizeIP(rawIP)
		if ip == "" {
			h.ErrorJSONWithHttpCode(http.StatusBadRequest, c, []error{services.ErrInvalidIP})
			return
		}
	} else {
		ip = utils.GetClientIP(c.Request)
		if ip == "" {
			h.ErrorJSONWithHttpCode(http.StatusBadRequest, c, []error{services.ErrEmptyIP})
			return
		}
	}
	result, err := h.service.GetIPLocation(ip, lang)
	if err == nil {
		h.SuccessJSON(c, result)
		return
	}
	if errors.Is(err, services.ErrEmptyIP) || errors.Is(err, services.ErrInvalidIP) {
		h.ErrorJSONWithHttpCode(http.StatusBadRequest, c, []error{err})
		return
	}
	h.ErrorJSONWithHttpCode(http.StatusInternalServerError, c, []error{err})
}

func (h *Handler) PublicIP(c *gin.Context) {
	h.Success(c, utils.GetClientIP(c.Request))
}
