package doctorhospital

import (
	"net/http"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func (ctl *Controller) DeleteNotification(c *gin.Context) {
	err := ctl.service.DeleteNotification(c.Request.Context(), util.GetUserID(c), c.Param("notification_id"))
	respond(c, constant.MsgNotificationDeleted, http.StatusOK, gin.H{"deleted": true}, err)
}
