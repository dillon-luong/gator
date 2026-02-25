package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

const configFileName = ".gatorconfig.json"

type Config struct {
	Db_Url            string `json:"db_url"`
	Current_User_Name string `json:"current_user_name"`
}

func Read() Config {
	path, err := getConfigFilePath()
	if err != nil {
		log.Fatalf("error getting config filepath: %v", err)
	}

	jsonData, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read config file: %v", err)
	}

	var config Config
	err = json.Unmarshal(jsonData, &config)
	if err != nil {
		log.Fatalf("failed to unmarshal config file: %v", err)
	}

	return config
}

func (c *Config) SetUser(user string) error {
	c.Current_User_Name = user

	if err := write(*c); err != nil {
		return fmt.Errorf("Error setting user: %v", err)
	}

	return nil
}

func getConfigFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %v", err)
	}
	path := fmt.Sprintf("%v/%v", homeDir, configFileName)
	return path, nil
}

func write(config Config) error {
	jsonData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal data to json: %v", err)
	}

	path, err := getConfigFilePath()
	if err != nil {
		return fmt.Errorf("error getting config filepath: %v", err)
	}
	// fileMode 0644 = read/write for owner, read-only for others
	err = os.WriteFile(path, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to config file: %v", err)
	}

	return nil
}
