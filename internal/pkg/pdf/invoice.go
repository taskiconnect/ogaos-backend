package pdf

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jung-kurt/gofpdf"

	"ogaos-backend/internal/domain/models"
)

func formatKobo(kobo int64, currency string) string {
	if currency == "" {
		currency = "NGN"
	}

	symbol := "NGN "
	if strings.EqualFold(currency, "NGN") {
		symbol = "NGN "
	} else {
		symbol = strings.ToUpper(currency) + " "
	}

	naira := float64(kobo) / 100
	return fmt.Sprintf("%s%s", symbol, humanMoney(naira))
}

func humanMoney(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	parts := strings.Split(s, ".")
	intPart := parts[0]
	frac := parts[1]

	n := len(intPart)
	if n <= 3 {
		return intPart + "." + frac
	}

	var out []byte
	count := 0
	for i := n - 1; i >= 0; i-- {
		out = append([]byte{intPart[i]}, out...)
		count++
		if count%3 == 0 && i != 0 {
			out = append([]byte{','}, out...)
		}
	}
	return string(out) + "." + frac
}

func dateText(t time.Time) string {
	return t.Format("02 Jan 2006")
}

func safeCustomerName(inv *models.Invoice) string {
	if inv.Customer == nil {
		return "Customer"
	}
	name := strings.TrimSpace(inv.Customer.FirstName + " " + inv.Customer.LastName)
	if name == "" {
		return "Customer"
	}
	return name
}

func drawLabelValue(pdf *gofpdf.Fpdf, x, y, labelW, valueW, rowH float64, label, value string, bold bool) {
	pdf.SetXY(x, y)
	if bold {
		pdf.SetFont("Arial", "B", 10)
	} else {
		pdf.SetFont("Arial", "", 10)
	}
	pdf.SetTextColor(71, 85, 105)
	pdf.CellFormat(labelW, rowH, label, "", 0, "L", false, 0, "")
	pdf.SetTextColor(15, 23, 42)
	if bold {
		pdf.SetFont("Arial", "B", 10)
	} else {
		pdf.SetFont("Arial", "", 10)
	}
	pdf.CellFormat(valueW, rowH, value, "", 1, "R", false, 0, "")
}

func BuildInvoicePDF(inv *models.Invoice, businessName string) ([]byte, error) {
	p := gofpdf.New("P", "mm", "A4", "")
	p.SetMargins(14, 14, 14)
	p.SetAutoPageBreak(true, 16)
	p.AddPage()

	pageW, _ := p.GetPageSize()
	left := 14.0
	right := pageW - 14.0
	contentW := right - left

	// Background card
	p.SetFillColor(255, 255, 255)
	p.Rect(left, 14, contentW, 268, "F")

	// Header band
	p.SetFillColor(4, 16, 44)
	p.RoundedRect(left, 14, contentW, 34, 4, "1234", "F")

	// Accent strip
	p.SetFillColor(0, 43, 157)
	p.Rect(left, 44, contentW, 4, "F")

	// Header content
	p.SetXY(left+8, 22)
	p.SetFont("Arial", "B", 18)
	p.SetTextColor(255, 255, 255)
	p.CellFormat(110, 8, businessName, "", 0, "L", false, 0, "")

	p.SetXY(right-60, 20)
	p.SetFont("Arial", "B", 20)
	p.CellFormat(52, 10, "INVOICE", "", 1, "R", false, 0, "")

	p.SetXY(right-60, 31)
	p.SetFont("Arial", "", 10)
	p.SetTextColor(214, 228, 255)
	p.CellFormat(52, 6, fmt.Sprintf("Invoice No: %s", inv.InvoiceNumber), "", 1, "R", false, 0, "")

	// Meta blocks
	p.SetTextColor(15, 23, 42)
	p.SetFont("Arial", "B", 11)
	p.SetXY(left+8, 58)
	p.CellFormat(60, 7, "Bill To", "", 1, "L", false, 0, "")

	p.SetFont("Arial", "", 10)
	p.SetTextColor(51, 65, 85)
	p.SetX(left + 8)
	p.CellFormat(80, 6, safeCustomerName(inv), "", 1, "L", false, 0, "")
	if inv.Customer != nil && inv.Customer.Email != nil {
		p.SetX(left + 8)
		p.CellFormat(80, 6, *inv.Customer.Email, "", 1, "L", false, 0, "")
	}
	if inv.Customer != nil && inv.Customer.PhoneNumber != nil {
		p.SetX(left + 8)
		p.CellFormat(80, 6, *inv.Customer.PhoneNumber, "", 1, "L", false, 0, "")
	}

	boxX := right - 76
	boxY := 56.0
	boxW := 68.0
	boxH := 34.0

	p.SetFillColor(248, 250, 252)
	p.SetDrawColor(226, 232, 240)
	p.RoundedRect(boxX, boxY, boxW, boxH, 3, "1234", "DF")

	drawLabelValue(p, boxX+5, boxY+4, 26, 32, 6, "Issue Date", dateText(inv.IssueDate), false)
	drawLabelValue(p, boxX+5, boxY+11, 26, 32, 6, "Due Date", dateText(inv.DueDate), false)
	drawLabelValue(p, boxX+5, boxY+18, 26, 32, 6, "Revision", fmt.Sprintf("%d", inv.RevisionNumber), false)
	drawLabelValue(p, boxX+5, boxY+25, 26, 32, 6, "Status", strings.Title(inv.Status), true)

	// Items header
	startY := 102.0
	colDesc := 84.0
	colQty := 18.0
	colUnit := 28.0
	colDisc := 26.0
	colTotal := 30.0

	p.SetXY(left, startY)
	p.SetFillColor(239, 244, 255)
	p.SetDrawColor(191, 219, 254)
	p.SetTextColor(30, 41, 59)
	p.SetFont("Arial", "B", 10)

	p.CellFormat(colDesc, 9, "Description", "1", 0, "L", true, 0, "")
	p.CellFormat(colQty, 9, "Qty", "1", 0, "C", true, 0, "")
	p.CellFormat(colUnit, 9, "Unit Price", "1", 0, "R", true, 0, "")
	p.CellFormat(colDisc, 9, "Discount", "1", 0, "R", true, 0, "")
	p.CellFormat(colTotal, 9, "Line Total", "1", 1, "R", true, 0, "")

	p.SetFont("Arial", "", 10)
	p.SetTextColor(15, 23, 42)
	p.SetDrawColor(226, 232, 240)

	for _, item := range inv.InvoiceItems {
		rowY := p.GetY()
		x := left

		descLines := p.SplitLines([]byte(item.Description), colDesc-4)
		rowH := float64(len(descLines)) * 6
		if rowH < 10 {
			rowH = 10
		}

		p.SetXY(x, rowY)
		p.MultiCell(colDesc, 6, item.Description, "1", "L", false)
		p.SetXY(x+colDesc, rowY)
		p.CellFormat(colQty, rowH, fmt.Sprintf("%d", item.Quantity), "1", 0, "C", false, 0, "")
		p.CellFormat(colUnit, rowH, formatKobo(item.UnitPrice, inv.Currency), "1", 0, "R", false, 0, "")
		p.CellFormat(colDisc, rowH, formatKobo(item.Discount, inv.Currency), "1", 0, "R", false, 0, "")
		p.CellFormat(colTotal, rowH, formatKobo(item.TotalPrice, inv.Currency), "1", 1, "R", false, 0, "")
	}

	// Totals box
	p.Ln(8)
	totalsX := right - 78
	totalsY := p.GetY()
	totalsW := 78.0
	totalsH := 48.0

	p.SetFillColor(248, 250, 252)
	p.SetDrawColor(226, 232, 240)
	p.RoundedRect(totalsX, totalsY, totalsW, totalsH, 3, "1234", "DF")

	drawLabelValue(p, totalsX+5, totalsY+4, 30, 38, 6, "Sub Total", formatKobo(inv.SubTotal, inv.Currency), false)
	drawLabelValue(p, totalsX+5, totalsY+10, 30, 38, 6, "Discount", formatKobo(inv.DiscountAmount, inv.Currency), false)
	drawLabelValue(p, totalsX+5, totalsY+16, 30, 38, 6, "VAT", formatKobo(inv.VATAmount, inv.Currency), false)
	drawLabelValue(p, totalsX+5, totalsY+22, 30, 38, 6, "WHT", "-"+formatKobo(inv.WHTAmount, inv.Currency), false)
	drawLabelValue(p, totalsX+5, totalsY+30, 30, 38, 7, "Total", formatKobo(inv.TotalAmount, inv.Currency), true)
	drawLabelValue(p, totalsX+5, totalsY+37, 30, 38, 6, "Paid", formatKobo(inv.AmountPaid, inv.Currency), false)
	drawLabelValue(p, totalsX+5, totalsY+43, 30, 38, 6, "Balance", formatKobo(inv.BalanceDue, inv.Currency), true)

	// Notes / payment terms
	notesY := totalsY + totalsH + 8
	if inv.Notes != nil && strings.TrimSpace(*inv.Notes) != "" {
		p.SetXY(left, notesY)
		p.SetFont("Arial", "B", 11)
		p.SetTextColor(15, 23, 42)
		p.CellFormat(0, 7, "Notes", "", 1, "L", false, 0, "")
		p.SetFont("Arial", "", 10)
		p.SetTextColor(51, 65, 85)
		p.SetX(left)
		p.MultiCell(120, 6, *inv.Notes, "", "L", false)
		notesY = p.GetY() + 4
	}

	if inv.PaymentTerms != nil && strings.TrimSpace(*inv.PaymentTerms) != "" {
		p.SetXY(left, notesY)
		p.SetFont("Arial", "B", 11)
		p.SetTextColor(15, 23, 42)
		p.CellFormat(0, 7, "Payment Terms", "", 1, "L", false, 0, "")
		p.SetFont("Arial", "", 10)
		p.SetTextColor(51, 65, 85)
		p.SetX(left)
		p.MultiCell(120, 6, *inv.PaymentTerms, "", "L", false)
	}

	// Footer
	p.SetY(274)
	p.SetDrawColor(226, 232, 240)
	p.Line(left, 274, right, 274)

	p.SetY(276)
	p.SetFont("Arial", "", 9)
	p.SetTextColor(100, 116, 139)
	p.CellFormat(contentW, 5, "Generated by OgaOs", "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
