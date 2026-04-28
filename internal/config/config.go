// Package config loads application config from environment variables.
package config

import (
	"errors"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	ListenAddr  string `envconfig:"LISTEN_ADDR" required:"true"`
	RedisURL    string `envconfig:"REDIS_URL" required:"true"`
	ZFSBin      string `envconfig:"ZFS_BIN" default:"/sbin/zfs"`
	ZpoolBin    string `envconfig:"ZPOOL_BIN" default:"/sbin/zpool"`
	LsblkBin    string `envconfig:"LSBLK_BIN" default:"/usr/bin/lsblk"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`
}

func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if c.ListenAddr == "" {
		return nil, errors.New("LISTEN_ADDR is required")
	}
	if c.RedisURL == "" {
		return nil, errors.New("REDIS_URL is required")
	}
	return &c, nil
}
