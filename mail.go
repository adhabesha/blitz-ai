package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
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
	return sendMailH(ctx, to, subject, htmlBody, textBody, nil, false)
}

// smtpReady says whether a real mailbox (SMTP) is configured; cold outreach should go through it.
func smtpReady() bool {
	return env("MAIL_FROM") != "" && env("SMTP_HOST") != "" && env("SMTP_USER") != ""
}

// sendMailH is sendMail with extra headers (Reply-To, List-Unsubscribe...). With preferSMTP the own mailbox is used
// even when a Resend key exists, because transactional services forbid cold outreach.
func sendMailH(ctx context.Context, to, subject, htmlBody, textBody string, hdr map[string]string, preferSMTP bool) error {
	from := env("MAIL_FROM")
	if !mailConfigured() {
		return errors.New("email sending is not set up yet (add MAIL_FROM and either RESEND_API_KEY or SMTP_HOST, SMTP_USER, SMTP_PASS in Admin)")
	}
	if strings.ContainsAny(to+from+subject, "\r\n") {
		return errors.New("invalid header")
	}
	for k, v := range hdr {
		if strings.ContainsAny(k+v, "\r\n") {
			return errors.New("invalid header")
		}
	}
	if k := env("RESEND_API_KEY"); k != "" && !(preferSMTP && smtpReady()) {
		pl := map[string]any{"from": from, "to": []string{to}, "subject": subject, "html": htmlBody, "text": textBody}
		if len(hdr) > 0 {
			pl["headers"] = hdr
		}
		b, _ := json.Marshal(pl)
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
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d.%s>\r\nMIME-Version: 1.0\r\n", from, to, mime.QEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z), time.Now().UnixNano(), hostOf(from))
	for k, v := range hdr {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", boundary)
	b64 := func(t string) string {
		e := base64.StdEncoding.EncodeToString([]byte(t))
		var o strings.Builder
		for len(e) > 76 {
			o.WriteString(e[:76] + "\r\n")
			e = e[76:]
		}
		return o.String() + e
	}
	fmt.Fprintf(&msg, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s\r\n--%s--\r\n", boundary, b64(textBody), boundary, b64(htmlBody), boundary)
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

func hostOf(from string) string {
	a := from
	if i := strings.LastIndex(a, "<"); i >= 0 {
		a = strings.Trim(a[i:], "<> ")
	}
	if i := strings.LastIndex(a, "@"); i >= 0 {
		return a[i+1:]
	}
	return "localhost"
}

func inviteMail(workspace, inviter, role, link string) (string, string, string) {
	subj := inviter + " invited you to " + workspace + " on Blitz CRM"
	txt := fmt.Sprintf("%s invited you to join %s on Blitz CRM as %s.\n\nOpen this link and sign in or create an account with THIS email address:\n%s\n\nYou join the workspace automatically.", inviter, workspace, role, link)
	h := fmt.Sprintf(`<div style="font-family:Arial,sans-serif;max-width:520px;margin:auto;padding:24px"><h2 style="margin:0 0 12px">You're invited</h2><p><b>%s</b> invited you to join <b>%s</b> on Blitz CRM as <b>%s</b>.</p><p><a href="%s" style="display:inline-block;background:#2563eb;color:#fff;padding:12px 22px;border-radius:10px;text-decoration:none;font-weight:bold">Accept invitation</a></p><p style="color:#555;font-size:13px">Sign in or create your account with <b>this email address</b> and you will join automatically.<br>If the button doesn't work, copy this link: %s</p></div>`,
		html.EscapeString(inviter), html.EscapeString(workspace), html.EscapeString(role), html.EscapeString(link), html.EscapeString(link))
	return subj, h, txt
}
