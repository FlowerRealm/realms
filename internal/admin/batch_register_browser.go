package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

type BrowserAutomation struct {
	browser *rod.Browser
}

func NewBrowserAutomation(browser *rod.Browser) *BrowserAutomation {
	return &BrowserAutomation{browser: browser}
}

func (b *BrowserAutomation) RegisterAccount(ctx context.Context, email, password string, logger func(string)) error {
	page := stealth.MustPage(b.browser)
	defer page.Close()

	logger("导航到ChatGPT注册页面...")
	if err := page.Navigate("https://chat.openai.com/chat"); err != nil {
		return err
	}
	if err := page.WaitLoad(); err != nil {
		return err
	}

	logger("点击Sign up...")
	signupBtn, err := page.Element(`[data-testid="signup-button"]`)
	if err != nil {
		return err
	}
	if err := signupBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)

	logger("输入邮箱: " + email)
	emailInput, err := page.Element("#email")
	if err != nil {
		return err
	}
	if err := emailInput.Input(email); err != nil {
		return err
	}

	continueBtn, err := page.Element(`button[type="submit"]`)
	if err != nil {
		return err
	}
	if err := continueBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)

	logger("输入密码...")
	passwordInput, err := page.Element(`input[autocomplete="new-password"]`)
	if err != nil {
		return err
	}

	for _, char := range password {
		if err := passwordInput.Input(string(char)); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}

	continueBtn2, err := page.Element(`button[type="submit"]`)
	if err != nil {
		return err
	}
	if err := continueBtn2.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)

	b.handleErrorPage(page, logger)

	return nil
}

func (b *BrowserAutomation) FillVerificationCode(ctx context.Context, page *rod.Page, code string, logger func(string)) error {
	logger("填写验证码: " + code)

	b.handleErrorPage(page, logger)

	codeInput, err := page.Element(`input[name="code"]`)
	if err != nil {
		return err
	}

	for _, char := range code {
		if err := codeInput.Input(string(char)); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	continueBtn, err := page.Element(`button[type="submit"]`)
	if err != nil {
		return err
	}
	if err := continueBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)

	b.handleErrorPage(page, logger)

	return nil
}

func (b *BrowserAutomation) FillPersonalInfo(ctx context.Context, page *rod.Page, logger func(string)) error {
	logger("填写个人信息...")

	nameInput, err := page.Element(`input[name="name"]`)
	if err != nil {
		return err
	}
	if err := nameInput.Input("xiaochuan sun"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	logger("输入生日...")
	yearInput, err := page.Element(`[data-type="year"]`)
	if err != nil {
		return err
	}
	if err := yearInput.SelectAllText(); err != nil {
		return err
	}
	if err := yearInput.Input("1990"); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	monthInput, err := page.Element(`[data-type="month"]`)
	if err != nil {
		return err
	}
	if err := monthInput.SelectAllText(); err != nil {
		return err
	}
	if err := monthInput.Input("05"); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)

	dayInput, err := page.Element(`[data-type="day"]`)
	if err != nil {
		return err
	}
	if err := dayInput.SelectAllText(); err != nil {
		return err
	}
	if err := dayInput.Input("12"); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)

	continueBtn, err := page.Element(`button[type="submit"]`)
	if err != nil {
		return err
	}
	if err := continueBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}

	return nil
}

func (b *BrowserAutomation) PerformCodexOAuth(ctx context.Context, authURL, email, password string, logger func(string)) (callbackURL string, err error) {
	page := stealth.MustPage(b.browser)
	defer page.Close()

	logger("开始Codex OAuth授权...")
	if err := page.Navigate(authURL); err != nil {
		return "", err
	}
	if err := page.WaitLoad(); err != nil {
		return "", err
	}
	time.Sleep(3 * time.Second)

	logger("输入邮箱进行授权...")
	emailInput, err := page.Element(`input[type="email"]`)
	if err != nil {
		return "", err
	}
	for _, char := range email {
		if err := emailInput.Input(string(char)); err != nil {
			return "", err
		}
		time.Sleep(30 * time.Millisecond)
	}

	continueBtn, err := page.Element(`button[type="submit"]`)
	if err != nil {
		return "", err
	}
	if err := continueBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return "", err
	}
	time.Sleep(3 * time.Second)

	logger("输入密码...")
	passwordInput, err := page.Element(`input[type="password"]`)
	if err != nil {
		return "", err
	}
	for _, char := range password {
		if err := passwordInput.Input(string(char)); err != nil {
			return "", err
		}
		time.Sleep(30 * time.Millisecond)
	}

	continueBtn2, err := page.Element(`button[type="submit"]`)
	if err != nil {
		return "", err
	}
	if err := continueBtn2.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return "", err
	}

	logger("等待授权回调...")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		currentURL := page.MustInfo().URL
		if strings.Contains(currentURL, "/auth/callback") && strings.Contains(currentURL, "code=") {
			logger("获取到回调URL")
			return currentURL, nil
		}

		buttons, _ := page.Elements(`button[type="submit"]`)
		for _, btn := range buttons {
			visible, _ := btn.Visible()
			if visible {
				btnText, _ := btn.Text()
				if strings.Contains(strings.ToLower(btnText), "allow") ||
					strings.Contains(strings.ToLower(btnText), "authorize") ||
					strings.Contains(strings.ToLower(btnText), "continue") ||
					strings.Contains(btnText, "授权") ||
					strings.Contains(btnText, "允许") ||
					strings.Contains(btnText, "继续") {
					logger("点击授权按钮: " + btnText)
					btn.Click(proto.InputMouseButtonLeft, 1)
					time.Sleep(2 * time.Second)
					break
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for OAuth callback")
}

func (b *BrowserAutomation) handleErrorPage(page *rod.Page, logger func(string)) {
	for i := 0; i < 5; i++ {
		html, err := page.HTML()
		if err != nil {
			return
		}

		if strings.Contains(html, "出错") || strings.Contains(html, "error") ||
			strings.Contains(html, "timed out") || strings.Contains(html, "timeout") {
			logger("检测到错误页面，点击重试...")

			retryBtn, err := page.Element(`button[data-dd-action-name="Try again"]`)
			if err == nil {
				retryBtn.Click(proto.InputMouseButtonLeft, 1)
				time.Sleep(5 * time.Second)
			} else {
				break
			}
		} else {
			break
		}
	}
}
