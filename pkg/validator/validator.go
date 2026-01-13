package validator

import (
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	FailedField string
	Tag         string
	Value       string
}

var validate = validator.New()

func init() {
	// Register custom validation for UUID
	validate.RegisterValidation("uuid_required", func(fl validator.FieldLevel) bool {
		if id, ok := fl.Field().Interface().(uuid.UUID); ok {
			return id != uuid.Nil
		}
		return false
	})
}

func ValidateStruct(data interface{}) []*ErrorResponse {
	var errors []*ErrorResponse
	err := validate.Struct(data)
	if err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			var element ErrorResponse
			element.FailedField = err.StructNamespace()
			element.Tag = err.Tag()
			element.Value = err.Param()
			errors = append(errors, &element)
		}
	}
	return errors
}
