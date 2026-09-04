package examination

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Cendana-Project/medikaone-api/internal/constant"
	"github.com/Cendana-Project/medikaone-api/internal/model/request"
	service "github.com/Cendana-Project/medikaone-api/internal/service/examination"
	"github.com/Cendana-Project/medikaone-api/internal/util"
)

const maxMedicalMultipartBytes = int64(10 * 1024 * 1024)

type Controller struct{ service *service.Service }

func NewController(service *service.Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) GetHospitalExamination(c *gin.Context) {
	result, err := ctl.service.GetHospitalExamination(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgExaminationRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) SaveVitalDraft(c *gin.Context) {
	var req request.VitalSignsRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.SaveVitalDraft(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgVitalsDraftSaved, http.StatusOK, result, err)
}

func (ctl *Controller) FinalizeVitals(c *gin.Context) {
	result, err := ctl.service.FinalizeVitals(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgVitalsFinalized, http.StatusOK, result, err)
}

func (ctl *Controller) CorrectVitals(c *gin.Context) {
	var req request.CorrectVitalSignsRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CorrectVitals(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgVitalsCorrected, http.StatusOK, result, err)
}

func (ctl *Controller) CorrectHospitalConsultation(c *gin.Context) {
	var req request.CorrectHospitalConsultationNoteRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CorrectHospitalConsultation(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgConsultationCorrected, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorExamination(c *gin.Context) {
	result, err := ctl.service.GetDoctorExamination(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgExaminationRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) ListDoctorMedicalHistory(c *gin.Context) {
	result, err := ctl.service.ListDoctorMedicalHistory(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgMedicalHistoryListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorHistoricalMedicalRecord(c *gin.Context) {
	result, err := ctl.service.GetDoctorHistoricalMedicalRecord(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), c.Param("encounter_id"))
	respond(c, constant.MsgMedicalRecordRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) SaveDoctorConsultationDraft(c *gin.Context) {
	var req request.ConsultationNoteRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.SaveDoctorConsultationDraft(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgConsultationDraftSaved, http.StatusOK, result, err)
}

func (ctl *Controller) CompleteDoctorExamination(c *gin.Context) {
	result, err := ctl.service.CompleteDoctorExamination(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"))
	respond(c, constant.MsgExaminationCompleted, http.StatusOK, result, err)
}

func (ctl *Controller) CorrectDoctorConsultation(c *gin.Context) {
	var req request.CorrectConsultationNoteRequest
	if err := util.BindAndValidate(c, &req); err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.CorrectDoctorConsultation(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), req)
	respond(c, constant.MsgConsultationCorrected, http.StatusOK, result, err)
}

func (ctl *Controller) ListPatientMedicalHistory(c *gin.Context) {
	result, err := ctl.service.ListPatientMedicalHistory(c.Request.Context(), util.GetUserID(c))
	respond(c, constant.MsgMedicalHistoryListed, http.StatusOK, result, err)
}

func (ctl *Controller) GetPatientMedicalRecord(c *gin.Context) {
	result, err := ctl.service.GetPatientMedicalRecord(c.Request.Context(), util.GetUserID(c), c.Param("encounter_id"))
	respond(c, constant.MsgMedicalRecordRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) UploadHospitalAttachment(c *gin.Context) {
	file, documentType, note, err := readMedicalMultipart(c)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.UploadHospitalAttachment(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), documentType, note, file)
	respond(c, constant.MsgMedicalAttachmentUploaded, http.StatusCreated, result, err)
}

func (ctl *Controller) UploadDoctorAttachment(c *gin.Context) {
	file, documentType, note, err := readMedicalMultipart(c)
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result, err := ctl.service.UploadDoctorAttachment(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), documentType, note, file)
	respond(c, constant.MsgMedicalAttachmentUploaded, http.StatusCreated, result, err)
}

func (ctl *Controller) GetHospitalAttachmentURL(c *gin.Context) {
	result, err := ctl.service.GetHospitalAttachmentURL(c.Request.Context(), hospitalID(c), util.GetUserID(c), c.Param("appointment_id"), c.Param("attachment_id"))
	respond(c, constant.MsgMedicalAttachmentURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorAttachmentURL(c *gin.Context) {
	result, err := ctl.service.GetDoctorAttachmentURL(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), c.Param("attachment_id"))
	respond(c, constant.MsgMedicalAttachmentURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetDoctorHistoricalAttachmentURL(c *gin.Context) {
	result, err := ctl.service.GetDoctorHistoricalAttachmentURL(c.Request.Context(), util.GetUserID(c), c.Param("appointment_id"), c.Param("encounter_id"), c.Param("attachment_id"))
	respond(c, constant.MsgMedicalAttachmentURLRetrieved, http.StatusOK, result, err)
}

func (ctl *Controller) GetPatientAttachmentURL(c *gin.Context) {
	result, err := ctl.service.GetPatientAttachmentURL(c.Request.Context(), util.GetUserID(c), c.Param("attachment_id"))
	respond(c, constant.MsgMedicalAttachmentURLRetrieved, http.StatusOK, result, err)
}

func readMedicalMultipart(c *gin.Context) (service.UploadedFile, string, *string, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMedicalMultipartBytes)
	if err := c.Request.ParseMultipartForm(maxMedicalMultipartBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return service.UploadedFile{}, "", nil, constant.ErrRequestTooLarge
		}
		return service.UploadedFile{}, "", nil, constant.ErrMedicalAttachmentInvalid
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return service.UploadedFile{}, "", nil, constant.ErrMedicalAttachmentInvalid
		}
		return service.UploadedFile{}, "", nil, constant.ErrMedicalAttachmentInvalid
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxMedicalMultipartBytes+1))
	if err != nil || int64(len(content)) > maxMedicalMultipartBytes {
		return service.UploadedFile{}, "", nil, constant.ErrMedicalAttachmentInvalid
	}
	documentType := strings.TrimSpace(c.PostForm("document_type"))
	var note *string
	if value := strings.TrimSpace(c.PostForm("note")); value != "" {
		note = &value
	}
	return service.UploadedFile{Filename: header.Filename, MIMEType: header.Header.Get("Content-Type"), Content: content}, documentType, note, nil
}

func hospitalID(c *gin.Context) string { return c.GetString("hospital_id") }

func respond(c *gin.Context, code constant.MessageCode, status int, data any, err error) {
	if err != nil {
		util.HandleError(c, err)
		return
	}
	result := constant.NewSuccessResponse(code)
	result.StatusCode = status
	result.Data = data
	util.HandleResponse(c, result, nil)
}
