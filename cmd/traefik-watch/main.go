package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider/docker"
	"github.com/traefik/traefik/v3/pkg/safe"
	"gopkg.in/yaml.v3"
)

func failOnError(err error) {
	if err != nil {
		log.Fatal().Err(err).Msg("Fatal error")
	}
}

type isZeroer interface {
	IsZero() bool
}

func isZero(v reflect.Value) bool {
	kind := v.Kind()
	if z, ok := v.Interface().(isZeroer); ok {
		if (kind == reflect.Ptr || kind == reflect.Interface) && v.IsNil() {
			return true
		}
		return z.IsZero()
	}
	switch kind {
	case reflect.String:
		return len(v.String()) == 0
	case reflect.Ptr:
		if !v.IsNil() {
			return isZero(v.Elem())
		}
		return true
	case reflect.Interface:
		return v.IsNil()
	case reflect.Slice:
		return v.Len() == 0
	case reflect.Map:
		return v.Len() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Struct:
		vt := v.Type()
		for i := v.NumField() - 1; i >= 0; i-- {
			if vt.Field(i).PkgPath != "" {
				continue // Private field
			}
			if !isZero(v.Field(i)) {

				return false
			}
		}
		return true
	}
	return false
}

func writeConfig(filename string, config any) error {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0640)
	if err == nil {
		defer file.Close()
		var data []byte
		data, err = yaml.Marshal(config)
		if err == nil {
			_, err = file.Write(data)
		}
	}
	return err
}

var configDir string
var configFilename string
var debug bool

func init() {
	u, err := user.Current()
	if err != nil {
		log.Err(err).Msg("Cannot determine current user")
		u = &user.User{Username: "traefik"}
	}

	flag.StringVar(&configDir, "output-dir", "/etc/traefik/conf.d", "Directory to store the dynamic traefik configuration")
	flag.StringVar(&configFilename, "output", fmt.Sprintf("%s.yaml", u.Username), "Output filename for the dynamic traefik configuration")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")
}

func main() {
	flag.Parse()
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	logger := zerolog.New(os.Stderr)
	ctx := logger.WithContext(context.Background())

	configFilename := filepath.Join(configDir, configFilename)

	dockerEndpoint := os.Getenv("DOCKER_HOST")
	if dockerEndpoint == "" {
		dockerEndpoint = "unix:///var/run/docker.socket"
	}

	logger.Debug().Str("dockerEndpoint", dockerEndpoint).Msg("Initializing Docker provider")

	provider := &docker.Provider{
		Shared: docker.Shared{
			Watch:       true,
			DefaultRule: docker.DefaultTemplateRule,
			Network:     "traefik",
		},
		ClientConfig: docker.ClientConfig{
			Endpoint: dockerEndpoint,
		},
	}
	err := provider.Init()
	failOnError(err)

	configChan := make(chan dynamic.Message)

	pool := safe.NewPool(ctx)

	logger.Debug().Msg("Starting provider loop")
	err = provider.Provide(configChan, pool)
	failOnError(err)

	logger.Debug().Msg("Listening for configuration messages")
	for msg := range configChan {
		logger.Debug().Interface("message", msg).Msg("Message received")
		config := msg.Configuration

		if isZero(reflect.ValueOf(config.TCP)) {
			config.TCP = nil
		}

		if isZero(reflect.ValueOf(config.UDP)) {
			config.UDP = nil
		}

		if isZero(reflect.ValueOf(config.HTTP)) {
			config.HTTP = nil
		}

		if isZero(reflect.ValueOf(config.TLS)) {
			config.TLS = nil
		}

		err := writeConfig(configFilename, msg.Configuration)
		if err != nil {
			logger.Error().Err(err).Msgf("Error writing configuration %s", configFilename)
		}
	}
}
