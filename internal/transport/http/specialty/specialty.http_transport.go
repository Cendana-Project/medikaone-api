package specialty

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

type Service interface {
	ListSpecialties(context.Context, request.SpecialtyQuery) ([]response.Specialty, error)
}

type Controller struct {
	svc Service
}

func NewController(svc Service) *Controller { return &Controller{svc: svc} }

func (ctl *Controller) ListSpecialties(c *gin.Context) {
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("query", "valid query parameters", "parameter kueri valid"))
		return
	}
	q := request.SpecialtyQuery{Limit: 100}
	for _, key := range []string{"q", "code", "category", "limit", "offset"} {
		items, provided := values[key]
		if !provided {
			continue
		}
		if len(items) != 1 || strings.TrimSpace(items[0]) == "" {
			util.HandleError(c, constant.NewInvalidFieldValueError(key, "one nonblank value", "satu nilai yang tidak kosong"))
			return
		}
		value := items[0]
		switch key {
		case "q":
			q.Search = value
		case "code":
			q.Code = value
		case "category":
			q.Category = value
		case "limit", "offset":
			n, parseErr := strconv.Atoi(value)
			if parseErr != nil || (key == "limit" && n < 1) || (key == "offset" && n < 0) {
				util.HandleError(c, constant.NewInvalidFieldValueError(key, "valid pagination integer", "angka bulat paginasi valid"))
				return
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		}
	}
	rows, err := ctl.svc.ListSpecialties(c.Request.Context(), q)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result := constant.NewSuccessResponse(constant.MsgSpecialtiesListed)
	result.Data = rows
	util.HandleResponse(c, result, nil)
}
