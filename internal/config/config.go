//nolint:cyclop // can't be less complex
package config

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

var (
	ErrMissing        = errors.New("missing")
	ErrMustBePositive = errors.New("must be positive")
)

type Config struct {
	SiteConfigs              []SiteConfig  `toml:"siteConfigs"`
	PauseBetweenQueries      time.Duration `toml:"pauseBetweenQueries"`
	PauseAfterError          time.Duration `toml:"pauseAfterError"`
	ExpectedResponseTime     time.Duration `toml:"expectedResponseTime"`
	TypingSpeedOneCharacter  time.Duration `toml:"typingSpeedOneCharacter"`
	SuggestionUpdateTimeout  time.Duration `toml:"suggestionUpdateTimeout"`
	TipsParentElementTimeout time.Duration `toml:"tipsParentElementTimeout"`
	RetryDelayOpenSite       time.Duration `toml:"retryDelayOpenSite"`
	RetriesOpenSite          int           `toml:"retriesOpenSite"`
	SavePath                 string        `toml:"savePath"`
	RemoveDirAfter           bool          `toml:"removeDirAfter"`
	Headless                 bool          `toml:"headless"`
}

func (c *Config) Validate() error {
	var errs error

	if len(c.SiteConfigs) == 0 {
		errs = errors.Join(errs, fmt.Errorf("siteConfigs is %w", ErrMissing))
	}
	for i, sc := range c.SiteConfigs {
		if err := sc.Validate(); err != nil {
			errs = errors.Join(errs, fmt.Errorf("siteConfig #%d not valid: %w", i, err))
		}
	}

	if c.PauseBetweenQueries < 0 {
		errs = errors.Join(errs, fmt.Errorf("pauseBetweenQueries %w", ErrMustBePositive))
	}
	if c.PauseAfterError < 0 {
		errs = errors.Join(errs, fmt.Errorf("PauseAfterError %w", ErrMustBePositive))
	}
	if c.ExpectedResponseTime < 0 {
		errs = errors.Join(errs, fmt.Errorf("expectedResponseTime %w", ErrMustBePositive))
	}
	if c.TypingSpeedOneCharacter < 0 {
		errs = errors.Join(errs, fmt.Errorf("typingSpeedOneCharacter %w", ErrMustBePositive))
	}
	if c.SuggestionUpdateTimeout < 0 {
		errs = errors.Join(errs, fmt.Errorf("checkPrefixRetryDelay %w", ErrMustBePositive))
	}
	if c.RetryDelayOpenSite < 0 {
		errs = errors.Join(errs, fmt.Errorf("retryDelayOpenSite %w", ErrMustBePositive))
	}
	if c.TipsParentElementTimeout < 0 {
		errs = errors.Join(errs, fmt.Errorf("tipsParentElementTimeout %w", ErrMustBePositive))
	}
	if c.RetriesOpenSite < 0 {
		errs = errors.Join(errs, fmt.Errorf("retriesOpenSite %w", ErrMustBePositive))
	}
	if c.SavePath == "" {
		errs = errors.Join(errs, fmt.Errorf("savePath is %w", ErrMissing))
	}

	return errs
}

type SiteConfig struct {
	SiteURL                   string `yaml:"siteURL"`
	SearchInputSelector       string `toml:"searchInputSelector"`
	TagsSelector              string `toml:"tagsSelector"`
	RequestsSelector          string `toml:"requestsSelector"`
	InitialRequestsSelector   string `toml:"initialRequestsSelector"`
	BrandsSelector            string `toml:"brandsSelector"`
	TipsParentElementSelector string `toml:"tipsParentElementSelector"`
}

func (sc *SiteConfig) Validate() error {
	var errs error

	if sc.SiteURL == "" {
		errs = errors.Join(errs, fmt.Errorf("siteURL %w", ErrMissing))
	}
	if _, err := url.Parse(sc.SiteURL); err != nil {
		errs = errors.Join(errs, fmt.Errorf("siteURL not valid: %w", err))
	}

	if sc.SearchInputSelector == "" {
		errs = errors.Join(errs, fmt.Errorf("searchInputSelector %w", ErrMissing))
	}
	if sc.TagsSelector == "" {
		errs = errors.Join(errs, fmt.Errorf("tagsSelector %w", ErrMissing))
	}
	if sc.RequestsSelector == "" {
		errs = errors.Join(errs, fmt.Errorf("requestsSelector %w", ErrMissing))
	}
	if sc.InitialRequestsSelector == "" {
		errs = errors.Join(errs, fmt.Errorf("initialRequestsSelector %w", ErrMissing))
	}
	if sc.BrandsSelector == "" {
		errs = errors.Join(errs, fmt.Errorf("brandsSelector %w", ErrMissing))
	}
	if sc.TipsParentElementSelector == "" {
		errs = errors.Join(errs, fmt.Errorf("tipsParentElementSelector %w", ErrMissing))
	}

	return errs
}
