package warmup

import (
	"net/http"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

type Controller struct{}

func NewController() *Controller {
	return &Controller{}
}

func (c *Controller) Ping(ctx *gin.Context) {
	resp := constant.NewSuccessResponse(constant.MsgServiceHealthy)
	resp.StatusCode = http.StatusOK
	resp.Data = map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"status":    "ok",
	}
	util.HandleResponse(ctx, resp, nil)
}
