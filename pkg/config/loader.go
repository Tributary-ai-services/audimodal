package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v3"
)

// Loader handles loading configuration from various sources
type Loader struct {
	envPrefix string
}

// NewLoader creates a new configuration loader
func NewLoader(envPrefix string) *Loader {
	return &Loader{
		envPrefix: envPrefix,
	}
}

// LoadFromFile loads configuration from a file (YAML or JSON based on extension)
func (l *Loader) LoadFromFile(configPath string, config interface{}) error {
	if configPath == "" {
		return nil // No config file specified
	}

	file, err := os.Open(configPath)
	if err != nil {
		return fmt.Errorf("failed to open config file %s: %w", configPath, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, config); err != nil {
			return fmt.Errorf("failed to parse YAML config file %s: %w", configPath, err)
		}
	case ".json":
		if err := json.Unmarshal(data, config); err != nil {
			return fmt.Errorf("failed to parse JSON config file %s: %w", configPath, err)
		}
	default:
		return fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}

	return nil
}

// LoadFromEnv loads configuration from environment variables
func (l *Loader) LoadFromEnv(config interface{}) error {
	return l.loadFromEnvRecursive(reflect.ValueOf(config).Elem(), "")
}

// loadFromEnvRecursive recursively loads environment variables into config struct
func (l *Loader) loadFromEnvRecursive(value reflect.Value, prefix string) error {
	if !value.IsValid() || !value.CanSet() {
		return nil
	}

	switch value.Kind() {
	case reflect.Struct:
		structType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := value.Field(i)
			fieldType := structType.Field(i)

			// Skip unexported fields
			if !field.CanSet() {
				continue
			}

			// Get env tag or use field name
			envTag := fieldType.Tag.Get("env")
			if envTag == "" {
				envTag = fieldType.Tag.Get("yaml")
				if envTag == "" {
					envTag = strings.ToUpper(fieldType.Name)
				}
			}

			// Build full environment variable name
			var envName string
			if prefix == "" {
				envName = l.buildEnvName(envTag)
			} else {
				envName = l.buildEnvName(prefix + "_" + envTag)
			}

			// Handle nested structs
			if field.Kind() == reflect.Struct {
				if err := l.loadFromEnvRecursive(field, envTag); err != nil {
					return err
				}
				continue
			}

			// Handle pointers to structs
			if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct {
				if field.IsNil() {
					field.Set(reflect.New(field.Type().Elem()))
				}
				if err := l.loadFromEnvRecursive(field.Elem(), envTag); err != nil {
					return err
				}
				continue
			}

			// Load environment variable value
			if envValue := os.Getenv(envName); envValue != "" {
				if err := l.setFieldFromString(field, envValue); err != nil {
					return fmt.Errorf("failed to set field %s from env %s: %w", fieldType.Name, envName, err)
				}
			}
		}

	case reflect.Ptr:
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		return l.loadFromEnvRecursive(value.Elem(), prefix)
	}

	return nil
}

// buildEnvName builds environment variable name with prefix
func (l *Loader) buildEnvName(name string) string {
	name = strings.ToUpper(name)
	if l.envPrefix != "" {
		return l.envPrefix + "_" + name
	}
	return name
}

// setFieldFromString sets a field value from string based on field type
func (l *Loader) setFieldFromString(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Bool:
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value: %s", value)
		}
		field.SetBool(boolVal)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type().String() == "time.Duration" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid duration value: %s", value)
			}
			field.SetInt(int64(duration))
		} else {
			intVal, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid int value: %s", value)
			}
			field.SetInt(intVal)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintVal, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid uint value: %s", value)
		}
		field.SetUint(uintVal)

	case reflect.Float32, reflect.Float64:
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid float value: %s", value)
		}
		field.SetFloat(floatVal)

	case reflect.Slice:
		// Handle string slices (e.g., CORS origins)
		if field.Type().Elem().Kind() == reflect.String {
			values := strings.Split(value, ",")
			for i, v := range values {
				values[i] = strings.TrimSpace(v)
			}
			field.Set(reflect.ValueOf(values))
		} else {
			return fmt.Errorf("unsupported slice type: %s", field.Type())
		}

	default:
		return fmt.Errorf("unsupported field type: %s", field.Type())
	}

	return nil
}

// Load loads configuration from file and environment variables
// File configuration is loaded first, then environment variables override
func (l *Loader) Load(configPath string, config interface{}) error {
	// Load from file first
	if err := l.LoadFromFile(configPath, config); err != nil {
		return fmt.Errorf("failed to load config from file: %w", err)
	}

	// Override with environment variables
	if err := l.LoadFromEnv(config); err != nil {
		return fmt.Errorf("failed to load config from environment: %w", err)
	}

	return nil
}

// WriteExample writes an example configuration file
func (l *Loader) WriteExample(configPath string, config interface{}) error {
	ext := strings.ToLower(filepath.Ext(configPath))

	var data []byte
	var err error

	switch ext {
	case ".yaml", ".yml":
		data, err = yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("failed to marshal config to YAML: %w", err)
		}
	case ".json":
		data, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config to JSON: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config file format: %s", ext)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ValidateConfigPath validates if a config file path is valid
func ValidateConfigPath(configPath string) error {
	if configPath == "" {
		return nil
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}

	// Check if it's a supported format
	ext := strings.ToLower(filepath.Ext(configPath))
	switch ext {
	case ".yaml", ".yml", ".json":
		return nil
	default:
		return fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}
}
