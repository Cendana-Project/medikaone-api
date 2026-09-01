package warmup

import (
	"net/http"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

type Controller struct{}

func NewController() *Controller {
	return &Controller{}
}

func (c *Controller) Ping(ctx *gin.Context) {
	resp := response.BaseResponse{
		StatusCode: http.StatusOK,
		Message:    "pong",
		MessageDetail: response.MessageDetail{
			TitleEng: "PONG",
			DescEng:  "Service is alive and ready",
			TitleIdn: "PONG",
			DescIdn:  "Layanan aktif dan siap",
		},
		Data: map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"status":    "ok",
		},
	}
	util.HandleResponse(ctx, &resp, nil)
}
