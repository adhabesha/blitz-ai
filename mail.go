package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// mailConfigured says whether the server can send email at all.
func mailConfigured() bool {
	return env("MAIL_FROM") != "" && (env("RESEND_API_KEY") != "" || (env("SMTP_HOST") != "" && env("SMTP_USER") != ""))
}

// sendMail sends through Resend (RESEND_API_KEY) or plain SMTP (SMTP_HOST/PORT/USER/PASS).
func sendMail(ctx context.Context, to, subject, htmlBody, textBody string) error {
	from := env("MAIL_FROM")
	if !mailConfigured() {
		return errors.New("email sending is not set up yet (add MAIL_FROM and either RESEND_API_KEY or SMTP_HOST, SMTP_USER, SMTP_PASS in Admin)")
	}
	if strings.ContainsAny(to+from+subject, "\r\n") {
		return errors.New("invalid header")
	}
	if k := env("RESEND_API_KEY"); k != "" {
		b, _ := json.Marshal(map[string]any{"from": from, "to": []string{to}, "subject": subject, "html": htmlBody, "text": textBody})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+k)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			x, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
			return fmt.Errorf("the email service refused it (%d): %s", resp.StatusCode, strings.ReplaceAll(string(x), "\n", " "))
		}
		return nil
	}
	host, port := env("SMTP_HOST"), firstNonEmptyStr(env("SMTP_PORT"), "587")
	addr := net.JoinHostPort(host, port)
	boundary := fmt.Sprintf("b%d", time.Now().UnixNano())
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n", from, to, subject, boundary)
	fmt.Fprintf(&msg, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=utf-8\r\n\r\n%s\r\n--%s--\r\n", boundary, textBody, boundary, htmlBody, boundary)
	auth := smtp.PlainAuth("", env("SMTP_USER"), env("SMTP_PASS"), host)
	addrOnly := from
	if i := strings.LastIndex(from, "<"); i >= 0 {
		addrOnly = strings.Trim(from[i:], "<> ")
	}
	done := make(chan error, 1)
	go func() {
		if port == "465" {
			conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
			if err != nil {
				done <- err
				return
			}
			c, err := smtp.NewClient(conn, host)
			if err != nil {
				done <- err
				return
			}
			defer c.Close()
			if err = c.Auth(auth); err != nil {
				done <- err
				return
			}
			if err = c.Mail(addrOnly); err != nil {
				done <- err
				return
			}
			if err = c.Rcpt(to); err != nil {
				done <- err
				return
			}
			wc, err := c.Data()
			if err != nil {
				done <- err
				return
			}
			wc.Write(msg.Bytes())
			done <- wc.Close()
			return
		}
		done <- smtp.SendMail(addr, auth, addrOnly, []string{to}, msg.Bytes())
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errors.New("the mail server did not answer in time")
	}
}

func inviteMail(workspace, inviter, role, link string) (string, string, string) {
	subj := inviter + " invited you to " + workspace + " on Blitz CRM"
	txt := fmt.Sprintf("%s invited you to join %s on Blitz CRM as %s.\n\nOpen this link and sign in or create an account with THIS email address:\n%s\n\nYou join the workspace automatically.", inviter, workspace, role, link)
	h := fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:520px;margin:auto;padding:24px"><h2 style="margin:0 0 12px">You're invited</h2><p><b>%s</b> invited you to join <b>%s</b> on Blitz CRM as <b>%s</b>.</p><p><a href="%s" style="display:inline-block;background:#2563eb;color:#fff;padding:12px 22px;border-radius:10px;text-decoration:none;font-weight:bold">Accept invitation</a></p><p style="color:#555;font-size:13px">Sign in or create your account with <b>this email address</b> and you will join automatically.<br>If the button doesn't work, copy this link: %s</p></div>`,
		html.EscapeString(inviter), html.EscapeString(workspace), html.EscapeString(role), html.EscapeString(link), html.EscapeString(link))
	return subj, h, txt
}
