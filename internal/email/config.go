package email

import "time"

type Config struct {
	Enabled     bool
	Host        string
	Port        int
	Username    string
	Password    string
	FromEmail   string
	FromName    string
	Timeout     time.Duration
	UseSTARTTLS bool
}
