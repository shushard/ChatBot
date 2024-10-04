package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/playwright-community/playwright-go"
	"github.com/rs/zerolog"
	"github.com/shushard/ChatBot/internal/config"
)

const (
	defaultViewportWidth  = 1920
	defaultViewportHeight = 1080
)

type Service struct {
	config       *config.Config
	logger       *zerolog.Logger
	seenMessages map[string]bool // To keep track of seen message IDs
}

func New(
	conf config.Config,
	logger *zerolog.Logger,
) (*Service, error) {
	if err := playwright.Install(); err != nil {
		return nil, fmt.Errorf("can't install playwright %s: %w", conf.SavePath, err)
	}

	if err := os.MkdirAll(conf.SavePath, os.ModePerm); err != nil {
		return nil, fmt.Errorf("can't create dir %s: %w", conf.SavePath, err)
	}

	s := Service{
		config:       &conf,
		logger:       logger,
		seenMessages: make(map[string]bool),
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

	// Open the Discord website
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

	err = s.ReadMessages(ctx, page)
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

func (s *Service) ReadMessages(ctx context.Context, page playwright.Page) error {
	s.seenMessages = make(map[string]bool) // Initialize or reset seen messages

	fmt.Println("Starting to read new messages...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Get all message elements
			messages, err := page.QuerySelectorAll("div[role='article']")
			if err != nil {
				return fmt.Errorf("failed to select message elements: %w", err)
			}

			for _, message := range messages {
				// Get message ID
				idAttr, err := message.GetAttribute("data-list-item-id")
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get message ID")
					continue
				}
				if idAttr == "" {
					continue
				}
				if s.seenMessages[idAttr] {
					// Already seen this message
					continue
				}
				s.seenMessages[idAttr] = true

				// Get message content
				contentElement, err := message.QuerySelector("div[id^='message-content-']")
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get message content element")
					continue
				}
				if contentElement == nil {
					s.logger.Error().Msg("Message content element not found")
					continue
				}
				content, err := contentElement.TextContent()
				if err != nil {
					s.logger.Error().Err(err).Msg("Failed to get message text")
					continue
				}

				// Output the new message
				fmt.Println("New message:", content)
			}

			// Wait for a short duration before checking again
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func (s *Service) Shutdown(context.Context) error {
	return nil
}
