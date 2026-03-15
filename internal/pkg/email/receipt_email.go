// internal/pkg/email/receipt_email.go
// Drop this file alongside email.go — same package, same directory.
package email

import (
	"fmt"
	"strings"
	"time"

	"ogaos-backend/internal/domain/models"
)

// SendReceiptEmail emails a formatted HTML receipt to the customer.
func SendReceiptEmail(to string, sale *models.Sale, businessName string) error {
	subject := fmt.Sprintf("Your receipt from %s – %s", businessName, sale.SaleNumber)

	// Build items rows
	var rows strings.Builder
	for _, item := range sale.SaleItems {
		rows.WriteString(fmt.Sprintf(`
			<tr>
				<td style="padding:10px 0;border-bottom:1px solid #f0f0f0;">%s</td>
				<td style="padding:10px 0;border-bottom:1px solid #f0f0f0;text-align:center;">%d</td>
				<td style="padding:10px 0;border-bottom:1px solid #f0f0f0;text-align:right;">%s</td>
				<td style="padding:10px 0;border-bottom:1px solid #f0f0f0;text-align:right;">%s</td>
			</tr>`,
			item.ProductName,
			item.Quantity,
			formatKobo(item.UnitPrice),
			formatKobo(item.TotalPrice),
		))
	}

	customerName := "Valued Customer"
	if sale.Customer != nil {
		customerName = sale.Customer.FirstName + " " + sale.Customer.LastName
	}

	statusText := "PAID IN FULL"
	statusColor := "#16a34a"
	if sale.BalanceDue > 0 && sale.AmountPaid > 0 {
		statusText = "PARTIAL PAYMENT"
		statusColor = "#d97706"
	} else if sale.AmountPaid == 0 {
		statusText = "PAYMENT PENDING"
		statusColor = "#dc2626"
	}

	receiptNumber := "—"
	if sale.ReceiptNumber != nil {
		receiptNumber = *sale.ReceiptNumber
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0;padding:0;background:#f9fafb;font-family:'Helvetica Neue',Arial,sans-serif;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#f9fafb;padding:40px 0;">
  <tr><td align="center">
    <table width="560" cellpadding="0" cellspacing="0" style="background:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 1px 4px rgba(0,0,0,0.08);">

      <tr>
        <td style="background:linear-gradient(135deg,#002b9d 0%%,#3f9af5 100%%);padding:32px 40px;text-align:center;">
          <h1 style="margin:0;color:#ffffff;font-size:24px;font-weight:700;">%s</h1>
          <p style="margin:8px 0 0;color:rgba(255,255,255,0.8);font-size:14px;">Receipt</p>
        </td>
      </tr>

      <tr>
        <td style="padding:32px 40px 0;">
          <table width="100%%" cellpadding="0" cellspacing="0">
            <tr>
              <td>
                <p style="margin:0;font-size:13px;color:#6b7280;">Receipt to</p>
                <p style="margin:4px 0 0;font-size:16px;font-weight:600;color:#111827;">%s</p>
              </td>
              <td align="right">
                <p style="margin:0;font-size:13px;color:#6b7280;">Receipt No.</p>
                <p style="margin:4px 0 0;font-size:14px;font-weight:600;color:#111827;">%s</p>
                <p style="margin:4px 0 0;font-size:12px;color:#9ca3af;">%s</p>
              </td>
            </tr>
          </table>
        </td>
      </tr>

      <tr>
        <td style="padding:24px 40px 0;">
          <table width="100%%" cellpadding="0" cellspacing="0">
            <thead>
              <tr style="background:#f9fafb;">
                <th style="padding:10px 0;text-align:left;font-size:12px;color:#6b7280;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;">Item</th>
                <th style="padding:10px 0;text-align:center;font-size:12px;color:#6b7280;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;">Qty</th>
                <th style="padding:10px 0;text-align:right;font-size:12px;color:#6b7280;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;">Unit Price</th>
                <th style="padding:10px 0;text-align:right;font-size:12px;color:#6b7280;font-weight:600;text-transform:uppercase;letter-spacing:0.05em;">Total</th>
              </tr>
            </thead>
            <tbody>%s</tbody>
          </table>
        </td>
      </tr>

      <tr>
        <td style="padding:20px 40px;">
          <table width="100%%" cellpadding="0" cellspacing="0">
            %s
            %s
            <tr>
              <td colspan="2" style="padding:12px 0 0;border-top:2px solid #111827;">
                <table width="100%%"><tr>
                  <td style="font-size:15px;font-weight:700;color:#111827;">Total</td>
                  <td style="font-size:18px;font-weight:700;color:#111827;text-align:right;">%s</td>
                </tr></table>
              </td>
            </tr>
            <tr>
              <td colspan="2" style="padding:8px 0;">
                <table width="100%%"><tr>
                  <td style="font-size:13px;color:#6b7280;">Amount Paid (%s)</td>
                  <td style="font-size:14px;font-weight:600;color:#16a34a;text-align:right;">%s</td>
                </tr></table>
              </td>
            </tr>
            %s
          </table>
        </td>
      </tr>

      <tr>
        <td style="padding:0 40px 32px;text-align:center;">
          <span style="display:inline-block;background:%s;color:#fff;font-size:12px;font-weight:700;letter-spacing:0.08em;padding:8px 20px;border-radius:100px;">%s</span>
        </td>
      </tr>

      <tr>
        <td style="background:#f9fafb;padding:20px 40px;text-align:center;border-top:1px solid #f0f0f0;">
          <p style="margin:0;font-size:12px;color:#9ca3af;">Thank you for your business!</p>
          <p style="margin:6px 0 0;font-size:11px;color:#d1d5db;">Powered by OgaOS</p>
        </td>
      </tr>

    </table>
  </td></tr>
</table>
</body>
</html>`,
		businessName,
		customerName,
		receiptNumber,
		time.Now().Format("2 Jan 2006, 3:04 PM"),
		rows.String(),
		receiptVATRow(sale),
		receiptDiscountRow(sale),
		formatKobo(sale.TotalAmount),
		sale.PaymentMethod,
		formatKobo(sale.AmountPaid),
		receiptBalanceDueRow(sale),
		statusColor,
		statusText,
	)

	// send() is the unexported helper in email.go — same package, so accessible
	return send(to, subject, html)
}

func receiptVATRow(sale *models.Sale) string {
	if sale.VATAmount == 0 {
		return ""
	}
	return fmt.Sprintf(`<tr><td colspan="2" style="padding:6px 0;">
		<table width="100%%"><tr>
			<td style="font-size:13px;color:#6b7280;">VAT (%.0f%%)</td>
			<td style="font-size:13px;color:#6b7280;text-align:right;">%s</td>
		</tr></table></td></tr>`, sale.VATRate, formatKobo(sale.VATAmount))
}

func receiptDiscountRow(sale *models.Sale) string {
	if sale.DiscountAmount == 0 {
		return ""
	}
	return fmt.Sprintf(`<tr><td colspan="2" style="padding:6px 0;">
		<table width="100%%"><tr>
			<td style="font-size:13px;color:#6b7280;">Discount</td>
			<td style="font-size:13px;color:#dc2626;text-align:right;">−%s</td>
		</tr></table></td></tr>`, formatKobo(sale.DiscountAmount))
}

func receiptBalanceDueRow(sale *models.Sale) string {
	if sale.BalanceDue == 0 {
		return ""
	}
	return fmt.Sprintf(`<tr><td colspan="2" style="padding:6px 0;">
		<table width="100%%"><tr>
			<td style="font-size:13px;font-weight:600;color:#dc2626;">Balance Due</td>
			<td style="font-size:14px;font-weight:700;color:#dc2626;text-align:right;">%s</td>
		</tr></table></td></tr>`, formatKobo(sale.BalanceDue))
}
