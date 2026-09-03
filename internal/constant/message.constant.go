package constant

import "github.com/Cendana-Project/medikaone-api/internal/model/response"

// MessageCode is kept for the success response catalog. Public API errors use
// response.CustomError.Code as their single machine-readable source of truth.
type MessageCode string

const MsgSuccess MessageCode = "SUCCESS"

var MessageCatalog = map[MessageCode]response.MessageDetail{
	MsgSuccess: {
		TitleEng: "SUCCESS",
		DescEng:  "Operation completed successfully",
		TitleIdn: "SUKSES",
		DescIdn:  "Operasi berhasil diselesaikan",
	},
}

func GetMessageDetail(code MessageCode) response.MessageDetail {
	return MessageCatalog[code]
}
