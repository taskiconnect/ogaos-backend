// internal/pkg/email/email.go
package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

const resendAPI = "https://api.resend.com/emails"

type resendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

func send(to, subject, html string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Println("[EMAIL] RESEND_API_KEY is not set — email not sent")
		return nil
	}

	fromEmail := os.Getenv("EMAIL_FROM")
	if fromEmail == "" {
		fromEmail = "hello@taskiconnect.com"
	}

	payload := resendPayload{
		From:    fromEmail,
		To:      []string{to},
		Subject: subject,
		HTML:    html,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal email payload: %w", err)
	}

	req, err := http.NewRequest("POST", resendAPI, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend API error: status %d", resp.StatusCode)
	}

	log.Printf("[EMAIL] Sent '%s' to %s", subject, to)
	return nil
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

// SendVerificationEmail sends an account verification email to a new business owner.
func SendVerificationEmail(to, token, frontendURL, businessName string) {
	link := fmt.Sprintf("%s/auth/verify?token=%s", frontendURL, token)
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Welcome to OgaOs, %s!</h2>
			<p>Please verify your email address to activate your account.</p>
			<p>
				<a href="%s" style="background-color:#4F46E5;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">Verify Email</a>
			</p>
			<p>Or copy this link into your browser:</p>
			<p>%s</p>
			<p>This link expires in 48 hours.</p>
			<p>If you did not create an account, please ignore this email.</p>
		</div>
	`, businessName, link, link)
	if err := send(to, "Verify your OgaOs account", html); err != nil {
		log.Printf("[EMAIL ERROR] SendVerificationEmail to %s: %v", to, err)
	}
}

// SendStaffInvitationEmail sends an invitation email to a new staff member.
func SendStaffInvitationEmail(to, token, frontendURL, businessName string) {
	link := fmt.Sprintf("%s/auth/verify-email?token=%s", frontendURL, token)
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>You've been invited to join %s on OgaOs</h2>
			<p>Your employer has created an account for you. Click below to verify your email and get started.</p>
			<p>
				<a href="%s" style="background-color:#4F46E5;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">Accept Invitation</a>
			</p>
			<p>Or copy this link into your browser:</p>
			<p>%s</p>
			<p>This link expires in 48 hours.</p>
			<p>Your password was set by your employer — ask them for it to log in.</p>
		</div>
	`, businessName, link, link)
	if err := send(to, fmt.Sprintf("You're invited to join %s on OgaOs", businessName), html); err != nil {
		log.Printf("[EMAIL ERROR] SendStaffInvitationEmail to %s: %v", to, err)
	}
}

// ─── Recruitment ──────────────────────────────────────────────────────────────

// SendAssessmentLink sends the timed assessment URL to a job applicant.
func SendAssessmentLink(to, applicantName, jobTitle, businessName, assessmentURL string, timeLimitMinutes int) {
	subject := fmt.Sprintf("Complete your assessment for %s at %s", jobTitle, businessName)
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Assessment: %s</h2>
			<p>Hi %s,</p>
			<p>Thank you for applying to <strong>%s</strong> at <strong>%s</strong>.</p>
			<p>You have been invited to complete an aptitude assessment. Please note:</p>
			<ul>
				<li>Time limit: <strong>%d minutes</strong></li>
				<li>This link expires in <strong>24 hours</strong></li>
				<li>Once you start, the timer begins and cannot be paused</li>
			</ul>
			<p>
				<a href="%s" style="background-color:#4F46E5;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">Start Assessment</a>
			</p>
			<p>Good luck!</p>
		</div>
	`, jobTitle, applicantName, jobTitle, businessName, timeLimitMinutes, assessmentURL)
	if err := send(to, subject, html); err != nil {
		log.Printf("[EMAIL ERROR] SendAssessmentLink to %s: %v", to, err)
	}
}

// SendAssessmentResult notifies an applicant of their pass/fail outcome.
// Score is not disclosed — only the pass/fail decision.
func SendAssessmentResult(to, applicantName, jobTitle, businessName string, passed bool) {
	var subject, heading, detail string
	if passed {
		subject = fmt.Sprintf("Assessment passed — %s at %s", jobTitle, businessName)
		heading = "Congratulations — you passed!"
		detail = "The hiring team will review your application and be in touch if you are shortlisted for the next stage."
	} else {
		subject = fmt.Sprintf("Assessment result — %s at %s", jobTitle, businessName)
		heading = "Thank you for completing the assessment."
		detail = "Unfortunately, you did not meet the pass threshold for this assessment. We encourage you to keep developing your skills and apply for future openings."
	}
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>%s</h2>
			<p>Hi %s,</p>
			<p>%s</p>
		</div>
	`, heading, applicantName, detail)
	if err := send(to, subject, html); err != nil {
		log.Printf("[EMAIL ERROR] SendAssessmentResult to %s: %v", to, err)
	}
}

// ─── Digital Store ────────────────────────────────────────────────────────────

// SendDigitalProductAccess emails the signed download link to a buyer after payment.
func SendDigitalProductAccess(to, buyerName, productTitle, businessName, downloadURL string) {
	subject := fmt.Sprintf("Your purchase: %s from %s", productTitle, businessName)
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Your download is ready</h2>
			<p>Hi %s,</p>
			<p>Thank you for purchasing <strong>%s</strong> from <strong>%s</strong>.</p>
			<p>
				<a href="%s" style="background-color:#16a34a;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">Download Now</a>
			</p>
			<p>This link is personal to you — please do not share it.</p>
		</div>
	`, buyerName, productTitle, businessName, downloadURL)
	if err := send(to, subject, html); err != nil {
		log.Printf("[EMAIL ERROR] SendDigitalProductAccess to %s: %v", to, err)
	}
}

// SendPayoutNotification tells a business owner their digital store payout has been sent.
func SendPayoutNotification(to, ownerName, productTitle string, amountKobo int64) {
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Payout sent</h2>
			<p>Hi %s,</p>
			<p>A payout of <strong>%s</strong> for your sale of <strong>%s</strong> has been sent to your registered bank account.</p>
			<p>It should arrive within 1 business day. Contact support if you have any issues.</p>
		</div>
	`, ownerName, formatKobo(amountKobo), productTitle)
	if err := send(to, "Payout sent — OgaOs", html); err != nil {
		log.Printf("[EMAIL ERROR] SendPayoutNotification to %s: %v", to, err)
	}
}

// SendPayoutFailed tells a business owner their payout failed and they need to act.
func SendPayoutFailed(to, ownerName, productTitle string, amountKobo int64, reason string) {
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Payout failed</h2>
			<p>Hi %s,</p>
			<p>We were unable to send your payout of <strong>%s</strong> for <strong>%s</strong>.</p>
			<p><strong>Reason:</strong> %s</p>
			<p>Please log in to your OgaOs dashboard to check or update your bank account details.</p>
		</div>
	`, ownerName, formatKobo(amountKobo), productTitle, reason)
	if err := send(to, "Payout failed — action required", html); err != nil {
		log.Printf("[EMAIL ERROR] SendPayoutFailed to %s: %v", to, err)
	}
}

// ─── Invoices & Receipts ─────────────────────────────────────────────────────

// SendInvoice emails an invoice view link to a customer.
func SendInvoice(to, customerName, businessName, invoiceNumber, viewURL string) {
	subject := fmt.Sprintf("Invoice %s from %s", invoiceNumber, businessName)
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Invoice from %s</h2>
			<p>Hi %s,</p>
			<p>Please find your invoice <strong>%s</strong> at the link below.</p>
			<p>
				<a href="%s" style="background-color:#4F46E5;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">View Invoice</a>
			</p>
			<p>Or copy this link: %s</p>
		</div>
	`, businessName, customerName, invoiceNumber, viewURL, viewURL)
	if err := send(to, subject, html); err != nil {
		log.Printf("[EMAIL ERROR] SendInvoice to %s: %v", to, err)
	}
}

// SendReceipt emails a receipt view link to a customer after a completed sale.
func SendReceipt(to, customerName, businessName, receiptNumber, viewURL string) {
	subject := fmt.Sprintf("Receipt %s from %s", receiptNumber, businessName)
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Receipt from %s</h2>
			<p>Hi %s,</p>
			<p>Thank you for your purchase. Your receipt <strong>%s</strong> is ready.</p>
			<p>
				<a href="%s" style="background-color:#16a34a;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">View Receipt</a>
			</p>
			<p>Or copy this link: %s</p>
		</div>
	`, businessName, customerName, receiptNumber, viewURL, viewURL)
	if err := send(to, subject, html); err != nil {
		log.Printf("[EMAIL ERROR] SendReceipt to %s: %v", to, err)
	}
}

// ─── Subscription ─────────────────────────────────────────────────────────────

// SendSubscriptionExpiring warns the owner 3 days before renewal.
func SendSubscriptionExpiring(to, ownerName, plan, renewalDate string) {
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Subscription renewal reminder</h2>
			<p>Hi %s,</p>
			<p>Your <strong>%s plan</strong> subscription renews on <strong>%s</strong>.</p>
			<p>To manage your subscription, visit your OgaOs dashboard.</p>
		</div>
	`, ownerName, plan, renewalDate)
	if err := send(to, "Your OgaOs subscription renews in 3 days", html); err != nil {
		log.Printf("[EMAIL ERROR] SendSubscriptionExpiring to %s: %v", to, err)
	}
}

// SendSubscriptionExpired notifies the owner when their subscription lapses.
func SendSubscriptionExpired(to, ownerName, plan string) {
	html := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
			<h2>Subscription expired</h2>
			<p>Hi %s,</p>
			<p>Your <strong>%s plan</strong> has expired. Some features on your dashboard are now locked.</p>
			<p>
				<a href="https://app.ogaos.com/billing" style="background-color:#4F46E5;color:white;padding:12px 24px;text-decoration:none;border-radius:6px;display:inline-block;">Renew Now</a>
			</p>
		</div>
	`, ownerName, plan)
	if err := send(to, "Your OgaOs subscription has expired", html); err != nil {
		log.Printf("[EMAIL ERROR] SendSubscriptionExpired to %s: %v", to, err)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// formatKobo converts kobo to a display Naira string. e.g. 150000 → ₦1,500.00
func formatKobo(kobo int64) string {
	naira := kobo / 100
	rem := kobo % 100
	return fmt.Sprintf("₦%d.%02d", naira, rem)
}
