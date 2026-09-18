package hospital

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	"github.com/Cendana-Project/medikaone-api/internal/util"
	"github.com/gin-gonic/gin"
)

func (ctl *Controller) ListImages(c *gin.Context) {
	rows, err := ctl.svc.ListImages(c.Request.Context(), c.Param("hospital_id"))
	hospitalRespond(c, constant.MsgHospitalImagesListed, rows, err)
}
func (ctl *Controller) UploadImage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10*1024*1024+64*1024)
	if err := c.Request.ParseMultipartForm(10 * 1024 * 1024); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			util.HandleError(c, constant.ErrRequestTooLarge)
		} else {
			util.HandleError(c, constant.ErrHospitalImageInvalid)
		}
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		util.HandleError(c, constant.ErrHospitalImageInvalid)
		return
	}
	defer file.Close()
	if header.Size > 10*1024*1024 {
		util.HandleError(c, constant.ErrHospitalImageInvalid)
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, 10*1024*1024+1))
	if err != nil || len(content) > 10*1024*1024 {
		util.HandleError(c, constant.ErrHospitalImageInvalid)
		return
	}
	sortOrder, err := strconv.Atoi(c.DefaultPostForm("sort_order", "0"))
	if err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("sort_order", "integer 0-1000", "angka 0-1000"))
		return
	}
	isCover, err := strconv.ParseBool(c.DefaultPostForm("is_cover", "false"))
	if err != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("is_cover", "boolean", "boolean"))
		return
	}
	row, err := ctl.svc.UploadImage(c.Request.Context(), hospitalHint(c), util.GetUserID(c), content, c.PostForm("caption"), sortOrder, isCover)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result := constant.NewSuccessResponse(constant.MsgHospitalImageUploaded)
	result.StatusCode = http.StatusCreated
	result.Data = row
	util.HandleResponse(c, result, nil)
}
func (ctl *Controller) UpdateImage(c *gin.Context) {
	var req request.UpdateHospitalImageRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	row, err := ctl.svc.UpdateImage(c.Request.Context(), hospitalHint(c), c.Param("image_id"), req)
	hospitalRespond(c, constant.MsgHospitalImageUpdated, row, err)
}
func (ctl *Controller) DeleteImage(c *gin.Context) {
	err := ctl.svc.DeleteImage(c.Request.Context(), hospitalHint(c), c.Param("image_id"))
	hospitalRespond(c, constant.MsgHospitalImageDeleted, gin.H{"deleted": true}, err)
}
func (ctl *Controller) ListReviews(c *gin.Context) {
	limit, e1 := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, e2 := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if e1 != nil || e2 != nil {
		util.HandleError(c, constant.NewInvalidFieldValueError("pagination", "integer limit/offset", "limit/offset angka bulat"))
		return
	}
	rows, err := ctl.svc.ListReviews(c.Request.Context(), c.Param("hospital_id"), limit, offset, c.Query("sort"))
	hospitalRespond(c, constant.MsgHospitalReviewsListed, rows, err)
}
func (ctl *Controller) GetOwnReview(c *gin.Context) {
	row, err := ctl.svc.GetOwnReview(c.Request.Context(), c.Param("hospital_id"), util.GetUserID(c))
	hospitalRespond(c, constant.MsgHospitalReviewRetrieved, row, err)
}
func (ctl *Controller) PutReview(c *gin.Context) {
	var req request.PutHospitalReviewRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	row, err := ctl.svc.PutReview(c.Request.Context(), c.Param("hospital_id"), util.GetUserID(c), req)
	hospitalRespond(c, constant.MsgHospitalReviewSaved, row, err)
}
func (ctl *Controller) DeleteOwnReview(c *gin.Context) {
	err := ctl.svc.DeleteOwnReview(c.Request.Context(), c.Param("hospital_id"), util.GetUserID(c))
	hospitalRespond(c, constant.MsgHospitalReviewDeleted, gin.H{"deleted": true}, err)
}
