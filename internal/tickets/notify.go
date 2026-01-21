package tickets

import (
	"context"
	"fmt"
	"html"
	"strings"

	"realms/internal/email"
	"realms/internal/store"
)

func NotifyRootsNewTicket(ctx context.Context, st *store.Store, mailer email.Mailer, baseURL string, ticketID int64, userEmail string, subject string, body string) error {
	roots, err := st.ListActiveUsersByRole(ctx, store.UserRoleRoot)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return nil
	}
	emailSubject := fmt.Sprintf("[Realms] 新工单 #%d：%s", ticketID, subject)
	adminURL := strings.TrimRight(strings.TrimSpace(baseURL), "/") + fmt.Sprintf("/admin/tickets/%d", ticketID)
	emailHTML := buildTicketEmailHTML(
		"收到新工单",
		adminURL,
		map[string]string{
			"来自用户": userEmail,
			"标题":   subject,
		},
		body,
	)
	for _, u := range roots {
		if err := mailer.SendHTML(ctx, emailSubject, u.Email, emailHTML); err != nil {
			return err
		}
	}
	return nil
}

func NotifyRootsUserReply(ctx context.Context, st *store.Store, mailer email.Mailer, baseURL string, ticketID int64, userEmail string, subject string, body string) error {
	roots, err := st.ListActiveUsersByRole(ctx, store.UserRoleRoot)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return nil
	}
	emailSubject := fmt.Sprintf("[Realms] 工单 #%d 用户回复：%s", ticketID, subject)
	adminURL := strings.TrimRight(strings.TrimSpace(baseURL), "/") + fmt.Sprintf("/admin/tickets/%d", ticketID)
	emailHTML := buildTicketEmailHTML(
		"工单有新回复（用户）",
		adminURL,
		map[string]string{
			"来自用户": userEmail,
			"标题":   subject,
		},
		body,
	)
	for _, u := range roots {
		if err := mailer.SendHTML(ctx, emailSubject, u.Email, emailHTML); err != nil {
			return err
		}
	}
	return nil
}

func NotifyUserAdminReply(ctx context.Context, mailer email.Mailer, baseURL string, ticketID int64, userEmail string, subject string, body string) error {
	emailSubject := fmt.Sprintf("[Realms] 工单 #%d 管理员回复：%s", ticketID, subject)
	userURL := strings.TrimRight(strings.TrimSpace(baseURL), "/") + fmt.Sprintf("/tickets/%d", ticketID)
	emailHTML := buildTicketEmailHTML(
		"工单有新回复（管理员）",
		userURL,
		map[string]string{
			"标题": subject,
		},
		body,
	)
	return mailer.SendHTML(ctx, emailSubject, userEmail, emailHTML)
}

func NotifyUserTicketStatus(ctx context.Context, mailer email.Mailer, baseURL string, ticketID int64, userEmail string, subject string, status string) error {
	emailSubject := fmt.Sprintf("[Realms] 工单 #%d 状态变更：%s", ticketID, subject)
	userURL := strings.TrimRight(strings.TrimSpace(baseURL), "/") + fmt.Sprintf("/tickets/%d", ticketID)
	emailHTML := buildTicketEmailHTML(
		"工单状态已更新",
		userURL,
		map[string]string{
			"标题": subject,
			"状态": status,
		},
		"",
	)
	return mailer.SendHTML(ctx, emailSubject, userEmail, emailHTML)
}

func buildTicketEmailHTML(title string, link string, meta map[string]string, message string) string {
	var b strings.Builder
	b.WriteString("<div style=\"font-family:system-ui,-apple-system,BlinkMacSystemFont,Segoe UI,Roboto,Helvetica,Arial; line-height:1.6;\">")
	b.WriteString("<h2 style=\"margin:0 0 12px 0;\">")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h2>")

	if strings.TrimSpace(link) != "" {
		b.WriteString("<p style=\"margin:0 0 12px 0;\">查看详情：")
		b.WriteString("<a href=\"")
		b.WriteString(html.EscapeString(link))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(link))
		b.WriteString("</a></p>")
	}

	if len(meta) > 0 {
		b.WriteString("<table style=\"border-collapse:collapse; margin:0 0 12px 0;\">")
		for k, v := range meta {
			b.WriteString("<tr>")
			b.WriteString("<td style=\"padding:4px 12px 4px 0; color:#64748b;\">")
			b.WriteString(html.EscapeString(k))
			b.WriteString("</td>")
			b.WriteString("<td style=\"padding:4px 0;\">")
			b.WriteString(html.EscapeString(v))
			b.WriteString("</td>")
			b.WriteString("</tr>")
		}
		b.WriteString("</table>")
	}

	msg := strings.TrimSpace(message)
	if msg != "" {
		b.WriteString("<div style=\"padding:12px; background:#f8fafc; border:1px solid #e2e8f0; border-radius:10px;\">")
		b.WriteString("<div style=\"white-space:pre-wrap;\">")
		b.WriteString(html.EscapeString(truncate(msg, 1200)))
		b.WriteString("</div></div>")
	}

	b.WriteString("<p style=\"margin:12px 0 0 0; color:#64748b; font-size:12px;\">提示：请勿在工单或邮件中发送 API Key/Token 等敏感信息。</p>")
	b.WriteString("</div>")
	return b.String()
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}
