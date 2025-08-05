package validation

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// Error implements the error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (value: %s)", e.Field, e.Message, e.Value)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

// Error implements the error interface
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(messages, "; "))
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, value, message, code string) {
	*e = append(*e, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
		Code:    code,
	})
}

// Validator provides configuration validation functionality
type Validator struct {
	errors ValidationErrors
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make(ValidationErrors, 0),
	}
}

// GetErrors returns all validation errors
func (v *Validator) GetErrors() ValidationErrors {
	return v.errors
}

// HasErrors returns true if there are validation errors
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// AddError adds a validation error to the validator
func (v *Validator) AddError(field, value, message, code string) {
	v.errors.Add(field, value, message, code)
}

// Reset clears all validation errors
func (v *Validator) Reset() {
	v.errors = make(ValidationErrors, 0)
}

// Basic validation methods

// Required validates that a field is not empty
func (v *Validator) Required(field, value, message string) *Validator {
	if strings.TrimSpace(value) == "" {
		if message == "" {
			message = "is required"
		}
		v.errors.Add(field, value, message, "required")
	}
	return v
}

// MinLength validates minimum string length
func (v *Validator) MinLength(field, value string, minLen int, message string) *Validator {
	if len(value) < minLen {
		if message == "" {
			message = fmt.Sprintf("must be at least %d characters", minLen)
		}
		v.errors.Add(field, value, message, "min_length")
	}
	return v
}

// MaxLength validates maximum string length
func (v *Validator) MaxLength(field, value string, maxLen int, message string) *Validator {
	if len(value) > maxLen {
		if message == "" {
			message = fmt.Sprintf("must be at most %d characters", maxLen)
		}
		v.errors.Add(field, value, message, "max_length")
	}
	return v
}

// Range validates that an integer is within a range
func (v *Validator) Range(field string, value, min, max int, message string) *Validator {
	if value < min || value > max {
		if message == "" {
			message = fmt.Sprintf("must be between %d and %d", min, max)
		}
		v.errors.Add(field, strconv.Itoa(value), message, "range")
	}
	return v
}

// Min validates minimum value
func (v *Validator) Min(field string, value, min int, message string) *Validator {
	if value < min {
		if message == "" {
			message = fmt.Sprintf("must be at least %d", min)
		}
		v.errors.Add(field, strconv.Itoa(value), message, "min")
	}
	return v
}

// Max validates maximum value
func (v *Validator) Max(field string, value, max int, message string) *Validator {
	if value > max {
		if message == "" {
			message = fmt.Sprintf("must be at most %d", max)
		}
		v.errors.Add(field, strconv.Itoa(value), message, "max")
	}
	return v
}

// Pattern validates that a string matches a regex pattern
func (v *Validator) Pattern(field, value, pattern, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	matched, err := regexp.MatchString(pattern, value)
	if err != nil || !matched {
		if message == "" {
			message = fmt.Sprintf("must match pattern %s", pattern)
		}
		v.errors.Add(field, value, message, "pattern")
	}
	return v
}

// OneOf validates that a value is one of the allowed values
func (v *Validator) OneOf(field, value string, allowed []string, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	for _, allowedValue := range allowed {
		if value == allowedValue {
			return v
		}
	}

	if message == "" {
		message = fmt.Sprintf("must be one of: %s", strings.Join(allowed, ", "))
	}
	v.errors.Add(field, value, message, "one_of")
	return v
}

// URL validates that a string is a valid URL
func (v *Validator) URL(field, value, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	_, err := url.Parse(value)
	if err != nil {
		if message == "" {
			message = "must be a valid URL"
		}
		v.errors.Add(field, value, message, "url")
	}
	return v
}

// Email validates that a string is a valid email address
func (v *Validator) Email(field, value, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	emailPattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	return v.Pattern(field, value, emailPattern, message)
}

// IP validates that a string is a valid IP address
func (v *Validator) IP(field, value, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	if net.ParseIP(value) == nil {
		if message == "" {
			message = "must be a valid IP address"
		}
		v.errors.Add(field, value, message, "ip")
	}
	return v
}

// Port validates that an integer is a valid port number
func (v *Validator) Port(field string, value int, message string) *Validator {
	return v.Range(field, value, 1, 65535, message)
}

// Duration validates that a string is a valid duration
func (v *Validator) Duration(field, value, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	_, err := time.ParseDuration(value)
	if err != nil {
		if message == "" {
			message = "must be a valid duration (e.g., 30s, 5m, 1h)"
		}
		v.errors.Add(field, value, message, "duration")
	}
	return v
}

// FileExists validates that a file exists
func (v *Validator) FileExists(field, value, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	if _, err := os.Stat(value); os.IsNotExist(err) {
		if message == "" {
			message = "file does not exist"
		}
		v.errors.Add(field, value, message, "file_exists")
	}
	return v
}

// DirectoryExists validates that a directory exists
func (v *Validator) DirectoryExists(field, value, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	stat, err := os.Stat(value)
	if os.IsNotExist(err) {
		if message == "" {
			message = "directory does not exist"
		}
		v.errors.Add(field, value, message, "directory_exists")
	} else if err == nil && !stat.IsDir() {
		if message == "" {
			message = "path is not a directory"
		}
		v.errors.Add(field, value, message, "not_directory")
	}
	return v
}

// FileExtension validates that a file has one of the allowed extensions
func (v *Validator) FileExtension(field, value string, allowed []string, message string) *Validator {
	if value == "" {
		return v // Skip validation for empty values
	}

	ext := strings.ToLower(filepath.Ext(value))
	for _, allowedExt := range allowed {
		if ext == strings.ToLower(allowedExt) {
			return v
		}
	}

	if message == "" {
		message = fmt.Sprintf("file extension must be one of: %s", strings.Join(allowed, ", "))
	}
	v.errors.Add(field, value, message, "file_extension")
	return v
}

// Custom validation with condition
func (v *Validator) When(condition bool, validationFn func(*Validator)) *Validator {
	if condition {
		validationFn(v)
	}
	return v
}

// RequiredWith validates that a field is required when another field has a value
func (v *Validator) RequiredWith(field, value, otherField, otherValue, message string) *Validator {
	if otherValue != "" && strings.TrimSpace(value) == "" {
		if message == "" {
			message = fmt.Sprintf("is required when %s is specified", otherField)
		}
		v.errors.Add(field, value, message, "required_with")
	}
	return v
}

// RequiredWithout validates that a field is required when another field is empty
func (v *Validator) RequiredWithout(field, value, otherField, otherValue, message string) *Validator {
	if otherValue == "" && strings.TrimSpace(value) == "" {
		if message == "" {
			message = fmt.Sprintf("is required when %s is not specified", otherField)
		}
		v.errors.Add(field, value, message, "required_without")
	}
	return v
}

// Struct validation using reflection and tags
func (v *Validator) ValidateStruct(s interface{}) *Validator {
	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return v
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		fieldName := fieldType.Name
		validateTag := fieldType.Tag.Get("validate")

		if validateTag != "" {
			v.validateField(fieldName, field, validateTag)
		}

		// Recursively validate nested structs
		if field.Kind() == reflect.Struct {
			v.ValidateStruct(field.Interface())
		} else if field.Kind() == reflect.Ptr && !field.IsNil() && field.Elem().Kind() == reflect.Struct {
			v.ValidateStruct(field.Interface())
		}
	}

	return v
}

// validateField validates a single field based on validation tags
func (v *Validator) validateField(fieldName string, field reflect.Value, rules string) {
	value := ""
	switch field.Kind() {
	case reflect.String:
		value = field.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value = strconv.FormatInt(field.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value = strconv.FormatUint(field.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		value = strconv.FormatFloat(field.Float(), 'f', -1, 64)
	case reflect.Bool:
		value = strconv.FormatBool(field.Bool())
	default:
		value = fmt.Sprintf("%v", field.Interface())
	}

	// Parse validation rules
	ruleList := strings.Split(rules, ",")
	for _, rule := range ruleList {
		rule = strings.TrimSpace(rule)
		v.applyRule(fieldName, value, field, rule)
	}
}

// applyRule applies a single validation rule
func (v *Validator) applyRule(fieldName, value string, field reflect.Value, rule string) {
	parts := strings.SplitN(rule, "=", 2)
	ruleName := parts[0]
	ruleValue := ""
	if len(parts) > 1 {
		ruleValue = parts[1]
	}

	switch ruleName {
	case "required":
		v.Required(fieldName, value, "")
	case "min":
		if minVal, err := strconv.Atoi(ruleValue); err == nil {
			if field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64 {
				v.Min(fieldName, int(field.Int()), minVal, "")
			} else if field.Kind() == reflect.String {
				v.MinLength(fieldName, value, minVal, "")
			}
		}
	case "max":
		if maxVal, err := strconv.Atoi(ruleValue); err == nil {
			if field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64 {
				v.Max(fieldName, int(field.Int()), maxVal, "")
			} else if field.Kind() == reflect.String {
				v.MaxLength(fieldName, value, maxVal, "")
			}
		}
	case "email":
		v.Email(fieldName, value, "")
	case "url":
		v.URL(fieldName, value, "")
	case "ip":
		v.IP(fieldName, value, "")
	case "port":
		if field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64 {
			v.Port(fieldName, int(field.Int()), "")
		}
	case "duration":
		v.Duration(fieldName, value, "")
	case "oneof":
		allowed := strings.Split(ruleValue, " ")
		v.OneOf(fieldName, value, allowed, "")
	}
}
