package admin

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
)

type GoRegisterExecutor struct {
	emailClient       *TempEmailClient
	crsClient         *CRSClient
	teamClient        *TeamInviteClient
	browserAutomation *BrowserAutomation
}

func NewGoRegisterExecutor() *GoRegisterExecutor {
	return &GoRegisterExecutor{
		teamClient: NewTeamInviteClient(),
	}
}

func (e *GoRegisterExecutor) Execute(ctx context.Context, task *BatchRegisterTask, config BatchRegisterConfig) error {
	e.emailClient = NewTempEmailClient(config.WorkerDomain, config.AdminToken)

	if config.CRSAPIBase != "" {
		e.crsClient = NewCRSClient(config.CRSAPIBase, config.CRSAdminToken)
	}

	browser, err := e.initBrowser()
	if err != nil {
		return fmt.Errorf("浏览器初始化失败: %w", err)
	}
	defer browser.Close()

	e.browserAutomation = NewBrowserAutomation(browser)

	for i := 0; i < config.Count; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		task.UpdateProgress(i, fmt.Sprintf("开始注册第 %d/%d 个账号", i+1, config.Count))

		result := e.registerOneAccount(ctx, task, config)
		task.AddResult(result)

		if result.Success {
			task.AddLog(fmt.Sprintf("✅ 账号 %d 注册成功: %s", i+1, result.Email))
		} else {
			task.AddLog(fmt.Sprintf("❌ 账号 %d 注册失败: %s", i+1, result.Error))
		}

		if i < config.Count-1 {
			delay := 5 + randomInt(10)
			task.AddLog(fmt.Sprintf("等待 %d 秒后继续...", delay))
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}

	return nil
}

func (e *GoRegisterExecutor) registerOneAccount(ctx context.Context, task *BatchRegisterTask, config BatchRegisterConfig) AccountResult {
	result := AccountResult{}

	task.AddLog("📧 创建临时邮箱...")
	email, err := e.emailClient.CreateEmail(ctx)
	if err != nil {
		result.Error = "创建邮箱失败: " + err.Error()
		return result
	}
	result.Email = email
	task.AddLog("✅ 邮箱创建成功: " + email)

	password := generateRandomPassword(16)
	result.Password = password

	task.AddLog("🌐 开始浏览器注册流程...")
	page := stealth.MustPage(e.browserAutomation.browser)
	defer page.Close()

	if err := e.browserAutomation.RegisterAccount(ctx, email, password, task.AddLog); err != nil {
		result.Error = "注册失败: " + err.Error()
		return result
	}

	task.AddLog("📬 等待验证邮件...")
	code, err := e.emailClient.FetchVerificationCode(ctx, email, 120*time.Second)
	if err != nil {
		result.Error = "获取验证码超时"
		return result
	}
	task.AddLog("✅ 获取验证码: " + code)

	if err := e.browserAutomation.FillVerificationCode(ctx, page, code, task.AddLog); err != nil {
		result.Error = "验证码填写失败: " + err.Error()
		return result
	}

	if err := e.browserAutomation.FillPersonalInfo(ctx, page, task.AddLog); err != nil {
		result.Error = "个人信息填写失败: " + err.Error()
		return result
	}

	task.AddLog("💾 保存账号信息...")
	if err := saveToCSV(email, password); err != nil {
		task.AddLog("⚠️ CSV保存失败: " + err.Error())
	}

	if config.EnableTeamInvite && len(config.Teams) > 0 {
		task.AddLog("📨 发送团队邀请...")
		team := getAvailableTeam(config.Teams)
		if team != nil {
			if err := e.teamClient.InviteToTeam(ctx, email, *team); err != nil {
				task.AddLog("⚠️ 团队邀请失败: " + err.Error())
			} else {
				task.AddLog("✅ 已邀请到团队: " + team.Name)
			}
		} else {
			task.AddLog("⚠️ 所有团队已满")
		}
	}

	if e.crsClient != nil {
		task.AddLog("🔐 开始Codex OAuth授权...")

		authURL, sessionID, err := e.crsClient.GenerateAuthURL(ctx)
		if err != nil {
			task.AddLog("⚠️ CRS授权URL生成失败: " + err.Error())
			goto SUCCESS
		}

		callbackURL, err := e.browserAutomation.PerformCodexOAuth(ctx, authURL, email, password, task.AddLog)
		if err != nil {
			task.AddLog("⚠️ OAuth授权失败: " + err.Error())
			goto SUCCESS
		}

		code := extractCodeFromURL(callbackURL)
		if code == "" {
			task.AddLog("⚠️ 无法从回调URL提取code")
			goto SUCCESS
		}

		tokens, err := e.crsClient.ExchangeCode(ctx, code, sessionID)
		if err != nil {
			task.AddLog("⚠️ 令牌交换失败: " + err.Error())
			goto SUCCESS
		}

		accountInfo := &CodexAccountInfo{Email: email}
		if err := e.crsClient.AddAccount(ctx, email, tokens, accountInfo); err != nil {
			task.AddLog("⚠️ CRS账号保存失败: " + err.Error())
			goto SUCCESS
		}

		task.AddLog("✅ Codex OAuth授权完成")
	}

SUCCESS:
	result.Success = true
	return result
}

func (e *GoRegisterExecutor) initBrowser() (*rod.Browser, error) {
	l := launcher.New().
		Headless(true).
		Devtools(false).
		Set("disable-blink-features", "AutomationControlled").
		Set("excludeSwitches", "enable-automation").
		Set("useAutomationExtension", "false")

	u, err := l.Launch()
	if err != nil {
		return nil, err
	}

	browser := rod.New().
		ControlURL(u).
		MustConnect()

	return browser, nil
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%"
	password := make([]byte, length)

	password[0] = charset[randomInt(26)+26]
	password[1] = charset[randomInt(26)]
	password[2] = charset[randomInt(10)+52]
	password[3] = charset[randomInt(5)+62]

	for i := 4; i < length; i++ {
		password[i] = charset[randomInt(len(charset))]
	}

	for i := len(password) - 1; i > 0; i-- {
		j := randomInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password)
}

func randomInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

func saveToCSV(email, password string) error {
	const csvFile = "registered_accounts.csv"

	fileExists := true
	if _, err := os.Stat(csvFile); os.IsNotExist(err) {
		fileExists = false
	}

	f, err := os.OpenFile(csvFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	if !fileExists {
		writer.Write([]string{"email", "password", "timestamp"})
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	return writer.Write([]string{email, password, timestamp})
}

func extractCodeFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("code")
}
