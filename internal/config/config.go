package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err = yaml.NewDecoder(file).Decode(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

type Config struct {
	MediaMTX             MediaMTXConfig `yaml:"mediamtx"`
	Grpc                 GrpcConfig     `yaml:"grpc"`
	AudioProviderAddress string         `yaml:"audio_provider_address"`
}

type GrpcConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type MediaMTXConfig struct {
	MediaMtxHost string `yaml:"mediamtx_host"`
}
