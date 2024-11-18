package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Ntfy struct {
		TopicURL string `mapstructure:"topic_url"`
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	} `mapstructure:"ntfy"`
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) Load() error {
	// Setup config
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Search config in current directory
	viper.AddConfigPath(".")

	// Manually bind environment variables
	// Check: https://github.com/spf13/viper/issues/188#issuecomment-255519149
	_ = viper.BindEnv("ntfy.topic_url")
	_ = viper.BindEnv("ntfy.username")
	_ = viper.BindEnv("ntfy.password")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			return nil
		}
		return err
	}

	// Unmarshal into the Config struct
	if err := viper.Unmarshal(c); err != nil {
		return err
	}

	return nil
}
