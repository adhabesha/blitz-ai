package main

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

// Public pages Meta, Google and TikTok require before an app can go live.

func legalPage(w http.ResponseWriter, r *http.Request, title, body string) {
	contact := strings.TrimSpace(env("CONTACT_EMAIL"))
	ct := "the contact details on our website"
	if contact != "" {
		ct = `<a href="mailto:` + html.EscapeString(contact) + `">` + html.EscapeString(contact) + `</a>`
	}
	app := firstNonEmptyStr(env("APP_NAME"), "ficx")
	base := baseURL(r)
	body = strings.NewReplacer("{{app}}", html.EscapeString(app), "{{contact}}", ct, "{{base}}", html.EscapeString(base)).Replace(body)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s – %s</title>
<style>body{font:16px/1.65 Inter,system-ui,sans-serif;max-width:780px;margin:40px auto;padding:0 18px;color:#0e1834}h1{font-size:30px}h2{margin-top:30px;font-size:19px}a{color:#b84a18}small{color:#4f5f80}</style></head><body>
<h1>%s</h1><small>Last updated: 1 November 2026</small>%s</body></html>`, html.EscapeString(app), title, title, body)
}

func (s *server) privacyPage(w http.ResponseWriter, r *http.Request) {
	legalPage(w, r, "Privacy Policy", `
<p>{{app}} ("we", "us") is a marketing, social media and customer relationship management (CRM) platform. This policy explains what information we collect, why, and the choices you have.</p>
<h2>Information we collect</h2>
<ul><li><b>Account information:</b> your name, email address and sign-in details when you create an account.</li>
<li><b>Connected accounts:</b> when you connect Facebook, Instagram, TikTok, Google or YouTube, we receive the data you authorize: Page and profile details, posts and their performance, comments, messages sent to your Page or account, lead-form submissions, advertising account performance, and analytics data.</li>
<li><b>Leads and customer records:</b> contact details and notes you or your visitors submit through forms, lead ads, messages or comments, and records your team creates in the CRM.</li>
<li><b>Technical data:</b> basic logs such as IP address and browser type for security and troubleshooting.</li></ul>
<h2>How we use it</h2>
<ul><li>To provide the service: analytics, recommendations, publishing, advertising tools, lead capture, automated replies and the CRM.</li>
<li>To generate reports and suggestions using AI services, which process only the content needed for the request.</li>
<li>To secure the service, prevent abuse and fix problems.</li></ul>
<p>We do not sell personal information. We only use data received from Meta, Google and TikTok to provide features you asked for, and in line with their platform terms.</p>
<h2>Who we share it with</h2>
<p>Service providers that help us run {{app}} (cloud hosting and database, email delivery, AI processing) and the platforms you connect, when you ask us to act on them (for example publishing a post or replying to a message). We may disclose information if required by law.</p>
<h2>Storage and security</h2>
<p>Data is stored with reputable cloud providers and protected with access controls and encryption in transit. Access tokens for connected accounts are stored encrypted. No system is perfectly secure, but we work to protect your information.</p>
<h2>Retention and deletion</h2>
<p>We keep information while your account is active. You can disconnect any account at any time, and you can ask us to delete your data. See <a href="{{base}}/data-deletion">how to delete your data</a>.</p>
<h2>Your rights</h2>
<p>You can access, correct or delete your information, and withdraw permissions you granted to us from your Facebook, Instagram, TikTok or Google settings.</p>
<h2>Children</h2>
<p>{{app}} is not intended for people under 18.</p>
<h2>Changes</h2>
<p>We may update this policy and will change the date above when we do.</p>
<h2>Contact</h2>
<p>Questions or requests: {{contact}}.</p>`)
}

func (s *server) termsPage(w http.ResponseWriter, r *http.Request) {
	legalPage(w, r, "Terms of Service", `
<p>By using {{app}} you agree to these terms.</p>
<h2>The service</h2><p>{{app}} provides marketing analytics, publishing, advertising tools, automations and CRM features. We may change or improve the service at any time.</p>
<h2>Your account</h2><p>You are responsible for your account, for the people you invite, and for the accuracy of what you submit. You must have the right to connect the accounts you connect.</p>
<h2>Acceptable use</h2><p>Do not use the service to break the law, send spam, harass people, or violate the terms of Meta, Google, TikTok or any other connected platform. You are responsible for getting any consent required to contact leads and customers.</p>
<h2>Third-party platforms</h2><p>Features that depend on Facebook, Instagram, TikTok, Google and others are subject to those platforms' rules and availability, which we do not control.</p>
<h2>Disclaimer</h2><p>The service is provided "as is". AI-generated suggestions may contain mistakes; review them before you publish or spend money. To the extent allowed by law, we are not liable for indirect or consequential losses.</p>
<h2>Termination</h2><p>You can stop using the service at any time. We may suspend accounts that break these terms.</p>
<h2>Contact</h2><p>{{contact}}</p>`)
}

func (s *server) deletionPage(w http.ResponseWriter, r *http.Request) {
	legalPage(w, r, "Delete your data", `
<p>You can remove your data from {{app}} at any time.</p>
<h2>Option 1: do it yourself</h2>
<ol><li>Sign in to {{app}} and disconnect your Facebook, Instagram, TikTok and Google accounts. This stops all access and deletes the stored access tokens.</li>
<li>In Facebook: Settings &amp; privacy &gt; Settings &gt; Apps and websites, find the app and choose Remove. In Instagram: Settings &gt; Website permissions &gt; Apps and websites.</li></ol>
<h2>Option 2: ask us to delete everything</h2>
<p>Email {{contact}} from the address you signed up with, with the subject "Delete my data". We will delete your account, connected-account data, leads, messages and CRM records within 30 days and confirm by email.</p>
<h2>If you are a lead or customer of one of our users</h2>
<p>Your information is controlled by the business you contacted. Ask that business to delete it, or email us at {{contact}} and we will pass the request on.</p>`)
}
