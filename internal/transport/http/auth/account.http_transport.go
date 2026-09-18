package auth

import (
	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func (ctl *Controller) DeleteAccount(c *gin.Context) {
	var req request.DeleteAccountRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	if err := ctl.svc.DeleteAccount(c.Request.Context(), util.GetUserID(c), req); err != nil {
		util.HandleError(c, err)
		return
	}
	result := constant.NewSuccessResponse(constant.MsgAccountDeleted)
	result.Data = gin.H{"deleted": true}
	util.HandleResponse(c, result, nil)
}
