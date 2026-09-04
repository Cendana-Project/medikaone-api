package prescription

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/skip2/go-qrcode"

	"github.com/Cendana-Project/medikaone-api/internal/model/response"
)

type pdfRenderer struct{}

func NewPDFRenderer() PDFRenderer { return &pdfRenderer{} }

func (r *pdfRenderer) Render(prescription *response.Prescription, verificationURL string) ([]byte, error) {
	if prescription == nil || prescription.CurrentRevision == nil || prescription.CurrentRevision.IssuedAt == nil {
		return nil, fmt.Errorf("issued prescription data is incomplete")
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 14, 16)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 8, cleanPDFText(prescription.HospitalName), "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	if prescription.HospitalAddress != nil {
		pdf.CellFormat(0, 5, cleanPDFText(*prescription.HospitalAddress), "", 1, "C", false, 0, "")
	}
	if prescription.HospitalPhone != nil {
		pdf.CellFormat(0, 5, "Tel: "+cleanPDFText(*prescription.HospitalPhone), "", 1, "C", false, 0, "")
	}
	pdf.Ln(3)
	pdf.SetDrawColor(40, 90, 120)
	pdf.Line(16, pdf.GetY(), 194, pdf.GetY())
	pdf.Ln(5)
	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(0, 8, "RESEP ELEKTRONIK", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	issuedAt := prescription.CurrentRevision.IssuedAt.UTC()
	writeLabelValue(pdf, "Nomor resep", prescription.PrescriptionNumber)
	writeLabelValue(pdf, "Tanggal terbit", issuedAt.Format("02-01-2006 15:04 UTC"))
	writeLabelValue(pdf, "Pasien", prescription.PatientName)
	writeLabelValue(pdf, "Tanggal lahir", prescription.PatientDateOfBirth)
	writeLabelValue(pdf, "Dokter", prescription.DoctorName)
	sip := "-"
	if prescription.DoctorSIPNumber != nil {
		sip = *prescription.DoctorSIPNumber
	}
	writeLabelValue(pdf, "Nomor SIP", sip)
	writeLabelValue(pdf, "Alergi tercatat", valueOr(prescription.PatientAllergies, "Tidak ada data alergi"))
	pdf.Ln(4)

	for index, item := range prescription.CurrentRevision.Items {
		pdf.SetFont("Arial", "B", 11)
		pdf.MultiCell(0, 6, fmt.Sprintf("%d. %s - %s, %s", index+1, cleanPDFText(item.MedicationName), cleanPDFText(item.Strength), cleanPDFText(item.DosageForm)), "1", "L", false)
		if item.Type == "COMPOUND" {
			pdf.SetFont("Arial", "", 9)
			for _, component := range item.Components {
				pdf.MultiCell(0, 5, fmt.Sprintf("   - %s %s %s (%s)", cleanPDFText(component.MedicationName), formatNumber(component.Amount), cleanPDFText(component.Unit), cleanPDFText(component.Strength)), "LR", "L", false)
			}
		}
		pdf.SetFont("Arial", "", 9)
		schedule := scheduleText(item)
		pdf.MultiCell(0, 5, "Dosis/rute: "+formatNumber(item.DoseAmount)+" "+cleanPDFText(item.DoseUnit)+" / "+cleanPDFText(item.Route), "LR", "L", false)
		pdf.MultiCell(0, 5, "Jadwal: "+schedule+"; Durasi: "+fmt.Sprintf("%d %s", item.DurationValue, cleanPDFText(item.DurationUnit)), "LR", "L", false)
		pdf.MultiCell(0, 5, "Jumlah: "+formatNumber(item.Quantity)+" "+cleanPDFText(item.QuantityUnit), "LR", "L", false)
		pdf.MultiCell(0, 5, "Aturan pakai: "+cleanPDFText(item.Directions), "LRB", "L", false)
		pdf.Ln(3)
	}
	if prescription.CurrentRevision.GeneralNote != nil {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(0, 6, "Catatan", "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		pdf.MultiCell(0, 5, cleanPDFText(*prescription.CurrentRevision.GeneralNote), "1", "L", false)
	}
	pdf.Ln(5)
	qrPNG, err := qrcode.Encode(verificationURL, qrcode.Medium, 256)
	if err != nil {
		return nil, err
	}
	options := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
	pdf.RegisterImageOptionsReader("prescription-verification", options, bytes.NewReader(qrPNG))
	y := pdf.GetY()
	if y > 240 {
		pdf.AddPage()
		y = pdf.GetY()
	}
	pdf.ImageOptions("prescription-verification", 16, y, 30, 30, false, options, 0, "")
	pdf.SetXY(50, y+3)
	pdf.SetFont("Arial", "B", 9)
	pdf.MultiCell(140, 5, "Verifikasi keaslian resep", "", "L", false)
	pdf.SetX(50)
	pdf.SetFont("Arial", "", 8)
	pdf.MultiCell(140, 4, cleanPDFText(verificationURL), "", "L", false)
	pdf.SetX(50)
	pdf.MultiCell(140, 4, "Dokumen dibuat oleh sistem MedikaOne. QR dan audit trail bukan pengganti tanda tangan elektronik tersertifikasi.", "", "L", false)
	pdf.SetY(y + 34)
	pdf.SetFont("Arial", "I", 8)
	pdf.MultiCell(0, 4, "Resep ini berisi obat non-narkotika dan non-psikotropika. Gunakan obat sesuai petunjuk dokter dan tenaga kefarmasian.", "", "C", false)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func writeLabelValue(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(34, 5, label, "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 9)
	pdf.MultiCell(0, 5, ": "+cleanPDFText(value), "", "L", false)
}

func scheduleText(item response.PrescriptionItem) string {
	parts := make([]string, 0, 4)
	if item.FrequencyPerDay != nil {
		parts = append(parts, fmt.Sprintf("%dx sehari", *item.FrequencyPerDay))
	}
	if item.IntervalHours != nil {
		parts = append(parts, fmt.Sprintf("setiap %d jam", *item.IntervalHours))
	}
	if item.AsNeeded {
		parts = append(parts, "bila perlu")
	}
	if item.TimingInstructions != nil {
		parts = append(parts, cleanPDFText(*item.TimingInstructions))
	}
	return strings.Join(parts, "; ")
}

func valueOr(value *string, fallback string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return fallback
	}
	return *value
}

func formatNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", value), "0"), ".")
}

func cleanPDFText(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
