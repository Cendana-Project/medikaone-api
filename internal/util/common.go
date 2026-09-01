package util

import (
	"errors"
	"net/http"
	"time"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GetUserIDFromContext(ctx *gin.Context) (*uuid.UUID, error) {
	userIDVal, exists := ctx.Get(string(constant.UserID))
	if !exists {
		return nil, errors.New("user ID not found in context")
	}

	userIDStr, ok := userIDVal.(string)
	if !ok {
		return nil, errors.New("user ID in context is not a string")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, err
	}
	return &userID, nil
}

func GetTraceID(ctx *gin.Context) string {
	// 1) ambil dari context key yang dipakai middleware TraceID
	if v, ok := ctx.Get("trace_id"); ok {
		if s, ok2 := v.(string); ok2 && s != "" {
			return s
		}
	}

	// 2) header yang diset middleware (kedua varian; header case-insensitive)
	if rid := ctx.GetHeader("X-Request-Id"); rid != "" { // varian "Id"
		return rid
	}
	if rid := ctx.GetHeader("X-Request-ID"); rid != "" { // varian "ID"
		return rid
	}

	// 3) fallback: key-key lama jika ada
	if ridCtx := ctx.GetString("request_id"); ridCtx != "" {
		return ridCtx
	}
	if v, ok := ctx.Get(string(constant.RequestID)); ok {
		if s, ok2 := v.(string); ok2 && s != "" {
			return s
		}
	}
	return ""
}

func HandleResponse(ctx *gin.Context, resp *response.BaseResponse, err error) {
	if err != nil {
		HandleError(ctx, err)
		ctx.Abort()
		return
	}

	if resp == nil {
		resp = &response.BaseResponse{}
	}

	if resp.StatusCode == 0 {
		resp.StatusCode = http.StatusOK
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		resp.Message = response.MessageOK
		if resp.MessageDetail == (response.MessageDetail{}) {
			resp.MessageDetail = constant.GetMessageDetail(constant.MsgSuccess)
		}
	}

	resp.TraceID = GetTraceID(ctx)
	resp.Timestamp = time.Now().UTC()

	ctx.JSON(resp.StatusCode, resp)
}

func GetUserID(ctx *gin.Context) string {
	id, err := GetUserIDFromContext(ctx)
	if err != nil || id == nil {
		return ""
	}
	return id.String()
}
