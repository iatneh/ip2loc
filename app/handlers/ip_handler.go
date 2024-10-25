package handlers

import (
	"github.com/gin-gonic/gin"
	"ip2loc/app/utils"
	"net/http"
)

func (h *Handler) Ip2Location(c *gin.Context) {
	ip := c.Query("ip")
	if len(ip) == 0 {
		ip = utils.GetClientIP(c.Request)
	}
	result, err := h.service.GetIPLocationInLocalDB(ip)
	if err == nil {
		h.SuccessJSON(c, result)
		return
	}
	h.ErrorJSONWithHttpCode(http.StatusInternalServerError, c, []error{err})
}

func (h *Handler) PublicIP(c *gin.Context) {
	h.Success(c, utils.GetClientIP(c.Request))
}
