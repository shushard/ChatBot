package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/rs/zerolog"
	"github.com/shushard/ChatBot/internal/config"
)

const (
	defaultViewportWidth  = 1024
	defaultViewportHeight = 600
)

type Service struct {
	config              *config.Config
	logger              *zerolog.Logger
	seenMessages        map[string]bool
	page                playwright.Page
	apiKey              string
	botUsername         string
	conversationHistory []map[string]string
}

func New(
	conf config.Config,
	logger *zerolog.Logger,
) (*Service, error) {
	apiKey := os.Getenv("PROXY_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("proxy API key is not set in environment variable PROXY_API_KEY")
	}

	botUsername := os.Getenv("BOT_USERNAME")
	if botUsername == "" {
		return nil, fmt.Errorf("bot username is not set in environment variable BOT_USERNAME")
	}

	if err := playwright.Install(); err != nil {
		return nil, fmt.Errorf("can't install playwright %s: %w", conf.SavePath, err)
	}

	if err := os.MkdirAll(conf.SavePath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("can't create dir %s: %w", conf.SavePath, err)
	}

	s := Service{
		config:              &conf,
		logger:              logger,
		seenMessages:        make(map[string]bool),
		apiKey:              apiKey,
		botUsername:         botUsername,
		conversationHistory: make([]map[string]string, 0),
	}

	return &s, nil
}

func (s *Service) Run(ctx context.Context) (err error) {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("can't launch browser: %w", err)
	}

	defer func() {
		if tmpErr := pw.Stop(); tmpErr != nil {
			err = errors.Join(err, fmt.Errorf("error stopping browser: %w", tmpErr))
		}
	}()

	for _, siteConfig := range s.config.SiteConfigs {
		if checkErr := s.checkSite(ctx, pw, siteConfig, nil); checkErr != nil {
			return fmt.Errorf("error checking site %s: %w", siteConfig.SiteURL, checkErr)
		}
	}

	return err
}

func (s *Service) checkSite(
	ctx context.Context,
	pw *playwright.Playwright,
	siteConfig config.SiteConfig,
	prefixes []string,
) (err error) {
	s.logger.Info().Str("site", siteConfig.SiteURL).Msg("starting check site")

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &s.config.Headless,
		Args: []string{
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		return fmt.Errorf("can't launch chromium: %w", err)
	}

	defer func() {
		if tmpErr := browser.Close(); tmpErr != nil {
			err = errors.Join(err, fmt.Errorf("error closing browser: %w", tmpErr))
		}
	}()

	page, err := s.createPage(browser)
	if err != nil {
		return fmt.Errorf("can't create page: %w", err)
	}

	s.page = page

	if err := s.openSite(ctx, page, siteConfig); err != nil {
		return fmt.Errorf("can't open site: %w", err)
	}

	fmt.Println("Please log in to your Discord account in the opened browser.")
	fmt.Println("Once logged in and navigated to the desired channel, enter 'start' to continue...")

	var input string
	for {
		fmt.Scanln(&input)
		if input == "start" {
			break
		}
		fmt.Println("Waiting for 'start' input...")
	}

	err = s.ReadMessages(ctx)
	if err != nil {
		return fmt.Errorf("can't read messages: %w", err)
	}

	return nil
}

func (s *Service) createPage(browser playwright.Browser) (playwright.Page, error) {
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{
			Width:  defaultViewportWidth,
			Height: defaultViewportHeight,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("can't create page: %w", err)
	}
	return page, nil
}

func (s *Service) openSite(ctx context.Context, page playwright.Page, siteConfig config.SiteConfig) error {
	_, err := page.Goto(siteConfig.SiteURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		return fmt.Errorf("can't go to URL: %w", err)
	}

	return nil
}

func (s *Service) ReadMessages(ctx context.Context) error {
	s.seenMessages = make(map[string]bool)

	fmt.Println("Initializing seen messages...")
	if err := s.initializeSeenMessages(); err != nil {
		return fmt.Errorf("failed to initialize seen messages: %w", err)
	}

	fmt.Println("Starting to read new messages...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			messages, err := s.page.QuerySelectorAll("div[role='article']")
			if err != nil {
				return fmt.Errorf("failed to select message elements: %w", err)
			}

			for _, message := range messages {
				idAttr, err := message.GetAttribute("data-list-item-id")
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get message ID")
					continue
				}
				if idAttr == "" {
					continue
				}
				if s.seenMessages[idAttr] {
					continue
				}
				s.seenMessages[idAttr] = true

				usernameElement, err := message.QuerySelector("h3 span span")
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get username element")
					continue
				}
				if usernameElement == nil {
					s.logger.Error().Msg("Username element not found")
					htmlContent, _ := message.InnerHTML()
					s.logger.Debug().Msgf("Message HTML: %s", htmlContent)
					continue
				}
				username, err := usernameElement.InnerText()
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get username text")
					continue
				}
				username = strings.TrimSpace(username)
				username = strings.TrimPrefix(username, "@")
				if strings.EqualFold(username, s.botUsername) {
					continue
				}

				isReply, err := s.isReplyToBot(message)
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to check if message is a reply to bot")
					continue
				}

				isMentioned := false
				mentionElements, err := message.QuerySelectorAll("div[class*='markup'] span.mention")
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get mention elements")
					continue
				}
				for _, mention := range mentionElements {
					mentionText, err := mention.InnerText()
					if err != nil {
						s.logger.Error().Err(err).Msg("Failed to get mention text")
						continue
					}
					mentionText = strings.TrimSpace(mentionText)
					mentionText = strings.TrimPrefix(mentionText, "@")
					if strings.EqualFold(mentionText, s.botUsername) {
						isMentioned = true
						break
					}
				}

				if isMentioned || isReply {
					contentElement, err := message.QuerySelector("div[class*='contents'] > div[class*='markup']")
					if err != nil {
						s.logger.Error().Err(err).Msg("Failed to get message content element")
						continue
					}
					if contentElement == nil {
						s.logger.Error().Msg("Message content element not found")
						continue
					}
					content, err := contentElement.InnerText()
					if err != nil {
						s.logger.Error().Err(err).Msg("Failed to get message text")
						continue
					}
					content = strings.TrimSpace(content)
					fmt.Println("Detected message to bot:", content)

					cleanContent := content
					for _, mention := range mentionElements {
						mentionText, _ := mention.InnerText()
						cleanContent = strings.ReplaceAll(cleanContent, mentionText, "")
					}
					cleanContent = strings.TrimSpace(cleanContent)

					responseText, err := s.askChatGPT(cleanContent)
					if err != nil {
						s.logger.Error().Err(err).Msg("Failed to get response from ChatGPT")
						continue
					}

					fmt.Println("ChatGPT response:", responseText)

					if err := s.typeInChat(responseText); err != nil {
						s.logger.Error().Err(err).Msg("Failed to reply in chat")
						continue
					}
				}
			}

			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func (s *Service) isReplyToBot(message playwright.ElementHandle) (bool, error) {
	replyContext, err := message.QuerySelector("div[id^='message-reply-context-']")
	if err != nil {
		return false, fmt.Errorf("failed to get reply context: %w", err)
	}
	if replyContext == nil {
		return false, nil
	}
	usernameElement, err := replyContext.QuerySelector("span[class*='username']")
	if err != nil {
		return false, fmt.Errorf("failed to get username in reply context: %w", err)
	}
	if usernameElement == nil {
		return false, nil
	}
	username, err := usernameElement.InnerText()
	if err != nil {
		return false, fmt.Errorf("failed to get username text: %w", err)
	}
	username = strings.TrimSpace(username)
	username = strings.TrimPrefix(username, "@")
	if strings.EqualFold(username, s.botUsername) {
		return true, nil
	}
	return false, nil
}

func (s *Service) initializeSeenMessages() error {
	messages, err := s.page.QuerySelectorAll("div[role='article']")
	if err != nil {
		return fmt.Errorf("failed to select message elements: %w", err)
	}

	for _, message := range messages {
		idAttr, err := message.GetAttribute("data-list-item-id")
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to get message ID during initialization")
			continue
		}
		if idAttr == "" {
			continue
		}
		s.seenMessages[idAttr] = true
	}

	return nil
}

func (s *Service) askChatGPT(message string) (string, error) {
	message = strings.ReplaceAll(message, ",", "")
	message = strings.ReplaceAll(message, ".", "\n")

	systemPrompt := `Отвечай пользователю от первого лица единственного числа.
Твои ответы всегда на русском языке.
Ты не используешь запятые в своих предложениях. Вместо точек начинай новую строку.
Не задавай вопросов вроде "Чем я могу помочь?" или подобных.
Твои ответы должны быть краткими, не более 50 слов, и создавать впечатление, что говорит реальный человек.
Все символы, кроме первого в строке, должны быть в нижнем регистре.
Ты можешь использовать только вопросительные и восклицательные знаки; не используй другие символы вроде дефисов.`
	messages := make([]map[string]string, 0)
	messages = append(messages, map[string]string{
		"role":    "system",
		"content": systemPrompt,
	})

	messages = append(messages, s.conversationHistory...)

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": message,
	})

	url := "https://api.proxyapi.ru/openai/v1/chat/completions"
	reqBody, err := json.Marshal(map[string]interface{}{
		"model":       "gpt-4o-mini",
		"messages":    messages,
		"max_tokens":  100,
		"temperature": 0.7,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-OK HTTP status: %s, body: %s", resp.Status, string(bodyBytes))
	}

	var respData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
		firstChoice := choices[0].(map[string]interface{})
		if messageMap, ok := firstChoice["message"].(map[string]interface{}); ok {
			if content, ok := messageMap["content"].(string); ok {
				content = strings.TrimSpace(content)
				content = strings.ReplaceAll(content, ",", "")
				content = strings.ReplaceAll(content, ".", "\n")
				words := strings.Fields(content)
				if len(words) > 50 {
					content = strings.Join(words[:50], " ")
				}
				s.updateConversationHistory(map[string]string{
					"role":    "user",
					"content": message,
				}, map[string]string{
					"role":    "assistant",
					"content": content,
				})
				return content, nil
			}
		}
	}

	return "", fmt.Errorf("invalid response format")
}

func (s *Service) updateConversationHistory(userMessage, assistantMessage map[string]string) {
	s.conversationHistory = append(s.conversationHistory, userMessage)
	s.conversationHistory = append(s.conversationHistory, assistantMessage)

	if len(s.conversationHistory) > 10 {
		s.conversationHistory = s.conversationHistory[len(s.conversationHistory)-10:]
	}
}

func (s *Service) typeInChat(response string) error {
	inputBox, err := s.page.QuerySelector("div[role='textbox']")
	if err != nil {
		return fmt.Errorf("failed to find text input box: %w", err)
	}
	if inputBox == nil {
		return fmt.Errorf("text input box not found")
	}

	if err = inputBox.Click(); err != nil {
		return fmt.Errorf("failed to click on text input box: %w", err)
	}

	if err = inputBox.Type(response, playwright.ElementHandleTypeOptions{
		Delay: playwright.Float(100),
	}); err != nil {
		return fmt.Errorf("failed to type response: %w", err)
	}

	if err = inputBox.Press("Enter"); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (s *Service) Shutdown(context.Context) error {
	return nil
}
