package config

import (
	"bytes"
	"errors"
	"log"
	"os"

	"github.com/spf13/viper"
)

type EnvConfig struct {
	TelegramID      []int64 `mapstructure:"TELEGRAM_ID"`
	TelegramToken   string  `mapstructure:"TELEGRAM_TOKEN"`
	EditWaitSeconds int     `mapstructure:"EDIT_WAIT_SECONDS"`
	OllamaURL       string  `mapstructure:"OLLAMA_URL"`
	OllamaModel     string  `mapstructure:"OLLAMA_MODEL"`
	DataDir         string  `mapstructure:"DATA_DIR"`
	ProjectsFile    string  `mapstructure:"PROJECTS_FILE"`
}

// emptyConfig is used to initialize viper.
// It is required to register config keys with viper when in case no config file is provided.
const emptyConfig = `TELEGRAM_ID=
TELEGRAM_TOKEN=
EDIT_WAIT_SECONDS=
OLLAMA_URL=
OLLAMA_MODEL=
DATA_DIR=./data
PROJECTS_FILE=projects.json`

func (e *EnvConfig) HasTelegramID(id int64) bool {
	for _, v := range e.TelegramID {
		if v == id {
			return true
		}
	}
	return false
}

// LoadEnvConfig loads config from .env file, variables from environment take precedence if provided.
// If no .env file is provided, config is loaded from environment variables.
func LoadEnvConfig(path string) (*EnvConfig, error) {
	fileExists := fileExists(path)
	if !fileExists {
		log.Printf("config file %s does not exist, using env variables", path)
	}

	v := viper.New()
	v.SetConfigType("env")
	v.AutomaticEnv()
	if err := v.ReadConfig(bytes.NewBufferString(emptyConfig)); err != nil {
		return nil, err
	}
	if fileExists {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	var cfg EnvConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = "http://localhost:11434"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.ProjectsFile == "" {
		cfg.ProjectsFile = "projects.json"
	}
	return &cfg, nil
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return os.IsExist(err)
	}
	return true
}

func (e *EnvConfig) ValidateWithDefaults() error {
	if e.TelegramToken == "" {
		return errors.New("TELEGRAM_TOKEN is not set")
	}
	if len(e.TelegramID) == 0 {
		return errors.New("TELEGRAM_ID must contain at least one allowed Telegram user ID")
	}
	for _, id := range e.TelegramID {
		if id <= 0 {
			return errors.New("TELEGRAM_ID values must be positive Telegram user IDs")
		}
	}
	if e.EditWaitSeconds <= 0 {
		log.Printf("EDIT_WAIT_SECONDS not set or invalid, defaulting to 1")
		e.EditWaitSeconds = 1
	}
	if e.OllamaURL == "" {
		return errors.New("OLLAMA_URL is not set")
	}
	if e.OllamaModel == "" {
		return errors.New("OLLAMA_MODEL is not set")
	}
	if e.DataDir == "" {
		return errors.New("DATA_DIR is not set")
	}
	if e.ProjectsFile == "" {
		return errors.New("PROJECTS_FILE is not set")
	}
	return nil
}
