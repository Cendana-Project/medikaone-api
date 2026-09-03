package response

import "time"

const (
	MessageOK = "ok"
)

type MessageDetail struct {
	TitleEng string `json:"title_eng,omitempty"`
	DescEng  string `json:"desc_eng,omitempty"`
	TitleIdn string `json:"title_idn,omitempty"`
	DescIdn  string `json:"desc_idn,omitempty"`
}

type CustomError struct {
	Code       string
	Message    string
	StatusCode int
	Detail     MessageDetail
}

func (m CustomError) Error() string {
	if m.Message == "" {
		return m.Code
	}
	return m.Message
}

func (m CustomError) ToResponse() BaseResponse {
	msg := m.Code
	if msg == "" {
		msg = "INTERNAL_SERVER_ERROR"
	}

	detail := m.Detail
	if detail == (MessageDetail{}) {
		detail = MessageDetail{
			TitleEng: "Request failed",
			DescEng:  "The request could not be completed.",
			TitleIdn: "Permintaan gagal",
			DescIdn:  "Permintaan tidak dapat diselesaikan.",
		}
	}
	statusCode := m.StatusCode
	if statusCode < 400 || statusCode > 599 {
		statusCode = 500
	}

	return BaseResponse{
		StatusCode:    statusCode,
		Message:       msg,
		MessageDetail: detail,
	}
}

type Meta struct {
	Page      int `json:"page"`
	PageSize  int `json:"page_size"`
	TotalData int `json:"total_data"`
}

type BaseResponse struct {
	StatusCode    int           `json:"-"`
	Message       string        `json:"message"`
	MessageDetail MessageDetail `json:"message_detail"`
	Data          any           `json:"data"`
	Meta          any           `json:"meta,omitempty"`
	TraceID       string        `json:"trace_id"`
	Timestamp     time.Time     `json:"timestamp"`
}

func NewResponseOK() *BaseResponse {
	return &BaseResponse{
		Message: MessageOK,
		MessageDetail: MessageDetail{
			TitleEng: "SUCCESS",
			DescEng:  "Operation completed successfully",
			TitleIdn: "SUKSES",
			DescIdn:  "Operasi berhasil diselesaikan",
		},
	}
}

type GetHealthCheckMemoryResp struct {
	Alloc      uint64 `json:"alloc"`
	TotalAlloc uint64 `json:"total_alloc"`
	Sys        uint64 `json:"sys"`
	HeapAlloc  uint64 `json:"heap_alloc"`
	HeapSys    uint64 `json:"heap_sys"`
}

type GetHealthCheckServiceStatusResp struct {
	Name string `json:"name"`
	IsUp bool   `json:"is_up"`
}

type GetHealthCheckResp struct {
	Status          string                            `json:"status"`
	Environtment    string                            `json:"environtment"`
	Version         string                            `json:"version"`
	GoVersion       string                            `json:"go_version"`
	GoRoutine       int                               `json:"go_routine"`
	Memory          GetHealthCheckMemoryResp          `json:"memory"`
	ServiceStatuses []GetHealthCheckServiceStatusResp `json:"service_statuses"`
}
